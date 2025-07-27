package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Auth       AuthConfig
	Cloudinary CloudinaryConfig
	Logging    LoggingConfig
	
	// 🚀 PRODUCTION ENHANCEMENTS
	Security   SecurityConfig   `json:"security"`
	Monitoring MonitoringConfig `json:"monitoring"`
	Features   FeatureConfig    `json:"features"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         string
	Environment  string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	Host         string
	TLSEnabled   bool
	
	// 🚀 PRODUCTION ENHANCEMENTS
	GracefulTimeout  time.Duration `json:"graceful_timeout"`
	MaxHeaderBytes   int           `json:"max_header_bytes"`
	KeepAlive        bool          `json:"keep_alive"`
	ServerName       string        `json:"server_name"`
	TrustedProxies   []string      `json:"trusted_proxies"`
}

// 🏭 ENHANCED DATABASE CONFIGURATION FOR PRODUCTION
type DatabaseConfig struct {
	// ✅ EXISTING FIELDS (BACKWARD COMPATIBLE)
	URL                 string
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	ConnMaxIdleTime     time.Duration
	SlowQueryThreshold  time.Duration
	EnableQueryLogging  bool
	EnableMetrics       bool
	HealthCheckInterval time.Duration
	MigrationsPath      string
	BackupRetentionDays int
	AutoVacuum          bool
	
	// 🚀 PRODUCTION ENHANCEMENTS
	// Connection Management
	ConnectTimeout      time.Duration `json:"connect_timeout"`
	ReadTimeout         time.Duration `json:"read_timeout"`
	WriteTimeout        time.Duration `json:"write_timeout"`
	
	// Security & SSL
	SSLMode             string        `json:"ssl_mode"`             // disable, require, verify-ca, verify-full
	SSLCert             string        `json:"ssl_cert"`
	SSLKey              string        `json:"ssl_key"`
	SSLRootCert         string        `json:"ssl_root_cert"`
	
	// High Availability
	ReadReplicas        []string      `json:"read_replicas"`
	EnableReadSplitting bool          `json:"enable_read_splitting"`
	LoadBalancer        string        `json:"load_balancer"`        // round_robin, least_conn
	
	// Performance & Monitoring
	StatementTimeout    time.Duration `json:"statement_timeout"`
	LockTimeout         time.Duration `json:"lock_timeout"`
	IdleInTxTimeout     time.Duration `json:"idle_in_tx_timeout"`
	
	// Retry & Circuit Breaker
	EnableRetries       bool          `json:"enable_retries"`
	MaxRetryAttempts    int           `json:"max_retry_attempts"`
	RetryBackoff        time.Duration `json:"retry_backoff"`
	CircuitBreakerThreshold int       `json:"circuit_breaker_threshold"`
	
	// Advanced Monitoring
	SlowQueryLog        bool          `json:"slow_query_log"`
	QueryStatsInterval  time.Duration `json:"query_stats_interval"`
	EnableTracing       bool          `json:"enable_tracing"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	SessionSecret string
	SessionName   string
	SessionExpiry int
	BCryptCost    int
	JWTSecret     string
	JWTExpiry     time.Duration
	
	// 🚀 PRODUCTION ENHANCEMENTS
	// Session Security
	SessionSecure       bool          `json:"session_secure"`
	SessionHttpOnly     bool          `json:"session_http_only"`
	SessionSameSite     string        `json:"session_same_site"`     // strict, lax, none
	SessionDomain       string        `json:"session_domain"`
	
	// Password Security
	MinPasswordLength   int           `json:"min_password_length"`
	RequireSpecialChars bool          `json:"require_special_chars"`
	MaxLoginAttempts    int           `json:"max_login_attempts"`
	LockoutDuration     time.Duration `json:"lockout_duration"`
	
	// OAuth Configuration
	GoogleClientID      string        `json:"google_client_id"`
	GoogleClientSecret  string        `json:"google_client_secret"`
	GoogleRedirectURL   string        `json:"google_redirect_url"`
	
	// Security Features
	Enable2FA           bool          `json:"enable_2fa"`
	RequireEmailVerification bool     `json:"require_email_verification"`
	TokenRefreshInterval time.Duration `json:"token_refresh_interval"`
}

// CloudinaryConfig holds Cloudinary configuration
type CloudinaryConfig struct {
	CloudName    string
	APIKey       string
	APISecret    string
	UploadPreset string
	MaxFileSize  int64
	
	// 🚀 PRODUCTION ENHANCEMENTS
	EnableTransformation bool     `json:"enable_transformation"`
	Quality             string   `json:"quality"`              // auto, best, good, eco
	Format              string   `json:"format"`               // auto, webp, jpg, png
	AllowedFormats      []string `json:"allowed_formats"`
	MaxImageDimensions  int      `json:"max_image_dimensions"` // pixels
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string
	Format     string
	Output     string
	EnableFile bool
	FilePath   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	
	// 🚀 PRODUCTION ENHANCEMENTS
	SentryDSN          string        `json:"sentry_dsn"`
	EnableStructured   bool          `json:"enable_structured"`
	EnableSampling     bool          `json:"enable_sampling"`
	SampleRate         float64       `json:"sample_rate"`
	EnableMetrics      bool          `json:"enable_metrics"`
	MetricsInterval    time.Duration `json:"metrics_interval"`
}

