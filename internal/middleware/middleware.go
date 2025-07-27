// file: internal/middleware/middleware.go
package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// MiddlewareConfig holds configuration for middleware
type MiddlewareConfig struct {
	Logger     *zap.Logger
	CORSOrigin string
}

// EnhancedLogging middleware with structured logging and correlation IDs
func EnhancedLogging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := GetRequestStart(r.Context())
			requestLogger := GetRequestLogger(r.Context())
			
			// Create a response writer that captures the status code
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			
			// Process the request
			next.ServeHTTP(rw, r)
			
			// Calculate duration
			duration := time.Since(start)
			
			// Log request completion with structured data
			requestLogger.Info("Request completed",
				zap.Int("status", rw.status),
				zap.Duration("duration", duration),
				zap.Int64("response_size", rw.bytesWritten),
			)
			
			// Log slow requests as warnings
			if duration > 2*time.Second {
				requestLogger.Warn("Slow request detected",
					zap.Duration("duration", duration),
					zap.String("threshold", "2s"),
				)
			}
		})
	}
}

// enhancedResponseWriter extends responseWriter to track bytes written
type enhancedResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
}

func (rw *enhancedResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *enhancedResponseWriter) Write(data []byte) (int, error) {
	written, err := rw.ResponseWriter.Write(data)
	rw.bytesWritten += int64(written)
	return written, err
}

// RecoverPanic middleware with enhanced logging
func RecoverPanic(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestLogger := GetRequestLogger(r.Context())
					requestID := GetRequestID(r.Context())
					
					requestLogger.Error("Panic recovered",
						zap.Any("panic", err),
						zap.String("request_id", requestID),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					
					// Return error response
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS middleware with enhanced configuration
func CORS(origin string) func(http.Handler) http.Handler {
	if origin == "" {
		origin = "*"
	}
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Correlation-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Correlation-ID")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// SecureHeaders middleware (unchanged but improved)
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		
		next.ServeHTTP(w, r)
	})
}

// AuthMiddlewareFunc is a legacy authentication middleware that will be replaced by the new AuthMiddleware struct
func AuthMiddlewareFunc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement in MT-04 with proper session store and context
		// For now, just pass through
		next.ServeHTTP(w, r)
	})
}

// Legacy responseWriter for backward compatibility
type responseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	written, err := rw.ResponseWriter.Write(data)
	rw.bytesWritten += int64(written)
	return written, err
}
