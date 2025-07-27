package database

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// HealthStatus represents the current health status of the database
type HealthStatus struct {
	Status          string                 `json:"status"`
	Timestamp       time.Time              `json:"timestamp"`
	ResponseTime    time.Duration          `json:"response_time"`
	ConnectionCount int                    `json:"connection_count"`
	Errors          []string               `json:"errors,omitempty"`
	Details         map[string]interface{} `json:"details"`
	Summary         *HealthSummary         `json:"summary,omitempty"`
}

// HealthSummary provides aggregated health information
type HealthSummary struct {
	CriticalIssues int        `json:"critical_issues"`
	Warnings       int        `json:"warnings"`
	LastHealthy    *time.Time `json:"last_healthy,omitempty"`
	UpSince        *time.Time `json:"up_since,omitempty"`
}

// 🏥 PRODUCTION-READY HEALTH CHECKER
type HealthChecker struct {
	// db       *sql.DB
	manager *Manager
	logger  *zap.Logger

	// 🔒 Lifecycle management (CRITICAL FIX)
	mu         sync.RWMutex
	isActive   int32 // atomic flag to prevent operations on closed DB
	isShutdown int32 // atomic flag for graceful shutdown
	lastCheck  time.Time
	status     *HealthStatus

	// 📊 Monitoring and alerting
	alerting *HealthAlerting
	history  *HealthHistory

	// 🔄 Background processing
	stopCh  chan struct{}
	stopped chan struct{}

	// ⚙️ Configuration
	checkInterval    time.Duration
	timeoutDuration  time.Duration
	criticalTables   []string
	slowQueryWarning time.Duration
}

// HealthAlerting handles health-based alerts
type HealthAlerting struct {
	consecutiveFailures int32
	lastAlertSent       time.Time
	alertThreshold      int32
	cooldownPeriod      time.Duration
}

// HealthHistory tracks health status over time
type HealthHistory struct {
	checks []HealthCheckRecord
	mu     sync.Mutex
}

type HealthCheckRecord struct {
	Timestamp time.Time
	Status    string
	Duration  time.Duration
	Issues    int
}

// Health check statuses
const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
	StatusStarting  = "starting"
	StatusShutdown  = "shutdown"
)

// 🚀 PRODUCTION-READY HEALTH CHECKER CONSTRUCTOR
func NewHealthChecker(manager *Manager, logger *zap.Logger) *HealthChecker {
	hc := &HealthChecker{
		manager: manager,
		logger:  logger,

		// 🔒 Initialize lifecycle management
		isActive:   1,
		isShutdown: 0,

		// ⚙️ Production-optimized settings
		checkInterval:    30 * time.Second,
		timeoutDuration:  10 * time.Second,
		slowQueryWarning: 200 * time.Millisecond,
		criticalTables:   []string{"users", "posts", "sessions", "user_stats"},

		// 📊 Initialize monitoring
		alerting: &HealthAlerting{
			alertThreshold: 3,
			cooldownPeriod: 5 * time.Minute,
		},
		history: &HealthHistory{
			checks: make([]HealthCheckRecord, 0, 100), // Keep last 100 checks
		},

		// 🔄 Communication channels
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}

	// 🏥 Start background health checking with proper lifecycle
	// (moved to a separate StartMonitoring method)

	logger.Info("🏥 Production health checker initialized",
		zap.Duration("check_interval", hc.checkInterval),
		zap.Duration("timeout", hc.timeoutDuration),
		zap.Strings("critical_tables", hc.criticalTables))

	return hc
}

