// File: internal/middleware/security.go
package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ===============================
// SECURITY CONFIGURATION
// ===============================

// SecurityConfig holds comprehensive security configuration
type SecurityConfig struct {
	// Content Security Policy
	EnableCSP     bool     `json:"enable_csp"`
	CSPDefaultSrc []string `json:"csp_default_src"`
	CSPScriptSrc  []string `json:"csp_script_src"`
	CSPStyleSrc   []string `json:"csp_style_src"`
	CSPImgSrc     []string `json:"csp_img_src"`
	CSPConnectSrc []string `json:"csp_connect_src"`
	CSPFontSrc    []string `json:"csp_font_src"`
	CSPObjectSrc  []string `json:"csp_object_src"`
	CSPMediaSrc   []string `json:"csp_media_src"`
	CSPFrameSrc   []string `json:"csp_frame_src"`
	CSPWorkerSrc  []string `json:"csp_worker_src"`
	CSPReportURI  string   `json:"csp_report_uri"`
	CSPReportOnly bool     `json:"csp_report_only"`

	// HTTP Strict Transport Security
	EnableHSTS            bool          `json:"enable_hsts"`
	HSTSMaxAge            time.Duration `json:"hsts_max_age"`
	HSTSIncludeSubdomains bool          `json:"hsts_include_subdomains"`
	HSTSPreload           bool          `json:"hsts_preload"`

	// Frame Options
	FrameOptions string `json:"frame_options"` // DENY, SAMEORIGIN, ALLOW-FROM uri

	// Content Type Options
	EnableContentTypeNosniff bool `json:"enable_content_type_nosniff"`

	// XSS Protection
	EnableXSSProtection bool   `json:"enable_xss_protection"`
	XSSProtectionMode   string `json:"xss_protection_mode"` // block, report

	// Referrer Policy
	ReferrerPolicy string `json:"referrer_policy"`

	// Permissions Policy (formerly Feature Policy)
	EnablePermissionsPolicy bool              `json:"enable_permissions_policy"`
	PermissionsPolicies     map[string]string `json:"permissions_policies"`

	// Cross-Origin Policies
	EnableCORP bool   `json:"enable_corp"` // Cross-Origin Resource Policy
	CORPPolicy string `json:"corp_policy"` // same-site, same-origin, cross-origin
	EnableCOEP bool   `json:"enable_coep"` // Cross-Origin Embedder Policy
	COEPPolicy string `json:"coep_policy"` // unsafe-none, require-corp

	// Security Monitoring
	EnableSecurityReporting bool   `json:"enable_security_reporting"`
	SecurityReportEndpoint  string `json:"security_report_endpoint"`
	LogSecurityViolations   bool   `json:"log_security_violations"`

	// Environment Settings
	IsDevelopment bool `json:"is_development"`
	IsProduction  bool `json:"is_production"`

	// Custom Headers
	CustomHeaders map[string]string `json:"custom_headers"`
}

// DefaultSecurityConfig returns production-ready security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		// CSP Settings
		EnableCSP:     true,
		CSPDefaultSrc: []string{"'self'"},
		CSPScriptSrc:  []string{"'self'", "'unsafe-inline'"},
		CSPStyleSrc:   []string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"},
		CSPImgSrc:     []string{"'self'", "data:", "https:"},
		CSPConnectSrc: []string{"'self'"},
		CSPFontSrc:    []string{"'self'", "https://fonts.gstatic.com"},
		CSPObjectSrc:  []string{"'none'"},
		CSPMediaSrc:   []string{"'self'"},
		CSPFrameSrc:   []string{"'none'"},
		CSPWorkerSrc:  []string{"'self'"},
		CSPReportURI:  "/api/security/csp-report",
		CSPReportOnly: false,

		// HSTS Settings
		EnableHSTS:            true,
		HSTSMaxAge:            365 * 24 * time.Hour, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,

		// Frame Options
		FrameOptions: "DENY",

		// Content Type Protection
		EnableContentTypeNosniff: true,

		// XSS Protection
		EnableXSSProtection: true,
		XSSProtectionMode:   "block",

		// Referrer Policy
		ReferrerPolicy: "strict-origin-when-cross-origin",

		// Permissions Policy
		EnablePermissionsPolicy: true,
		PermissionsPolicies: map[string]string{
			"camera":      "()",
			"microphone":  "()",
			"geolocation": "()",
			"payment":     "()",
			"usb":         "()",
		},

		// Cross-Origin Policies
		EnableCORP: true,
		CORPPolicy: "same-origin",
		EnableCOEP: false, // Can break some functionality
		COEPPolicy: "unsafe-none",

		// Security Monitoring
		EnableSecurityReporting: true,
		SecurityReportEndpoint:  "/api/security/violations",
		LogSecurityViolations:   true,

		// Environment
		IsProduction: true,

		// Custom Headers
		CustomHeaders: make(map[string]string),
	}
}

