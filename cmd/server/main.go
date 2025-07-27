// @title           EvalHub API
// @version         1.0.0
// @description     Enterprise API for EvalHub platform with role-based security
// @termsOfService  https://evalhub.com/terms

// @contact.name   EvalHub API Support
// @contact.url    https://evalhub.com/support
// @contact.email  api-support@evalhub.com

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:9000
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @securityDefinitions.apikey SessionAuth
// @in cookie
// @name session_token
// @description Session-based authentication cookie

package main

import (
	"context"
	"evalhub/internal/cache"
	"evalhub/internal/config"
	"evalhub/internal/database"
	"evalhub/internal/handlers/web"
	"evalhub/internal/middleware"
	"evalhub/internal/monitoring"
	"evalhub/internal/response"
	"evalhub/internal/router"
	"evalhub/internal/services"
	"evalhub/internal/utils"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()
	logger.Info("Starting EvalHub application")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize database
	var dbManager *database.Manager
	if err := database.InitDB(cfg, logger); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("environment", cfg.Server.Environment),
		zap.String("port", cfg.Server.Port),
	)

	// Get database connection
	dbManager = database.GetDB()
	if dbManager == nil {
		logger.Fatal("Database connection is not initialized")
	}
	defer dbManager.Close()

	// // Get the underlying *sql.DB from the database manager
	// db := dbManager.DB()
	// if db == nil {
	// 	logger.Fatal("Failed to get database connection from manager")
	// }

	logger.Info("Database initialized successfully")

	// Database health check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	healthStatus := database.Health(ctx)
	if healthStatus.Status != database.StatusHealthy {
		logger.Fatal("Database is not healthy",
			zap.String("status", healthStatus.Status),
			zap.Strings("errors", healthStatus.Errors),
		)
	}
	logger.Info("Database health check passed", zap.String("status", healthStatus.Status))

	// Initialize Cloudinary
	if _, err = utils.GetCloudinaryService(); err != nil {
		logger.Warn("Cloudinary initialization failed", zap.Error(err))
	} else {
		logger.Info("Cloudinary service initialized successfully")
	}

	// Initialize templates
	if err := web.InitTemplates("."); err != nil {
		logger.Fatal("Failed to initialize templates", zap.Error(err))
	}
	logger.Info("Templates initialized successfully")

	// Set up enhanced logging config
	loggingConfig := middleware.DefaultLoggingConfig()
	loggingConfig.LogRequestBody = false
	loggingConfig.LogResponseBody = false
	loggingConfig.SlowRequestThreshold = 2 * time.Second
	loggingConfig.SampleRate = 1.0

	// Create cache
	cacheConfig := cache.DefaultConfig()
	cacheInstance, err := cache.NewCache(cacheConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create cache", zap.Error(err))
	}

	// 🆕 Initialize Metrics Collector for Monitoring
	metricsConfig := middleware.DefaultMetricsConfig()

	// Adjust metrics config based on environment
	switch cfg.Server.Environment {
	case "production":
		metricsConfig.SampleRate = 0.1 // 10% sampling in production
		metricsConfig.EnableDetailedMetrics = false
		metricsConfig.MaxEndpointsTracked = 50
	case "development":
		metricsConfig.SampleRate = 1.0 // 100% sampling in development
		metricsConfig.EnableDetailedMetrics = true
		metricsConfig.MaxEndpointsTracked = 200
	}

	metricsCollector := middleware.NewMetricsCollector(metricsConfig, logger)
	logger.Info("Metrics collector initialized",
		zap.Float64("sample_rate", metricsConfig.SampleRate),
		zap.Bool("detailed_metrics", metricsConfig.EnableDetailedMetrics),
		zap.Int("max_endpoints", metricsConfig.MaxEndpointsTracked),
	)

	// Rate limiter
	rateLimitConfig := middleware.DefaultRateLimiterConfig()
	rateLimitConfig.DefaultIPLimit = 2000
	rateLimitConfig.DefaultUserLimit = 10000
	rateLimiter := middleware.NewRateLimiter(cacheInstance, rateLimitConfig, logger)

	// Initialize services
	serviceCollection, err := services.NewServiceCollection(dbManager, cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize services", zap.Error(err))
	}

	// ✅ Initialize web handlers with service collection
	web.InitWebHandler(serviceCollection, logger)
	logger.Info("Web handlers initialized with service collection")

	// Auth middleware
	authConfig := middleware.DefaultAuthConfig()
	authConfig.JWTSecret = cfg.Auth.JWTSecret
	authConfig.CookieSecure = cfg.Server.Environment == "production"

	// Get required repositories and services
	sessionRepo := serviceCollection.Repositories.Session
	userRepo := serviceCollection.Repositories.User
	authService := serviceCollection.GetAuthService()

	authMiddleware, err := middleware.NewAuthMiddleware(
		authConfig,
		cacheInstance,
		sessionRepo,
		userRepo,
		authService,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to create auth middleware", zap.Error(err))
	}

	// ✅ Validation middleware with caching
	validationConfig := middleware.DefaultValidationConfig()
	requestValidator := middleware.NewRequestValidator(validationConfig, logger)
	validationCache := middleware.NewValidationCache(validationConfig.CacheTTL)

	// ✅ Response middleware configuration
	responseConfig := response.DefaultConfig()
	responseConfig.APIVersion = "v1"
	responseConfig.IncludeErrorStack = cfg.Server.Environment != "production"
	responseConfig.MaskInternalErrors = cfg.Server.Environment == "production"
	responseMiddleware := response.CreateResponseMiddlewareStack(responseConfig, logger)

	// 🆕 Create Response Builder for API controllers
	responseBuilder := response.NewBuilder(responseConfig, logger)
	logger.Info("Response builder initialized",
		zap.String("api_version", responseConfig.APIVersion),
		zap.Bool("include_error_stack", responseConfig.IncludeErrorStack),
		zap.Bool("mask_internal_errors", responseConfig.MaskInternalErrors),
	)

	// 🆕 ENHANCED ERROR HANDLING & RECOVERY CONFIGURATION
	errorConfig, recoveryConfig := configureErrorHandling(cfg.Server.Environment, logger)

	// Create enhanced error handling and recovery stacks
	errorHandlingStack := middleware.CreateErrorHandlingStack(errorConfig, logger)
	recoveryStack := middleware.CreateEnhancedRecoveryStack(recoveryConfig, logger)

	// 🆕 ENHANCED SECURITY CONFIGURATION
	securityStack := configureSecurityMiddleware(cfg, logger)

	// 🆕 Initialize Error Tracker for Monitoring
	errorTracker := middleware.NewErrorTracker(errorConfig, logger)

	// 🆕 Initialize Monitoring Dashboard
	dashboard := monitoring.NewDashboard(
		metricsCollector,
		logger,
		getApplicationVersion(),
		cfg.Server.Environment,
	)

	// Setup base router with required dependencies
	baseRouter := router.SetupRouter(serviceCollection, authMiddleware, responseBuilder, logger)

	// Convert to ServeMux for monitoring setup
	mux, ok := baseRouter.(*http.ServeMux)
	if !ok {
		logger.Fatal("Router is not a *http.ServeMux")
	}

	// 🆕 API v1 routes integration
	// router.AddAPIv1Routes(mux, serviceCollection, authMiddleware, responseBuilder, logger)

	// 🆕 Setup comprehensive monitoring routes
	router.SetupMonitoringRoutes(mux, dashboard, logger)

	// 🆕 Setup error monitoring
	router.SetupErrorMonitoring(mux, errorTracker)

	// Setup enhanced middleware chain
	handler := setupMiddlewareChain(
		mux,
		logger,
		loggingConfig,
		rateLimiter,
		requestValidator,
		validationCache,
		responseMiddleware,
		authMiddleware,
		errorHandlingStack,
		recoveryStack,
		securityStack,
		metricsCollector,
	)

	// HTTP server
	server := &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:        handler,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: 1 << 20,
		// 🆕 Enhanced server security settings
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 🆕 Security headers for the server itself
	if cfg.Server.Environment == "production" {
		// Additional server-level security configurations
		logger.Info("Applying production security configurations")
	}

	// Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Starting HTTP server",
			zap.String("address", server.Addr),
			zap.String("environment", cfg.Server.Environment),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// 🆕 Start background monitoring tasks
	startBackgroundMonitoring(dashboard, logger)

	// Log initial DB metrics
	go func() {
		time.Sleep(5 * time.Second)
		metrics := database.GetMetrics()
		logger.Info("Initial database metrics",
			zap.Int64("query_count", metrics.QueryCount),
			zap.Duration("avg_query_duration", metrics.AvgQueryDuration),
			zap.Int("open_connections", metrics.DBStats.OpenConnections),
		)
	}()

	// 🆕 Enhanced startup logging
	logger.Info("Application started successfully with comprehensive monitoring",
		zap.String("url", fmt.Sprintf("http://localhost:%s", cfg.Server.Port)),
		zap.String("security_level", cfg.Server.Environment),
		zap.Bool("enhanced_error_handling", true),
		zap.Bool("enhanced_security", true),
		zap.Bool("comprehensive_monitoring", true),
		zap.String("monitoring_endpoints", "/internal/dashboard"),
		zap.String("health_check", "/health"),
		zap.String("metrics", "/internal/metrics"),
	)

	<-quit
	logger.Info("Shutting down application...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	} else {
		logger.Info("Server shutdown completed")
	}

	// 🆕 Log final comprehensive metrics
	finalMetrics := database.GetMetrics()
	finalAPIMetrics := metricsCollector.GetAPIMetrics()

	logger.Info("Final application metrics",
		zap.Int64("total_queries", finalMetrics.QueryCount),
		zap.Int64("total_errors", finalMetrics.ErrorCount),
		zap.Int64("slow_queries", finalMetrics.SlowQueryCount),
		zap.Duration("avg_query_duration", finalMetrics.AvgQueryDuration),
		zap.Int64("total_requests", finalAPIMetrics.TotalRequests),
		zap.Int64("success_requests", finalAPIMetrics.SuccessRequests),
		zap.Int64("error_requests", finalAPIMetrics.ErrorRequests),
	)

	if err := database.Close(); err != nil {
		logger.Error("Failed to close database connections", zap.Error(err))
	} else {
		logger.Info("Database connections closed successfully")
	}

	// 🆕 Stop metrics collector
	metricsCollector.Stop()
	logger.Info("Metrics collector stopped")

	logger.Info("Application shutdown completed")
}

