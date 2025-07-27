package database

import (
	"context"
	"database/sql"
	"evalhub/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// DB is the global database manager instance
var DB *Manager

// initMutex prevents concurrent initialization
var initMutex sync.Mutex

// 🚀 PRODUCTION-READY DATABASE INITIALIZATION
// InitDB initializes the enterprise database manager and runs migrations
func InitDB(cfg *config.Config, logger *zap.Logger) error {
	initMutex.Lock()
	defer initMutex.Unlock()
	
	// Prevent double initialization
	if DB != nil {
		logger.Info("Database manager already initialized")
		return nil
	}

	if logger == nil {
		// Create a default logger if none provided
		logger, _ = zap.NewProduction()
	}

	logger.Info("🚀 Starting production database initialization", 
		zap.String("environment", cfg.Server.Environment))

	// Validate and enhance database configuration based on environment
	if err := validateAndEnhanceDatabaseConfig(&cfg.Database, cfg.Server.Environment); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}

	// Create the database manager with enhanced error handling
	manager, err := NewManager(&cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("failed to create database manager: %w", err)
	}

	logger.Info("🔍 [DEBUG] Testing manager connection after creation")
	if testErr := manager.DB().Ping(); testErr != nil {
		logger.Error("🔴 [DEBUG] Manager connection test failed", zap.Error(testErr))
	} else {
		logger.Info("✅ [DEBUG] Manager connection test passed")
	}	

	// 🔒 CRITICAL: Set global instance BEFORE health checks
	DB = manager

	logger.Info("🔍 [DEBUG] Testing connection before migrations")
	if testErr := manager.DB().Ping(); testErr != nil {
		logger.Error("🔴 [DEBUG] Connection failed before migrations", zap.Error(testErr))
	} else {
		logger.Info("✅ [DEBUG] Connection OK before migrations")
	}

	// Determine migrations path with fallback options
	migrationsPath := determineMigrationsPath(cfg.Database.MigrationsPath)
	logger.Info("Using migrations path", zap.String("path", migrationsPath))

	// Run migrations with proper error handling
	if err := runMigrationsWithRetry(manager, migrationsPath, logger, 3); err != nil {
		DB = nil // Reset on failure
		manager.Close()
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Before health check (around line 76):
	logger.Info("🔍 [DEBUG] Testing connection before health check")
	if testErr := manager.DB().Ping(); testErr != nil {
		logger.Error("🔴 [DEBUG] Connection failed before health check", zap.Error(testErr))
	} else {
		logger.Info("✅ [DEBUG] Connection OK before health check")
	}	

	// 🏥 PRODUCTION HEALTH CHECK STRATEGY
	// Wait for database health with proper timeout based on environment
	healthTimeout := getHealthTimeoutForEnvironment(cfg.Server.Environment)
	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()

	// Use exponential backoff for health checks
	if err := waitForHealthWithBackoff(ctx, manager, logger); err != nil {
		DB = nil // Reset on failure
		manager.Close()
		return fmt.Errorf("database failed to become healthy: %w", err)
	}

	// 🏥 Start background monitoring ONLY AFTER database is confirmed healthy
	manager.health.StartMonitoring()	

	// 📊 Log successful initialization with metrics
	logInitializationSuccess(manager, migrationsPath, logger)

	// 🚀 Start background monitoring for production
	if cfg.Server.Environment == "production" {
		startProductionMonitoring(manager, logger)
	}

	return nil
}

// 🔧 ENHANCED CONFIGURATION VALIDATION
func validateAndEnhanceDatabaseConfig(cfg *config.DatabaseConfig, environment string) error {
	if cfg.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// Environment-specific optimizations
	switch environment {
	case "production":
		// 🏭 Production: High performance, secure settings
		if cfg.MaxOpenConns == 0 { cfg.MaxOpenConns = 50 }
		if cfg.MaxIdleConns == 0 { cfg.MaxIdleConns = 20 }
		if cfg.ConnMaxLifetime == 0 { cfg.ConnMaxLifetime = 15 * time.Minute }
		if cfg.SlowQueryThreshold == 0 { cfg.SlowQueryThreshold = 200 * time.Millisecond }
		
		// Ensure SSL is enabled for production
		if !strings.Contains(cfg.URL, "sslmode=") {
			cfg.URL += " sslmode=require"
		}
		
	case "staging":
		// 🧪 Staging: Medium performance, debugging enabled
		if cfg.MaxOpenConns == 0 { cfg.MaxOpenConns = 25 }
		if cfg.MaxIdleConns == 0 { cfg.MaxIdleConns = 10 }
		if cfg.ConnMaxLifetime == 0 { cfg.ConnMaxLifetime = 10 * time.Minute }
		if cfg.SlowQueryThreshold == 0 { cfg.SlowQueryThreshold = 100 * time.Millisecond }
		
	default: // development
		// 🔧 Development: Lower limits, detailed logging
		if cfg.MaxOpenConns == 0 { cfg.MaxOpenConns = 10 }
		if cfg.MaxIdleConns == 0 { cfg.MaxIdleConns = 5 }
		if cfg.ConnMaxLifetime == 0 { cfg.ConnMaxLifetime = 5 * time.Minute }
		if cfg.SlowQueryThreshold == 0 { cfg.SlowQueryThreshold = 50 * time.Millisecond }
	}

	// Set default health check interval if not specified
	if cfg.HealthCheckInterval == 0 {
		cfg.HealthCheckInterval = 30 * time.Second
	}

	// Validate pool settings
	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}

	return nil
}

