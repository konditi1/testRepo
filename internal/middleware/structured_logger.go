// file: internal/middleware/structured_logger.go
package middleware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoggingConfig holds configuration for structured logging middleware
type LoggingConfig struct {
	// Request logging
	LogRequestHeaders  []string `json:"log_request_headers"`
	LogResponseHeaders []string `json:"log_response_headers"`
	LogRequestBody     bool     `json:"log_request_body"`
	LogResponseBody    bool     `json:"log_response_body"`
	MaxBodySize        int64    `json:"max_body_size"`

	// Performance monitoring
	SlowRequestThreshold time.Duration `json:"slow_request_threshold"`
	VerySlowThreshold    time.Duration `json:"very_slow_threshold"`

	// Sampling
	EnableSampling bool    `json:"enable_sampling"`
	SampleRate     float64 `json:"sample_rate"`

	// Security
	LogUserAgent     bool     `json:"log_user_agent"`
	LogReferer       bool     `json:"log_referer"`
	SensitiveHeaders []string `json:"sensitive_headers"`

	// Audit
	AuditEndpoints     []string `json:"audit_endpoints"`
	LogLevel           string   `json:"log_level"`
	EnableErrorDetails bool     `json:"enable_error_details"`
}

// DefaultLoggingConfig returns production-ready logging configuration
func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		LogRequestHeaders: []string{
			"Content-Type", "Content-Length", "Accept", "Accept-Language",
			"X-Forwarded-For", "X-Real-IP", "X-Request-ID", "Authorization",
		},
		LogResponseHeaders: []string{
			"Content-Type", "Content-Length", "X-Request-ID", "X-Response-Time",
		},
		LogRequestBody:       false, // Usually false in production for performance
		LogResponseBody:      false, // Usually false in production for performance
		MaxBodySize:          1024,  // 1KB max for body logging
		SlowRequestThreshold: 1 * time.Second,
		VerySlowThreshold:    5 * time.Second,
		EnableSampling:       true,
		SampleRate:           1.0, // 100% sampling by default
		LogUserAgent:         true,
		LogReferer:           true,
		SensitiveHeaders: []string{
			"Authorization", "Cookie", "Set-Cookie", "X-API-Key", "X-Auth-Token",
		},
		AuditEndpoints: []string{
			"/api/auth/", "/api/admin/", "/api/users/", "/api/payments/",
		},
		LogLevel:           "info",
		EnableErrorDetails: true,
	}
}

// StructuredLogging creates enhanced structured logging middleware
func StructuredLogging(logger *zap.Logger, config *LoggingConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultLoggingConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := GetRequestStart(r.Context())
			requestLogger := GetRequestLogger(r.Context())
			requestID := GetRequestID(r.Context())

			// Check if we should sample this request
			if config.EnableSampling && !shouldSampleRequest(r.URL.Path, config.SampleRate) {
				next.ServeHTTP(w, r)
				return
			}

			// Create enhanced response writer
			writer := &StructuredResponseWriter{
				ResponseWriter: w,
				config:         config,
				logger:         requestLogger,
				startTime:      start,
			}

			// Log incoming request
			logIncomingRequest(requestLogger, r, config)

			// Capture request body if configured
			if config.LogRequestBody && shouldLogBody(r) {
				r = captureRequestBody(r, config.MaxBodySize, requestLogger)
			}

			// Process request
			next.ServeHTTP(writer, r)

			// Log completed request with comprehensive metrics
			logCompletedRequest(requestLogger, r, writer, start, config)

			// Log performance warnings
			logPerformanceMetrics(requestLogger, r, writer, start, config)

			// Log security events if needed
			logSecurityEvents(requestLogger, r, writer, config)

			// Log audit events for sensitive endpoints
			if isAuditEndpoint(r.URL.Path, config.AuditEndpoints) {
				logAuditEvent(requestLogger, r, writer, requestID)
			}
		})
	}
}

// ===============================
// STRUCTURED RESPONSE WRITER
// ===============================

// StructuredResponseWriter captures response data for logging
type StructuredResponseWriter struct {
	http.ResponseWriter
	config          *LoggingConfig
	logger          *zap.Logger
	startTime       time.Time
	status          int
	bytesWritten    int64
	responseBody    *bytes.Buffer
	headersCaptured bool
	responseHeaders http.Header
}