// 🔒 SECURITY CONFIGURATION
type SecurityConfig struct {
	// HTTPS & TLS
	ForceHTTPS          bool          `json:"force_https"`
	HSTSMaxAge          time.Duration `json:"hsts_max_age"`
	HSTSIncludeSubdomains bool        `json:"hsts_include_subdomains"`
	
	// CORS
	CORSAllowedOrigins  []string      `json:"cors_allowed_origins"`
	CORSAllowedMethods  []string      `json:"cors_allowed_methods"`
	CORSAllowedHeaders  []string      `json:"cors_allowed_headers"`
	CORSMaxAge          time.Duration `json:"cors_max_age"`
	CORSAllowCredentials bool         `json:"cors_allow_credentials"`
	
	// Content Security Policy
	CSPDefaultSrc       []string      `json:"csp_default_src"`
	CSPScriptSrc        []string      `json:"csp_script_src"`
	CSPStyleSrc         []string      `json:"csp_style_src"`
	CSPImgSrc           []string      `json:"csp_img_src"`
	
	// Rate Limiting
	RateLimitRequests   int           `json:"rate_limit_requests"`
	RateLimitWindow     time.Duration `json:"rate_limit_window"`
	RateLimitBurst      int           `json:"rate_limit_burst"`
	
	// Security Headers
	EnableSecurityHeaders bool        `json:"enable_security_headers"`
	FrameOptions         string       `json:"frame_options"`        // DENY, SAMEORIGIN
	ContentTypeNosniff   bool         `json:"content_type_nosniff"`
	XSSProtection        bool         `json:"xss_protection"`
}

// 📊 MONITORING CONFIGURATION
type MonitoringConfig struct {
	EnableMetrics       bool          `json:"enable_metrics"`
	EnableTracing       bool          `json:"enable_tracing"`
	EnableProfiling     bool          `json:"enable_profiling"`
	
	// Health Checks
	HealthCheckPath     string        `json:"health_check_path"`
	ReadinessPath       string        `json:"readiness_path"`
	LivenessPath        string        `json:"liveness_path"`
	
	// Metrics Collection
	MetricsPort         int           `json:"metrics_port"`
	MetricsPath         string        `json:"metrics_path"`
	CollectionInterval  time.Duration `json:"collection_interval"`
	
	// Alerting
	AlertingEnabled     bool          `json:"alerting_enabled"`
	SlackWebhookURL     string        `json:"slack_webhook_url"`
	AlertThresholds     AlertThresholds `json:"alert_thresholds"`
}

// 🚨 ALERT THRESHOLDS
type AlertThresholds struct {
	ErrorRate           float64       `json:"error_rate"`            // percentage
	ResponseTime        time.Duration `json:"response_time"`         // 95th percentile
	DatabaseConnections int           `json:"database_connections"`  // max connections
	MemoryUsage         float64       `json:"memory_usage"`          // percentage
	CPUUsage           float64       `json:"cpu_usage"`             // percentage
}

// 🚀 FEATURE FLAGS
type FeatureConfig struct {
	EnableRegistration  bool `json:"enable_registration"`
	EnableGoogleAuth    bool `json:"enable_google_auth"`
	EnableFileUploads   bool `json:"enable_file_uploads"`
	EnableComments      bool `json:"enable_comments"`
	EnableNotifications bool `json:"enable_notifications"`
	EnableAnalytics     bool `json:"enable_analytics"`
	MaintenanceMode     bool `json:"maintenance_mode"`
}

// 🚀 PRODUCTION-READY CONFIGURATION LOADER
func Load() (*Config, error) {
	// Load environment file based on GO_ENV
	env := getEnv("GO_ENV", "development")
	if env != "production" {
		envFile := fmt.Sprintf(".env.%s", env)
		if _, err := os.Stat(envFile); err == nil {
			_ = godotenv.Load(envFile)
		} else {
			_ = godotenv.Load() // fallback to .env
		}
	}

	config := &Config{
		Server:     loadServerConfig(env),
		Database:   loadEnhancedDatabaseConfig(env),
		Auth:       loadEnhancedAuthConfig(env),
		Cloudinary: loadEnhancedCloudinaryConfig(),
		Logging:    loadEnhancedLoggingConfig(env),
		Security:   loadSecurityConfig(env),
		Monitoring: loadMonitoringConfig(env),
		Features:   loadFeatureConfig(env),
	}

	// 🔍 Enhanced validation
	if err := config.ValidateAll(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// 🚀 Apply environment-specific optimizations
	config.optimizeForEnvironment(env)

	return config, nil
}

// 🖥️ ENHANCED SERVER CONFIGURATION
func loadServerConfig(env string) ServerConfig {
	config := ServerConfig{
		Port:         getEnv("PORT", "9000"),
		Environment:  env,
		Host:         getEnv("SERVER_HOST", "0.0.0.0"),
		ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 10*time.Second),
		WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 120*time.Second),
		TLSEnabled:   getBoolEnv("TLS_ENABLED", env == "production"),
		
		// 🚀 Production enhancements
		GracefulTimeout: getDurationEnv("GRACEFUL_TIMEOUT", 30*time.Second),
		MaxHeaderBytes:  getIntEnv("MAX_HEADER_BYTES", 1<<20), // 1MB
		KeepAlive:      getBoolEnv("KEEP_ALIVE", true),
		ServerName:     getEnv("SERVER_NAME", "EvalHub"),
	}
	// Original load functions remain unchanged for backward compatibility
// func loadServerConfig() ServerConfig {
// 	return ServerConfig{
// 		Port:         getEnv("PORT", "9000"),
// 		Environment:  getEnv("GO_ENV", "development"),
// 		Host:         getEnv("SERVER_HOST", "0.0.0.0"),
// 		ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 10*time.Second),
// 		WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second),
// 		IdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 120*time.Second),
// 		TLSEnabled:   getBoolEnv("TLS_ENABLED", false),
// 	}
// }
	
	// Environment-specific optimizations
	switch env {
	case "production":
		// Heroku-specific optimizations
		if isHeroku() {
			config.TrustedProxies = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
			config.GracefulTimeout = 25 * time.Second // Heroku dyno shutdown time
		}
		config.TLSEnabled = true
		
	case "staging":
		config.GracefulTimeout = 20 * time.Second
		
	default: // development
		config.TLSEnabled = false
		config.GracefulTimeout = 10 * time.Second
	}
	
	return config
}