// DevelopmentSecurityConfig returns development-friendly security configuration
func DevelopmentSecurityConfig() *SecurityConfig {
	config := DefaultSecurityConfig()

	// Relax CSP for development
	config.CSPScriptSrc = append(config.CSPScriptSrc, "'unsafe-eval'")
	config.CSPConnectSrc = append(config.CSPConnectSrc, "ws:", "wss:")
	config.CSPReportOnly = true

	// Disable HSTS in development
	config.EnableHSTS = false

	// Allow frames in development (for dev tools)
	config.FrameOptions = "SAMEORIGIN"

	// Environment settings
	config.IsDevelopment = true
	config.IsProduction = false

	return config
}

// ===============================
// ENHANCED SECURITY MIDDLEWARE
// ===============================

// EnhancedSecurity creates comprehensive security headers middleware
func EnhancedSecurity(config *SecurityConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestLogger := GetRequestLogger(r.Context())
			requestID := GetRequestID(r.Context())

			// Apply security headers
			applySecurityHeaders(w, r, config, requestLogger)

			// Log security events if monitoring is enabled
			if config.EnableSecurityReporting {
				logSecurityEvent(r, config, requestLogger, requestID)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ===============================
// SECURITY HEADERS APPLICATION
// ===============================

// applySecurityHeaders applies all configured security headers
func applySecurityHeaders(w http.ResponseWriter, r *http.Request, config *SecurityConfig, logger *zap.Logger) {
	// Content Security Policy
	if config.EnableCSP {
		csp := buildCSP(config)
		if config.CSPReportOnly {
			w.Header().Set("Content-Security-Policy-Report-Only", csp)
		} else {
			w.Header().Set("Content-Security-Policy", csp)
		}
	}

	// HTTP Strict Transport Security
	if config.EnableHSTS && r.TLS != nil {
		hsts := buildHSTS(config)
		w.Header().Set("Strict-Transport-Security", hsts)
	}

	// Frame Options
	if config.FrameOptions != "" {
		w.Header().Set("X-Frame-Options", config.FrameOptions)
	}

	// Content Type Options
	if config.EnableContentTypeNosniff {
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}

	// XSS Protection
	if config.EnableXSSProtection {
		xss := buildXSSProtection(config)
		w.Header().Set("X-XSS-Protection", xss)
	}

	// Referrer Policy
	if config.ReferrerPolicy != "" {
		w.Header().Set("Referrer-Policy", config.ReferrerPolicy)
	}

	// Permissions Policy
	if config.EnablePermissionsPolicy && len(config.PermissionsPolicies) > 0 {
		permissions := buildPermissionsPolicy(config)
		w.Header().Set("Permissions-Policy", permissions)
	}

	// Cross-Origin Resource Policy
	if config.EnableCORP {
		w.Header().Set("Cross-Origin-Resource-Policy", config.CORPPolicy)
	}

	// Cross-Origin Embedder Policy
	if config.EnableCOEP {
		w.Header().Set("Cross-Origin-Embedder-Policy", config.COEPPolicy)
	}

	// Custom headers
	for key, value := range config.CustomHeaders {
		w.Header().Set(key, value)
	}

	// Security reporting endpoint
	if config.EnableSecurityReporting && config.SecurityReportEndpoint != "" {
		w.Header().Set("Report-To", buildReportTo(config))
	}
}

// ===============================
// HEADER BUILDERS
// ===============================

// buildCSP constructs the Content Security Policy header
func buildCSP(config *SecurityConfig) string {
	var policies []string

	if len(config.CSPDefaultSrc) > 0 {
		policies = append(policies, fmt.Sprintf("default-src %s", strings.Join(config.CSPDefaultSrc, " ")))
	}
	if len(config.CSPScriptSrc) > 0 {
		policies = append(policies, fmt.Sprintf("script-src %s", strings.Join(config.CSPScriptSrc, " ")))
	}
	if len(config.CSPStyleSrc) > 0 {
		policies = append(policies, fmt.Sprintf("style-src %s", strings.Join(config.CSPStyleSrc, " ")))
	}
	if len(config.CSPImgSrc) > 0 {
		policies = append(policies, fmt.Sprintf("img-src %s", strings.Join(config.CSPImgSrc, " ")))
	}
	if len(config.CSPConnectSrc) > 0 {
		policies = append(policies, fmt.Sprintf("connect-src %s", strings.Join(config.CSPConnectSrc, " ")))
	}
	if len(config.CSPFontSrc) > 0 {
		policies = append(policies, fmt.Sprintf("font-src %s", strings.Join(config.CSPFontSrc, " ")))
	}
	if len(config.CSPObjectSrc) > 0 {
		policies = append(policies, fmt.Sprintf("object-src %s", strings.Join(config.CSPObjectSrc, " ")))
	}
	if len(config.CSPMediaSrc) > 0 {
		policies = append(policies, fmt.Sprintf("media-src %s", strings.Join(config.CSPMediaSrc, " ")))
	}
	if len(config.CSPFrameSrc) > 0 {
		policies = append(policies, fmt.Sprintf("frame-src %s", strings.Join(config.CSPFrameSrc, " ")))
	}
	if len(config.CSPWorkerSrc) > 0 {
		policies = append(policies, fmt.Sprintf("worker-src %s", strings.Join(config.CSPWorkerSrc, " ")))
	}

	// Add report URI if configured
	if config.CSPReportURI != "" {
		policies = append(policies, fmt.Sprintf("report-uri %s", config.CSPReportURI))
	}

	return strings.Join(policies, "; ")
}

// buildHSTS constructs the HTTP Strict Transport Security header
func buildHSTS(config *SecurityConfig) string {
	hsts := fmt.Sprintf("max-age=%d", int(config.HSTSMaxAge.Seconds()))

	if config.HSTSIncludeSubdomains {
		hsts += "; includeSubDomains"
	}

	if config.HSTSPreload {
		hsts += "; preload"
	}

	return hsts
}

// buildXSSProtection constructs the X-XSS-Protection header
func buildXSSProtection(config *SecurityConfig) string {
	if config.XSSProtectionMode == "block" {
		return "1; mode=block"
	}
	return "1"
}

// buildPermissionsPolicy constructs the Permissions-Policy header
func buildPermissionsPolicy(config *SecurityConfig) string {
	var policies []string

	for directive, allowlist := range config.PermissionsPolicies {
		policies = append(policies, fmt.Sprintf("%s=%s", directive, allowlist))
	}

	return strings.Join(policies, ", ")
}

// buildReportTo constructs the Report-To header
func buildReportTo(config *SecurityConfig) string {
	return fmt.Sprintf(`{"group":"default","max_age":31536000,"endpoints":[{"url":"%s"}]}`, config.SecurityReportEndpoint)
}

// ===============================
// ENHANCED CORS MIDDLEWARE
// ===============================

// CORSConfig holds CORS configuration
type CORSConfig struct {
	// Origins
	AllowedOrigins        []string `json:"allowed_origins"`
	AllowedOriginPatterns []string `json:"allowed_origin_patterns"`
	AllowCredentials      bool     `json:"allow_credentials"`

	// Methods and Headers
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	ExposedHeaders []string `json:"exposed_headers"`

	// Preflight
	MaxAge              time.Duration `json:"max_age"`
	AllowPrivateNetwork bool          `json:"allow_private_network"`

	// Security
	VaryOrigin        bool `json:"vary_origin"`
	LogCORSViolations bool `json:"log_cors_violations"`

	// Development
	EnableInDevelopment bool `json:"enable_in_development"`
}

// DefaultCORSConfig returns production-ready CORS configuration
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins:        []string{}, // Configure specific origins for production
		AllowedOriginPatterns: []string{},
		AllowCredentials:      true,
		AllowedMethods: []string{
			"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS",
		},
		AllowedHeaders: []string{
			"Accept", "Accept-Language", "Content-Language", "Content-Type",
			"Authorization", "X-Request-ID", "X-Correlation-ID", "X-CSRF-Token",
		},
		ExposedHeaders: []string{
			"X-Request-ID", "X-Correlation-ID", "X-RateLimit-Limit",
			"X-RateLimit-Remaining", "X-RateLimit-Reset",
		},
		MaxAge:              24 * time.Hour,
		AllowPrivateNetwork: false,
		VaryOrigin:          true,
		LogCORSViolations:   true,
		EnableInDevelopment: true,
	}
}