// 🆕 ENHANCED ERROR HANDLING CONFIGURATION FUNCTION
func configureErrorHandling(environment string, logger *zap.Logger) (*middleware.ErrorHandlerConfig, *middleware.RecoveryConfig) {
	var errorConfig *middleware.ErrorHandlerConfig
	var recoveryConfig *middleware.RecoveryConfig

	switch environment {
	case "production":
		logger.Info("Configuring error handling for production environment")
		errorConfig = &middleware.ErrorHandlerConfig{
			EnableErrorClassification: true,
			EnableErrorMetrics:        true,
			IncludeErrorDetails:       false, // Hide details in production
			SanitizeSensitiveData:     true,
			EnableCircuitBreaker:      true,
			UseCustomErrorPages:       true,
		}
		recoveryConfig = &middleware.RecoveryConfig{
			EnableStackTrace:          true,
			StackTraceInResponse:      false, // Don't expose stack traces
			MaskInternalErrors:        true,
			EnablePanicAlerts:         true,
			EnableGracefulDegradation: true,
		}

	case "staging":
		logger.Info("Configuring error handling for staging environment")
		errorConfig = &middleware.ErrorHandlerConfig{
			EnableErrorClassification: true,
			EnableErrorMetrics:        true,
			IncludeErrorDetails:       true, // Show details in staging
			SanitizeSensitiveData:     true,
			EnableCircuitBreaker:      true,
			UseCustomErrorPages:       false, // Use default error pages
		}
		recoveryConfig = &middleware.RecoveryConfig{
			EnableStackTrace:          true,
			StackTraceInResponse:      true, // Show stack traces in staging
			MaskInternalErrors:        false,
			EnablePanicAlerts:         true,
			EnableGracefulDegradation: true,
		}

	case "development":
		logger.Info("Configuring error handling for development environment")
		errorConfig = middleware.DefaultErrorHandlerConfig()
		errorConfig.IncludeErrorDetails = true
		errorConfig.SanitizeSensitiveData = false
		errorConfig.EnableErrorMetrics = true

		recoveryConfig = middleware.DefaultRecoveryConfig()
		recoveryConfig.StackTraceInResponse = true
		recoveryConfig.MaskInternalErrors = false

	default:
		logger.Info("Configuring error handling for default environment")
		errorConfig = middleware.DefaultErrorHandlerConfig()
		recoveryConfig = middleware.DefaultRecoveryConfig()
	}

	logger.Info("Error handling configuration completed",
		zap.String("environment", environment),
		zap.Bool("include_error_details", errorConfig.IncludeErrorDetails),
		zap.Bool("stack_trace_in_response", recoveryConfig.StackTraceInResponse),
	)

	return errorConfig, recoveryConfig
}

