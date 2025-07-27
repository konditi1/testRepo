// File: internal/middleware/error_handler.go
package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ===============================
// ERROR HANDLING CONFIGURATION
// ===============================

// ErrorHandlerConfig holds configuration for error handling middleware
type ErrorHandlerConfig struct {
	// Error classification
	EnableErrorClassification bool     `json:"enable_error_classification"`
	BusinessErrorCodes        []string `json:"business_error_codes"`
	SystemErrorCodes          []string `json:"system_error_codes"`

	// Error monitoring
	EnableErrorMetrics     bool          `json:"enable_error_metrics"`
	EnableErrorAggregation bool          `json:"enable_error_aggregation"`
	AggregationWindow      time.Duration `json:"aggregation_window"`

	// Error response handling
	IncludeErrorDetails   bool     `json:"include_error_details"`
	IncludeRequestContext bool     `json:"include_request_context"`
	SanitizeSensitiveData bool     `json:"sanitize_sensitive_data"`
	SensitiveFields       []string `json:"sensitive_fields"`

	// Circuit breaker
	EnableCircuitBreaker  bool          `json:"enable_circuit_breaker"`
	FailureThreshold      int           `json:"failure_threshold"`
	CircuitBreakerTimeout time.Duration `json:"circuit_breaker_timeout"`

	// Retry and backoff
	EnableRetryLogic bool          `json:"enable_retry_logic"`
	MaxRetries       int           `json:"max_retries"`
	RetryDelay       time.Duration `json:"retry_delay"`

	// Error templates
	UseCustomErrorPages   bool   `json:"use_custom_error_pages"`
	DefaultErrorTemplate  string `json:"default_error_template"`
	NotFoundTemplate      string `json:"not_found_template"`
	UnauthorizedTemplate  string `json:"unauthorized_template"`
	ForbiddenTemplate     string `json:"forbidden_template"`
	InternalErrorTemplate string `json:"internal_error_template"`
}

// DefaultErrorHandlerConfig returns production-ready error handling configuration
func DefaultErrorHandlerConfig() *ErrorHandlerConfig {
	return &ErrorHandlerConfig{
		EnableErrorClassification: true,
		BusinessErrorCodes: []string{
			"VALIDATION_ERROR", "BUSINESS_ERROR", "CONFLICT", "NOT_FOUND",
		},
		SystemErrorCodes: []string{
			"INTERNAL_ERROR", "SERVICE_UNAVAILABLE", "TIMEOUT", "DATABASE_ERROR",
		},
		EnableErrorMetrics:     true,
		EnableErrorAggregation: true,
		AggregationWindow:      5 * time.Minute,
		IncludeErrorDetails:    true,
		IncludeRequestContext:  true,
		SanitizeSensitiveData:  true,
		SensitiveFields: []string{
			"password", "token", "key", "secret", "auth", "session",
		},
		EnableCircuitBreaker:  true,
		FailureThreshold:      10,
		CircuitBreakerTimeout: 30 * time.Second,
		EnableRetryLogic:      false, // Usually handled at service level
		MaxRetries:            3,
		RetryDelay:            100 * time.Millisecond,
		UseCustomErrorPages:   true,
		DefaultErrorTemplate:  "error",
		NotFoundTemplate:      "404",
		UnauthorizedTemplate:  "401",
		ForbiddenTemplate:     "403",
		InternalErrorTemplate: "500",
	}
}

// ===============================
// ERROR CONTEXT AND TRACKING
// ===============================