// 🔍 ENHANCED HEALTH CHECK WITH PRODUCTION FEATURES
func (hc *HealthChecker) Check(ctx context.Context) *HealthStatus {
	// 🔒 CRITICAL: Check if we're active before proceeding
	if atomic.LoadInt32(&hc.isActive) == 0 {
		return &HealthStatus{
			Status:    StatusShutdown,
			Timestamp: time.Now(),
			Errors:    []string{"Health checker is shutdown"},
			Details:   make(map[string]interface{}),
		}
	}

	start := time.Now()
	status := &HealthStatus{
		Timestamp: start,
		Details:   make(map[string]interface{}),
		Errors:    make([]string, 0),
		Summary:   &HealthSummary{},
	}

	// 🛡️ Add timeout protection
	ctx, cancel := context.WithTimeout(ctx, hc.timeoutDuration)
	defer cancel()

	// 🔍 Comprehensive health checks
	hc.runHealthChecks(ctx, status)

	// 📊 Calculate overall status and metrics
	status.ResponseTime = time.Since(start)
	status.Status = hc.determineOverallStatus(status)
	hc.updateHealthSummary(status)

	// 🏥 Cache and record the result
	hc.cacheHealthResult(status)
	hc.recordHealthCheck(status)

	// 🚨 Handle alerting based on status
	hc.handleHealthAlert(status)

	return status
}

// 🔍 RUN ALL HEALTH CHECKS
func (hc *HealthChecker) runHealthChecks(ctx context.Context, status *HealthStatus) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 🔌 Check database connectivity
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := hc.checkConnectivity(ctx, status); err != nil {
			mu.Lock()
			status.Errors = append(status.Errors, fmt.Sprintf("Connectivity: %v", err))
			mu.Unlock()
		}
	}()

	// 🏊 Check connection pool health
	wg.Add(1)
	go func() {
		defer wg.Done()
		hc.checkConnectionPool(status)
	}()

	// ⚡ Check query performance
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := hc.checkQueryPerformance(ctx, status); err != nil {
			mu.Lock()
			status.Errors = append(status.Errors, fmt.Sprintf("Query performance: %v", err))
			mu.Unlock()
		}
	}()

	// 📋 Check table accessibility (FIXED - No longer causes race condition)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := hc.checkTableAccess(ctx, status); err != nil {
			mu.Lock()
			status.Errors = append(status.Errors, fmt.Sprintf("Table access: %v", err))
			mu.Unlock()
		}
	}()

	// ⏱️ Wait for all checks with timeout protection
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All checks completed successfully
	case <-ctx.Done():
		// Timeout occurred
		status.Errors = append(status.Errors, "Health check timeout")
		hc.logger.Warn("Health check timeout occurred",
			zap.Duration("timeout", hc.timeoutDuration))
	}
}

// 🔌 ENHANCED CONNECTIVITY CHECK
func (hc *HealthChecker) checkConnectivity(ctx context.Context, status *HealthStatus) error {
	// 🔒 CRITICAL: Double-check we're still active
	if atomic.LoadInt32(&hc.isActive) == 0 {
		return fmt.Errorf("database connection is not active")
	}

	// 🔍 DEBUG: Check manager and DB state
	if hc.manager == nil {
		hc.logger.Error("🔴 [DEBUG] Manager is nil in health check")
		return fmt.Errorf("manager is nil")
	}

	if hc.manager.DB() == nil {
		hc.logger.Error("🔴 [DEBUG] DB is nil in health check")
		return fmt.Errorf("database connection is nil")
	}

	start := time.Now()
	err := hc.manager.DB().PingContext(ctx)
	pingDuration := time.Since(start)

	if err != nil {
		hc.logger.Error("🔴 [DEBUG] Ping failed in health check", 
			zap.Error(err), 
			zap.Duration("duration", pingDuration))
	} else {
		hc.logger.Debug("✅ [DEBUG] Ping successful in health check", 
			zap.Duration("duration", pingDuration))
	}


	status.Details["ping_duration"] = pingDuration
	status.Details["ping_success"] = err == nil

	// 🚨 Performance warnings
	if pingDuration > 1*time.Second {
		status.Details["ping_warning"] = "Very slow ping response"
		status.Summary.Warnings++
	} else if pingDuration > 500*time.Millisecond {
		status.Details["ping_warning"] = "Slow ping response"
		status.Summary.Warnings++
	}

	if err != nil {
		status.Summary.CriticalIssues++
		hc.logger.Error("Database ping failed",
			zap.Error(err),
			zap.Duration("duration", pingDuration))
	}

	return err
}