// 🗄️ ENHANCED DATABASE CONFIGURATION
func loadEnhancedDatabaseConfig(env string) DatabaseConfig {
	// Start with existing configuration
	config := loadDatabaseConfig() // Call original function
	
	// Add production enhancements
	config.ConnectTimeout = getDurationEnv("DB_CONNECT_TIMEOUT", 10*time.Second)
	config.ReadTimeout = getDurationEnv("DB_READ_TIMEOUT", 30*time.Second)
	config.WriteTimeout = getDurationEnv("DB_WRITE_TIMEOUT", 30*time.Second)
	
	// SSL Configuration
	config.SSLMode = getEnv("DB_SSL_MODE", getDefaultSSLMode(env))
	config.SSLCert = getEnv("DB_SSL_CERT", "")
	config.SSLKey = getEnv("DB_SSL_KEY", "")
	config.SSLRootCert = getEnv("DB_SSL_ROOT_CERT", "")
	
	// High Availability
	if replicas := getEnv("DB_READ_REPLICAS", ""); replicas != "" {
		config.ReadReplicas = strings.Split(replicas, ",")
		config.EnableReadSplitting = getBoolEnv("DB_ENABLE_READ_SPLITTING", len(config.ReadReplicas) > 0)
	}
	config.LoadBalancer = getEnv("DB_LOAD_BALANCER", "round_robin")
	
	// Timeouts
	config.StatementTimeout = getDurationEnv("DB_STATEMENT_TIMEOUT", 30*time.Second)
	config.LockTimeout = getDurationEnv("DB_LOCK_TIMEOUT", 10*time.Second)
	config.IdleInTxTimeout = getDurationEnv("DB_IDLE_IN_TX_TIMEOUT", 60*time.Second)
	
	// Retry Logic
	config.EnableRetries = getBoolEnv("DB_ENABLE_RETRIES", env == "production")
	config.MaxRetryAttempts = getIntEnv("DB_MAX_RETRY_ATTEMPTS", 3)
	config.RetryBackoff = getDurationEnv("DB_RETRY_BACKOFF", 1*time.Second)
	config.CircuitBreakerThreshold = getIntEnv("DB_CIRCUIT_BREAKER_THRESHOLD", 5)
	
	// Enhanced Monitoring
	config.SlowQueryLog = getBoolEnv("DB_SLOW_QUERY_LOG", env != "production")
	config.QueryStatsInterval = getDurationEnv("DB_QUERY_STATS_INTERVAL", 5*time.Minute)
	config.EnableTracing = getBoolEnv("DB_ENABLE_TRACING", env == "development")
	
	// Environment-specific database optimizations
	optimizeDatabaseForEnvironment(&config, env)
	
	return config
}

// 🔐 ENHANCED AUTH CONFIGURATION
func loadEnhancedAuthConfig(env string) AuthConfig {
	// Start with existing configuration
	config := loadAuthConfig() // Call original function
	
	// Add OAuth configuration
	config.GoogleClientID = getEnv("GOOGLE_CLIENT_ID", "")
	config.GoogleClientSecret = getEnv("GOOGLE_CLIENT_SECRET", "")
	config.GoogleRedirectURL = getEnv("GOOGLE_REDIRECT_URL", "")
	
	// Session Security
	config.SessionSecure = getBoolEnv("SESSION_SECURE", env == "production")
	config.SessionHttpOnly = getBoolEnv("SESSION_HTTP_ONLY", true)
	config.SessionSameSite = getEnv("SESSION_SAME_SITE", "lax")
	config.SessionDomain = getEnv("SESSION_DOMAIN", "")
	
	// Password Security
	config.MinPasswordLength = getIntEnv("MIN_PASSWORD_LENGTH", 8)
	config.RequireSpecialChars = getBoolEnv("REQUIRE_SPECIAL_CHARS", env == "production")
	config.MaxLoginAttempts = getIntEnv("MAX_LOGIN_ATTEMPTS", 5)
	config.LockoutDuration = getDurationEnv("LOCKOUT_DURATION", 15*time.Minute)
	
	// Security Features
	config.Enable2FA = getBoolEnv("ENABLE_2FA", false)
	config.RequireEmailVerification = getBoolEnv("REQUIRE_EMAIL_VERIFICATION", env == "production")
	config.TokenRefreshInterval = getDurationEnv("TOKEN_REFRESH_INTERVAL", 15*time.Minute)
	
	return config
}

// ☁️ ENHANCED CLOUDINARY CONFIGURATION
func loadEnhancedCloudinaryConfig() CloudinaryConfig {
	// Start with existing configuration
	config := loadCloudinaryConfig() // Call original function
	
	// Add enhancements
	config.EnableTransformation = getBoolEnv("CLOUDINARY_ENABLE_TRANSFORMATION", true)
	config.Quality = getEnv("CLOUDINARY_QUALITY", "auto")
	config.Format = getEnv("CLOUDINARY_FORMAT", "auto")
	
	// Parse allowed formats
	if formats := getEnv("CLOUDINARY_ALLOWED_FORMATS", "jpg,jpeg,png,webp,gif"); formats != "" {
		config.AllowedFormats = strings.Split(formats, ",")
	}
	
	config.MaxImageDimensions = getIntEnv("CLOUDINARY_MAX_DIMENSIONS", 2048)
	
	return config
}

// 📝 ENHANCED LOGGING CONFIGURATION
func loadEnhancedLoggingConfig(env string) LoggingConfig {
	// Start with existing configuration
	config := loadLoggingConfig() // Call original function
	
	// Add enhancements
	config.SentryDSN = getEnv("SENTRY_DSN", "")
	config.EnableStructured = getBoolEnv("LOG_ENABLE_STRUCTURED", env == "production")
	config.EnableSampling = getBoolEnv("LOG_ENABLE_SAMPLING", env == "production")
	config.SampleRate = getFloat64Env("LOG_SAMPLE_RATE", getSampleRateForEnv(env))
	config.EnableMetrics = getBoolEnv("LOG_ENABLE_METRICS", true)
	config.MetricsInterval = getDurationEnv("LOG_METRICS_INTERVAL", 1*time.Minute)
	
	return config
}