// ErrorContext contains comprehensive error context information
type ErrorContext struct {
	RequestID      string                 `json:"request_id"`
	UserID         *int64                 `json:"user_id,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Error          error                  `json:"-"`
	ErrorType      string                 `json:"error_type"`
	ErrorCode      string                 `json:"error_code"`
	ErrorMessage   string                 `json:"error_message"`
	HTTPStatus     int                    `json:"http_status"`
	Request        *ErrorRequestInfo      `json:"request"`
	Response       *ErrorResponseInfo     `json:"response,omitempty"`
	StackTrace     []string               `json:"stack_trace,omitempty"`
	Metadata       map[string]interface{} `json:"metadata"`
	Classification string                 `json:"classification"` // "business", "system", "user"
}

// ErrorRequestInfo contains request information for error context
type ErrorRequestInfo struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	UserAgent  string            `json:"user_agent"`
	RemoteAddr string            `json:"remote_addr"`
	Duration   time.Duration     `json:"duration"`
}

// ErrorResponseInfo contains response information for error context
type ErrorResponseInfo struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	Size       int64             `json:"size"`
}

// ===============================
// ERROR METRICS AND MONITORING
// ===============================

// ErrorMetrics tracks error statistics
type ErrorMetrics struct {
	TotalErrors      int64            `json:"total_errors"`
	ErrorsByType     map[string]int64 `json:"errors_by_type"`
	ErrorsByEndpoint map[string]int64 `json:"errors_by_endpoint"`
	ErrorsByUser     map[int64]int64  `json:"errors_by_user"`
	ErrorsByStatus   map[int]int64    `json:"errors_by_status"`
	RecentErrors     []time.Time      `json:"recent_errors"`
	LastError        time.Time        `json:"last_error"`
	ErrorRate        float64          `json:"error_rate"`
	MeanTimeBetween  time.Duration    `json:"mean_time_between"`
}

// ErrorTracker tracks and aggregates error metrics
type ErrorTracker struct {
	config  *ErrorHandlerConfig
	logger  *zap.Logger
	metrics *ErrorMetrics
	errors  []ErrorContext
	mu      sync.RWMutex
}

// NewErrorTracker creates a new error tracker
func NewErrorTracker(config *ErrorHandlerConfig, logger *zap.Logger) *ErrorTracker {
	return &ErrorTracker{
		config: config,
		logger: logger,
		metrics: &ErrorMetrics{
			ErrorsByType:     make(map[string]int64),
			ErrorsByEndpoint: make(map[string]int64),
			ErrorsByUser:     make(map[int64]int64),
			ErrorsByStatus:   make(map[int]int64),
			RecentErrors:     make([]time.Time, 0),
		},
		errors: make([]ErrorContext, 0),
	}
}

// ===============================
// MAIN ERROR HANDLING MIDDLEWARE
// ===============================

// ErrorHandler creates comprehensive error handling middleware
func ErrorHandler(config *ErrorHandlerConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultErrorHandlerConfig()
	}

	tracker := NewErrorTracker(config, logger)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create error-aware response writer
			writer := &ErrorResponseWriter{
				ResponseWriter: w,
				config:         config,
				logger:         GetRequestLogger(r.Context()),
				startTime:      start,
			}

			// Process request
			next.ServeHTTP(writer, r)

			// Handle any errors that occurred
			if writer.errorOccurred {
				errorCtx := &ErrorContext{
					RequestID: GetRequestID(r.Context()),
					Timestamp: time.Now(),
					Request:   captureErrorRequestInfo(r, start),
					Response:  writer.getResponseInfo(),
					Metadata:  make(map[string]interface{}),
				}

				// Add user context
				if userID := getUserIDFromContext(r.Context()); userID > 0 {
					errorCtx.UserID = &userID
				}

				// Process the error
				processError(errorCtx, writer.statusCode, config, tracker, logger)
			}
		})
	}
}

// ===============================
// ERROR-AWARE RESPONSE WRITER
// ===============================

// ErrorResponseWriter wraps http.ResponseWriter to capture error information
type ErrorResponseWriter struct {
	http.ResponseWriter
	config        *ErrorHandlerConfig
	logger        *zap.Logger
	startTime     time.Time
	statusCode    int
	bytesWritten  int64
	errorOccurred bool
	responseBody  []byte
}

func (w *ErrorResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	if code >= 400 {
		w.errorOccurred = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *ErrorResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	// Capture response body for error analysis
	if w.errorOccurred && len(w.responseBody) < 1024 { // Limit to 1KB
		w.responseBody = append(w.responseBody, data...)
	}

	written, err := w.ResponseWriter.Write(data)
	w.bytesWritten += int64(written)
	return written, err
}

func (w *ErrorResponseWriter) getResponseInfo() *ErrorResponseInfo {
	headers := make(map[string]string)
	for k, v := range w.Header() {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &ErrorResponseInfo{
		StatusCode: w.statusCode,
		Headers:    headers,
		Body:       string(w.responseBody),
		Size:       w.bytesWritten,
	}
}

// ===============================
// ERROR PROCESSING
// ===============================

// processError processes an error and updates metrics
func processError(errorCtx *ErrorContext, statusCode int, config *ErrorHandlerConfig, tracker *ErrorTracker, logger *zap.Logger) {
	// Classify the error
	errorCtx.Classification = classifyError(statusCode, config)
	errorCtx.HTTPStatus = statusCode
	errorCtx.ErrorType = getErrorTypeFromStatus(statusCode)

	// Log the error
	logError(logger, errorCtx, config)

	// Track metrics if enabled
	if config.EnableErrorMetrics {
		tracker.RecordError(errorCtx)
	}

	// Check circuit breaker if enabled
	if config.EnableCircuitBreaker {
		checkCircuitBreaker(errorCtx, config, tracker, logger)
	}
}

// classifyError classifies an error as business, system, or user error
func classifyError(statusCode int, config *ErrorHandlerConfig) string {
	switch {
	case statusCode >= 400 && statusCode < 500:
		if statusCode == 401 || statusCode == 403 {
			return "authentication"
		}
		if statusCode == 400 || statusCode == 422 {
			return "validation"
		}
		return "client"
	case statusCode >= 500:
		return "system"
	default:
		return "success"
	}
}

// getErrorTypeFromStatus maps HTTP status to error type
func getErrorTypeFromStatus(statusCode int) string {
	switch statusCode {
	case 400:
		return "BAD_REQUEST"
	case 401:
		return "UNAUTHORIZED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	case 422:
		return "VALIDATION_ERROR"
	case 429:
		return "RATE_LIMIT"
	case 500:
		return "INTERNAL_ERROR"
	case 502:
		return "BAD_GATEWAY"
	case 503:
		return "SERVICE_UNAVAILABLE"
	case 504:
		return "TIMEOUT"
	default:
		return "UNKNOWN_ERROR"
	}
}

// ===============================
// ERROR LOGGING
// ===============================

// logError logs error with appropriate level and context
func logError(logger *zap.Logger, errorCtx *ErrorContext, config *ErrorHandlerConfig) {
	fields := []zap.Field{
		zap.String("event", "error_handled"),
		zap.String("request_id", errorCtx.RequestID),
		zap.String("error_type", errorCtx.ErrorType),
		zap.String("classification", errorCtx.Classification),
		zap.Int("status_code", errorCtx.HTTPStatus),
		zap.Time("timestamp", errorCtx.Timestamp),
		zap.String("method", errorCtx.Request.Method),
		zap.String("url", errorCtx.Request.URL),
		zap.Duration("duration", errorCtx.Request.Duration),
		zap.String("remote_addr", errorCtx.Request.RemoteAddr),
	}

	// Add user context
	if errorCtx.UserID != nil {
		fields = append(fields, zap.Int64("user_id", *errorCtx.UserID))
	}

	// Add error details if enabled
	if config.IncludeErrorDetails {
		if errorCtx.Response != nil {
			fields = append(fields,
				zap.String("response_body", errorCtx.Response.Body),
				zap.Int64("response_size", errorCtx.Response.Size),
			)
		}
	}

	// Add metadata
	if len(errorCtx.Metadata) > 0 {
		fields = append(fields, zap.Any("metadata", errorCtx.Metadata))
	}

	// Determine log level based on error classification
	switch errorCtx.Classification {
	case "system":
		logger.Error("System error occurred", fields...)
	case "authentication":
		logger.Warn("Authentication error", fields...)
	case "validation", "client":
		logger.Info("Client error", fields...)
	default:
		logger.Info("Error handled", fields...)
	}
}

// ===============================
// ERROR METRICS TRACKING
// ===============================

// RecordError records an error in the metrics
func (et *ErrorTracker) RecordError(errorCtx *ErrorContext) {
	et.mu.Lock()
	defer et.mu.Unlock()

	// Update metrics
	et.metrics.TotalErrors++
	et.metrics.ErrorsByType[errorCtx.ErrorType]++
	et.metrics.ErrorsByStatus[errorCtx.HTTPStatus]++

	// Track by endpoint
	endpoint := fmt.Sprintf("%s %s", errorCtx.Request.Method, errorCtx.Request.URL)
	et.metrics.ErrorsByEndpoint[endpoint]++

	// Track by user
	if errorCtx.UserID != nil {
		et.metrics.ErrorsByUser[*errorCtx.UserID]++
	}

	// Update recent errors
	et.metrics.RecentErrors = append(et.metrics.RecentErrors, errorCtx.Timestamp)
	et.metrics.LastError = errorCtx.Timestamp

	// Keep only recent errors within window
	cutoff := time.Now().Add(-et.config.AggregationWindow)
	filtered := et.metrics.RecentErrors[:0]
	for _, t := range et.metrics.RecentErrors {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	et.metrics.RecentErrors = filtered

	// Calculate error rate
	if len(et.metrics.RecentErrors) > 1 {
		window := et.metrics.RecentErrors[len(et.metrics.RecentErrors)-1].Sub(et.metrics.RecentErrors[0])
		et.metrics.ErrorRate = float64(len(et.metrics.RecentErrors)) / window.Seconds()
		et.metrics.MeanTimeBetween = window / time.Duration(len(et.metrics.RecentErrors)-1)
	}

	// Store error context
	et.errors = append(et.errors, *errorCtx)

	// Keep only recent errors to prevent memory leaks
	if len(et.errors) > 1000 {
		et.errors = et.errors[len(et.errors)-500:] // Keep last 500
	}
}

// GetMetrics returns current error metrics
func (et *ErrorTracker) GetMetrics() *ErrorMetrics {
	et.mu.RLock()
	defer et.mu.RUnlock()

	// Return a copy of metrics
	metricsCopy := *et.metrics
	return &metricsCopy
}

// ===============================
// CIRCUIT BREAKER
// ===============================

// checkCircuitBreaker checks if circuit breaker should be triggered
func checkCircuitBreaker(errorCtx *ErrorContext, config *ErrorHandlerConfig, tracker *ErrorTracker, logger *zap.Logger) {
	if errorCtx.Classification != "system" {
		return // Only trigger on system errors
	}

	metrics := tracker.GetMetrics()
	recentSystemErrors := 0

	cutoff := time.Now().Add(-config.CircuitBreakerTimeout)
	for _, t := range metrics.RecentErrors {
		if t.After(cutoff) {
			recentSystemErrors++
		}
	}

	if recentSystemErrors >= config.FailureThreshold {
		logger.Warn("Circuit breaker threshold reached",
			zap.String("request_id", errorCtx.RequestID),
			zap.Int("recent_errors", recentSystemErrors),
			zap.Int("threshold", config.FailureThreshold),
			zap.Duration("window", config.CircuitBreakerTimeout),
		)

		// Here you would implement circuit breaker logic
		// - Reject requests for a period
		// - Return cached responses
		// - Redirect to backup services
		// - Enable degraded mode
	}
}

// ===============================
// UTILITY FUNCTIONS
// ===============================

// captureErrorRequestInfo captures request information for error context
func captureErrorRequestInfo(r *http.Request, startTime time.Time) *ErrorRequestInfo {
	headers := make(map[string]string)

	// Capture important headers (sanitized)
	importantHeaders := []string{
		"Content-Type", "Accept", "User-Agent", "Authorization",
		"X-Forwarded-For", "X-Real-IP", "X-Request-ID",
	}

	for _, header := range importantHeaders {
		if value := r.Header.Get(header); value != "" {
			// Sanitize sensitive headers
			if header == "Authorization" && len(value) > 10 {
				headers[header] = value[:10] + "***"
			} else {
				headers[header] = value
			}
		}
	}

	return &ErrorRequestInfo{
		Method:     r.Method,
		URL:        r.URL.String(),
		Headers:    headers,
		UserAgent:  r.UserAgent(),
		RemoteAddr: getClientIP(r),
		Duration:   time.Since(startTime),
	}
}

// ===============================
// SERVICE ERROR INTEGRATION
// ===============================

// ServiceErrorHandler handles service errors specifically
func ServiceErrorHandler(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writer := &ServiceErrorCapturingWriter{
				ResponseWriter: w,
				request:        r,
				logger:         GetRequestLogger(r.Context()),
			}

			next.ServeHTTP(writer, r)
		})
	}
}

// ServiceErrorCapturingWriter captures service errors from response body
type ServiceErrorCapturingWriter struct {
	http.ResponseWriter
	request      *http.Request
	logger       *zap.Logger
	responseBody []byte
	statusCode   int
}

func (w *ServiceErrorCapturingWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *ServiceErrorCapturingWriter) Write(data []byte) (int, error) {
	// Capture response body for service error analysis
	if w.statusCode >= 400 && len(w.responseBody) < 2048 {
		w.responseBody = append(w.responseBody, data...)

		// Try to parse as service error
		if err := w.parseAndLogServiceError(); err != nil {
			w.logger.Debug("Failed to parse service error", zap.Error(err))
		}
	}

	return w.ResponseWriter.Write(data)
}

func (w *ServiceErrorCapturingWriter) parseAndLogServiceError() error {
	var errorResponse map[string]interface{}
	if err := json.Unmarshal(w.responseBody, &errorResponse); err != nil {
		return err
	}

	// Extract error information
	if errorData, ok := errorResponse["error"].(map[string]interface{}); ok {
		w.logger.Info("Service error captured",
			zap.String("request_id", GetRequestID(w.request.Context())),
			zap.Any("error_type", errorData["type"]),
			zap.Any("error_message", errorData["message"]),
			zap.Any("error_code", errorData["code"]),
			zap.Int("status_code", w.statusCode),
		)
	}

	return nil
}

// ===============================
// INTEGRATION HELPERS
// ===============================

// CreateErrorHandlingStack creates a complete error handling middleware stack
func CreateErrorHandlingStack(config *ErrorHandlerConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultErrorHandlerConfig()
	}

	return func(next http.Handler) http.Handler {
		// Stack error handling middleware
		handler := next
		handler = ServiceErrorHandler(logger)(handler)  // Service error capture
		handler = ErrorHandler(config, logger)(handler) // Main error handling
		return handler
	}
}

// GetErrorMetricsHandler creates an endpoint for error metrics
func GetErrorMetricsHandler(tracker *ErrorTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics := tracker.GetMetrics()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
			return
		}
	}
}