// 🔄 MIGRATIONS WITH RETRY LOGIC
func runMigrationsWithRetry(manager *Manager, migrationsPath string, logger *zap.Logger, maxRetries int) error {
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Info("Running database migrations", 
			zap.String("path", migrationsPath),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries))
		
		if err := manager.Migrate(migrationsPath); err != nil {
			lastErr = err
			if attempt < maxRetries {
				waitTime := time.Duration(attempt) * time.Second
				logger.Warn("Migration attempt failed, retrying",
					zap.Error(err),
					zap.Int("attempt", attempt),
					zap.Duration("retry_in", waitTime))
				time.Sleep(waitTime)
				continue
			}
		} else {
			logger.Info("Database migrations completed successfully")
			return nil
		}
	}
	
	return fmt.Errorf("migrations failed after %d attempts: %w", maxRetries, lastErr)
}

// 🏥 EXPONENTIAL BACKOFF HEALTH CHECK
func waitForHealthWithBackoff(ctx context.Context, manager *Manager, logger *zap.Logger) error {
	logger.Info("⏳ Waiting for database to become healthy...")
	
	backoff := time.Second
	maxBackoff := 10 * time.Second
	
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for database health: %w", ctx.Err())
		default:
		}
		
		// Check health
		healthStatus := manager.Health(ctx)
		if healthStatus.Status == StatusHealthy {
			logger.Info("✅ Database is healthy", 
				zap.Duration("response_time", healthStatus.ResponseTime))
			return nil
		}
		
		logger.Debug("Database not healthy yet, retrying",
			zap.String("status", healthStatus.Status),
			zap.Strings("errors", healthStatus.Errors),
			zap.Duration("backoff", backoff))
		
		// Wait with exponential backoff
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for database health: %w", ctx.Err())
		case <-time.After(backoff):
		}
		
		// Increase backoff time
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// 📁 SMART MIGRATIONS PATH DETECTION
func determineMigrationsPath(configPath string) string {
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}
	
	// Try multiple common paths
	paths := []string{
		"./migrations",
		"./internal/database/migrations", 
		"./db/migrations",
		"../migrations",
		"../../migrations",
	}
	
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	// Default fallback
	return "./migrations"
}

// ⏱️ ENVIRONMENT-SPECIFIC HEALTH TIMEOUTS
func getHealthTimeoutForEnvironment(environment string) time.Duration {
	switch environment {
	case "production":
		return 60 * time.Second  // Longer timeout for production
	case "staging":
		return 45 * time.Second  // Medium timeout for staging
	default:
		return 30 * time.Second  // Shorter timeout for development
	}
}

// 📊 INITIALIZATION SUCCESS LOGGING
func logInitializationSuccess(manager *Manager, migrationsPath string, logger *zap.Logger) {
	snapshot := manager.Metrics()
	stats := manager.Stats()
	
	logger.Info("🎉 Database initialized successfully",
		zap.String("migrations_path", migrationsPath),
		zap.String("status", "healthy"),
		zap.Int("max_open_connections", stats.MaxOpenConnections),
		zap.Int("open_connections", stats.OpenConnections),
		zap.Duration("avg_query_duration", snapshot.AvgQueryDuration),
	)
}

