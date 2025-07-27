// file: internal/services/transaction_services.go
package services

import (
	"context"
	"database/sql"
	"evalhub/internal/events"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// transactionService implements cross-service transaction coordination
type transactionService struct {
	db     *sql.DB
	events events.EventBus
	logger *zap.Logger
	config *TransactionConfig
	
	// Active transaction tracking
	activeTxs map[string]*TransactionContext
	mu        sync.RWMutex
}

// TransactionConfig holds transaction service configuration
type TransactionConfig struct {
	DefaultTimeout    time.Duration `json:"default_timeout"`
	MaxRetries        int           `json:"max_retries"`
	RetryDelay        time.Duration `json:"retry_delay"`
	EnableMetrics     bool          `json:"enable_metrics"`
	EnableDeadlock    bool          `json:"enable_deadlock_detection"`
	IsolationLevel    string        `json:"isolation_level"`
	MaxConcurrentTxs  int           `json:"max_concurrent_txs"`
}

// NewTransactionService creates a new enterprise transaction service
func NewTransactionService(
	db *sql.DB,
	events events.EventBus,
	logger *zap.Logger,
	config *TransactionConfig,
) TransactionService {
	if config == nil {
		config = DefaultTransactionConfig()
	}

	service := &transactionService{
		db:        db,
		events:    events,
		logger:    logger,
		config:    config,
		activeTxs: make(map[string]*TransactionContext),
	}

	// Start background cleanup
	go service.cleanupExpiredTransactions()

	return service
}

// DefaultTransactionConfig returns default transaction configuration
func DefaultTransactionConfig() *TransactionConfig {
	return &TransactionConfig{
		DefaultTimeout:   30 * time.Second,
		MaxRetries:       3,
		RetryDelay:       1 * time.Second,
		EnableMetrics:    true,
		EnableDeadlock:   true,
		IsolationLevel:   "READ_COMMITTED",
		MaxConcurrentTxs: 100,
	}
}

// ===============================
// TRANSACTION MANAGEMENT
// ===============================

// BeginTransaction starts a new coordinated transaction
func (s *transactionService) BeginTransaction(ctx context.Context, req *BeginTransactionRequest) (*TransactionContext, error) {
	// Check concurrent transaction limit
	s.mu.RLock()
	activeCount := len(s.activeTxs)
	s.mu.RUnlock()

	if activeCount >= s.config.MaxConcurrentTxs {
		return nil, NewRateLimitError("maximum concurrent transactions reached", map[string]interface{}{
			"limit":  s.config.MaxConcurrentTxs,
			"active": activeCount,
		})
	}

	// Validate request
	if err := s.validateBeginRequest(req); err != nil {
		return nil, NewValidationError("invalid begin transaction request", err)
	}

	// Generate transaction ID
	txID := s.generateTransactionID()

	// Set timeout
	timeout := s.config.DefaultTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	// Begin database transaction
	var tx *sql.Tx
	var err error

	if req.IsolationLevel != "" {
		// Set isolation level if specified
		tx, err = s.beginWithIsolation(ctx, req.IsolationLevel)
	} else {
		tx, err = s.db.BeginTx(ctx, &sql.TxOptions{
			Isolation: s.getIsolationLevel(s.config.IsolationLevel),
			ReadOnly:  req.ReadOnly,
		})
	}

	if err != nil {
		s.logger.Error("Failed to begin database transaction", zap.Error(err))
		return nil, NewInternalError("failed to begin transaction")
	}

	// Create transaction context
	txCtx := &TransactionContext{
		ID:         txID,
		Tx:         tx,
		StartTime:  time.Now(),
		Timeout:    time.Now().Add(timeout),
		UserID:     req.UserID,
		Operations: []TransactionOp{},
		Status:     TransactionStatusActive,
		Metadata:   req.Metadata,
	}

	// Store active transaction
	s.mu.Lock()
	s.activeTxs[txID] = txCtx
	s.mu.Unlock()

	// Publish transaction started event
	if err := s.events.Publish(ctx, events.NewTransactionStartedEvent(txID, req.UserID)); err != nil {
		s.logger.Warn("Failed to publish transaction started event", zap.Error(err))
	}

	s.logger.Info("Transaction started",
		zap.String("transaction_id", txID),
		zap.Duration("timeout", timeout),
		zap.Int64p("user_id", req.UserID),
	)

	return txCtx, nil
}

// CommitTransaction commits a coordinated transaction
func (s *transactionService) CommitTransaction(ctx context.Context, transactionID string) error {
	// Get transaction context
	txCtx, err := s.getTransaction(transactionID)
	if err != nil {
		return err
	}

	// Lock transaction for commit
	txCtx.mu.Lock()
	defer txCtx.mu.Unlock()

	// Check transaction status
	if txCtx.Status != TransactionStatusActive {
		return NewBusinessError("transaction is not active", "TRANSACTION_NOT_ACTIVE")
	}

	// Check timeout
	if time.Now().After(txCtx.Timeout) {
		txCtx.Status = TransactionStatusFailed
		s.rollbackTransactionUnsafe(ctx, txCtx)
		return NewBusinessError("transaction has timed out", "TRANSACTION_TIMEOUT")
	}

	// Commit database transaction
	if err := txCtx.Tx.Commit(); err != nil {
		s.logger.Error("Failed to commit database transaction",
			zap.Error(err),
			zap.String("transaction_id", transactionID),
		)
		txCtx.Status = TransactionStatusFailed
		return NewInternalError("failed to commit transaction")
	}

	// Update status
	txCtx.Status = TransactionStatusCommitted

	// Remove from active transactions
	s.mu.Lock()
	delete(s.activeTxs, transactionID)
	s.mu.Unlock()

	// Publish transaction committed event
	if err := s.events.Publish(ctx, &events.TransactionCommittedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "transaction.committed",
			Timestamp: time.Now(),
			UserID:    txCtx.UserID,
		},
		TransactionID: transactionID,
		Duration:      time.Since(txCtx.StartTime),
		OperationCount: len(txCtx.Operations),
	}); err != nil {
		s.logger.Warn("Failed to publish transaction committed event", zap.Error(err))
	}

	s.logger.Info("Transaction committed successfully",
		zap.String("transaction_id", transactionID),
		zap.Duration("duration", time.Since(txCtx.StartTime)),
		zap.Int("operations", len(txCtx.Operations)),
	)

	return nil
}