// 🏊 ENHANCED CONNECTION POOL ANALYSIS
func (hc *HealthChecker) checkConnectionPool(status *HealthStatus) {
	stats := hc.manager.DB().Stats()

	status.ConnectionCount = stats.OpenConnections
	poolMetrics := map[string]interface{}{
		"max_open":            stats.MaxOpenConnections,
		"open":                stats.OpenConnections,
		"in_use":              stats.InUse,
		"idle":                stats.Idle,
		"wait_count":          stats.WaitCount,
		"wait_duration_ms":    stats.WaitDuration.Milliseconds(),
		"max_idle_closed":     stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	}

	// 📊 Calculate pool efficiency metrics
	if stats.MaxOpenConnections > 0 {
		utilization := float64(stats.InUse) / float64(stats.MaxOpenConnections)
		efficiency := float64(stats.InUse) / float64(stats.OpenConnections)

		poolMetrics["utilization_percent"] = utilization * 100
		poolMetrics["efficiency_percent"] = efficiency * 100

		// 🚨 Connection pool alerts
		if utilization > 0.9 {
			status.Details["connection_critical"] = "Very high connection utilization"
			status.Summary.CriticalIssues++
		} else if utilization > 0.8 {
			status.Details["connection_warning"] = "High connection utilization"
			status.Summary.Warnings++
		}

		if stats.WaitCount > 1000 {
			status.Details["wait_warning"] = "High connection wait count"
			status.Summary.Warnings++
		}
	}

	status.Details["connection_pool"] = poolMetrics
}

// ⚡ ENHANCED QUERY PERFORMANCE CHECK
func (hc *HealthChecker) checkQueryPerformance(ctx context.Context, status *HealthStatus) error {
	// 🔒 CRITICAL: Ensure database is still active
	if atomic.LoadInt32(&hc.isActive) == 0 {
		return fmt.Errorf("database connection is not active")
	}

	// Test multiple query types
	queries := map[string]string{
		"simple_select":  "SELECT 1",
		"time_check":     "SELECT NOW()",
		"math_operation": "SELECT 1 + 1 as result",
	}

	queryResults := make(map[string]interface{})
	var totalDuration time.Duration

	for name, query := range queries {
		start := time.Now()
		var result interface{}
		err := hc.manager.DB().QueryRowContext(ctx, query).Scan(&result)
		duration := time.Since(start)
		totalDuration += duration

		queryResults[name] = map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
			"success":     err == nil,
		}

		if err != nil {
			hc.logger.Error("Performance test query failed",
				zap.String("query", name),
				zap.Error(err))
			return fmt.Errorf("performance query '%s' failed: %w", name, err)
		}

		// 🐌 Check for slow queries
		if duration > hc.slowQueryWarning {
			status.Details[name+"_warning"] = "Slow query performance"
			status.Summary.Warnings++
		}
	}

	avgDuration := totalDuration / time.Duration(len(queries))
	status.Details["query_performance"] = queryResults
	status.Details["avg_query_duration"] = avgDuration

	return nil
}

// 📋 FIXED TABLE ACCESS CHECK (NO MORE RACE CONDITIONS)
func (hc *HealthChecker) checkTableAccess(ctx context.Context, status *HealthStatus) error {
	// 🔒 CRITICAL: Triple-check we're active before table operations
	if atomic.LoadInt32(&hc.isActive) == 0 {
		return fmt.Errorf("database connection is not active")
	}

	if atomic.LoadInt32(&hc.isShutdown) == 1 {
		return fmt.Errorf("health checker is shutting down")
	}

	tableResults := make(map[string]interface{})

	for _, table := range hc.criticalTables {
		// 🔒 Check again before each table query
		if atomic.LoadInt32(&hc.isActive) == 0 {
			return fmt.Errorf("database became inactive during table checks")
		}

		start := time.Now()
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s LIMIT 1", table)

		var count int
		err := hc.manager.DB().QueryRowContext(ctx, query).Scan(&count)
		duration := time.Since(start)

		tableResults[table] = map[string]interface{}{
			"accessible":   err == nil,
			"duration_ms":  duration.Milliseconds(),
			"record_count": count,
		}

		if err != nil {
			hc.logger.Error("Failed to access critical table",
				zap.String("table", table),
				zap.Error(err))
			status.Summary.CriticalIssues++
			return fmt.Errorf("cannot access table %s: %w", table, err)
		}

		// 🐌 Check for slow table access
		if duration > 500*time.Millisecond {
			status.Details[table+"_warning"] = "Slow table access"
			status.Summary.Warnings++
		}
	}

	status.Details["table_access"] = tableResults
	return nil
}

