// file: internal/services/service_collection.go
package services

import (
	"context"
	"evalhub/internal/cache"
	"evalhub/internal/config"
	"evalhub/internal/database"
	"evalhub/internal/events"
	"evalhub/internal/repositories"
	"fmt"
	"sync"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"go.uber.org/zap"
)

// ServiceCollection holds all enterprise services with dependency injection
type ServiceCollection struct {
	// Core Services
	UserService         UserService         `json:"-"`
	PostService         PostService         `json:"-"`
	CommentService      CommentService      `json:"-"`
	AuthService         AuthService         `json:"-"`
	JobService          JobService          `json:"-"`
	NotificationService NotificationService `json:"-"`

	// Infrastructure Services
	FileService        FileService        `json:"-"`
	CacheService       CacheService       `json:"-"`
	EventService       EventService       `json:"-"`
	TransactionService TransactionService `json:"-"`
	EmailService       EmailService       `json:"-"`

	// Repository Collection
	Repositories *repositories.Collection `json:"-"`

	// Infrastructure Components
	Cache      cache.Cache            `json:"-"`
	EventBus   events.EventBus        `json:"-"`
	Logger     *zap.Logger            `json:"-"`
	Config     *config.Config         `json:"-"`
	DBManager  *database.Manager      `json:"-"`
	Cloudinary *cloudinary.Cloudinary `json:"-"`

	// Service Management
	healthCheckers map[string]HealthChecker `json:"-"`
	metrics        *ServiceMetrics          `json:"-"`
	shutdown       chan struct{}            `json:"-"`
	wg             sync.WaitGroup           `json:"-"`
	mu             sync.RWMutex             `json:"-"`
	initialized    bool                     `json:"-"`
}

// ServiceMetrics tracks overall service collection performance
type ServiceMetrics struct {
	StartTime           time.Time              `json:"start_time"`
	TotalRequests       int64                  `json:"total_requests"`
	SuccessfulRequests  int64                  `json:"successful_requests"`
	FailedRequests      int64                  `json:"failed_requests"`
	AverageResponseTime time.Duration          `json:"average_response_time"`
	ServiceMetrics      map[string]interface{} `json:"service_metrics"`
	LastHealthCheck     time.Time              `json:"last_health_check"`
	mu                  sync.RWMutex           `json:"-"`
}

// ServiceHealth represents the health status of the service collection
type ServiceHealth struct {
	Status          string                   `json:"status"`
	Timestamp       time.Time                `json:"timestamp"`
	Services        map[string]ServiceStatus `json:"services"`
	Dependencies    map[string]ServiceStatus `json:"dependencies"`
	Uptime          time.Duration            `json:"uptime"`
	TotalServices   int                      `json:"total_services"`
	HealthyServices int                      `json:"healthy_services"`
	Issues          []string                 `json:"issues,omitempty"`
}

// ServiceStatus represents the status of an individual service
type ServiceStatus struct {
	Name         string                 `json:"name"`
	Status       string                 `json:"status"` // healthy, degraded, unhealthy
	LastCheck    time.Time              `json:"last_check"`
	ResponseTime time.Duration          `json:"response_time"`
	Error        string                 `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// HealthChecker interface for service health checks
type HealthChecker interface {
	HealthCheck(ctx context.Context) error
	ServiceName() string
}

// ServiceConfig holds service collection configuration
type ServiceConfig struct {
	HealthCheckInterval  time.Duration         `json:"health_check_interval"`
	MetricsInterval      time.Duration         `json:"metrics_interval"`
	ShutdownTimeout      time.Duration         `json:"shutdown_timeout"`
	EnableMetrics        bool                  `json:"enable_metrics"`
	EnableHealthChecks   bool                  `json:"enable_health_checks"`
	CircuitBreakerConfig *CircuitBreakerConfig `json:"circuit_breaker_config"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold int           `json:"failure_threshold"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`
	CheckInterval    time.Duration `json:"check_interval"`
	HalfOpenRequests int           `json:"half_open_requests"`
}

