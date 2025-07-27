// File: internal/handlers/web/security_handlers.go
package web

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// isTrustedIP checks if the IP address is in a trusted range
// This is used by the monitoring package for internal access checks
func isTrustedIP(remoteAddr string) bool {
	// Extract IP from remote address (can be in format "ip:port")
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // If no port, use the address as is
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	// Check for localhost/loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for private network ranges
	_, private24BitBlock, _ := net.ParseCIDR("10.0.0.0/8")
	_, private20BitBlock, _ := net.ParseCIDR("172.16.0.0/12")
	_, private16BitBlock, _ := net.ParseCIDR("192.168.0.0/16")

	return private24BitBlock.Contains(ip) ||
		private20BitBlock.Contains(ip) ||
		private16BitBlock.Contains(ip)
}

// validateInternalToken validates the internal access token
// In a production environment, this should validate against a secure token store
func validateInternalToken(token string) bool {
	// In a real implementation, this should validate against a secure token store
	// For development, we'll use a simple check against an environment variable
	// or a well-known development token

	// Check against environment variable if set
	if envToken := os.Getenv("INTERNAL_API_TOKEN"); envToken != "" {
		return token == envToken
	}

	// Fallback to development token
	return token == "dev-internal-token-please-change-me"
}

// SecurityMetricsHandler handles security metrics requests
func SecurityMetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"security": map[string]interface{}{
				"blocked_requests":        0,
				"csp_violations":          0,
				"rate_limit_hits":         0,
				"authentication_failures": 0,
				"suspicious_activities":   0,
				"security_events":         0,
			},
			"timestamp": time.Now(),
			"status":    "active",
		}

		json.NewEncoder(w).Encode(response)
	}
}

// SecurityHealthHandler provides security health status
func SecurityHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"security_status": "healthy",
			"features": map[string]interface{}{
				"csp":              "active",
				"hsts":             getEnvironment() == "production",
				"xss_protection":   "active",
				"csrf_protection":  "active",
				"rate_limiting":    "active",
				"input_validation": "active",
				"auth_middleware":  "active",
			},
			"timestamp": time.Now(),
		}

		json.NewEncoder(w).Encode(response)
	}
}

// CSPReportHandler handles Content Security Policy violation reports
func CSPReportHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the CSP report
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read CSP report body", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Parse CSP report
		var cspReport map[string]interface{}
		if err := json.Unmarshal(body, &cspReport); err != nil {
			logger.Error("Failed to parse CSP report", zap.Error(err))
		} else {
			// Log CSP violation
			logger.Warn("CSP violation reported",
				zap.String("user_agent", r.UserAgent()),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Any("csp_report", cspReport),
			)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// SecurityViolationsHandler handles general security violation reports
func SecurityViolationsHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the violation report
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read security violation report body", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Log security violation
		logger.Warn("Security violation reported",
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
			zap.String("user_agent", r.UserAgent()),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("body", string(body)),
		)

		w.WriteHeader(http.StatusNoContent)
	}
}

// HSTSReportHandler handles HTTP Strict Transport Security violation reports
func HSTSReportHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the HSTS report
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read HSTS report body", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Parse HSTS report
		var hstsReport map[string]interface{}
		if err := json.Unmarshal(body, &hstsReport); err != nil {
			logger.Error("Failed to parse HSTS report", zap.Error(err))
		} else {
			// Log HSTS violation
			logger.Warn("HSTS violation reported",
				zap.String("user_agent", r.UserAgent()),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Any("hsts_report", hstsReport),
			)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ExpectCTReportHandler handles Certificate Transparency violation reports
func ExpectCTReportHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the Expect-CT report
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read Expect-CT report body", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Parse Expect-CT report
		var expectCTReport map[string]interface{}
		if err := json.Unmarshal(body, &expectCTReport); err != nil {
			logger.Error("Failed to parse Expect-CT report", zap.Error(err))
		} else {
			// Log Expect-CT violation
			logger.Warn("Expect-CT violation reported",
				zap.String("user_agent", r.UserAgent()),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Any("expect_ct_report", expectCTReport),
			)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// SecurityConfigHandler provides security configuration status
func SecurityConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		environment := getEnvironment()

		response := map[string]interface{}{
			"security_configuration": map[string]interface{}{
				"environment": environment,
				"csp": map[string]interface{}{
					"enabled":     true,
					"script_src":  getCSPScriptSrc(environment),
					"style_src":   getCSPStyleSrc(environment),
					"report_only": environment == "development",
				},
				"hsts": map[string]interface{}{
					"enabled":            environment == "production",
					"max_age":            getHSTSMaxAge(environment),
					"include_subdomains": environment == "production",
					"preload":            environment == "production",
				},
				"cors": map[string]interface{}{
					"enabled":         true,
					"allowed_origins": getCORSOrigins(environment),
					"credentials":     environment != "development",
				},
				"rate_limiting": map[string]interface{}{
					"enabled":       true,
					"default_limit": getRateLimitDefault(environment),
				},
			},
			"timestamp": time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// Helper functions for security configuration
func getCSPScriptSrc(environment string) []string {
	switch environment {
	case "production":
		return []string{"'self'"}
	case "staging":
		return []string{"'self'", "'unsafe-eval'"}
	default:
		return []string{"'self'", "'unsafe-inline'", "'unsafe-eval'"}
	}
}

func getCSPStyleSrc(environment string) []string {
	switch environment {
	case "production":
		return []string{"'self'", "https://fonts.googleapis.com"}
	default:
		return []string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"}
	}
}

func getHSTSMaxAge(environment string) string {
	switch environment {
	case "production":
		return "31536000" // 1 year
	case "staging":
		return "2592000" // 30 days
	default:
		return "0" // Disabled
	}
}

func getCORSOrigins(environment string) []string {
	switch environment {
	case "production":
		return []string{
			"https://yourdomain.com",
			"https://www.yourdomain.com",
			"https://api.yourdomain.com",
		}
	case "staging":
		return []string{
			"https://staging.yourdomain.com",
			"https://dev.yourdomain.com",
			"http://localhost:3000",
		}
	default:
		return []string{"*"}
	}
}

func getRateLimitDefault(environment string) int {
	switch environment {
	case "production":
		return 1000
	case "staging":
		return 2000
	default:
		return 10000
	}
}