// 🔒 SECURITY CONFIGURATION
func loadSecurityConfig(env string) SecurityConfig {
	config := SecurityConfig{
		// HTTPS & TLS
		ForceHTTPS:              getBoolEnv("FORCE_HTTPS", env == "production"),
		HSTSMaxAge:             getDurationEnv("HSTS_MAX_AGE", 365*24*time.Hour),
		HSTSIncludeSubdomains:  getBoolEnv("HSTS_INCLUDE_SUBDOMAINS", env == "production"),
		
		// CORS - Environment specific
		CORSMaxAge:             getDurationEnv("CORS_MAX_AGE", 24*time.Hour),
		CORSAllowCredentials:   getBoolEnv("CORS_ALLOW_CREDENTIALS", true),
		
		// CSP - Environment specific defaults
		CSPDefaultSrc:          []string{"'self'"},
		CSPScriptSrc:           getCSPScriptSrc(env),
		CSPStyleSrc:            []string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"},
		CSPImgSrc:             []string{"'self'", "data:", "https:", "*.cloudinary.com"},
		
		// Rate Limiting
		RateLimitRequests:      getIntEnv("RATE_LIMIT_REQUESTS", getRateLimitForEnv(env)),
		RateLimitWindow:        getDurationEnv("RATE_LIMIT_WINDOW", 1*time.Minute),
		RateLimitBurst:         getIntEnv("RATE_LIMIT_BURST", 50),
		
		// Security Headers
		EnableSecurityHeaders:  true,
		FrameOptions:          getEnv("FRAME_OPTIONS", "SAMEORIGIN"),
		ContentTypeNosniff:    true,
		XSSProtection:         true,
	}
	
	// Environment-specific CORS settings
	switch env {
	case "production":
		// Production CORS - Restrict to your domains
		config.CORSAllowedOrigins = getCORSOriginsFromEnv("https://evalhub-app-5c7202605196.herokuapp.com")
		config.CORSAllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
		config.CORSAllowedHeaders = []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"}
		
	case "staging":
		config.CORSAllowedOrigins = getCORSOriginsFromEnv("https://staging.yourdomain.com,http://localhost:3000")
		config.CORSAllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
		config.CORSAllowedHeaders = []string{"*"}
		
	default: // development
		config.CORSAllowedOrigins = []string{"*"}
		config.CORSAllowedMethods = []string{"*"}
		config.CORSAllowedHeaders = []string{"*"}
		config.ForceHTTPS = false
	}
	
	return config
}

// 📊 MONITORING CONFIGURATION
func loadMonitoringConfig(env string) MonitoringConfig {
	return MonitoringConfig{
		EnableMetrics:      getBoolEnv("ENABLE_METRICS", true),
		EnableTracing:      getBoolEnv("ENABLE_TRACING", env == "development"),
		EnableProfiling:    getBoolEnv("ENABLE_PROFILING", env != "production"),
		
		// Health Check Endpoints
		HealthCheckPath:    getEnv("HEALTH_CHECK_PATH", "/health"),
		ReadinessPath:     getEnv("READINESS_PATH", "/ready"),
		LivenessPath:      getEnv("LIVENESS_PATH", "/live"),
		
		// Metrics
		MetricsPort:       getIntEnv("METRICS_PORT", 9001),
		MetricsPath:       getEnv("METRICS_PATH", "/metrics"),
		CollectionInterval: getDurationEnv("COLLECTION_INTERVAL", 30*time.Second),
		
		// Alerting
		AlertingEnabled:   getBoolEnv("ALERTING_ENABLED", env == "production"),
		SlackWebhookURL:   getEnv("SLACK_WEBHOOK_URL", ""),
		AlertThresholds:   loadAlertThresholds(),
	}
}

// 🚨 ALERT THRESHOLDS
func loadAlertThresholds() AlertThresholds {
	return AlertThresholds{
		ErrorRate:           getFloat64Env("ALERT_ERROR_RATE", 5.0),        // 5%
		ResponseTime:        getDurationEnv("ALERT_RESPONSE_TIME", 2*time.Second),
		DatabaseConnections: getIntEnv("ALERT_DB_CONNECTIONS", 80),         // 80% of max
		MemoryUsage:        getFloat64Env("ALERT_MEMORY_USAGE", 80.0),     // 80%
		CPUUsage:          getFloat64Env("ALERT_CPU_USAGE", 80.0),         // 80%
	}
}

// 🚀 FEATURE CONFIGURATION
func loadFeatureConfig(env string) FeatureConfig {
	return FeatureConfig{
		EnableRegistration:  getBoolEnv("ENABLE_REGISTRATION", true),
		EnableGoogleAuth:    getBoolEnv("ENABLE_GOOGLE_AUTH", getEnv("GOOGLE_CLIENT_ID", "") != ""),
		EnableFileUploads:   getBoolEnv("ENABLE_FILE_UPLOADS", getEnv("CLOUDINARY_CLOUD_NAME", "") != ""),
		EnableComments:      getBoolEnv("ENABLE_COMMENTS", true),
		EnableNotifications: getBoolEnv("ENABLE_NOTIFICATIONS", env != "development"),
		EnableAnalytics:     getBoolEnv("ENABLE_ANALYTICS", env == "production"),
		MaintenanceMode:     getBoolEnv("MAINTENANCE_MODE", false),
	}
}

// 🔍 COMPREHENSIVE VALIDATION
func (c *Config) ValidateAll() error {
	validators := []func() error{
		c.Server.Validate,
		c.Database.Validate,
		c.Auth.ValidateEnhanced,
		c.Security.Validate,
		c.Monitoring.Validate,
	}
	
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	
	// Cross-validation
	if err := c.validateCrossConfig(); err != nil {
		return fmt.Errorf("cross-config validation failed: %w", err)
	}
	
	return nil
}

// 🔗 CROSS-CONFIGURATION VALIDATION
func (c *Config) validateCrossConfig() error {
	// OAuth validation
	if c.Features.EnableGoogleAuth {
		if c.Auth.GoogleClientID == "" || c.Auth.GoogleClientSecret == "" {
			return fmt.Errorf("google oauth is enabled but credentials are missing")
		}
	}
	
	// File upload validation
	if c.Features.EnableFileUploads {
		if c.Cloudinary.CloudName == "" || c.Cloudinary.APIKey == "" {
			return fmt.Errorf("file uploads are enabled but cloudinary configuration is missing")
		}
	}
	
	// Production security checks
	if c.Server.Environment == "production" {
		if !c.Security.ForceHTTPS {
			return fmt.Errorf("https must be enabled in production")
		}
		
		if c.Auth.SessionSecret == "default-session-secret-change-in-production" {
			return fmt.Errorf("default session secret cannot be used in production")
		}
		
		if strings.Contains(c.Database.URL, "sslmode=disable") {
			return fmt.Errorf("SSL must be enabled for database in production")
		}
	}
	
	return nil
}