// NewServiceCollection creates a new enterprise service collection
func NewServiceCollection(
	dbManager *database.Manager,
	cfg *config.Config,
	logger *zap.Logger,
) (*ServiceCollection, error) {
	if dbManager == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	collection := &ServiceCollection{
		DBManager:      dbManager,
		Config:         cfg,
		Logger:         logger,
		healthCheckers: make(map[string]HealthChecker),
		metrics: &ServiceMetrics{
			StartTime:      time.Now(),
			ServiceMetrics: make(map[string]interface{}),
		},
		shutdown: make(chan struct{}),
	}

	// Initialize in dependency order
	if err := collection.initializeInfrastructure(); err != nil {
		return nil, fmt.Errorf("failed to initialize infrastructure: %w", err)
	}

	if err := collection.initializeRepositories(); err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %w", err)
	}

	if err := collection.initializeServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	if err := collection.initializeMonitoring(); err != nil {
		return nil, fmt.Errorf("failed to initialize monitoring: %w", err)
	}

	collection.initialized = true
	logger.Info("Service collection initialized successfully",
		zap.Int("total_services", collection.getServiceCount()),
	)

	return collection, nil
}

// ===============================
// INITIALIZATION METHODS
// ===============================

// initializeInfrastructure sets up infrastructure components
func (sc *ServiceCollection) initializeInfrastructure() error {
	sc.Logger.Info("Initializing infrastructure components")

	// Initialize cache (this would depend on your cache implementation)
	sc.Cache = cache.NewMemoryCache(cache.DefaultConfig(), sc.Logger) // Using default config and logger

	// Initialize event bus with default configuration
	sc.EventBus = events.NewInMemoryEventBus(events.DefaultEventBusConfig(), sc.Logger)

	// Initialize Cloudinary
	if sc.Config.Cloudinary.CloudName != "" {
		cloudinary, err := cloudinary.NewFromParams(
			sc.Config.Cloudinary.CloudName,
			sc.Config.Cloudinary.APIKey,
			sc.Config.Cloudinary.APISecret,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize Cloudinary: %w", err)
		}
		sc.Cloudinary = cloudinary
	}

	sc.Logger.Info("Infrastructure components initialized")
	return nil
}

// initializeRepositories sets up repository layer
func (sc *ServiceCollection) initializeRepositories() error {
	sc.Logger.Info("Initializing repositories")

	db := sc.DBManager.DB() // Get *sql.DB from Manager
	if db == nil {
		return fmt.Errorf("failed to get database connection from manager")
	}

	repoConfig := &repositories.RepositoryConfig{
		EnableQueryLogging: true,
		SlowQueryThreshold: 100 * time.Millisecond,
		CacheEnabled:       true,
	}

	var err error
	sc.Repositories, err = repositories.NewCollection(sc.DBManager, sc.Logger, repoConfig)
	if err != nil {
		return fmt.Errorf("failed to create repository collection: %w", err)
	}

	sc.Logger.Info("Repositories initialized")
	return nil
}

// initializeServices sets up service layer with dependency injection
func (sc *ServiceCollection) initializeServices() error {
	sc.Logger.Info("Initializing services")

	// Initialize infrastructure services first
	if err := sc.initializeInfrastructureServices(); err != nil {
		return fmt.Errorf("failed to initialize infrastructure services: %w", err)
	}

	// Initialize core business services
	if err := sc.initializeCoreServices(); err != nil {
		return fmt.Errorf("failed to initialize core services: %w", err)
	}

	sc.Logger.Info("All services initialized")
	return nil
}

// initializeInfrastructureServices initializes infrastructure services
func (sc *ServiceCollection) initializeInfrastructureServices() error {
	// Cache Service
	sc.CacheService = NewCacheService(
		nil,      // primary cache backend - replace with Redis
		sc.Cache, // fallback cache
		sc.Logger,
		DefaultCacheConfig(),
	)

	// Event Service
	sc.EventService = NewEventService(
		sc.EventBus,
		sc.Logger,
		DefaultEventConfig(),
	)

	// Transaction Service
	sc.TransactionService = NewTransactionService(
		sc.DBManager.DB(),
		sc.EventBus,
		sc.Logger,
		DefaultTransactionConfig(),
	)

	// Email Service
	sc.EmailService = NewEmailService(
		sc.Logger,
	)

	// File Service
	if sc.Cloudinary != nil {
		sc.FileService = NewFileService(
			sc.Cloudinary,
			sc.Cache,
			sc.EventBus,
			sc.Logger,
			DefaultFileConfig(),
		)
	}

	return nil
}