// DevelopmentCORSConfig returns development-friendly CORS configuration
func DevelopmentCORSConfig() *CORSConfig {
	config := DefaultCORSConfig()
	config.AllowedOrigins = []string{"*"} // Allow all origins in development
	config.AllowPrivateNetwork = true
	return config
}

// EnhancedCORS creates sophisticated CORS middleware
func EnhancedCORS(config *CORSConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestLogger := GetRequestLogger(r.Context())
			origin := r.Header.Get("Origin")

			// Check if CORS should be applied
			if origin == "" && r.Method != "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Validate origin
			if !isOriginAllowed(origin, config) {
				if config.LogCORSViolations {
					requestLogger.Warn("CORS violation: origin not allowed",
						zap.String("origin", origin),
						zap.String("request_id", GetRequestID(r.Context())),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
				}

				// For security, don't expose that the origin is not allowed
				// Just don't add CORS headers
				next.ServeHTTP(w, r)
				return
			}

			// Apply CORS headers
			applyCORSHeaders(w, r, origin, config)

			// Handle preflight OPTIONS request
			if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
				handlePreflightRequest(w, r, config, requestLogger)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ===============================
// CORS HELPERS
// ===============================

// isOriginAllowed checks if the origin is allowed
func isOriginAllowed(origin string, config *CORSConfig) bool {
	if origin == "" {
		return true
	}

	// Check wildcard
	for _, allowed := range config.AllowedOrigins {
		if allowed == "*" {
			return true
		}
		if allowed == origin {
			return true
		}
	}

	// Check patterns (basic wildcard matching)
	for _, pattern := range config.AllowedOriginPatterns {
		if matchOriginPattern(origin, pattern) {
			return true
		}
	}

	return false
}

// matchOriginPattern performs basic wildcard matching for origin patterns
func matchOriginPattern(origin, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// Simple wildcard matching for subdomains
	if strings.HasPrefix(pattern, "*.") {
		domain := pattern[2:]
		return strings.HasSuffix(origin, "."+domain) || origin == domain
	}

	return origin == pattern
}

// applyCORSHeaders applies CORS headers to the response
func applyCORSHeaders(w http.ResponseWriter, r *http.Request, origin string, config *CORSConfig) {
	// Set origin
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	// Set credentials
	if config.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Set exposed headers
	if len(config.ExposedHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
	}

	// Vary Origin for security
	if config.VaryOrigin {
		w.Header().Add("Vary", "Origin")
	}
}

// handlePreflightRequest handles CORS preflight OPTIONS requests
func handlePreflightRequest(w http.ResponseWriter, r *http.Request, config *CORSConfig, logger *zap.Logger) {
	requestMethod := r.Header.Get("Access-Control-Request-Method")
	requestHeaders := r.Header.Get("Access-Control-Request-Headers")

	// Validate method
	if !isMethodAllowed(requestMethod, config.AllowedMethods) {
		logger.Warn("CORS preflight: method not allowed",
			zap.String("method", requestMethod),
			zap.String("origin", r.Header.Get("Origin")),
		)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Validate headers
	if requestHeaders != "" && !areHeadersAllowed(requestHeaders, config.AllowedHeaders) {
		logger.Warn("CORS preflight: headers not allowed",
			zap.String("headers", requestHeaders),
			zap.String("origin", r.Header.Get("Origin")),
		)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Set preflight response headers
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
	w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%.0f", config.MaxAge.Seconds()))

	// Handle private network access
	if config.AllowPrivateNetwork && r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
	}

	w.WriteHeader(http.StatusNoContent)
}

// isMethodAllowed checks if the HTTP method is allowed
func isMethodAllowed(method string, allowedMethods []string) bool {
	for _, allowed := range allowedMethods {
		if strings.EqualFold(method, allowed) {
			return true
		}
	}
	return false
}

// areHeadersAllowed checks if the request headers are allowed
func areHeadersAllowed(requestHeaders string, allowedHeaders []string) bool {
	headers := strings.Split(requestHeaders, ",")

	for _, header := range headers {
		header = strings.TrimSpace(header)
		if !isHeaderAllowed(header, allowedHeaders) {
			return false
		}
	}

	return true
}

// isHeaderAllowed checks if a single header is allowed
func isHeaderAllowed(header string, allowedHeaders []string) bool {
	header = strings.ToLower(header)

	// Some headers are always allowed by CORS spec
	simpleHeaders := []string{
		"accept", "accept-language", "content-language", "content-type",
	}

	for _, simple := range simpleHeaders {
		if header == simple {
			return true
		}
	}

	// Check against allowed headers
	for _, allowed := range allowedHeaders {
		if strings.EqualFold(header, allowed) {
			return true
		}
	}

	return false
}

// ===============================
// SECURITY MONITORING
// ===============================

// logSecurityEvent logs security-related events
func logSecurityEvent(r *http.Request, config *SecurityConfig, logger *zap.Logger, requestID string) {
	// Log suspicious patterns
	if containsSuspiciousHeaders(r) {
		logger.Warn("Suspicious request headers detected",
			zap.String("event", "suspicious_headers"),
			zap.String("request_id", requestID),
			zap.String("user_agent", r.UserAgent()),
			zap.String("remote_addr", getClientIP(r)),
		)
	}

	// Log potential XSS attempts
	if containsXSSAttempt(r) {
		logger.Warn("Potential XSS attempt detected",
			zap.String("event", "xss_attempt"),
			zap.String("request_id", requestID),
			zap.String("url", r.URL.String()),
			zap.String("remote_addr", getClientIP(r)),
		)
	}

	// Log CSP violations (this would be called by a separate violation handler)
	// logCSPViolation would be called by your CSP report endpoint
}

// containsSuspiciousHeaders checks for suspicious request headers
func containsSuspiciousHeaders(r *http.Request) bool {
	suspiciousHeaders := []string{
		"X-Forwarded-Host", "X-Original-URL", "X-Rewrite-URL",
	}

	for _, header := range suspiciousHeaders {
		if r.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

// containsXSSAttempt checks for potential XSS attempts
func containsXSSAttempt(r *http.Request) bool {
	xssPatterns := []string{
		"<script", "javascript:", "vbscript:", "onload=", "onerror=",
	}

	// Check URL and query parameters
	fullURL := r.URL.String()
	for _, pattern := range xssPatterns {
		if strings.Contains(strings.ToLower(fullURL), pattern) {
			return true
		}
	}

	return false
}

// ===============================
// SECURITY REPORT HANDLERS
// ===============================

// CSPViolationReport represents a CSP violation report
type CSPViolationReport struct {
	DocumentURI        string `json:"document-uri"`
	Referrer           string `json:"referrer"`
	ViolatedDirective  string `json:"violated-directive"`
	EffectiveDirective string `json:"effective-directive"`
	OriginalPolicy     string `json:"original-policy"`
	BlockedURI         string `json:"blocked-uri"`
	StatusCode         int    `json:"status-code"`
	ScriptSample       string `json:"script-sample"`
}

// CreateCSPReportHandler creates a handler for CSP violation reports
func CreateCSPReportHandler(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var report struct {
			CSPReport CSPViolationReport `json:"csp-report"`
		}

		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			logger.Error("Failed to parse CSP violation report", zap.Error(err))
			http.Error(w, "Invalid report", http.StatusBadRequest)
			return
		}

		// Log CSP violation
		logger.Warn("CSP violation reported",
			zap.String("event", "csp_violation"),
			zap.String("document_uri", report.CSPReport.DocumentURI),
			zap.String("violated_directive", report.CSPReport.ViolatedDirective),
			zap.String("blocked_uri", report.CSPReport.BlockedURI),
			zap.String("user_agent", r.UserAgent()),
			zap.String("remote_addr", getClientIP(r)),
		)

		w.WriteHeader(http.StatusNoContent)
	}
}

// ===============================
// INTEGRATION HELPERS
// ===============================

// CreateSecurityMiddlewareStack creates complete security middleware stack
func CreateSecurityMiddlewareStack(securityConfig *SecurityConfig, corsConfig *CORSConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Stack security middleware
		handler := next
		handler = EnhancedSecurity(securityConfig, logger)(handler)
		handler = EnhancedCORS(corsConfig, logger)(handler)
		return handler
	}
}

// ReplaceBasicSecurity replaces the existing basic security middleware
func ReplaceBasicSecurity(environment string, logger *zap.Logger) func(http.Handler) http.Handler {
	var securityConfig *SecurityConfig
	var corsConfig *CORSConfig

	if environment == "development" {
		securityConfig = DevelopmentSecurityConfig()
		corsConfig = DevelopmentCORSConfig()
	} else {
		securityConfig = DefaultSecurityConfig()
		corsConfig = DefaultCORSConfig()

		// Configure production-specific origins
		corsConfig.AllowedOrigins = []string{
			"https://yourdomain.com",
			"https://www.yourdomain.com",
			// Add your production domains
		}
	}

	return CreateSecurityMiddlewareStack(securityConfig, corsConfig, logger)
}