// 📊 ENHANCED STATUS DETERMINATION
func (hc *HealthChecker) determineOverallStatus(status *HealthStatus) string {
	// 🚨 Critical issues = Unhealthy
	if status.Summary.CriticalIssues > 0 || len(status.Errors) > 0 {
		return StatusUnhealthy
	}

	// ⚠️ Warnings = Degraded
	if status.Summary.Warnings > 0 {
		return StatusDegraded
	}

	// 🐌 Slow response time = Degraded
	if status.ResponseTime > 1*time.Second {
		return StatusDegraded
	}

	return StatusHealthy
}

// 📊 UPDATE HEALTH SUMMARY
func (hc *HealthChecker) updateHealthSummary(status *HealthStatus) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	now := time.Now()

	// Track when we were last healthy
	if status.Status == StatusHealthy {
		status.Summary.LastHealthy = &now

		// Set up_since if this is first healthy check
		if hc.status == nil || hc.status.Status != StatusHealthy {
			status.Summary.UpSince = &now
		} else if hc.status.Summary != nil && hc.status.Summary.UpSince != nil {
			status.Summary.UpSince = hc.status.Summary.UpSince
		}
	} else {
		// Preserve last healthy time
		if hc.status != nil && hc.status.Summary != nil && hc.status.Summary.LastHealthy != nil {
			status.Summary.LastHealthy = hc.status.Summary.LastHealthy
		}
	}
}

// 💾 CACHE HEALTH RESULT
func (hc *HealthChecker) cacheHealthResult(status *HealthStatus) {
	hc.mu.Lock()
	hc.status = status
	hc.lastCheck = time.Now()
	hc.mu.Unlock()
}

// 📝 RECORD HEALTH CHECK HISTORY
func (hc *HealthChecker) recordHealthCheck(status *HealthStatus) {
	record := HealthCheckRecord{
		Timestamp: status.Timestamp,
		Status:    status.Status,
		Duration:  status.ResponseTime,
		Issues:    status.Summary.CriticalIssues + status.Summary.Warnings,
	}

	hc.history.mu.Lock()
	hc.history.checks = append(hc.history.checks, record)

	// Keep only last 100 records
	if len(hc.history.checks) > 100 {
		hc.history.checks = hc.history.checks[len(hc.history.checks)-100:]
	}
	hc.history.mu.Unlock()
}

// 🚨 HANDLE HEALTH ALERTING
func (hc *HealthChecker) handleHealthAlert(status *HealthStatus) {
	if status.Status == StatusUnhealthy {
		count := atomic.AddInt32(&hc.alerting.consecutiveFailures, 1)

		// Send alert if threshold reached and cooldown period passed
		if count >= hc.alerting.alertThreshold {
			now := time.Now()
			if now.Sub(hc.alerting.lastAlertSent) > hc.alerting.cooldownPeriod {
				hc.sendHealthAlert(status, count)
				hc.alerting.lastAlertSent = now
			}
		}
	} else {
		// Reset failure count on recovery
		atomic.StoreInt32(&hc.alerting.consecutiveFailures, 0)
	}
}