func (w *StructuredResponseWriter) WriteHeader(code int) {
	w.status = code
	if !w.headersCaptured {
		w.responseHeaders = make(http.Header)
		for k, v := range w.ResponseWriter.Header() {
			w.responseHeaders[k] = v
		}
		w.headersCaptured = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *StructuredResponseWriter) Write(data []byte) (int, error) {
	// Capture response body if configured
	if w.config.LogResponseBody && w.responseBody == nil {
		w.responseBody = &bytes.Buffer{}
	}

	if w.responseBody != nil && w.responseBody.Len() < int(w.config.MaxBodySize) {
		w.responseBody.Write(data)
	}

	written, err := w.ResponseWriter.Write(data)
	w.bytesWritten += int64(written)
	return written, err
}

func (w *StructuredResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not support hijacking")
}

func (w *StructuredResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Status returns the HTTP status code
func (w *StructuredResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// ===============================
// REQUEST LOGGING FUNCTIONS
// ===============================

// logIncomingRequest logs the incoming HTTP request
func logIncomingRequest(logger *zap.Logger, r *http.Request, config *LoggingConfig) {
	fields := []zap.Field{
		zap.String("event", "request_started"),
		zap.String("method", r.Method),
		zap.String("url", r.URL.String()),
		zap.String("protocol", r.Proto),
		zap.String("remote_addr", getClientIP(r)),
	}

	// Add request headers
	if len(config.LogRequestHeaders) > 0 {
		headers := make(map[string]string)
		for _, headerName := range config.LogRequestHeaders {
			if value := r.Header.Get(headerName); value != "" {
				if isSensitiveHeader(headerName, config.SensitiveHeaders) {
					headers[headerName] = maskSensitiveValue(value)
				} else {
					headers[headerName] = value
				}
			}
		}
		fields = append(fields, zap.Any("request_headers", headers))
	}

	// Add user agent and referer if configured
	if config.LogUserAgent {
		fields = append(fields, zap.String("user_agent", r.UserAgent()))
	}
	if config.LogReferer {
		fields = append(fields, zap.String("referer", r.Referer()))
	}

	// Add request size
	if r.ContentLength > 0 {
		fields = append(fields, zap.Int64("request_size", r.ContentLength))
	}

	// Add query parameters (sanitized)
	if r.URL.RawQuery != "" {
		fields = append(fields, zap.String("query_params", sanitizeQueryParams(r.URL.RawQuery)))
	}

	// Add user context if available
	if userID := getUserIDFromContext(r.Context()); userID > 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}

	logger.Info("HTTP request started", fields...)
}

// logCompletedRequest logs the completed HTTP request with comprehensive metrics
func logCompletedRequest(logger *zap.Logger, r *http.Request, w *StructuredResponseWriter, start time.Time, config *LoggingConfig) {
	duration := time.Since(start)

	fields := []zap.Field{
		zap.String("event", "request_completed"),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Int("status", w.Status()),
		zap.Duration("duration", duration),
		zap.Float64("duration_ms", float64(duration.Nanoseconds())/1e6),
		zap.Int64("response_size", w.bytesWritten),
	}

	// Add response headers
	if len(config.LogResponseHeaders) > 0 && w.responseHeaders != nil {
		headers := make(map[string]string)
		for _, headerName := range config.LogResponseHeaders {
			if values := w.responseHeaders[headerName]; len(values) > 0 {
				headers[headerName] = values[0]
			}
		}
		fields = append(fields, zap.Any("response_headers", headers))
	}

	// Add request/response body if configured and captured
	if config.LogRequestBody && hasRequestBody(r) {
		if body := getRequestBodyFromContext(r.Context()); body != "" {
			fields = append(fields, zap.String("request_body", truncateString(body, 500)))
		}
	}

	if config.LogResponseBody && w.responseBody != nil {
		fields = append(fields, zap.String("response_body", truncateString(w.responseBody.String(), 500)))
	}

	// Add performance metrics
	fields = append(fields,
		zap.Float64("requests_per_second", 1.0/duration.Seconds()),
		zap.Int64("memory_usage", getMemoryUsage()),
	)

	// Add user context
	if userID := getUserIDFromContext(r.Context()); userID > 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}

	// Determine log level based on status and duration
	logLevel := getLogLevel(w.Status(), duration, config)

	switch logLevel {
	case zapcore.ErrorLevel:
		logger.Error("HTTP request completed with error", fields...)
	case zapcore.WarnLevel:
		logger.Warn("HTTP request completed with warning", fields...)
	default:
		logger.Info("HTTP request completed", fields...)
	}
}

// logPerformanceMetrics logs performance-related warnings and metrics
func logPerformanceMetrics(logger *zap.Logger, r *http.Request, w *StructuredResponseWriter, start time.Time, config *LoggingConfig) {
	duration := time.Since(start)

	// Log slow requests
	if duration > config.SlowRequestThreshold {
		severity := "slow"
		if duration > config.VerySlowThreshold {
			severity = "very_slow"
		}

		logger.Warn("Slow request detected",
			zap.String("event", "slow_request"),
			zap.String("severity", severity),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Duration("duration", duration),
			zap.Duration("threshold", config.SlowRequestThreshold),
			zap.Int("status", w.Status()),
			zap.Int64("response_size", w.bytesWritten),
		)
	}

	// Log large responses
	if w.bytesWritten > 10*1024*1024 { // 10MB
		logger.Warn("Large response detected",
			zap.String("event", "large_response"),
			zap.String("path", r.URL.Path),
			zap.Int64("response_size", w.bytesWritten),
		)
	}

	// Log high memory usage (if available)
	if memUsage := getMemoryUsage(); memUsage > 100*1024*1024 { // 100MB
		logger.Warn("High memory usage detected",
			zap.String("event", "high_memory"),
			zap.Int64("memory_usage", memUsage),
			zap.String("path", r.URL.Path),
		)
	}
}

// logSecurityEvents logs security-related events
func logSecurityEvents(logger *zap.Logger, r *http.Request, w *StructuredResponseWriter, config *LoggingConfig) {
	// Log authentication failures
	if w.Status() == http.StatusUnauthorized {
		logger.Warn("Authentication failure",
			zap.String("event", "auth_failure"),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", getClientIP(r)),
			zap.String("user_agent", r.UserAgent()),
		)
	}

	// Log authorization failures
	if w.Status() == http.StatusForbidden {
		logger.Warn("Authorization failure",
			zap.String("event", "authz_failure"),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", getClientIP(r)),
		)

		if userID := getUserIDFromContext(r.Context()); userID > 0 {
			logger.With(zap.Int64("user_id", userID))
		}
	}

	// Log rate limiting
	if w.Status() == http.StatusTooManyRequests {
		logger.Warn("Rate limit exceeded",
			zap.String("event", "rate_limit"),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", getClientIP(r)),
		)
	}

	// Log potential security issues
	if w.Status() >= 400 && w.Status() < 500 {
		// Check for suspicious patterns
		if containsSuspiciousPatterns(r.URL.Path) {
			logger.Warn("Suspicious request pattern",
				zap.String("event", "suspicious_request"),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", getClientIP(r)),
				zap.String("user_agent", r.UserAgent()),
			)
		}
	}
}

// logAuditEvent logs audit events for sensitive operations
func logAuditEvent(logger *zap.Logger, r *http.Request, w *StructuredResponseWriter, requestID string) {
	fields := []zap.Field{
		zap.String("event", "audit"),
		zap.String("request_id", requestID),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Int("status", w.Status()),
		zap.String("remote_addr", getClientIP(r)),
		zap.Time("timestamp", time.Now()),
	}

	// Add user context
	if userID := getUserIDFromContext(r.Context()); userID > 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}

	// Add query parameters for audit
	if r.URL.RawQuery != "" {
		fields = append(fields, zap.String("query_params", r.URL.RawQuery))
	}

	// Create audit record
	logger.Info("Audit event", fields...)
}