// 🆕 ENHANCED SECURITY CONFIGURATION FUNCTION
func configureSecurityMiddleware(cfg *config.Config, logger *zap.Logger) func(http.Handler) http.Handler {
	environment := cfg.Server.Environment

	switch environment {
	case "production":
		logger.Info("Configuring enhanced security for production environment")

		// Create custom security config for production
		securityConfig := middleware.DefaultSecurityConfig()
		corsConfig := middleware.DefaultCORSConfig()

		// 🔒 Production CORS Configuration
		// TODO: Replace with your actual production domains
		corsConfig.AllowedOrigins = []string{
			"https://yourdomain.com",
			"https://www.yourdomain.com",
			"https://api.yourdomain.com",
			// Add all your legitimate origins here
		}
		corsConfig.AllowCredentials = true
		corsConfig.MaxAge = 86400 // 24 hours

		// 🔒 Strengthen CSP for production
		securityConfig.CSPScriptSrc = []string{"'self'"} // Remove 'unsafe-inline'
		securityConfig.CSPStyleSrc = []string{"'self'", "https://fonts.googleapis.com"}
		securityConfig.CSPImgSrc = []string{"'self'", "data:", "https:"}
		securityConfig.CSPConnectSrc = []string{"'self'"}

		// 🔒 Enable strict HSTS
		securityConfig.HSTSMaxAge = 365 * 24 * time.Hour // 1 year
		securityConfig.HSTSPreload = true
		securityConfig.HSTSIncludeSubdomains = true

		// 🔒 Additional production security headers
		securityConfig.EnableXSSProtection = true
		securityConfig.EnableContentTypeNosniff = true
		securityConfig.FrameOptions = "DENY"

		// 🔒 Referrer Policy
		securityConfig.ReferrerPolicy = "strict-origin-when-cross-origin"

		// Create custom security stack for production
		return middleware.CreateSecurityMiddlewareStack(securityConfig, corsConfig, logger)

	case "staging":
		logger.Info("Configuring enhanced security for staging environment")

		securityConfig := middleware.DefaultSecurityConfig()
		corsConfig := middleware.DefaultCORSConfig()

		// 🔒 Staging CORS Configuration
		corsConfig.AllowedOrigins = []string{
			"https://staging.yourdomain.com",
			"https://dev.yourdomain.com",
			"http://localhost:3000", // For development testing
			"http://localhost:9000",
		}

		// 🔒 Moderate CSP for staging (allow some debugging)
		securityConfig.CSPScriptSrc = []string{"'self'", "'unsafe-eval'"} // Allow eval for debugging
		securityConfig.CSPStyleSrc = []string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"}

		// 🔒 Shorter HSTS for staging
		securityConfig.HSTSMaxAge = 30 * 24 * time.Hour // 30 days

		return middleware.CreateSecurityMiddlewareStack(securityConfig, corsConfig, logger)

	case "development":
		logger.Info("Configuring enhanced security for development environment")

		// 🔒 Development-friendly security (less restrictive)
		securityConfig := middleware.DefaultSecurityConfig()
		corsConfig := middleware.DefaultCORSConfig()

		// 🔒 Development CORS (allow localhost)
		corsConfig.AllowedOrigins = []string{"*"} // Allow all origins in development
		corsConfig.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"}
		corsConfig.AllowedHeaders = []string{"*"}

		// 🔒 Relaxed CSP for development
		securityConfig.CSPScriptSrc = []string{"'self'", "'unsafe-inline'", "'unsafe-eval'"}
		securityConfig.CSPStyleSrc = []string{"'self'", "'unsafe-inline'"}

		// 🔒 Disable HSTS in development
		securityConfig.HSTSMaxAge = 0

		return middleware.CreateSecurityMiddlewareStack(securityConfig, corsConfig, logger)

	default:
		logger.Info("Using default enhanced security configuration")
		return middleware.ReplaceBasicSecurity(environment, logger)
	}
}