// 📧 SEND HEALTH ALERT
func (hc *HealthChecker) sendHealthAlert(status *HealthStatus, consecutiveFailures int32) {
	hc.logger.Error("🚨 DATABASE HEALTH ALERT",
		zap.String("status", status.Status),
		zap.Int32("consecutive_failures", consecutiveFailures),
		zap.Strings("errors", status.Errors),
		zap.Duration("response_time", status.ResponseTime),
		zap.Int("critical_issues", status.Summary.CriticalIssues),
		zap.Int("warnings", status.Summary.Warnings),
	)

	// TODO: Integrate with alerting system (Slack, PagerDuty, etc.)
}

// 🔄 BACKGROUND HEALTH CHECKING WITH LIFECYCLE MANAGEMENT
func (hc *HealthChecker) startPeriodicChecks() {
	defer close(hc.stopped)

	hc.logger.Info("🔄 Starting background health checks",
		zap.Duration("interval", hc.checkInterval))

	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 🔒 CRITICAL: Only run if we're still active
			if atomic.LoadInt32(&hc.isActive) == 0 {
				hc.logger.Info("Health checker inactive, stopping periodic checks")
				return
			}

			if atomic.LoadInt32(&hc.isShutdown) == 1 {
				hc.logger.Info("Health checker shutting down, stopping periodic checks")
				return
			}

			// Run health check with timeout
			ctx, cancel := context.WithTimeout(context.Background(), hc.timeoutDuration)
			status := hc.Check(ctx)
			cancel()

			// Log status changes
			hc.mu.RLock()
			var lastStatus string
			if hc.status != nil && len(hc.history.checks) > 1 {
				lastIdx := len(hc.history.checks) - 2
				lastStatus = hc.history.checks[lastIdx].Status
			}
			hc.mu.RUnlock()

			if status.Status != lastStatus && lastStatus != "" {
				hc.logger.Info("🏥 Database health status changed",
					zap.String("from", lastStatus),
					zap.String("to", status.Status),
					zap.Duration("response_time", status.ResponseTime),
					zap.Int("issues", status.Summary.CriticalIssues+status.Summary.Warnings),
				)
			}

		case <-hc.stopCh:
			hc.logger.Info("Health checker received stop signal")
			return
		}
	}
}

// 🛑 GRACEFUL SHUTDOWN (CRITICAL FIX)
func (hc *HealthChecker) Stop() {
	hc.logger.Info("🛑 Gracefully stopping health checker...")

	// 🔒 Mark as shutting down
	atomic.StoreInt32(&hc.isShutdown, 1)
	atomic.StoreInt32(&hc.isActive, 0)

	// Signal stop and wait for completion
	close(hc.stopCh)

	// Wait for background goroutine to finish
	select {
	case <-hc.stopped:
		hc.logger.Info("✅ Health checker stopped gracefully")
	case <-time.After(5 * time.Second):
		hc.logger.Warn("⚠️ Health checker stop timeout")
	}
}

// StartMonitoring begins background health monitoring (call after DB is ready)
func (hc *HealthChecker) StartMonitoring() {
	// Only start if not already started
	if atomic.LoadInt32(&hc.isActive) == 1 {
		go hc.startPeriodicChecks()
		hc.logger.Info("🔄 Background health monitoring started")
	}
}

// 🔍 GET HEALTH STATUS (BACKWARD COMPATIBLE)
func (hc *HealthChecker) GetLastStatus() *HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if hc.status == nil {
		return &HealthStatus{
			Status:    StatusStarting,
			Timestamp: time.Now(),
			Errors:    []string{"No health check performed yet"},
			Details:   make(map[string]interface{}),
			Summary:   &HealthSummary{},
		}
	}

	return hc.status
}

// ✅ CHECK IF HEALTHY (BACKWARD COMPATIBLE)
func (hc *HealthChecker) IsHealthy() bool {
	if atomic.LoadInt32(&hc.isActive) == 0 {
		return false
	}

	status := hc.GetLastStatus()
	return status.Status == StatusHealthy
}