// 🔐 ENHANCED AUTH VALIDATION
func (a *AuthConfig) ValidateEnhanced() error {
	// Call original validation
	if err := a.Validate(); err != nil {
		return err
	}
	
	// Enhanced validations
	if a.MinPasswordLength < 6 {
		return fmt.Errorf("minimum password length must be at least 6")
	}
	
	if a.MaxLoginAttempts < 3 || a.MaxLoginAttempts > 20 {
		return fmt.Errorf("max login attempts must be between 3 and 20")
	}
	
	if a.LockoutDuration < 1*time.Minute {
		return fmt.Errorf("lockout duration must be at least 1 minute")
	}
	
	return nil
}

// 🔒 SECURITY VALIDATION
func (s *SecurityConfig) Validate() error {
	if s.RateLimitRequests <= 0 {
		return fmt.Errorf("rate limit requests must be positive")
	}
	
	if s.RateLimitWindow <= 0 {
		return fmt.Errorf("rate limit window must be positive")
	}
	
	if s.FrameOptions != "DENY" && s.FrameOptions != "SAMEORIGIN" {
		return fmt.Errorf("frame options must be DENY or SAMEORIGIN")
	}
	
	return nil
}

// 📊 MONITORING VALIDATION
func (m *MonitoringConfig) Validate() error {
	if m.MetricsPort < 1 || m.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535")
	}
	
	if m.CollectionInterval < 1*time.Second {
		return fmt.Errorf("collection interval must be at least 1 second")
	}
	
	return nil
}

// 🚀 ENVIRONMENT OPTIMIZATION
func (c *Config) optimizeForEnvironment(env string) {
	switch env {
	case "production":
		// Production optimizations
		if isHeroku() {
			c.optimizeForHeroku()
		}
		
	case "staging":
		// Staging optimizations
		
	default: // development
		// Development optimizations
		c.Database.EnableQueryLogging = true
		c.Logging.Level = "debug"
	}
}

// 🟦 HEROKU OPTIMIZATIONS
func (c *Config) optimizeForHeroku() {
	// Heroku-specific database optimizations
	if c.Database.MaxOpenConns > 20 {
		c.Database.MaxOpenConns = 20 // Heroku Postgres connection limits
	}
	
	// Optimize for dyno lifecycle
	c.Server.GracefulTimeout = 25 * time.Second // Heroku gives 30s
	
	// Memory optimizations for hobby dynos
	if getEnv("DYNO", "") != "" {
		c.Database.MaxIdleConns = min(c.Database.MaxIdleConns, 5)
	}
}

// 🛠️ UTILITY FUNCTIONS

// Check if running on Heroku
func isHeroku() bool {
	return getEnv("DYNO", "") != ""
}

func getDefaultSSLMode(env string) string {
	switch env {
	case "production":
		return "require"
	case "staging":
		return "prefer"  
	default:
		return "disable"
	}
}

func getCSPScriptSrc(env string) []string {
	switch env {
	case "production":
		return []string{"'self'"}
	default:
		return []string{"'self'", "'unsafe-inline'", "'unsafe-eval'"}
	}
}

func getRateLimitForEnv(env string) int {
	switch env {
	case "production":
		return 100 // requests per minute
	case "staging":
		return 200
	default:
		return 1000
	}
}

func getSampleRateForEnv(env string) float64 {
	switch env {
	case "production":
		return 0.1 // 10% sampling
	case "staging":
		return 0.5 // 50% sampling  
	default:
		return 1.0 // 100% sampling
	}
}

func getCORSOriginsFromEnv(defaultValue string) []string {
	origins := getEnv("CORS_ALLOWED_ORIGINS", defaultValue)
	return strings.Split(origins, ",")
}

func optimizeDatabaseForEnvironment(config *DatabaseConfig, env string) {
	switch env {
	case "production":
		// Production database optimizations
		if config.MaxOpenConns < 20 {
			config.MaxOpenConns = 50
		}
		if config.ConnMaxLifetime < 5*time.Minute {
			config.ConnMaxLifetime = 15 * time.Minute
		}
		
	case "staging":
		// Staging optimizations
		if config.MaxOpenConns < 10 {
			config.MaxOpenConns = 25
		}
		
	default: // development
		// Development optimizations
		if config.MaxOpenConns > 10 {
			config.MaxOpenConns = 10
		}
	}
}

// 🔧 ENHANCED HELPER FUNCTIONS

func getFloat64Env(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Parse DATABASE_URL for additional connection parameters
func (d *DatabaseConfig) ParseDatabaseURL() (map[string]string, error) {
	if d.URL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}
	
	u, err := url.Parse(d.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}
	
	params := make(map[string]string)
	params["host"] = u.Hostname()
	params["port"] = u.Port()
	params["database"] = strings.TrimPrefix(u.Path, "/")
	
	if u.User != nil {
		params["user"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			params["password"] = password
		}
	}
	
	// Parse query parameters
	for key, values := range u.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	
	return params, nil
}

// 🔄 BACKWARD COMPATIBILITY - Keep all original functions



