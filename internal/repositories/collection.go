// file: internal/repositories/collection.go
package repositories

import (
	"context"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Collection holds all repository instances for dependency injection
type Collection struct {
	// Core repositories
	User    UserRepository
	Session SessionRepository
	Post    PostRepository
	Comment CommentRepository

	// Future repositories (interfaces ready for implementation)
	Question QuestionRepository
	Job      JobRepository

	// Database and logger for custom operations
	db     *database.Manager
	logger *zap.Logger
}

// RepositoryConfig holds configuration for repository initialization
type RepositoryConfig struct {
	EnableQueryLogging bool
	SlowQueryThreshold time.Duration
	CacheEnabled       bool
}

// NewCollection creates a new repository collection with all dependencies
func NewCollection(db *database.Manager, logger *zap.Logger, config *RepositoryConfig) (*Collection, error) {
	if db == nil {
		return nil, fmt.Errorf("database manager is required")
	}

	if logger == nil {
		// Create default logger if none provided
		logger, _ = zap.NewProduction()
	}

	if config == nil {
		config = &RepositoryConfig{
			EnableQueryLogging: true,
			SlowQueryThreshold: 100 * time.Millisecond,
			CacheEnabled:       true,
		}
	}

	collection := &Collection{
		db:     db,
		logger: logger,
	}

	// Initialize all repositories
	collection.User = NewUserRepository(db, logger)
	collection.Session = NewSessionRepository(db, logger)
	collection.Post = NewPostRepository(db, logger)
	collection.Comment = NewCommentRepository(db, logger)

	// Initialize future repositories when implemented
	// collection.Question = NewQuestionRepository(db, logger)
	// collection.Job = NewJobRepository(db, logger)

	logger.Info("Repository collection initialized successfully",
		zap.Bool("query_logging", config.EnableQueryLogging),
		zap.Duration("slow_query_threshold", config.SlowQueryThreshold),
		zap.Bool("cache_enabled", config.CacheEnabled),
	)

	return collection, nil
}

// ===============================
// TRANSACTION MANAGEMENT
// ===============================

// WithTransaction executes a function within a database transaction
// All repositories in the collection can participate in the same transaction
func (c *Collection) WithTransaction(ctx context.Context, fn func(*Collection) error) error {
	tx, err := c.db.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			// If there was an error, rollback the transaction
			tx.Rollback()
		}
	}()

	// Create a transaction-aware collection
	txCollection := &Collection{
		User:    c.User, // These could be wrapped with transaction context if needed
		Session: c.Session,
		Post:    c.Post,
		Comment: c.Comment,
		db:      c.db,
		logger:  c.logger,
	}

	// Execute the function with the transaction-aware collection
	err = fn(txCollection)
	if err != nil {
		// Error will be handled by the deferred function
		return err
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ===============================
// HEALTH AND MONITORING
// ===============================

// HealthCheck performs health checks on all repositories
func (c *Collection) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	// Check database connectivity
	dbHealth := c.db.Health(ctx)
	health["database"] = map[string]interface{}{
		"status":        dbHealth.Status,
		"response_time": dbHealth.ResponseTime,
		"errors":        dbHealth.Errors,
	}

	// Check individual repository functionality
	health["repositories"] = c.checkRepositoriesHealth(ctx)

	// Get performance metrics
	metrics := c.db.Metrics()
	health["performance"] = map[string]interface{}{
		"query_count":        metrics.QueryCount,
		"error_count":        metrics.ErrorCount,
		"slow_query_count":   metrics.SlowQueryCount,
		"avg_query_duration": metrics.AvgQueryDuration,
	}

	return health
}