// 🆕 ENHANCED MIDDLEWARE CHAIN SETUP FUNCTION
func setupMiddlewareChain(
	baseHandler http.Handler,
	logger *zap.Logger,
	loggingConfig *middleware.LoggingConfig,
	rateLimiter *middleware.RateLimiter,
	requestValidator *middleware.RequestValidator,
	validationCache *middleware.ValidationCache,
	responseMiddleware func(http.Handler) http.Handler,
	authMiddleware *middleware.AuthMiddleware,
	errorHandlingStack func(http.Handler) http.Handler,
	recoveryStack func(http.Handler) http.Handler,
	securityStack func(http.Handler) http.Handler,
	metricsCollector *middleware.MetricsCollector,
) http.Handler {

	handler := baseHandler

	// 📋 COMPLETE ENHANCED MIDDLEWARE CHAIN (ORDER MATTERS!)
	// 1. Request ID (first for tracing)
	handler = middleware.RequestID(logger)(handler)

	// 2. 🆕 Metrics collection (early for accurate measurements)
	handler = middleware.APIMetricsMiddleware(metricsCollector)(handler)

	// 3. Enhanced logging with request correlation
	handler = middleware.CreateEnhancedLoggingStack(logger, loggingConfig)(handler)

	// 4. Rate limiting (early protection)
	handler = middleware.RateLimit(rateLimiter)(handler)

	// 5. Request validation with caching
	handler = middleware.ValidateRequestWithCache(requestValidator, validationCache)(handler)

	// 6. Response formatting
	handler = responseMiddleware(handler)

	// 7. Authentication (optional)
	handler = authMiddleware.OptionalAuth()(handler)

	// 8. 🆕 Enhanced error handling (before recovery)
	handler = errorHandlingStack(handler)

	// 9. 🆕 Enhanced panic recovery (before security)
	handler = recoveryStack(handler)

	// 10. 🆕 Enhanced Security + CORS (replaces basic security)
	handler = securityStack(handler)

	logger.Info("Complete middleware chain setup completed",
		zap.Bool("enhanced_error_handling", true),
		zap.Bool("enhanced_recovery", true),
		zap.Bool("enhanced_security", true),
		zap.Bool("metrics_collection", true),
	)

	return handler
}