// RollbackTransaction rolls back a coordinated transaction
func (s *transactionService) RollbackTransaction(ctx context.Context, transactionID string) error {
	// Get transaction context
	txCtx, err := s.getTransaction(transactionID)
	if err != nil {
		return err
	}

	// Lock transaction for rollback
	txCtx.mu.Lock()
	defer txCtx.mu.Unlock()

	return s.rollbackTransactionUnsafe(ctx, txCtx)
}

// rollbackTransactionUnsafe performs rollback without locking (internal use)
func (s *transactionService) rollbackTransactionUnsafe(ctx context.Context, txCtx *TransactionContext) error {
	// Check transaction status
	if txCtx.Status == TransactionStatusCommitted {
		return NewBusinessError("cannot rollback committed transaction", "TRANSACTION_ALREADY_COMMITTED")
	}

	if txCtx.Status == TransactionStatusRolledBack {
		return nil // Already rolled back
	}

	// Rollback database transaction
	if err := txCtx.Tx.Rollback(); err != nil {
		s.logger.Error("Failed to rollback database transaction",
			zap.Error(err),
			zap.String("transaction_id", txCtx.ID),
		)
		txCtx.Status = TransactionStatusFailed
		return NewInternalError("failed to rollback transaction")
	}

	// Update status
	txCtx.Status = TransactionStatusRolledBack

	// Remove from active transactions
	s.mu.Lock()
	delete(s.activeTxs, txCtx.ID)
	s.mu.Unlock()

	// Publish transaction rolled back event
	event := events.NewTransactionRolledBackEvent(
		txCtx.ID,
		time.Since(txCtx.StartTime),
		len(txCtx.Operations),
		txCtx.UserID,
	)

	if err := s.events.Publish(ctx, event); err != nil {
		s.logger.Warn("Failed to publish transaction rolled back event", zap.Error(err))
	}

	s.logger.Info("Transaction rolled back",
		zap.String("transaction_id", txCtx.ID),
		zap.Duration("duration", time.Since(txCtx.StartTime)),
	)

	return nil
}