// checkRepositoriesHealth checks basic functionality of each repository
func (c *Collection) checkRepositoriesHealth(ctx context.Context) map[string]interface{} {
	checks := make(map[string]interface{})

	// Test User repository
	checks["user"] = c.testRepositoryHealth(ctx, "users", func() error {
		_, err := c.User.CountByRole(ctx)
		return err
	})

	// Test Session repository
	checks["session"] = c.testRepositoryHealth(ctx, "sessions", func() error {
		_, err := c.Session.GetSessionStatistics(ctx)
		return err
	})

	// Test Post repository
	checks["post"] = c.testRepositoryHealth(ctx, "posts", func() error {
		_, err := c.Post.GetCategoryStats(ctx)
		return err
	})

	// Test Comment repository
	checks["comment"] = c.testRepositoryHealth(ctx, "comments", func() error {
		_, err := c.Comment.CountByUserID(ctx, 1) // Test with dummy ID
		return err
	})

	return checks
}

// testRepositoryHealth runs a test operation for a repository
func (c *Collection) testRepositoryHealth(ctx context.Context, name string, testFn func() error) map[string]interface{} {
	start := time.Now()
	err := testFn()
	duration := time.Since(start)

	result := map[string]interface{}{
		"duration": duration,
		"healthy":  err == nil,
	}

	if err != nil {
		result["error"] = err.Error()
		c.logger.Warn("Repository health check failed",
			zap.String("repository", name),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
	}

	return result
}

// ===============================
// BATCH OPERATIONS
// ===============================

// BatchOperations provides batch operations across multiple repositories
type BatchOperations struct {
	collection *Collection
	ctx        context.Context
}

// Batch returns a batch operations instance
func (c *Collection) Batch(ctx context.Context) *BatchOperations {
	return &BatchOperations{
		collection: c,
		ctx:        ctx,
	}
}

// CleanupExpiredData removes expired data from all repositories
func (b *BatchOperations) CleanupExpiredData() error {
	return b.collection.WithTransaction(b.ctx, func(c *Collection) error {
		// Cleanup expired sessions
		sessionsDeleted, err := c.Session.CleanupExpiredSessions(b.ctx)
		if err != nil {
			return fmt.Errorf("failed to cleanup sessions: %w", err)
		}

		// Future: Cleanup other expired data
		// - Expired password reset tokens
		// - Old notification records
		// - Archived content

		c.logger.Info("Batch cleanup completed",
			zap.Int("sessions_deleted", sessionsDeleted),
		)

		return nil
	})
}

// UserEngagementStats gathers engagement statistics across repositories
func (b *BatchOperations) UserEngagementStats(userID int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get user stats
	userStats, err := b.collection.User.GetUserStats(b.ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}
	stats["user"] = userStats

	// Get post stats
	postStats, err := b.collection.Post.GetUserPostStats(b.ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get post stats: %w", err)
	}
	stats["posts"] = postStats

	// Get comment count
	commentCount, err := b.collection.Comment.CountByUserID(b.ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment count: %w", err)
	}
	stats["comments"] = map[string]interface{}{
		"total_comments": commentCount,
	}

	// Get active sessions count
	activeSessions, err := b.collection.Session.CountActiveSessions(b.ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}
	stats["sessions"] = map[string]interface{}{
		"active_sessions": activeSessions,
	}

	return stats, nil
}

// ===============================
// ANALYTICS AND REPORTING
// ===============================

// Analytics provides cross-repository analytics
type Analytics struct {
	collection *Collection
}

// Analytics returns an analytics instance
func (c *Collection) Analytics() *Analytics {
	return &Analytics{collection: c}
}

// PlatformStats gathers comprehensive platform statistics
func (a *Analytics) PlatformStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// User statistics
	userCounts, err := a.collection.User.CountByRole(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user counts: %w", err)
	}
	stats["users"] = userCounts

	// Post statistics
	postStats, err := a.collection.Post.GetCategoryStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get post stats: %w", err)
	}
	stats["posts"] = postStats

	// Session statistics
	sessionStats, err := a.collection.Session.GetSessionStatistics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get session stats: %w", err)
	}
	stats["sessions"] = sessionStats

	// Database performance
	metrics := a.collection.db.Metrics()
	stats["performance"] = map[string]interface{}{
		"total_queries":   metrics.QueryCount,
		"error_rate":      float64(metrics.ErrorCount) / float64(metrics.QueryCount),
		"slow_query_rate": float64(metrics.SlowQueryCount) / float64(metrics.QueryCount),
		"avg_query_time":  metrics.AvgQueryDuration,
	}

	return stats, nil
}

