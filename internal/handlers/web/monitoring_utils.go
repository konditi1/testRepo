// File: internal/handlers/web/monitoring_utils.go
package web

import (
	"net/http"
	"os"
	"strings"
)

// Helper function to get environment
func getEnvironment() string {
	env := os.Getenv("GO_ENV")
	if env == "" {
		return "development"
	}
	return env
}

// Helper function to get logging level
func getLoggingLevel() string {
	switch getEnvironment() {
	case "production":
		return "info"
	case "staging":
		return "debug"
	default:
		return "debug"
	}
}

// IsAuthorizedForInternalAccess checks if the request is authorized for internal access
func IsAuthorizedForInternalAccess(r *http.Request) bool {
	// In development, allow all requests
	if getEnvironment() != "production" {
		return true
	}

	// Check for internal IPs
	ip := getClientIP(r)
	allowedIPs := getInternalAllowedIPs()
	for _, allowedIP := range allowedIPs {
		if ip == allowedIP {
			return true
		}
	}

	// Check for internal user agents
	userAgent := r.UserAgent()
	for _, ua := range getInternalUserAgents() {
		if strings.Contains(strings.ToLower(userAgent), strings.ToLower(ua)) {
			return true
		}
	}

	return false
}

// getInternalAllowedIPs returns the list of allowed IPs for internal access
func getInternalAllowedIPs() []string {
	// In a real application, this would come from configuration
	return []string{
		"127.0.0.1",
		"::1",
		// Add other internal IPs as needed
	}
}

// getInternalUserAgents returns the list of recognized internal service user agents
func getInternalUserAgents() []string {
	return []string{
		"kube-probe",
		"GoogleHC",
		// Add other internal service user agents as needed
	}
}

// isInternalRoute checks if a route is an internal monitoring route
func isInternalRoute(path string) bool {
	internalRoutes := []string{
		"/health",
		"/metrics",
		"/debug",
		"/internal",
	}

	for _, route := range internalRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}

// getClientIP gets the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header first
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can be a comma-separated list of IPs
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}

// sanitizeUserAgent removes potentially sensitive information from user agent
func sanitizeUserAgent(userAgent string) string {
	// Basic sanitization - you can enhance this based on your needs
	if len(userAgent) > 200 {
		return userAgent[:200] + "..."
	}
	return userAgent
}

// sanitizeIP removes potentially sensitive IP information for logging
func sanitizeIP(ip string) string {
	// For privacy, you might want to mask part of the IP
	// This is a basic example - adjust based on your privacy requirements
	if strings.Contains(ip, ":") {
		// IPv6 - mask last 64 bits
		parts := strings.Split(ip, ":")
		if len(parts) > 4 {
			for i := len(parts) - 4; i < len(parts); i++ {
				parts[i] = "xxxx"
			}
			return strings.Join(parts, ":")
		}
	} else if strings.Contains(ip, ".") {
		// IPv4 - mask last octet
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			parts[3] = "xxx"
			return strings.Join(parts, ".")
		}
	}
	return ip
}

// validateContentType checks if the content type is acceptable for monitoring endpoints
func validateContentType(r *http.Request, acceptedTypes []string) bool {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return true // No content type specified, accept by default
	}

	for _, t := range acceptedTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}

	return false
}

// setSecurityHeaders sets appropriate security headers for monitoring endpoints
func setSecurityHeaders(w http.ResponseWriter, environment string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	if environment == "production" {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

// createErrorResponse creates a standardized error response for monitoring endpoints
func createErrorResponse(code int, message string) map[string]interface{} {
	return map[string]interface{}{
		"status":  "error",
		"code":    code,
		"message": message,
	}
}

// createSuccessResponse creates a standardized success response wrapper
func createSuccessResponse(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"status": "success",
		"data":   data,
	}
}

// isHealthCheckEndpoint determines if the request is for a health check endpoint
func isHealthCheckEndpoint(path string) bool {
	healthEndpoints := []string{
		"/health",
		"/healthz",
		"/ready",
		"/readyz",
		"/live",
		"/livez",
	}

	for _, endpoint := range healthEndpoints {
		if path == endpoint {
			return true
		}
	}

	return false
}

// shouldAllowPublicAccess determines if an endpoint should allow public access
func shouldAllowPublicAccess(path string) bool {
	publicEndpoints := []string{
		"/health",
		"/metrics",
		"/version",
	}

	for _, endpoint := range publicEndpoints {
		if path == endpoint {
			return true
		}
	}

	return false
}

// getAuthorizationLevel returns the authorization level required for a path
func getAuthorizationLevel(path string) string {
	switch {
	case strings.HasPrefix(path, "/internal"):
		return "internal"
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/api"):
		return "user"
	default:
		return "public"
	}
}

// validateRequestMethod validates if the HTTP method is allowed for the endpoint
func validateRequestMethod(r *http.Request, allowedMethods []string) bool {
	for _, method := range allowedMethods {
		if r.Method == method {
			return true
		}
	}
	return false
}

// getRequestFingerprint creates a unique fingerprint for the request (for rate limiting, etc.)
func getRequestFingerprint(r *http.Request) string {
	ip := getClientIP(r)
	path := r.URL.Path
	method := r.Method
	return ip + "|" + method + "|" + path
}
