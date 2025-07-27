package database

import (
	"context"
	"database/sql"
	"evalhub/internal/config"
	"fmt"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// Manager represents the enterprise database manager
type Manager struct {
	db      *sql.DB
	logger  *zap.Logger
	metrics *Metrics
	health  *HealthChecker
	config  *config.DatabaseConfig
	mu      sync.RWMutex
}

// NewManager creates a new enterprise database manager
func NewManager(cfg *config.DatabaseConfig, logger *zap.Logger) (*Manager, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	logger.Info("🔧 [DEBUG] Creating database manager", 
		zap.String("url", cfg.URL[:20]+"...")) // Don't log full URL

	// Create connection with optimized settings
	db, err := sql.Open("postgres", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
		// 🔍 DEBUG: Check initial connection state
		logger.Info("🔍 [DEBUG] Database opened, checking initial state")
		if err := db.Ping(); err != nil {
			logger.Error("🔴 [DEBUG] Initial ping failed", zap.Error(err))
		} else {
			logger.Info("✅ [DEBUG] Initial ping successful")
		}

	// Configure connection pool for enterprise workloads
	configureConnectionPool(db, cfg)

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("✅ [DEBUG] Database manager creation completed successfully")

	manager := &Manager{
		db:     db,
		logger: logger,
		config: cfg,
	}

	// Initialize monitoring components
	manager.metrics = NewMetrics(db, logger)
	manager.health = NewHealthChecker(manager, logger)

	logger.Info("✅ [DEBUG] Database manager initialized successfully",
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
	)

	return manager, nil
}

// configureConnectionPool sets up enterprise-grade connection pooling
func configureConnectionPool(db *sql.DB, cfg *config.DatabaseConfig) {
	// Set maximum number of open connections
	db.SetMaxOpenConns(cfg.MaxOpenConns)

	// Set maximum number of idle connections
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	// Set maximum lifetime of connections
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Set maximum idle time for connections (Go 1.15+)
	db.SetConnMaxIdleTime(30 * time.Minute)
}

// DB returns the underlying database connection
func (m *Manager) DB() *sql.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 🔍 DEBUG: Check connection state when requested
	if m.db != nil {
		if err := m.db.Ping(); err != nil {
			m.logger.Error("🔴 [DEBUG] DB() called but connection is dead", zap.Error(err))
		} else {
			m.logger.Debug("✅ [DEBUG] DB() called - connection alive")
		}
	} else {
		m.logger.Error("🔴 [DEBUG] DB() called but m.db is nil")
	}
	
	return m.db
}

// Migrate runs database migrations using a separate connection
func (m *Manager) Migrate(migrationsPath string) error {
	m.logger.Info("🔧 [DEBUG] Starting database migrations", zap.String("path", migrationsPath))

	// 🔧 CRITICAL FIX: Create a separate connection for migrations
	// This prevents the migrator from closing our main database connection
	migrationDB, err := sql.Open("postgres", m.config.URL)
	if err != nil {
		return fmt.Errorf("failed to create migration connection: %w", err)
	}
	defer migrationDB.Close() // Safe to close this separate connection

	m.logger.Info("🔧 [DEBUG] Created separate migration connection")

	// Test the migration connection
	if err := migrationDB.Ping(); err != nil {
		return fmt.Errorf("migration connection failed: %w", err)
	}

	driver, err := postgres.WithInstance(migrationDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	migrator, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close() // Now safe - closes migration connection, not main connection

	// Get current version
	currentVersion, dirty, err := migrator.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		m.logger.Warn("Database is in dirty state", zap.Uint("version", currentVersion))
		return fmt.Errorf("database is in dirty state at version %d", currentVersion)
	}

	// Run migrations
	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	newVersion, _, err := migrator.Version()
	if err != nil {
		return fmt.Errorf("failed to get new migration version: %w", err)
	}

	m.logger.Info("Migrations completed successfully",
		zap.Uint("from_version", currentVersion),
		zap.Uint("to_version", newVersion),
	)

	// 🔧 VERIFY: Test that our main connection is still alive
	if err := m.db.Ping(); err != nil {
		m.logger.Error("🔴 [DEBUG] Main connection died during migration", zap.Error(err))
		return fmt.Errorf("main database connection lost during migration: %w", err)
	}

	m.logger.Info("✅ [DEBUG] Main connection survived migrations")
	return nil
}

// ExecContext executes a query with context and metrics
func (m *Manager) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		m.metrics.RecordQuery("exec", duration, nil)

		if duration > 100*time.Millisecond {
			m.logger.Warn("Slow query detected",
				zap.String("type", "exec"),
				zap.Duration("duration", duration),
				zap.String("query", truncateQuery(query)),
			)
		}
	}()

	result, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		m.metrics.RecordQuery("exec", time.Since(start), err)
		m.logger.Error("Query execution failed",
			zap.Error(err),
			zap.String("query", truncateQuery(query)),
		)
	}

	return result, err
}

// QueryContext executes a query with context and metrics
func (m *Manager) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		m.metrics.RecordQuery("query", duration, nil)

		if duration > 100*time.Millisecond {
			m.logger.Warn("Slow query detected",
				zap.String("type", "query"),
				zap.Duration("duration", duration),
				zap.String("query", truncateQuery(query)),
			)
		}
	}()

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		m.metrics.RecordQuery("query", time.Since(start), err)
		m.logger.Error("Query execution failed",
			zap.Error(err),
			zap.String("query", truncateQuery(query)),
		)
	}

	return rows, err
}

// QueryRowContext executes a single-row query with context and metrics
func (m *Manager) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		m.metrics.RecordQuery("query_row", duration, nil)

		if duration > 50*time.Millisecond {
			m.logger.Warn("Slow query detected",
				zap.String("type", "query_row"),
				zap.Duration("duration", duration),
				zap.String("query", truncateQuery(query)),
			)
		}
	}()

	return m.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a new transaction with context
func (m *Manager) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	start := time.Now()
	tx, err := m.db.BeginTx(ctx, opts)

	m.metrics.RecordQuery("begin_tx", time.Since(start), err)

	if err != nil {
		m.logger.Error("Failed to begin transaction", zap.Error(err))
	}

	return tx, err
}

// Health returns the current health status
func (m *Manager) Health(ctx context.Context) *HealthStatus {
	return m.health.Check(ctx)
}

// Metrics returns current database metrics
func (m *Manager) Metrics() *MetricsSnapshot {
	return m.metrics.Snapshot()
}

// Close closes the database connection and cleanup resources
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.health != nil {
		m.health.Stop()
	}

	if m.metrics != nil {
		m.metrics.Stop()
	}

	if m.db != nil {
		m.logger.Info("Closing database connection")
		return m.db.Close()
	}

	return nil
}

// truncateQuery truncates long queries for logging
func truncateQuery(query string) string {
	const maxLength = 200
	if len(query) <= maxLength {
		return query
	}
	return query[:maxLength] + "..."
}

// Stats returns database statistics
func (m *Manager) Stats() sql.DBStats {
	return m.db.Stats()
}