// initializeCoreServices initializes core business services
func (sc *ServiceCollection) initializeCoreServices() error {
	// User Service (foundational service)
	sc.UserService = NewUserService(
		sc.Repositories.User,
		sc.Repositories.Session,
		sc.Cache,
		sc.EventBus,
		sc.FileService,
		sc.Logger,
	)

	// Auth Service (depends on User Service and Email Service)
	sc.AuthService = NewAuthService(
		sc.Repositories.User,
		sc.Repositories.Session,
		sc.Cache,
		sc.EventBus,
		sc.UserService,
		sc.FileService,
		sc.EmailService,
		sc.Logger,
		DefaultAuthConfig(),
	)

	// Post Service (depends on User Service, Transaction Service)
	sc.PostService = NewPostService(
		sc.Repositories.Post,
		sc.Repositories.User,
		sc.Repositories.Comment,
		sc.Cache,
		sc.EventBus,
		sc.FileService,
		sc.UserService,
		sc.TransactionService,
		sc.Logger,
		DefaultPostConfig(),
	)

	// Comment Service (depends on Post Service, User Service)
	sc.CommentService = NewCommentService(
		sc.Repositories.Comment,
		sc.Repositories.Post,
		sc.Repositories.User,
		sc.Cache,
		sc.EventBus,
		sc.UserService,
		sc.TransactionService,
		sc.Logger,
		DefaultCommentConfig(),
	)

	// Job Service (basic implementation)
	sc.JobService = NewJobService(sc.Repositories.Job)

	// Initialize Notification Service (placeholder)
	// sc.NotificationService = NewNotificationService(...)

	return nil
}

// initializeMonitoring sets up monitoring and health checks
func (sc *ServiceCollection) initializeMonitoring() error {
	sc.Logger.Info("Initializing monitoring")

	// Register health checkers for services that support it
	if hc, ok := sc.CacheService.(HealthChecker); ok {
		sc.registerHealthChecker(hc)
	}
	if hc, ok := sc.TransactionService.(HealthChecker); ok {
		sc.registerHealthChecker(hc)
	}

	// Start background monitoring
	if sc.Config.IsProduction() {
		go sc.startHealthCheckMonitoring()
		go sc.startMetricsCollection()
	}

	sc.Logger.Info("Monitoring initialized")
	return nil
}

// ===============================
// SERVICE ACCESS METHODS
// ===============================

// GetUserService returns the user service
func (sc *ServiceCollection) GetUserService() UserService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.UserService
}

// GetPostService returns the post service
func (sc *ServiceCollection) GetPostService() PostService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.PostService
}

// GetCommentService returns the comment service
func (sc *ServiceCollection) GetCommentService() CommentService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.CommentService
}

// GetAuthService returns the auth service
func (sc *ServiceCollection) GetAuthService() AuthService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.AuthService
}

// GetJobService returns the job service
func (sc *ServiceCollection) GetJobService() JobService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.JobService
}

// GetFileService returns the file service
func (sc *ServiceCollection) GetFileService() FileService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.FileService
}

// GetCacheService returns the cache service
func (sc *ServiceCollection) GetCacheService() CacheService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.CacheService
}

// GetEventService returns the event service
func (sc *ServiceCollection) GetEventService() EventService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.EventService
}

// GetTransactionService returns the transaction service
func (sc *ServiceCollection) GetTransactionService() TransactionService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.TransactionService
}

// ===============================
// HEALTH AND MONITORING
// ===============================