func loadDatabaseConfig() DatabaseConfig {
	env := getEnv("GO_ENV", "development")

	var defaultMaxOpen, defaultMaxIdle int
	var defaultConnLifetime time.Duration

	switch env {
	case "production":
		defaultMaxOpen = 50
		defaultMaxIdle = 20
		defaultConnLifetime = 15 * time.Minute
	case "staging":
		defaultMaxOpen = 25
		defaultMaxIdle = 10
		defaultConnLifetime = 10 * time.Minute
	default: // development
		defaultMaxOpen = 10
		defaultMaxIdle = 5
		defaultConnLifetime = 5 * time.Minute
	}

	return DatabaseConfig{
		URL:                 os.Getenv("DATABASE_URL"),
		MaxOpenConns:        getIntEnv("DB_MAX_OPEN_CONNS", defaultMaxOpen),
		MaxIdleConns:        getIntEnv("DB_MAX_IDLE_CONNS", defaultMaxIdle),
		ConnMaxLifetime:     getDurationEnv("DB_CONN_MAX_LIFETIME", defaultConnLifetime),
		ConnMaxIdleTime:     getDurationEnv("DB_CONN_MAX_IDLE_TIME", 30*time.Minute),
		SlowQueryThreshold:  getDurationEnv("DB_SLOW_QUERY_THRESHOLD", 100*time.Millisecond),
		EnableQueryLogging:  getBoolEnv("DB_ENABLE_QUERY_LOGGING", env == "development"),
		EnableMetrics:       getBoolEnv("DB_ENABLE_METRICS", true),
		HealthCheckInterval: getDurationEnv("DB_HEALTH_CHECK_INTERVAL", 30*time.Second),
		MigrationsPath:      getEnv("DB_MIGRATIONS_PATH", "./migrations"),
		BackupRetentionDays: getIntEnv("DB_BACKUP_RETENTION_DAYS", 30),
		AutoVacuum:          getBoolEnv("DB_AUTO_VACUUM", env == "production"),
	}
}

func loadAuthConfig() AuthConfig {
	return AuthConfig{
		SessionSecret: getEnv("SESSION_SECRET", "default-session-secret-change-in-production"),
		SessionName:   getEnv("SESSION_NAME", "evalhub_session"),
		SessionExpiry: getIntEnv("SESSION_EXPIRY", 24*60*60), // 24 hours
		BCryptCost:    getIntEnv("BCRYPT_COST", 12),
		JWTSecret:     getEnv("JWT_SECRET", ""),
		JWTExpiry:     getDurationEnv("JWT_EXPIRY", 24*time.Hour),
	}
}

func loadCloudinaryConfig() CloudinaryConfig {
	return CloudinaryConfig{
		CloudName:    os.Getenv("CLOUDINARY_CLOUD_NAME"),
		APIKey:       os.Getenv("CLOUDINARY_API_KEY"),
		APISecret:    os.Getenv("CLOUDINARY_API_SECRET"),
		UploadPreset: getEnv("CLOUDINARY_UPLOAD_PRESET", ""),
		MaxFileSize:  getInt64Env("CLOUDINARY_MAX_FILE_SIZE", 10*1024*1024), // 10MB
	}
}

func loadLoggingConfig() LoggingConfig {
	env := getEnv("GO_ENV", "development")

	return LoggingConfig{
		Level:      getEnv("LOG_LEVEL", getDefaultLogLevel(env)),
		Format:     getEnv("LOG_FORMAT", getDefaultLogFormat(env)),
		Output:     getEnv("LOG_OUTPUT", "stdout"),
		EnableFile: getBoolEnv("LOG_ENABLE_FILE", env == "production"),
		FilePath:   getEnv("LOG_FILE_PATH", "/var/log/evalhub/app.log"),
		MaxSize:    getIntEnv("LOG_MAX_SIZE", 100), // MB
		MaxBackups: getIntEnv("LOG_MAX_BACKUPS", 3),
		MaxAge:     getIntEnv("LOG_MAX_AGE", 28), // days
	}
}

// All original validation and helper functions remain unchanged
func (c *Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("database config: %w", err)
	}

	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}

	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	return nil
}

func (d *DatabaseConfig) Validate() error {
	if d.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if d.MaxOpenConns <= 0 {
		return fmt.Errorf("MaxOpenConns must be positive")
	}

	if d.MaxIdleConns < 0 {
		return fmt.Errorf("MaxIdleConns cannot be negative")
	}

	if d.MaxIdleConns > d.MaxOpenConns {
		return fmt.Errorf("MaxIdleConns cannot be greater than MaxOpenConns")
	}

	if d.ConnMaxLifetime <= 0 {
		return fmt.Errorf("ConnMaxLifetime must be positive")
	}

	if d.SlowQueryThreshold <= 0 {
		return fmt.Errorf("SlowQueryThreshold must be positive")
	}

	return nil
}

func (a *AuthConfig) Validate() error {
	if a.SessionSecret == "" || a.SessionSecret == "default-session-secret-change-in-production" {
		env := getEnv("GO_ENV", "development")
		if env == "production" {
			return fmt.Errorf("SESSION_SECRET must be set for production")
		}
	}

	if a.BCryptCost < 4 || a.BCryptCost > 31 {
		return fmt.Errorf("BCryptCost must be between 4 and 31")
	}

	if a.SessionExpiry <= 0 {
		return fmt.Errorf("SessionExpiry must be positive")
	}

	return nil
}

func (s *ServerConfig) Validate() error {
	if s.Port == "" {
		return fmt.Errorf("PORT is required")
	}

	if s.ReadTimeout <= 0 {
		return fmt.Errorf("ReadTimeout must be positive")
	}

	if s.WriteTimeout <= 0 {
		return fmt.Errorf("WriteTimeout must be positive")
	}

	return nil
}

func (c *Config) IsProduction() bool {
	return c.Server.Environment == "production"
}