// 🆕 BACKGROUND MONITORING TASKS
func startBackgroundMonitoring(dashboard *monitoring.Dashboard, logger *zap.Logger) {
	// Start periodic health checks
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				health := dashboard.GetSystemHealth(ctx)
				cancel()

				if health.Status != "healthy" {
					logger.Warn("System health check detected issues",
						zap.String("status", health.Status),
						zap.Int("alerts", len(health.Alerts)),
						zap.Int("critical_issues", health.Summary.CriticalIssues),
					)
				}
			}
		}
	}()

	// Start periodic metrics logging
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				metrics := dashboard.GetComprehensiveMetrics()
				if apiMetrics, ok := metrics["api"]; ok {
					logger.Info("Periodic metrics report", zap.Any("api_metrics", apiMetrics))
				}
			}
		}
	}()

	logger.Info("Background monitoring tasks started")
}

// 🆕 UTILITY FUNCTIONS
func getApplicationVersion() string {
	// You can set this via build flags: -ldflags "-X main.version=1.2.3"
	if version := os.Getenv("APP_VERSION"); version != "" {
		return version
	}
	return "1.0.0"
}

// initLogger initializes the structured logger based on environment
func initLogger() (*zap.Logger, error) {
	env := os.Getenv("GO_ENV")
	var config zap.Config

	switch env {
	case "production":
		config = zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "staging":
		config = zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	default:
		config = zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	return logger, nil
}