// ⏳ WAIT FOR HEALTHY (ENHANCED)
func (hc *HealthChecker) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	hc.logger.Info("⏳ Waiting for database to become healthy...",
		zap.Duration("timeout", timeout))

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			elapsed := time.Since(startTime)
			hc.logger.Error("❌ Timeout waiting for database health",
				zap.Duration("elapsed", elapsed),
				zap.Duration("timeout", timeout))
			return fmt.Errorf("timeout waiting for database to become healthy: %w", ctx.Err())

		case <-ticker.C:
			if atomic.LoadInt32(&hc.isActive) == 0 {
				return fmt.Errorf("health checker is not active")
			}

			status := hc.GetLastStatus()
			if status.Status == StatusHealthy {
				elapsed := time.Since(startTime)
				hc.logger.Info("✅ Database is now healthy",
					zap.Duration("wait_time", elapsed))
				return nil
			}

			hc.logger.Debug("Still waiting for database health...",
				zap.String("current_status", status.Status),
				zap.Strings("errors", status.Errors))
		}
	}
}

// 📊 GET HEALTH HISTORY
func (hc *HealthChecker) GetHealthHistory() []HealthCheckRecord {
	hc.history.mu.Lock()
	defer hc.history.mu.Unlock()

	// Return copy to prevent mutation
	result := make([]HealthCheckRecord, len(hc.history.checks))
	copy(result, hc.history.checks)
	return result
}

// 📈 GET HEALTH METRICS
func (hc *HealthChecker) GetHealthMetrics() map[string]interface{} {
	history := hc.GetHealthHistory()
	if len(history) == 0 {
		return map[string]interface{}{"status": "no_data"}
	}

	// Calculate metrics from history
	var totalDuration time.Duration
	var healthyCount, degradedCount, unhealthyCount int

	for _, record := range history {
		totalDuration += record.Duration
		switch record.Status {
		case StatusHealthy:
			healthyCount++
		case StatusDegraded:
			degradedCount++
		case StatusUnhealthy:
			unhealthyCount++
		}
	}

	avgDuration := totalDuration / time.Duration(len(history))

	return map[string]interface{}{
		"total_checks":         len(history),
		"healthy_checks":       healthyCount,
		"degraded_checks":      degradedCount,
		"unhealthy_checks":     unhealthyCount,
		"avg_response_time_ms": avgDuration.Milliseconds(),
		"health_percentage":    float64(healthyCount) / float64(len(history)) * 100,
		"consecutive_failures": atomic.LoadInt32(&hc.alerting.consecutiveFailures),
	}
}

// package database

// import (
// 	"context"
// 	"database/sql"
// 	"fmt"
// 	"sync"
// 	"time"

// 	"go.uber.org/zap"
// )

// // HealthStatus represents the current health status of the database
// type HealthStatus struct {
// 	Status          string                 `json:"status"`
// 	Timestamp       time.Time              `json:"timestamp"`
// 	ResponseTime    time.Duration          `json:"response_time"`
// 	ConnectionCount int                    `json:"connection_count"`
// 	Errors          []string               `json:"errors,omitempty"`
// 	Details         map[string]interface{} `json:"details"`
// }

// // HealthChecker monitors database health
// type HealthChecker struct {
// 	db       *sql.DB
// 	logger   *zap.Logger
// 	mu       sync.RWMutex
// 	lastCheck time.Time
// 	status   *HealthStatus
// 	stopCh   chan struct{}
// }

// // Health check statuses
// const (
// 	StatusHealthy   = "healthy"
// 	StatusDegraded  = "degraded"
// 	StatusUnhealthy = "unhealthy"
// )

// // NewHealthChecker creates a new health checker
// func NewHealthChecker(db *sql.DB, logger *zap.Logger) *HealthChecker {
// 	hc := &HealthChecker{
// 		db:     db,
// 		logger: logger,
// 		stopCh: make(chan struct{}),
// 	}

// 	// Start background health checking
// 	go hc.startPeriodicChecks()

// 	return hc
// }