// HealthCheck performs comprehensive health check of all services
func (sc *ServiceCollection) HealthCheck(ctx context.Context) (*ServiceHealth, error) {
	sc.Logger.Debug("Performing service collection health check")

	health := &ServiceHealth{
		Status:       "healthy",
		Timestamp:    time.Now(),
		Services:     make(map[string]ServiceStatus),
		Dependencies: make(map[string]ServiceStatus),
		Uptime:       time.Since(sc.metrics.StartTime),
		Issues:       []string{},
	}

	// Check database connectivity
	dbStatus := sc.checkDatabaseHealth(ctx)
	health.Dependencies["database"] = dbStatus
	if dbStatus.Status != "healthy" {
		health.Status = "degraded"
		health.Issues = append(health.Issues, fmt.Sprintf("Database: %s", dbStatus.Error))
	}

	// Check cache connectivity
	cacheStatus := sc.checkCacheHealth(ctx)
	health.Dependencies["cache"] = cacheStatus
	if cacheStatus.Status != "healthy" {
		health.Status = "degraded"
		health.Issues = append(health.Issues, fmt.Sprintf("Cache: %s", cacheStatus.Error))
	}

	// Check individual services
	healthyCount := 0
	totalCount := 0

	for name, checker := range sc.healthCheckers {
		totalCount++
		status := sc.checkServiceHealth(ctx, checker)
		health.Services[name] = status

		if status.Status == "healthy" {
			healthyCount++
		} else {
			if health.Status == "healthy" {
				health.Status = "degraded"
			}
			health.Issues = append(health.Issues, fmt.Sprintf("%s: %s", name, status.Error))
		}
	}

	health.TotalServices = totalCount
	health.HealthyServices = healthyCount

	// Determine overall status
	if len(health.Issues) == 0 {
		health.Status = "healthy"
	} else if healthyCount > totalCount/2 {
		health.Status = "degraded"
	} else {
		health.Status = "unhealthy"
	}

	// Update metrics
	sc.metrics.mu.Lock()
	sc.metrics.LastHealthCheck = time.Now()
	sc.metrics.mu.Unlock()

	sc.Logger.Debug("Health check completed",
		zap.String("status", health.Status),
		zap.Int("healthy_services", healthyCount),
		zap.Int("total_services", totalCount),
		zap.Int("issues", len(health.Issues)),
	)

	return health, nil
}

// GetMetrics returns service collection metrics
func (sc *ServiceCollection) GetMetrics(ctx context.Context) (*ServiceMetrics, error) {
	sc.metrics.mu.RLock()
	defer sc.metrics.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := &ServiceMetrics{
		StartTime:           sc.metrics.StartTime,
		TotalRequests:       sc.metrics.TotalRequests,
		SuccessfulRequests:  sc.metrics.SuccessfulRequests,
		FailedRequests:      sc.metrics.FailedRequests,
		AverageResponseTime: sc.metrics.AverageResponseTime,
		ServiceMetrics:      make(map[string]interface{}),
		LastHealthCheck:     sc.metrics.LastHealthCheck,
	}

	// Copy service metrics
	for k, v := range sc.metrics.ServiceMetrics {
		metrics.ServiceMetrics[k] = v
	}

	// Collect current metrics from services
	if sc.CacheService != nil {
		if stats := sc.CacheService.GetStats(ctx); stats != nil {
			metrics.ServiceMetrics["cache"] = stats
		}
	}

	if sc.EventService != nil {
		if eventMetrics := sc.EventService.GetMetrics(); eventMetrics != nil {
			metrics.ServiceMetrics["events"] = eventMetrics
		}
	}

	if sc.TransactionService != nil {
		if txMetrics, err := sc.TransactionService.GetTransactionMetrics(ctx); err == nil {
			metrics.ServiceMetrics["transactions"] = txMetrics
		}
	}

	return metrics, nil
}

// ===============================
// SERVICE LIFECYCLE MANAGEMENT
// ===============================

// Start starts background services and monitoring
func (sc *ServiceCollection) Start(ctx context.Context) error {
	if !sc.initialized {
		return fmt.Errorf("service collection not initialized")
	}

	sc.Logger.Info("Starting service collection")

	// Start event processing
	if starter, ok := sc.EventService.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(ctx); err != nil {
			return fmt.Errorf("failed to start event service: %w", err)
		}
	}

	// Start cache background processes
	if starter, ok := sc.CacheService.(interface{ Start(context.Context) error }); ok {
		if err := starter.Start(ctx); err != nil {
			return fmt.Errorf("failed to start cache service: %w", err)
		}
	}

	// Start monitoring
	go sc.startHealthCheckMonitoring()
	go sc.startMetricsCollection()

	sc.Logger.Info("Service collection started successfully")
	return nil
}

