// file: internal/middleware/request_id.go
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gofrs/uuid"
	"go.uber.org/zap"
)

// ContextKey type for context keys to avoid conflicts
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// LoggerKey is the context key for request-scoped logger
	LoggerKey ContextKey = "logger"
	// RequestStartKey is the context key for request start time
	RequestStartKey ContextKey = "request_start"
)

// Request ID header constants
const (
	HeaderXRequestID     = "X-Request-ID"
	HeaderXCorrelationID = "X-Correlation-ID"
)

// RequestID middleware generates and injects unique correlation IDs for request tracing
func RequestID(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Try to get existing request ID from headers (for distributed tracing)
			requestID := r.Header.Get(HeaderXRequestID)
			if requestID == "" {
				requestID = r.Header.Get(HeaderXCorrelationID)
			}
			
			// Generate new UUID if no existing request ID
			if requestID == "" {
				if id, err := uuid.NewV4(); err == nil {
					requestID = id.String()
				} else {
					// Fallback to timestamp-based ID
					requestID = generateFallbackID(start)
				}
			}
			
			// Add request ID to response headers for client visibility
			w.Header().Set(HeaderXRequestID, requestID)
			w.Header().Set(HeaderXCorrelationID, requestID)
			
			// Create request-scoped logger with correlation ID
			requestLogger := logger.With(
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", getClientIP(r)),
				zap.String("user_agent", r.UserAgent()),
			)
			
			// Inject into request context
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			ctx = context.WithValue(ctx, LoggerKey, requestLogger)
			ctx = context.WithValue(ctx, RequestStartKey, start)
			
			// Log incoming request
			requestLogger.Info("Request started",
				zap.String("query", r.URL.RawQuery),
				zap.Int64("content_length", r.ContentLength),
			)
			
			// Continue with enhanced context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