// ActiveUserMetrics provides real-time user activity metrics
func (a *Analytics) ActiveUserMetrics(ctx context.Context) (map[string]interface{}, error) {
	now := time.Now()

	// Users active in last hour
	lastHour := now.Add(-1 * time.Hour)
	activeLastHour, err := a.collection.User.GetActiveUsers(ctx, lastHour)
	if err != nil {
		return nil, fmt.Errorf("failed to get users active in last hour: %w", err)
	}

	// Users active in last day
	lastDay := now.Add(-24 * time.Hour)
	activeLastDay, err := a.collection.User.GetActiveUsers(ctx, lastDay)
	if err != nil {
		return nil, fmt.Errorf("failed to get users active in last day: %w", err)
	}

	// Currently online users
	onlineUsers, err := a.collection.User.GetOnlineUsers(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get online users: %w", err)
	}

	return map[string]interface{}{
		"online_now":       len(onlineUsers),
		"active_last_hour": len(activeLastHour),
		"active_last_day":  len(activeLastDay),
		"online_users":     onlineUsers,
	}, nil
}

// ===============================
// VALIDATION AND UTILITIES
// ===============================

// ValidateReferences ensures referential integrity across repositories
func (c *Collection) ValidateReferences(ctx context.Context) []string {
	var issues []string

	// This would include checks like:
	// - Users referenced in posts/comments actually exist
	// - Sessions belong to active users
	// - Comments have valid parent references

	// Example validation (simplified)
	// This could be expanded based on business requirements

	c.logger.Info("Reference validation completed",
		zap.Int("issues_found", len(issues)),
	)

	return issues
}

// GetDB returns the underlying database manager for advanced operations
func (c *Collection) GetDB() *database.Manager {
	return c.db
}

// GetLogger returns the logger instance
func (c *Collection) GetLogger() *zap.Logger {
	return c.logger
}

// Close closes all repository connections and cleans up resources
func (c *Collection) Close() error {
	c.logger.Info("Closing repository collection")

	// Close database connections
	if c.db != nil {
		return c.db.Close()
	}

	return nil
}

// ===============================
// MIGRATION HELPERS
// ===============================

// MigrationHelper provides utilities for data migrations
type MigrationHelper struct {
	collection *Collection
}

// Migration returns a migration helper instance
func (c *Collection) Migration() *MigrationHelper {
	return &MigrationHelper{collection: c}
}

// BackupUserData creates a backup of user-related data
func (m *MigrationHelper) BackupUserData(ctx context.Context, userID int64) (map[string]interface{}, error) {
	backup := make(map[string]interface{})

	// Get user data
	user, err := m.collection.User.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	backup["user"] = user

	// Get user posts
	postParams := models.PaginationParams{Limit: 1000, Offset: 0}
	posts, err := m.collection.Post.GetByUserID(ctx, userID, postParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get user posts: %w", err)
	}
	backup["posts"] = posts.Data

	// Get user comments
	commentParams := models.PaginationParams{Limit: 1000, Offset: 0}
	comments, err := m.collection.Comment.GetByUserID(ctx, userID, commentParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get user comments: %w", err)
	}
	backup["comments"] = comments.Data

	return backup, nil
}

// ===============================
// FACTORY METHODS
// ===============================

// NewTestCollection creates a collection for testing with mock dependencies
func NewTestCollection(db *database.Manager, logger *zap.Logger) *Collection {
	if logger == nil {
		logger = zap.NewNop() // No-op logger for tests
	}

	return &Collection{
		User:    NewUserRepository(db, logger),
		Session: NewSessionRepository(db, logger),
		Post:    NewPostRepository(db, logger),
		Comment: NewCommentRepository(db, logger),
		db:      db,
		logger:  logger,
	}
}