// Shutdown gracefully shuts down all services
func (sc *ServiceCollection) Shutdown(ctx context.Context) error {
	sc.Logger.Info("Shutting down service collection")

	// Signal shutdown
	close(sc.shutdown)

	// Shutdown services in reverse dependency order
	var shutdownErrors []error

	// Shutdown infrastructure services
	if sc.EventService != nil {
		if err := sc.EventService.Shutdown(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("event service shutdown: %w", err))
		}
	}

	if sc.TransactionService != nil {
		// Cancel any active transactions
		if activeTransactions, err := sc.TransactionService.GetActiveTransactions(ctx); err == nil {
			for _, tx := range activeTransactions {
				sc.TransactionService.RollbackTransaction(ctx, tx.ID)
			}
		}
	}

	// Wait for background processes to finish
	done := make(chan struct{})
	go func() {
		sc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		sc.Logger.Info("All background processes stopped")
	case <-ctx.Done():
		sc.Logger.Warn("Shutdown timeout exceeded")
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown timeout exceeded"))
	}

	// Close database connections if needed
	if sc.DBManager != nil {
		if err := sc.DBManager.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("database close: %w", err))
		}
	}

	if len(shutdownErrors) > 0 {
		sc.Logger.Error("Errors occurred during shutdown",
			zap.Int("error_count", len(shutdownErrors)),
		)
		return fmt.Errorf("shutdown completed with %d errors", len(shutdownErrors))
	}

	sc.Logger.Info("Service collection shutdown completed successfully")
	return nil
}

// ===============================
// PRIVATE HELPER METHODS
// ===============================

// registerHealthChecker registers a health checker
func (sc *ServiceCollection) registerHealthChecker(hc HealthChecker) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.healthCheckers[hc.ServiceName()] = hc
}

// checkServiceHealth checks the health of an individual service
func (sc *ServiceCollection) checkServiceHealth(ctx context.Context, checker HealthChecker) ServiceStatus {
	start := time.Now()

	status := ServiceStatus{
		Name:         checker.ServiceName(),
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: 0,
		Metadata:     make(map[string]interface{}),
	}

	// Create timeout context for health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := checker.HealthCheck(checkCtx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
	}

	status.ResponseTime = time.Since(start)

	return status
}

// checkDatabaseHealth checks database connectivity
func (sc *ServiceCollection) checkDatabaseHealth(ctx context.Context) ServiceStatus {
	start := time.Now()
	status := ServiceStatus{
		Name:         "database",
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: 0,
	}

	if err := sc.DBManager.DB().PingContext(ctx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
	}

	status.ResponseTime = time.Since(start)
	return status
}

// checkCacheHealth checks cache connectivity
func (sc *ServiceCollection) checkCacheHealth(ctx context.Context) ServiceStatus {
	start := time.Now()
	status := ServiceStatus{
		Name:         "cache",
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: 0,
	}

	// Simple cache test
	testKey := "health_check_test"
	testValue := "ok"

	if err := sc.Cache.Set(ctx, testKey, testValue, 1*time.Minute); err != nil {
		status.Status = "unhealthy"
		status.Error = fmt.Sprintf("cache set failed: %v", err)
	} else {
		if _, found := sc.Cache.Get(ctx, testKey); !found {
			status.Status = "unhealthy"
			status.Error = "cache get failed"
		}
		// Clean up test key
		sc.Cache.Delete(ctx, testKey)
	}

	status.ResponseTime = time.Since(start)
	return status
}