// ===============================
// OPERATION TRACKING
// ===============================

// AddOperation adds an operation to a transaction for tracking
func (s *transactionService) AddOperation(ctx context.Context, transactionID string, req *AddOperationRequest) error {
	// Get transaction context
	txCtx, err := s.getTransaction(transactionID)
	if err != nil {
		return err
	}

	// Lock transaction for operation addition
	txCtx.mu.Lock()
	defer txCtx.mu.Unlock()

	// Check transaction status
	if txCtx.Status != TransactionStatusActive {
		return NewBusinessError("transaction is not active", "TRANSACTION_NOT_ACTIVE")
	}

	// Create operation
	op := TransactionOp{
		ID:        s.generateOperationID(),
		Type:      req.Type,
		Service:   req.Service,
		Method:    req.Method,
		StartTime: time.Now(),
		Status:    OperationStatusRunning,
		Metadata:  req.Metadata,
	}

	// Add to transaction operations
	txCtx.Operations = append(txCtx.Operations, op)

	s.logger.Debug("Operation added to transaction",
		zap.String("transaction_id", transactionID),
		zap.String("operation_id", op.ID),
		zap.String("service", req.Service),
		zap.String("method", req.Method),
	)

	return nil
}

// CompleteOperation marks an operation as completed
func (s *transactionService) CompleteOperation(ctx context.Context, transactionID, operationID string) error {
	return s.updateOperationStatus(transactionID, operationID, OperationStatusCompleted, nil)
}

// FailOperation marks an operation as failed
func (s *transactionService) FailOperation(ctx context.Context, transactionID, operationID string, err error) error {
	errorMsg := err.Error()
	return s.updateOperationStatus(transactionID, operationID, OperationStatusFailed, &errorMsg)
}

// updateOperationStatus updates the status of an operation
func (s *transactionService) updateOperationStatus(transactionID, operationID string, status OperationStatus, errorMsg *string) error {
	// Get transaction context
	txCtx, err := s.getTransaction(transactionID)
	if err != nil {
		return err
	}

	// Lock transaction for operation update
	txCtx.mu.Lock()
	defer txCtx.mu.Unlock()

	// Find and update operation
	for i := range txCtx.Operations {
		if txCtx.Operations[i].ID == operationID {
			now := time.Now()
			txCtx.Operations[i].Status = status
			txCtx.Operations[i].EndTime = &now
			if errorMsg != nil {
				txCtx.Operations[i].Error = errorMsg
			}

			s.logger.Debug("Operation status updated",
				zap.String("transaction_id", transactionID),
				zap.String("operation_id", operationID),
				zap.String("status", string(status)),
			)

			return nil
		}
	}

	return NewNotFoundError("operation not found in transaction")
}

// ===============================
// TRANSACTION EXECUTION PATTERNS
// ===============================

// ExecuteInTransaction executes a function within a managed transaction
func (s *transactionService) ExecuteInTransaction(ctx context.Context, req *ExecuteInTransactionRequest, fn TransactionFunc) error {
	// Begin transaction
	txCtx, err := s.BeginTransaction(ctx, &BeginTransactionRequest{
		UserID:         req.UserID,
		Timeout:        req.Timeout,
		IsolationLevel: req.IsolationLevel,
		ReadOnly:       req.ReadOnly,
		Metadata:       req.Metadata,
	})
	if err != nil {
		return err
	}

	// Setup automatic rollback on panic
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic in transaction, rolling back",
				zap.String("transaction_id", txCtx.ID),
				zap.Any("panic", r),
			)
			s.RollbackTransaction(ctx, txCtx.ID)
			panic(r) // Re-panic after cleanup
		}
	}()

	// Execute function
	if err := fn(ctx, txCtx); err != nil {
		// Rollback on error
		if rollbackErr := s.RollbackTransaction(ctx, txCtx.ID); rollbackErr != nil {
			s.logger.Error("Failed to rollback transaction after error",
				zap.Error(rollbackErr),
				zap.String("transaction_id", txCtx.ID),
			)
		}
		return err
	}

	// Commit transaction
	return s.CommitTransaction(ctx, txCtx.ID)
}