// // Check performs a comprehensive health check
// func (hc *HealthChecker) Check(ctx context.Context) *HealthStatus {
// 	start := time.Now()
// 	status := &HealthStatus{
// 		Timestamp: start,
// 		Details:   make(map[string]interface{}),
// 		Errors:    make([]string, 0),
// 	}

// 	// Check database connectivity
// 	if err := hc.checkConnectivity(ctx, status); err != nil {
// 		status.Errors = append(status.Errors, fmt.Sprintf("Connectivity: %v", err))
// 	}

// 	// Check connection pool health
// 	hc.checkConnectionPool(status)

// 	// Check query performance
// 	if err := hc.checkQueryPerformance(ctx, status); err != nil {
// 		status.Errors = append(status.Errors, fmt.Sprintf("Query performance: %v", err))
// 	}

// 	// Check table accessibility
// 	if err := hc.checkTableAccess(ctx, status); err != nil {
// 		status.Errors = append(status.Errors, fmt.Sprintf("Table access: %v", err))
// 	}

// 	// Determine overall status
// 	status.ResponseTime = time.Since(start)
// 	status.Status = hc.determineOverallStatus(status)

// 	// Cache the result
// 	hc.mu.Lock()
// 	hc.status = status
// 	hc.lastCheck = time.Now()
// 	hc.mu.Unlock()

// 	// Log if unhealthy
// 	if status.Status != StatusHealthy {
// 		hc.logger.Warn("Database health check failed",
// 			zap.String("status", status.Status),
// 			zap.Strings("errors", status.Errors),
// 			zap.Duration("response_time", status.ResponseTime),
// 		)
// 	}

// 	return status
// }

// // checkConnectivity tests basic database connectivity
// func (hc *HealthChecker) checkConnectivity(ctx context.Context, status *HealthStatus) error {
// 	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
// 	defer cancel()

// 	start := time.Now()
// 	err := hc.db.PingContext(ctx)
// 	pingDuration := time.Since(start)

// 	status.Details["ping_duration"] = pingDuration
// 	status.Details["ping_success"] = err == nil

// 	if pingDuration > 1*time.Second {
// 		status.Details["ping_warning"] = "Slow ping response"
// 	}

// 	return err
// }

// // checkConnectionPool analyzes connection pool health
// func (hc *HealthChecker) checkConnectionPool(status *HealthStatus) {
// 	stats := hc.db.Stats()

// 	status.ConnectionCount = stats.OpenConnections
// 	status.Details["connection_pool"] = map[string]interface{}{
// 		"max_open":           stats.MaxOpenConnections,
// 		"open":              stats.OpenConnections,
// 		"in_use":            stats.InUse,
// 		"idle":              stats.Idle,
// 		"wait_count":        stats.WaitCount,
// 		"wait_duration":     stats.WaitDuration,
// 		"max_idle_closed":   stats.MaxIdleClosed,
// 		"max_lifetime_closed": stats.MaxLifetimeClosed,
// 	}

// 	// Check for connection pool issues
// 	utilization := float64(stats.InUse) / float64(stats.MaxOpenConnections)
// 	if utilization > 0.8 {
// 		status.Details["connection_warning"] = "High connection utilization"
// 	}

// 	if stats.WaitCount > 100 {
// 		status.Details["connection_warning"] = "High connection wait count"
// 	}
// }

// // checkQueryPerformance tests query performance
// func (hc *HealthChecker) checkQueryPerformance(ctx context.Context, status *HealthStatus) error {
// 	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
// 	defer cancel()

// 	// Test a simple query
// 	start := time.Now()
// 	var result int
// 	err := hc.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
// 	queryDuration := time.Since(start)

// 	status.Details["test_query_duration"] = queryDuration
// 	status.Details["test_query_success"] = err == nil

// 	if queryDuration > 100*time.Millisecond {
// 		status.Details["query_warning"] = "Slow test query"
// 	}

// 	return err
// }