// startHealthCheckMonitoring starts background health check monitoring
func (sc *ServiceCollection) startHealthCheckMonitoring() {
	sc.wg.Add(1)
	defer sc.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Health check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			health, err := sc.HealthCheck(ctx)
			cancel()

			if err != nil {
				sc.Logger.Error("Health check failed", zap.Error(err))
			} else if health.Status != "healthy" {
				sc.Logger.Warn("Service health degraded",
					zap.String("status", health.Status),
					zap.Strings("issues", health.Issues),
				)
			}

		case <-sc.shutdown:
			sc.Logger.Info("Health check monitoring stopped")
			return
		}
	}
}

// startMetricsCollection starts background metrics collection
func (sc *ServiceCollection) startMetricsCollection() {
	sc.wg.Add(1)
	defer sc.wg.Done()

	ticker := time.NewTicker(1 * time.Minute) // Collect metrics every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			metrics, err := sc.GetMetrics(ctx)
			cancel()

			if err != nil {
				sc.Logger.Error("Metrics collection failed", zap.Error(err))
			} else {
				sc.Logger.Debug("Metrics collected",
					zap.Int64("total_requests", metrics.TotalRequests),
					zap.Int64("successful_requests", metrics.SuccessfulRequests),
					zap.Duration("avg_response_time", metrics.AverageResponseTime),
				)
			}

		case <-sc.shutdown:
			sc.Logger.Info("Metrics collection stopped")
			return
		}
	}
}

// getServiceCount returns the total number of initialized services
func (sc *ServiceCollection) getServiceCount() int {
	count := 0

	if sc.UserService != nil {
		count++
	}
	if sc.PostService != nil {
		count++
	}
	if sc.CommentService != nil {
		count++
	}
	if sc.AuthService != nil {
		count++
	}
	if sc.JobService != nil {
		count++
	}
	if sc.FileService != nil {
		count++
	}
	if sc.CacheService != nil {
		count++
	}
	if sc.EventService != nil {
		count++
	}
	if sc.TransactionService != nil {
		count++
	}

	return count
}

// ===============================
// METRICS TRACKING
// ===============================

// RecordRequest records a service request for metrics
func (sc *ServiceCollection) RecordRequest(successful bool, responseTime time.Duration) {
	sc.metrics.mu.Lock()
	defer sc.metrics.mu.Unlock()

	sc.metrics.TotalRequests++
	if successful {
		sc.metrics.SuccessfulRequests++
	} else {
		sc.metrics.FailedRequests++
	}

	// Update average response time (simple moving average)
	if sc.metrics.AverageResponseTime == 0 {
		sc.metrics.AverageResponseTime = responseTime
	} else {
		sc.metrics.AverageResponseTime = (sc.metrics.AverageResponseTime + responseTime) / 2
	}
}

// ===============================
// CONFIGURATION HELPERS
// ===============================

// GetDefaultServiceConfig returns default service configuration
func GetDefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		HealthCheckInterval: 30 * time.Second,
		MetricsInterval:     1 * time.Minute,
		ShutdownTimeout:     30 * time.Second,
		EnableMetrics:       true,
		EnableHealthChecks:  true,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			CheckInterval:    10 * time.Second,
			HalfOpenRequests: 3,
		},
	}
}

// IsInitialized returns whether the service collection is fully initialized
func (sc *ServiceCollection) IsInitialized() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.initialized
}

// GetLogger returns the logger instance
func (sc *ServiceCollection) GetLogger() *zap.Logger {
	return sc.Logger
}

// GetConfig returns the configuration instance
func (sc *ServiceCollection) GetConfig() *config.Config {
	return sc.Config
}

// ===============================
// MIDDLEWARE AND INTERCEPTORS
// ===============================

// WithMetrics wraps a service call with metrics collection
func (sc *ServiceCollection) WithMetrics(serviceName string, operation func() error) error {
	start := time.Now()
	err := operation()
	duration := time.Since(start)

	successful := err == nil
	sc.RecordRequest(successful, duration)

	// Log service call
	sc.Logger.Debug("Service call completed",
		zap.String("service", serviceName),
		zap.Duration("duration", duration),
		zap.Bool("successful", successful),
		zap.Error(err),
	)

	return err
}

// WithTimeout wraps a service call with timeout
func (sc *ServiceCollection) WithTimeout(timeout time.Duration, operation func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return operation(ctx)
}