// ExecuteWithRetry executes a transaction with automatic retry logic
func (s *transactionService) ExecuteWithRetry(ctx context.Context, req *ExecuteWithRetryRequest, fn TransactionFunc) error {
	var lastErr error

	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-time.After(s.config.RetryDelay * time.Duration(attempt)):
			case <-ctx.Done():
				return ctx.Err()
			}

			s.logger.Info("Retrying transaction",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", s.config.MaxRetries),
			)
		}

		// Execute transaction
		err := s.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
			UserID:         req.UserID,
			Timeout:        req.Timeout,
			IsolationLevel: req.IsolationLevel,
			ReadOnly:       req.ReadOnly,
			Metadata:       req.Metadata,
		}, fn)

		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !s.isRetryableError(err) {
			s.logger.Info("Non-retryable error, not retrying",
				zap.Error(err),
				zap.Int("attempt", attempt),
			)
			break
		}

		s.logger.Warn("Transaction failed, will retry",
			zap.Error(err),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", s.config.MaxRetries),
		)
	}

	return fmt.Errorf("transaction failed after %d attempts: %w", s.config.MaxRetries+1, lastErr)
}

// ===============================
// TRANSACTION QUERY AND MONITORING
// ===============================

// convertToTransactionInfo converts a TransactionContext to a TransactionInfo
func convertToTransactionInfo(txCtx *TransactionContext) *TransactionInfo {
	// Convert TransactionOp slice to OperationInfo slice
	operations := make([]*OperationInfo, 0, len(txCtx.Operations))
	for _, op := range txCtx.Operations {
		operationInfo := &OperationInfo{
			ID:        op.ID,
			Type:      op.Type,
			Status:    string(op.Status),
			StartTime: op.StartTime,
			EndTime:   op.EndTime,
		}
		if op.EndTime != nil {
			duration := op.EndTime.Sub(op.StartTime)
			operationInfo.Duration = &duration
		}
		if op.Error != nil {
			errorMsg := *op.Error
			operationInfo.Error = &errorMsg
		}
		operationInfo.Metadata = op.Metadata
		operations = append(operations, operationInfo)
	}

	return &TransactionInfo{
		ID:             txCtx.ID,
		Status:         string(txCtx.Status),
		StartTime:      txCtx.StartTime,
		Duration:       time.Since(txCtx.StartTime),
		UserID:         txCtx.UserID,
		OperationCount: len(operations),
		Operations:     operations,
		Metadata:       txCtx.Metadata,
	}
}

// GetTransaction retrieves transaction information
func (s *transactionService) GetTransaction(ctx context.Context, transactionID string) (*TransactionInfo, error) {
	txCtx, err := s.getTransaction(transactionID)
	if err != nil {
		return nil, err
	}

	txCtx.mu.RLock()
	defer txCtx.mu.RUnlock()

	return convertToTransactionInfo(txCtx), nil
}

// GetActiveTransactions retrieves all active transactions
func (s *transactionService) GetActiveTransactions(ctx context.Context) ([]*TransactionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	transactions := make([]*TransactionInfo, 0, len(s.activeTxs))

	for _, txCtx := range s.activeTxs {
		txCtx.mu.RLock()
		transactions = append(transactions, convertToTransactionInfo(txCtx))
		txCtx.mu.RUnlock()
	}

	return transactions, nil
}