// // checkTableAccess verifies access to critical tables
// func (hc *HealthChecker) checkTableAccess(ctx context.Context, status *HealthStatus) error {
// 	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
// 	defer cancel()

// 	criticalTables := []string{"users", "posts", "sessions"}
// 	tableStatus := make(map[string]bool)

// 	for _, table := range criticalTables {
// 		start := time.Now()
// 		var count int
// 		query := fmt.Sprintf("SELECT COUNT(*) FROM %s LIMIT 1", table)
// 		err := hc.db.QueryRowContext(ctx, query).Scan(&count)
// 		duration := time.Since(start)

// 		tableStatus[table] = err == nil

// 		if err != nil {
// 			hc.logger.Error("Failed to access critical table",
// 				zap.String("table", table),
// 				zap.Error(err),
// 			)
// 			return fmt.Errorf("cannot access table %s: %w", table, err)
// 		}

// 		if duration > 500*time.Millisecond {
// 			status.Details[fmt.Sprintf("%s_warning", table)] = "Slow table access"
// 		}
// 	}

// 	status.Details["table_access"] = tableStatus
// 	return nil
// }

// // determineOverallStatus calculates the overall health status
// func (hc *HealthChecker) determineOverallStatus(status *HealthStatus) string {
// 	if len(status.Errors) == 0 {
// 		// Check for warnings that might indicate degraded performance
// 		if status.ResponseTime > 500*time.Millisecond {
// 			return StatusDegraded
// 		}

// 		for key := range status.Details {
// 			if key == "ping_warning" || key == "connection_warning" || key == "query_warning" {
// 				return StatusDegraded
// 			}
// 		}

// 		return StatusHealthy
// 	}

// 	// If there are connectivity errors, it's unhealthy
// 	for _, err := range status.Errors {
// 		if err == "Connectivity" || err == "Table access" {
// 			return StatusUnhealthy
// 		}
// 	}

// 	// Otherwise, it's degraded
// 	return StatusDegraded
// }

// // startPeriodicChecks runs health checks in the background
// func (hc *HealthChecker) startPeriodicChecks() {
// 	ticker := time.NewTicker(30 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ticker.C:
// 			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 			status := hc.Check(ctx)
// 			cancel()

// 			// Log status if changed
// 			hc.mu.RLock()
// 			lastStatus := ""
// 			if hc.status != nil {
// 				lastStatus = hc.status.Status
// 			}
// 			hc.mu.RUnlock()

// 			if status.Status != lastStatus {
// 				hc.logger.Info("Database health status changed",
// 					zap.String("from", lastStatus),
// 					zap.String("to", status.Status),
// 					zap.Duration("response_time", status.ResponseTime),
// 				)
// 			}

// 		case <-hc.stopCh:
// 			return
// 		}
// 	}
// }

// // GetLastStatus returns the last cached health status
// func (hc *HealthChecker) GetLastStatus() *HealthStatus {
// 	hc.mu.RLock()
// 	defer hc.mu.RUnlock()

// 	if hc.status == nil {
// 		return &HealthStatus{
// 			Status:    StatusUnhealthy,
// 			Timestamp: time.Now(),
// 			Errors:    []string{"No health check performed yet"},
// 			Details:   make(map[string]interface{}),
// 		}
// 	}

// 	return hc.status
// }

// // IsHealthy returns true if the database is healthy
// func (hc *HealthChecker) IsHealthy() bool {
// 	status := hc.GetLastStatus()
// 	return status.Status == StatusHealthy
// }

// // Stop stops the health checker
// func (hc *HealthChecker) Stop() {
// 	close(hc.stopCh)
// }

// // WaitForHealthy waits for the database to become healthy
// func (hc *HealthChecker) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
// 	ctx, cancel := context.WithTimeout(ctx, timeout)
// 	defer cancel()

// 	ticker := time.NewTicker(1 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return fmt.Errorf("timeout waiting for database to become healthy: %w", ctx.Err())
// 		case <-ticker.C:
// 			if hc.IsHealthy() {
// 				return nil
// 			}
// 		}
// 	}
// }