func (c *Config) IsDevelopment() bool {
	return c.Server.Environment == "development"
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getInt64Env(key string, defaultValue int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getDefaultLogLevel(env string) string {
	switch env {
	case "production":
		return "info"
	case "staging":
		return "debug"
	default:
		return "debug"
	}
}

func getDefaultLogFormat(env string) string {
	switch env {
	case "production":
		return "json"
	default:
		return "console"
	}
}

// package config

// import (
// 	"fmt"
// 	"os"
// 	"strconv"
// 	"time"

// 	"github.com/joho/godotenv"
// )

// // Config holds all configuration for the application
// type Config struct {
// 	Server     ServerConfig
// 	Database   DatabaseConfig
// 	Auth       AuthConfig
// 	Cloudinary CloudinaryConfig
// 	Logging    LoggingConfig
// }

// // ServerConfig holds server configuration
// type ServerConfig struct {
// 	Port         string
// 	Environment  string
// 	ReadTimeout  time.Duration
// 	WriteTimeout time.Duration
// 	IdleTimeout  time.Duration
// 	Host         string
// 	TLSEnabled   bool
// }

// // DatabaseConfig holds enterprise database configuration
// type DatabaseConfig struct {
// 	URL                 string
// 	MaxOpenConns        int
// 	MaxIdleConns        int
// 	ConnMaxLifetime     time.Duration
// 	ConnMaxIdleTime     time.Duration
// 	SlowQueryThreshold  time.Duration
// 	EnableQueryLogging  bool
// 	EnableMetrics       bool
// 	HealthCheckInterval time.Duration
// 	MigrationsPath      string
// 	BackupRetentionDays int
// 	AutoVacuum          bool
// }

// // AuthConfig holds authentication configuration
// type AuthConfig struct {
// 	SessionSecret string
// 	SessionName   string
// 	SessionExpiry int
// 	BCryptCost    int
// 	JWTSecret     string
// 	JWTExpiry     time.Duration
// }

// // CloudinaryConfig holds Cloudinary configuration
// type CloudinaryConfig struct {
// 	CloudName    string
// 	APIKey       string
// 	APISecret    string
// 	UploadPreset string
// 	MaxFileSize  int64
// }

// // LoggingConfig holds logging configuration
// type LoggingConfig struct {
// 	Level      string
// 	Format     string
// 	Output     string
// 	EnableFile bool
// 	FilePath   string
// 	MaxSize    int
// 	MaxBackups int
// 	MaxAge     int
// }

// // Load loads configuration from environment variables with enhanced validation
// func Load() (*Config, error) {
// 	// Load .env file if in development
// 	if os.Getenv("GO_ENV") != "production" {
// 		_ = godotenv.Load()
// 	}

// 	config := &Config{
// 		Server:     loadServerConfig(),
// 		Database:   loadDatabaseConfig(),
// 		Auth:       loadAuthConfig(),
// 		Cloudinary: loadCloudinaryConfig(),
// 		Logging:    loadLoggingConfig(),
// 	}

// 	// Validate configuration
// 	if err := config.Validate(); err != nil {
// 		return nil, fmt.Errorf("configuration validation failed: %w", err)
// 	}

// 	return config, nil
// }

// // loadServerConfig loads server configuration
// func loadServerConfig() ServerConfig {
// 	return ServerConfig{
// 		Port:         getEnv("PORT", "9000"),
// 		Environment:  getEnv("GO_ENV", "development"),
// 		Host:         getEnv("SERVER_HOST", "0.0.0.0"),
// 		ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 10*time.Second),
// 		WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second),
// 		IdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 120*time.Second),
// 		TLSEnabled:   getBoolEnv("TLS_ENABLED", false),
// 	}
// }

// // loadDatabaseConfig loads enterprise database configuration
// func loadDatabaseConfig() DatabaseConfig {
// 	// Default values based on environment
// 	env := getEnv("GO_ENV", "development")

// 	var defaultMaxOpen, defaultMaxIdle int
// 	var defaultConnLifetime time.Duration

// 	switch env {
// 	case "production":
// 		defaultMaxOpen = 50
// 		defaultMaxIdle = 20
// 		defaultConnLifetime = 15 * time.Minute
// 	case "staging":
// 		defaultMaxOpen = 25
// 		defaultMaxIdle = 10
// 		defaultConnLifetime = 10 * time.Minute
// 	default: // development
// 		defaultMaxOpen = 10
// 		defaultMaxIdle = 5
// 		defaultConnLifetime = 5 * time.Minute
// 	}

// 	return DatabaseConfig{
// 		URL:                 os.Getenv("DATABASE_URL"),
// 		MaxOpenConns:        getIntEnv("DB_MAX_OPEN_CONNS", defaultMaxOpen),
// 		MaxIdleConns:        getIntEnv("DB_MAX_IDLE_CONNS", defaultMaxIdle),
// 		ConnMaxLifetime:     getDurationEnv("DB_CONN_MAX_LIFETIME", defaultConnLifetime),
// 		ConnMaxIdleTime:     getDurationEnv("DB_CONN_MAX_IDLE_TIME", 30*time.Minute),
// 		SlowQueryThreshold:  getDurationEnv("DB_SLOW_QUERY_THRESHOLD", 100*time.Millisecond),
// 		EnableQueryLogging:  getBoolEnv("DB_ENABLE_QUERY_LOGGING", env == "development"),
// 		EnableMetrics:       getBoolEnv("DB_ENABLE_METRICS", true),
// 		HealthCheckInterval: getDurationEnv("DB_HEALTH_CHECK_INTERVAL", 30*time.Second),
// 		MigrationsPath:      getEnv("DB_MIGRATIONS_PATH", "./migrations"),
// 		BackupRetentionDays: getIntEnv("DB_BACKUP_RETENTION_DAYS", 30),
// 		AutoVacuum:          getBoolEnv("DB_AUTO_VACUUM", env == "production"),
// 	}
// }

// // loadAuthConfig loads authentication configuration
// func loadAuthConfig() AuthConfig {
// 	return AuthConfig{
// 		SessionSecret: getEnv("SESSION_SECRET", "default-session-secret-change-in-production"),
// 		SessionName:   getEnv("SESSION_NAME", "evalhub_session"),
// 		SessionExpiry: getIntEnv("SESSION_EXPIRY", 24*60*60), // 24 hours
// 		BCryptCost:    getIntEnv("BCRYPT_COST", 12),
// 		JWTSecret:     getEnv("JWT_SECRET", ""),
// 		JWTExpiry:     getDurationEnv("JWT_EXPIRY", 24*time.Hour),
// 	}
// }

// // loadCloudinaryConfig loads Cloudinary configuration
// func loadCloudinaryConfig() CloudinaryConfig {
// 	return CloudinaryConfig{
// 		CloudName:    os.Getenv("CLOUDINARY_CLOUD_NAME"),
// 		APIKey:       os.Getenv("CLOUDINARY_API_KEY"),
// 		APISecret:    os.Getenv("CLOUDINARY_API_SECRET"),
// 		UploadPreset: getEnv("CLOUDINARY_UPLOAD_PRESET", ""),
// 		MaxFileSize:  getInt64Env("CLOUDINARY_MAX_FILE_SIZE", 10*1024*1024), // 10MB
// 	}
// }

// // loadLoggingConfig loads logging configuration
// func loadLoggingConfig() LoggingConfig {
// 	env := getEnv("GO_ENV", "development")

// 	return LoggingConfig{
// 		Level:      getEnv("LOG_LEVEL", getDefaultLogLevel(env)),
// 		Format:     getEnv("LOG_FORMAT", getDefaultLogFormat(env)),
// 		Output:     getEnv("LOG_OUTPUT", "stdout"),
// 		EnableFile: getBoolEnv("LOG_ENABLE_FILE", env == "production"),
// 		FilePath:   getEnv("LOG_FILE_PATH", "/var/log/evalhub/app.log"),
// 		MaxSize:    getIntEnv("LOG_MAX_SIZE", 100), // MB
// 		MaxBackups: getIntEnv("LOG_MAX_BACKUPS", 3),
// 		MaxAge:     getIntEnv("LOG_MAX_AGE", 28), // days
// 	}
// }

// // Validate validates the entire configuration
// func (c *Config) Validate() error {
// 	if err := c.Database.Validate(); err != nil {
// 		return fmt.Errorf("database config: %w", err)
// 	}

// 	if err := c.Auth.Validate(); err != nil {
// 		return fmt.Errorf("auth config: %w", err)
// 	}

// 	if err := c.Server.Validate(); err != nil {
// 		return fmt.Errorf("server config: %w", err)
// 	}

// 	return nil
// }

// // Validate validates database configuration
// func (d *DatabaseConfig) Validate() error {
// 	if d.URL == "" {
// 		return fmt.Errorf("DATABASE_URL is required")
// 	}

// 	if d.MaxOpenConns <= 0 {
// 		return fmt.Errorf("MaxOpenConns must be positive")
// 	}

// 	if d.MaxIdleConns < 0 {
// 		return fmt.Errorf("MaxIdleConns cannot be negative")
// 	}

// 	if d.MaxIdleConns > d.MaxOpenConns {
// 		return fmt.Errorf("MaxIdleConns cannot be greater than MaxOpenConns")
// 	}

// 	if d.ConnMaxLifetime <= 0 {
// 		return fmt.Errorf("ConnMaxLifetime must be positive")
// 	}

// 	if d.SlowQueryThreshold <= 0 {
// 		return fmt.Errorf("SlowQueryThreshold must be positive")
// 	}

// 	return nil
// }

// // Validate validates auth configuration
// func (a *AuthConfig) Validate() error {
// 	if a.SessionSecret == "" || a.SessionSecret == "default-session-secret-change-in-production" {
// 		env := getEnv("GO_ENV", "development")
// 		if env == "production" {
// 			return fmt.Errorf("SESSION_SECRET must be set for production")
// 		}
// 	}

// 	if a.BCryptCost < 4 || a.BCryptCost > 31 {
// 		return fmt.Errorf("BCryptCost must be between 4 and 31")
// 	}

// 	if a.SessionExpiry <= 0 {
// 		return fmt.Errorf("SessionExpiry must be positive")
// 	}

// 	return nil
// }

// // Validate validates server configuration
// func (s *ServerConfig) Validate() error {
// 	if s.Port == "" {
// 		return fmt.Errorf("PORT is required")
// 	}

// 	if s.ReadTimeout <= 0 {
// 		return fmt.Errorf("ReadTimeout must be positive")
// 	}

// 	if s.WriteTimeout <= 0 {
// 		return fmt.Errorf("WriteTimeout must be positive")
// 	}

// 	return nil
// }

// // IsProduction returns true if running in production environment
// func (c *Config) IsProduction() bool {
// 	return c.Server.Environment == "production"
// }

// // IsDevelopment returns true if running in development environment
// func (c *Config) IsDevelopment() bool {
// 	return c.Server.Environment == "development"
// }

// // Helper functions

// // getEnv gets an environment variable or returns a default value
// func getEnv(key, defaultValue string) string {
// 	if value, exists := os.LookupEnv(key); exists {
// 		return value
// 	}
// 	return defaultValue
// }

// // getIntEnv gets an integer environment variable or returns a default value
// func getIntEnv(key string, defaultValue int) int {
// 	if value, exists := os.LookupEnv(key); exists {
// 		if intValue, err := strconv.Atoi(value); err == nil {
// 			return intValue
// 		}
// 	}
// 	return defaultValue
// }

// // getInt64Env gets an int64 environment variable or returns a default value
// func getInt64Env(key string, defaultValue int64) int64 {
// 	if value, exists := os.LookupEnv(key); exists {
// 		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
// 			return intValue
// 		}
// 	}
// 	return defaultValue
// }

// // getBoolEnv gets a boolean environment variable or returns a default value
// func getBoolEnv(key string, defaultValue bool) bool {
// 	if value, exists := os.LookupEnv(key); exists {
// 		if boolValue, err := strconv.ParseBool(value); err == nil {
// 			return boolValue
// 		}
// 	}
// 	return defaultValue
// }

// // getDurationEnv gets a duration environment variable or returns a default value
// func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
// 	if value, exists := os.LookupEnv(key); exists {
// 		if duration, err := time.ParseDuration(value); err == nil {
// 			return duration
// 		}
// 	}
// 	return defaultValue
// }

// // getDefaultLogLevel returns the default log level for the environment
// func getDefaultLogLevel(env string) string {
// 	switch env {
// 	case "production":
// 		return "info"
// 	case "staging":
// 		return "debug"
// 	default:
// 		return "debug"
// 	}
// }

// // getDefaultLogFormat returns the default log format for the environment
// func getDefaultLogFormat(env string) string {
// 	switch env {
// 	case "production":
// 		return "json"
// 	default:
// 		return "console"
// 	}
// }
