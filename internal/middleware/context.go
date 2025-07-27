// file: internal/middleware/context.go
package middleware

import (
	"context"
	"net/http"
	"time"

	"evalhub/internal/contextutils"
	"go.uber.org/zap"
)

// GetRequestID extracts the request ID from context
// Deprecated: Use contextutils.GetRequestID instead
func GetRequestID(ctx context.Context) string {
	return contextutils.GetRequestID(ctx)
}

// GetRequestLogger extracts the request-scoped logger from context
func GetRequestLogger(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(LoggerKey).(*zap.Logger); ok {
		return logger
	}
	// Fallback to basic logger if not found
	logger, _ := zap.NewProduction()
	return logger
}

// GetRequestStart extracts the request start time from context
func GetRequestStart(ctx context.Context) time.Time {
	if start, ok := ctx.Value(RequestStartKey).(time.Time); ok {
		return start
	}
	return time.Now()
}

// GetSanitizedData extracts sanitized data from request context
func GetSanitizedData(ctx context.Context) map[string]interface{} {
	if val := ctx.Value(SanitizedDataKey); val != nil {
		if data, ok := val.(map[string]interface{}); ok {
			return data
		}
	}
	return nil
}

// GetValidatedFiles extracts validated files from request context
func GetValidatedFiles(ctx context.Context) []ValidatedFile {
	if val := ctx.Value(ValidatedFilesKey); val != nil {
		if files, ok := val.([]ValidatedFile); ok {
			return files
		}
	}
	return nil
}

// WithRequestContext adds request context fields to an existing logger
func WithRequestContext(logger *zap.Logger, ctx context.Context) *zap.Logger {
	requestID := contextutils.GetRequestID(ctx)
	if requestID != "" {
		return logger.With(zap.String("request_id", requestID))
	}
	return logger
}

// generateFallbackID creates a fallback ID when UUID generation fails
func generateFallbackID(start time.Time) string {
	return "req_" + start.Format("20060102150405") + "_" + generateRandomSuffix()
}

// generateRandomSuffix creates a simple random suffix
func generateRandomSuffix() string {
	// Simple implementation - could be enhanced with crypto/rand if needed
	return "000"
}

// getClientIP extracts the real client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can be "client, proxy1, proxy2"
		// Take the first IP which should be the original client
		if idx := len(xff); idx > 0 {
			if commaIdx := 0; commaIdx < len(xff) {
				for i, char := range xff {
					if char == ',' {
						commaIdx = i
						break
					}
				}
				if commaIdx > 0 {
					return xff[:commaIdx]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header (nginx proxy)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Check X-Forwarded header
	if xf := r.Header.Get("X-Forwarded"); xf != "" {
		return xf
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}