// ===============================
// ERROR LOGGING INTEGRATION
// ===============================

// ErrorLogging middleware integrates with your existing ServiceError system
func ErrorLogging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap response writer to capture errors
			writer := &ErrorCapturingWriter{
				ResponseWriter: w,
				logger:         GetRequestLogger(r.Context()),
				request:        r,
			}

			next.ServeHTTP(writer, r)
		})
	}
}

// ErrorCapturingWriter captures and logs error responses
type ErrorCapturingWriter struct {
	http.ResponseWriter
	logger  *zap.Logger
	request *http.Request
	body    *bytes.Buffer
}

func (w *ErrorCapturingWriter) WriteHeader(code int) {
	if code >= 400 {
		w.body = &bytes.Buffer{}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *ErrorCapturingWriter) Write(data []byte) (int, error) {
	if w.body != nil {
		w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

// LogServiceError logs errors using your ServiceError format
func LogServiceError(logger *zap.Logger, r *http.Request, err error) {
	requestID := GetRequestID(r.Context())

	fields := []zap.Field{
		zap.String("event", "service_error"),
		zap.String("request_id", requestID),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Error(err),
	}

	// Add user context
	if userID := getUserIDFromContext(r.Context()); userID > 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}

	// Parse your ServiceError if it matches the format
	if serviceErr := parseServiceError(err); serviceErr != nil {
		fields = append(fields,
			zap.String("error_type", serviceErr.Type),
			zap.String("error_code", serviceErr.Code),
			zap.Any("error_details", serviceErr.Details),
		)
	}

	logger.Error("Service error occurred", fields...)
}

// ===============================
// HELPER FUNCTIONS
// ===============================

// shouldSampleRequest determines if a request should be sampled
func shouldSampleRequest(path string, sampleRate float64) bool {
	if sampleRate >= 1.0 {
		return true
	}

	// Always sample health checks and errors
	if strings.Contains(path, "/health") || strings.Contains(path, "/error") {
		return true
	}

	// Simple hash-based sampling (deterministic)
	hash := simpleHash(path)
	return float64(hash%100)/100.0 < sampleRate
}

// shouldLogBody determines if request body should be logged
func shouldLogBody(r *http.Request) bool {
	// Only log POST, PUT, PATCH requests
	return r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH"
}

// captureRequestBody captures request body for logging
func captureRequestBody(r *http.Request, maxSize int64, logger *zap.Logger) *http.Request {
	if r.Body == nil {
		return r
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		logger.Warn("Failed to read request body", zap.Error(err))
		return r
	}

	// Store body in context
	ctx := context.WithValue(r.Context(), "request_body", string(body))

	// Replace body with new reader
	r.Body = io.NopCloser(bytes.NewReader(body))

	return r.WithContext(ctx)
}

// getLogLevel determines appropriate log level based on status and duration
func getLogLevel(status int, duration time.Duration, config *LoggingConfig) zapcore.Level {
	if status >= 500 {
		return zapcore.ErrorLevel
	}
	if status >= 400 || duration > config.VerySlowThreshold {
		return zapcore.WarnLevel
	}
	return zapcore.InfoLevel
}

// Utility functions for request processing
func hasRequestBody(r *http.Request) bool {
	return r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") == "chunked"
}

func getRequestBodyFromContext(ctx context.Context) string {
	if body, ok := ctx.Value("request_body").(string); ok {
		return body
	}
	return ""
}

func getUserIDFromContext(ctx context.Context) int64 {
	// This would integrate with your auth system
	if userID, ok := ctx.Value("userID").(int64); ok {
		return userID
	}
	return 0
}

func isSensitiveHeader(header string, sensitiveHeaders []string) bool {
	header = strings.ToLower(header)
	for _, sensitive := range sensitiveHeaders {
		if strings.ToLower(sensitive) == header {
			return true
		}
	}
	return false
}

func maskSensitiveValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

func sanitizeQueryParams(query string) string {
	// Remove sensitive parameters like passwords, tokens, etc.
	sensitiveParams := []string{"password", "token", "key", "secret", "auth"}

	parts := strings.Split(query, "&")
	var sanitized []string

	for _, part := range parts {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) == 2 {
			key := strings.ToLower(keyValue[0])
			for _, sensitive := range sensitiveParams {
				if strings.Contains(key, sensitive) {
					keyValue[1] = "***"
					break
				}
			}
		}
		sanitized = append(sanitized, strings.Join(keyValue, "="))
	}

	return strings.Join(sanitized, "&")
}

func isAuditEndpoint(path string, auditEndpoints []string) bool {
	for _, endpoint := range auditEndpoints {
		if strings.HasPrefix(path, endpoint) {
			return true
		}
	}
	return false
}

func containsSuspiciousPatterns(path string) bool {
	suspicious := []string{
		"../", "..\\", "/etc/", "/proc/", "cmd.exe", "powershell",
		"<script", "javascript:", "eval(", "base64",
	}

	lowerPath := strings.ToLower(path)
	for _, pattern := range suspicious {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func simpleHash(s string) int {
	hash := 0
	for _, c := range s {
		hash = 31*hash + int(c)
	}
	return hash
}

func getMemoryUsage() int64 {
	// This would integrate with runtime.MemStats or your monitoring system
	// For now, return 0 as placeholder
	return 0
}

// parseServiceError attempts to parse your ServiceError format
func parseServiceError(err error) *ServiceErrorForLogging {
	// This would integrate with your services.ServiceError
	// For now, return nil as placeholder
	return nil
}

// ServiceErrorForLogging represents your ServiceError for logging
type ServiceErrorForLogging struct {
	Type    string                 `json:"type"`
	Code    string                 `json:"code"`
	Details map[string]interface{} `json:"details"`
}

// ===============================
// INTEGRATION HELPERS
// ===============================

// Enhanced logging middleware factory
func CreateEnhancedLoggingStack(logger *zap.Logger, config *LoggingConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultLoggingConfig()
	}

	return func(next http.Handler) http.Handler {
		// Stack multiple logging middleware
		handler := next
		handler = ErrorLogging(logger)(handler)              // Error capture
		handler = StructuredLogging(logger, config)(handler) // Main logging
		return handler
	}
}