// 🔍 PRODUCTION MONITORING
func startProductionMonitoring(manager *Manager, logger *zap.Logger) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				snapshot := manager.Metrics()
				stats := manager.Stats()
				
				// Log performance metrics
				logger.Info("📊 Database performance metrics",
					zap.Int64("total_queries", snapshot.QueryCount),
					zap.Int64("error_count", snapshot.ErrorCount),
					zap.Int64("slow_queries", snapshot.SlowQueryCount),
					zap.Duration("avg_duration", snapshot.AvgQueryDuration),
					zap.Int("open_connections", stats.OpenConnections),
					zap.Int("idle_connections", stats.Idle),
				)
				
				// Alert on concerning metrics
				if snapshot.ErrorCount > 100 {
					logger.Warn("🚨 High database error count detected",
						zap.Int64("errors", snapshot.ErrorCount))
				}
				
				if stats.OpenConnections > int(float64(stats.MaxOpenConnections) * 0.8) {
					logger.Warn("⚠️ High database connection usage",
						zap.Int("current", stats.OpenConnections),
						zap.Int("max", stats.MaxOpenConnections))
				}
			}
		}
	}()
	
	logger.Info("🔍 Production database monitoring started")
}

// 🔄 BACKWARD COMPATIBILITY - Existing functions remain unchanged
func GetDB() *Manager {
	return DB
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

func Health(ctx context.Context) *HealthStatus {
	if DB == nil {
		return &HealthStatus{
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Errors:    []string{"Database not initialized"},
			Details:   make(map[string]interface{}),
		}
	}
	return DB.Health(ctx)
}

func GetMetrics() *MetricsSnapshot {
	if DB == nil {
		return &MetricsSnapshot{
			Timestamp: time.Now(),
		}
	}
	return DB.Metrics()
}

// All other existing functions remain exactly the same for backward compatibility
func ExecuteTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %v, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func BatchExecute(ctx context.Context, queries []string, args [][]interface{}) error {
	return ExecuteTransaction(ctx, func(tx *sql.Tx) error {
		for i, query := range queries {
			var queryArgs []interface{}
			if i < len(args) {
				queryArgs = args[i]
			}

			if _, err := tx.ExecContext(ctx, query, queryArgs...); err != nil {
				return fmt.Errorf("failed to execute query %d: %w", i+1, err)
			}
		}
		return nil
	})
}

func IsConnected(ctx context.Context) bool {
	if DB == nil {
		return false
	}

	status := DB.Health(ctx)
	return status.Status == StatusHealthy
}

func WaitForConnection(ctx context.Context, timeout time.Duration) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	return DB.health.WaitForHealthy(ctx, timeout)
}

func GetConnectionStats() map[string]interface{} {
	if DB == nil {
		return map[string]interface{}{
			"error": "database not initialized",
		}
	}

	stats := DB.Stats()
	return map[string]interface{}{
		"max_open_connections":     stats.MaxOpenConnections,
		"open_connections":         stats.OpenConnections,
		"in_use":                  stats.InUse,
		"idle":                    stats.Idle,
		"wait_count":              stats.WaitCount,
		"wait_duration_ms":        stats.WaitDuration.Milliseconds(),
		"max_idle_closed":         stats.MaxIdleClosed,
		"max_idle_time_closed":    stats.MaxIdleTimeClosed,
		"max_lifetime_closed":     stats.MaxLifetimeClosed,
		"utilization_percent":     float64(stats.InUse) / float64(stats.MaxOpenConnections) * 100,
	}
}

func CreateMigrationFile(name string) (string, string, error) {
	timestamp := time.Now().Format("20060102150405")
	baseName := fmt.Sprintf("%s_%s", timestamp, strings.ReplaceAll(name, " ", "_"))
	
	upFile := filepath.Join("migrations", baseName+".up.sql")
	downFile := filepath.Join("migrations", baseName+".down.sql")
	
	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll("migrations", 0755); err != nil {
		return "", "", fmt.Errorf("failed to create migrations directory: %w", err)
	}
	
	// Create up migration file
	upContent := fmt.Sprintf("-- Migration: %s\n-- Created: %s\n\n-- Add your up migration here\n", name, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
		return "", "", fmt.Errorf("failed to create up migration file: %w", err)
	}
	
	// Create down migration file
	downContent := fmt.Sprintf("-- Rollback: %s\n-- Created: %s\n\n-- Add your down migration here\n", name, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		return "", "", fmt.Errorf("failed to create down migration file: %w", err)
	}
	
	return upFile, downFile, nil
}