// GetTransactionMetrics retrieves transaction service metrics
func (s *transactionService) GetTransactionMetrics(ctx context.Context) (*TransactionMetrics, error) {
	s.mu.RLock()
	activeCount := len(s.activeTxs)
	
	var oldestTx *TransactionContext
	for _, tx := range s.activeTxs {
		if oldestTx == nil || tx.StartTime.Before(oldestTx.StartTime) {
			oldestTx = tx
		}
	}
	s.mu.RUnlock()

	metrics := &TransactionMetrics{
		ActiveTransactions: int64(activeCount),
		MaxConcurrent:      s.config.MaxConcurrentTxs,
		ConfiguredTimeout:  s.config.DefaultTimeout,
	}

	if oldestTx != nil {
		metrics.OldestTransaction = &TransactionSummary{
			ID:       oldestTx.ID,
			Duration: time.Since(oldestTx.StartTime),
			UserID:   oldestTx.UserID,
		}
	}

	return metrics, nil
}

// ===============================
// HELPER METHODS
// ===============================

// getTransaction safely retrieves a transaction context
func (s *transactionService) getTransaction(transactionID string) (*TransactionContext, error) {
	s.mu.RLock()
	txCtx, exists := s.activeTxs[transactionID]
	s.mu.RUnlock()

	if !exists {
		return nil, NewNotFoundError("transaction not found")
	}

	return txCtx, nil
}

// validateBeginRequest validates begin transaction request
func (s *transactionService) validateBeginRequest(req *BeginTransactionRequest) error {
	if req.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	if req.Timeout > 10*time.Minute {
		return fmt.Errorf("timeout cannot exceed 10 minutes")
	}

	return nil
}

// generateTransactionID generates a unique transaction ID
func (s *transactionService) generateTransactionID() string {
	return fmt.Sprintf("tx_%d", time.Now().UnixNano())
}

// generateOperationID generates a unique operation ID
func (s *transactionService) generateOperationID() string {
	return fmt.Sprintf("op_%d", time.Now().UnixNano())
}

// beginWithIsolation begins a transaction with specific isolation level
func (s *transactionService) beginWithIsolation(ctx context.Context, isolationLevel string) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: s.getIsolationLevel(isolationLevel),
	})
}

// getIsolationLevel converts string isolation level to sql.IsolationLevel
func (s *transactionService) getIsolationLevel(level string) sql.IsolationLevel {
	switch level {
	case "READ_UNCOMMITTED":
		return sql.LevelReadUncommitted
	case "READ_COMMITTED":
		return sql.LevelReadCommitted
	case "REPEATABLE_READ":
		return sql.LevelRepeatableRead
	case "SERIALIZABLE":
		return sql.LevelSerializable
	default:
		return sql.LevelReadCommitted
	}
}

// isRetryableError determines if an error is retryable
func (s *transactionService) isRetryableError(err error) bool {
	// Check for common retryable errors
	errStr := err.Error()
	
	retryablePatterns := []string{
		"deadlock",
		"timeout",
		"connection reset",
		"temporary failure",
		"serialization failure",
	}

	for _, pattern := range retryablePatterns {
		if fmt.Sprintf("%v", errStr) == pattern {
			return true
		}
	}

	return false
}

// cleanupExpiredTransactions runs periodic cleanup of expired transactions
func (s *transactionService) cleanupExpiredTransactions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		
		s.mu.Lock()
		expiredTxs := make([]*TransactionContext, 0)
		
		for id, txCtx := range s.activeTxs {
			if now.After(txCtx.Timeout) {
				expiredTxs = append(expiredTxs, txCtx)
				delete(s.activeTxs, id)
			}
		}
		s.mu.Unlock()

		// Rollback expired transactions
		for _, txCtx := range expiredTxs {
			txCtx.mu.Lock()
			if txCtx.Status == TransactionStatusActive {
				s.rollbackTransactionUnsafe(context.Background(), txCtx)
				s.logger.Warn("Rolled back expired transaction",
					zap.String("transaction_id", txCtx.ID),
					zap.Duration("duration", time.Since(txCtx.StartTime)),
				)
			}
			txCtx.mu.Unlock()
		}
	}
}
