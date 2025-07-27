// File: internal/response/status.go
package response

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ===============================
// HTTP STATUS CODE CONSTANTS
// ===============================

// Standard HTTP status codes with semantic meaning
const (
	// Success codes
	StatusOK             = http.StatusOK             // 200
	StatusCreated        = http.StatusCreated        // 201
	StatusAccepted       = http.StatusAccepted       // 202
	StatusNoContent      = http.StatusNoContent      // 204
	StatusPartialContent = http.StatusPartialContent // 206

	// Redirect codes
	StatusMovedPermanently  = http.StatusMovedPermanently  // 301
	StatusFound             = http.StatusFound             // 302
	StatusSeeOther          = http.StatusSeeOther          // 303
	StatusNotModified       = http.StatusNotModified       // 304
	StatusTemporaryRedirect = http.StatusTemporaryRedirect // 307
	StatusPermanentRedirect = http.StatusPermanentRedirect // 308

	// Client error codes
	StatusBadRequest            = http.StatusBadRequest            // 400
	StatusUnauthorized          = http.StatusUnauthorized          // 401
	StatusPaymentRequired       = http.StatusPaymentRequired       // 402
	StatusForbidden             = http.StatusForbidden             // 403
	StatusNotFound              = http.StatusNotFound              // 404
	StatusMethodNotAllowed      = http.StatusMethodNotAllowed      // 405
	StatusNotAcceptable         = http.StatusNotAcceptable         // 406
	StatusRequestTimeout        = http.StatusRequestTimeout        // 408
	StatusConflict              = http.StatusConflict              // 409
	StatusGone                  = http.StatusGone                  // 410
	StatusLengthRequired        = http.StatusLengthRequired        // 411
	StatusPreconditionFailed    = http.StatusPreconditionFailed    // 412
	StatusRequestEntityTooLarge = http.StatusRequestEntityTooLarge // 413
	StatusUnsupportedMediaType  = http.StatusUnsupportedMediaType  // 415
	StatusUnprocessableEntity   = http.StatusUnprocessableEntity   // 422
	StatusTooManyRequests       = http.StatusTooManyRequests       // 429

	// Server error codes
	StatusInternalServerError = http.StatusInternalServerError // 500
	StatusNotImplemented      = http.StatusNotImplemented      // 501
	StatusBadGateway          = http.StatusBadGateway          // 502
	StatusServiceUnavailable  = http.StatusServiceUnavailable  // 503
	StatusGatewayTimeout      = http.StatusGatewayTimeout      // 504
)

// ===============================
// STATUS CODE MAPPING
// ===============================

// StatusCodeMap maps error types to HTTP status codes
var StatusCodeMap = map[string]int{
	"VALIDATION_ERROR":     StatusBadRequest,
	"AUTHENTICATION_ERROR": StatusUnauthorized,
	"AUTHORIZATION_ERROR":  StatusForbidden,
	"NOT_FOUND":            StatusNotFound,
	"CONFLICT":             StatusConflict,
	"BUSINESS_ERROR":       StatusUnprocessableEntity,
	"RATE_LIMIT":           StatusTooManyRequests,
	"INTERNAL_ERROR":       StatusInternalServerError,
	"NOT_IMPLEMENTED":      StatusNotImplemented,
	"SERVICE_UNAVAILABLE":  StatusServiceUnavailable,
}

// ===============================
// STATUS CODE HELPERS
// ===============================

// GetStatusCodeFromErrorType returns appropriate HTTP status code for error type
func GetStatusCodeFromErrorType(errorType string) int {
	if code, exists := StatusCodeMap[errorType]; exists {
		return code
	}
	return StatusInternalServerError
}

// IsSuccessStatus checks if status code indicates success (2xx)
func IsSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}

// IsClientError checks if status code indicates client error (4xx)
func IsClientError(code int) bool {
	return code >= 400 && code < 500
}

// IsServerError checks if status code indicates server error (5xx)
func IsServerError(code int) bool {
	return code >= 500 && code < 600
}

// IsRedirect checks if status code indicates redirect (3xx)
func IsRedirect(code int) bool {
	return code >= 300 && code < 400
}

// GetStatusText returns human-readable status text
func GetStatusText(code int) string {
	return http.StatusText(code)
}

// ===============================
// RESPONSE STATUS MANAGER
// ===============================

// StatusManager helps manage HTTP status codes and responses
type StatusManager struct {
	customMessages map[int]string
}

// NewStatusManager creates a new status manager
func NewStatusManager() *StatusManager {
	return &StatusManager{
		customMessages: make(map[int]string),
	}
}

// SetCustomMessage sets a custom message for a status code
func (sm *StatusManager) SetCustomMessage(code int, message string) {
	sm.customMessages[code] = message
}

// GetMessage returns the message for a status code
func (sm *StatusManager) GetMessage(code int) string {
	if custom, exists := sm.customMessages[code]; exists {
		return custom
	}
	return GetStatusText(code)
}

// ===============================
// STATUS RESPONSE BUILDERS
// ===============================

// StatusResponse represents a response with just status information
type StatusResponse struct {
	Status    int    `json:"status"`
	Message   string `json:"message"`
	Success   bool   `json:"success"`
	RequestID string `json:"request_id,omitempty"`
}

// BuildStatusResponse creates a status-only response
func (b *Builder) BuildStatusResponse(code int, message string, requestID string) *StatusResponse {
	if message == "" {
		message = GetStatusText(code)
	}

	return &StatusResponse{
		Status:    code,
		Message:   message,
		Success:   IsSuccessStatus(code),
		RequestID: requestID,
	}
}

// WriteStatusResponse writes a status-only response
func (b *Builder) WriteStatusResponse(w http.ResponseWriter, r *http.Request, code int, message string) {
	statusResp := b.BuildStatusResponse(code, message, b.getRequestID(r.Context()))

	// Convert StatusResponse to APIResponse
	apiResp := &APIResponse{
		Success:   statusResp.Success,
		Data:      nil, // No data in status response
		Error:     nil, // No error in status response
		RequestID: statusResp.RequestID,
		Timestamp: time.Now().Unix(),
	}

	b.WriteJSON(w, r, apiResp, code)
}

// ===============================
// SPECIALIZED STATUS RESPONSES
// ===============================

// WriteAccepted writes an accepted response (202)
func (b *Builder) WriteAccepted(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Request accepted for processing"
	}
	b.WriteStatusResponse(w, r, StatusAccepted, message)
}

// WritePartialContent writes a partial content response (206)
func (b *Builder) WritePartialContent(w http.ResponseWriter, r *http.Request, data interface{}, contentRange string) {
	w.Header().Set("Content-Range", contentRange)
	response := b.Success(r.Context(), data)
	b.WriteJSON(w, r, response, StatusPartialContent)
}

// WriteBadRequest writes a bad request response (400)
func (b *Builder) WriteBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Bad request"
	}
	b.WriteStatusResponse(w, r, StatusBadRequest, message)
}

// WriteUnauthorized writes an unauthorized response (401)
func (b *Builder) WriteUnauthorized(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Authentication required"
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	b.WriteStatusResponse(w, r, StatusUnauthorized, message)
}

// WriteForbidden writes a forbidden response (403)
func (b *Builder) WriteForbidden(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Access forbidden"
	}
	b.WriteStatusResponse(w, r, StatusForbidden, message)
}

// WriteNotFound writes a not found response (404)
func (b *Builder) WriteNotFound(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Resource not found"
	}
	b.WriteStatusResponse(w, r, StatusNotFound, message)
}

// WriteMethodNotAllowed writes a method not allowed response (405)
func (b *Builder) WriteMethodNotAllowed(w http.ResponseWriter, r *http.Request, allowedMethods []string) {
	if len(allowedMethods) > 0 {
		w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
	}
	b.WriteStatusResponse(w, r, StatusMethodNotAllowed, "Method not allowed")
}

// WriteConflict writes a conflict response (409)
func (b *Builder) WriteConflict(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Resource conflict"
	}
	b.WriteStatusResponse(w, r, StatusConflict, message)
}

// WriteUnprocessableEntity writes an unprocessable entity response (422)
func (b *Builder) WriteUnprocessableEntity(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Unprocessable entity"
	}
	b.WriteStatusResponse(w, r, StatusUnprocessableEntity, message)
}

// WriteTooManyRequests writes a rate limit response (429)
func (b *Builder) WriteTooManyRequests(w http.ResponseWriter, r *http.Request, retryAfter string) {
	if retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	b.WriteStatusResponse(w, r, StatusTooManyRequests, "Rate limit exceeded")
}

// WriteInternalServerError writes an internal server error response (500)
func (b *Builder) WriteInternalServerError(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Internal server error"
	}
	b.WriteStatusResponse(w, r, StatusInternalServerError, message)
}

// WriteNotImplemented writes a not implemented response (501)
func (b *Builder) WriteNotImplemented(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Not implemented"
	}
	b.WriteStatusResponse(w, r, StatusNotImplemented, message)
}

// WriteServiceUnavailable writes a service unavailable response (503)
func (b *Builder) WriteServiceUnavailable(w http.ResponseWriter, r *http.Request, retryAfter string) {
	if retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	b.WriteStatusResponse(w, r, StatusServiceUnavailable, "Service temporarily unavailable")
}

// ===============================
// REDIRECT HELPERS
// ===============================

// WriteRedirect writes a redirect response
func (b *Builder) WriteRedirect(w http.ResponseWriter, r *http.Request, url string, code int) {
	// Validate redirect code
	if !IsRedirect(code) {
		code = StatusFound // Default to 302
	}

	http.Redirect(w, r, url, code)
}

// WriteTemporaryRedirect writes a temporary redirect (307)
func (b *Builder) WriteTemporaryRedirect(w http.ResponseWriter, r *http.Request, url string) {
	b.WriteRedirect(w, r, url, StatusTemporaryRedirect)
}

// WritePermanentRedirect writes a permanent redirect (308)
func (b *Builder) WritePermanentRedirect(w http.ResponseWriter, r *http.Request, url string) {
	b.WriteRedirect(w, r, url, StatusPermanentRedirect)
}

// WriteSeeOther writes a see other redirect (303)
func (b *Builder) WriteSeeOther(w http.ResponseWriter, r *http.Request, url string) {
	b.WriteRedirect(w, r, url, StatusSeeOther)
}

// ===============================
// HEALTH CHECK RESPONSES
// ===============================

// HealthStatus represents system health status
type HealthStatus struct {
	Status      string                 `json:"status"`
	Timestamp   int64                  `json:"timestamp"`
	Version     string                 `json:"version,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Uptime      float64                `json:"uptime_seconds,omitempty"`
	Services    map[string]interface{} `json:"services,omitempty"`
}

// WriteHealthCheck writes a health check response
func (b *Builder) WriteHealthCheck(w http.ResponseWriter, r *http.Request, health *HealthStatus) {
	code := StatusOK
	if health.Status != "healthy" {
		code = StatusServiceUnavailable
	}

	response := b.Success(r.Context(), health)
	b.WriteJSON(w, r, response, code)
}

// ===============================
// CONTENT NEGOTIATION
// ===============================

// ContentType represents supported content types
type ContentType string

const (
	ContentTypeJSON = "application/json"
	ContentTypeXML  = "application/xml"
	ContentTypeHTML = "text/html"
	ContentTypeText = "text/plain"
)

// GetPreferredContentType determines preferred content type from Accept header
func GetPreferredContentType(r *http.Request) ContentType {
	accept := r.Header.Get("Accept")

	if strings.Contains(accept, "application/json") {
		return ContentTypeJSON
	}
	if strings.Contains(accept, "text/html") {
		return ContentTypeHTML
	}
	if strings.Contains(accept, "application/xml") {
		return ContentTypeXML
	}
	if strings.Contains(accept, "text/plain") {
		return ContentTypeText
	}

	// Default to JSON for API endpoints
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return ContentTypeJSON
	}

	// Default to HTML for web endpoints
	return ContentTypeHTML
}

// WriteWithContentNegotiation writes response based on Accept header
func (b *Builder) WriteWithContentNegotiation(w http.ResponseWriter, r *http.Request, data interface{}) {
	contentType := GetPreferredContentType(r)

	switch contentType {
	case ContentTypeJSON:
		b.WriteSuccess(w, r, data)
	case ContentTypeHTML:
		// For HTML, you might want to render a template
		// This is where you'd integrate with your template system
		templateData := b.BuildTemplateData(r.Context(), "Data", data, nil)
		b.writeTemplate(w, r, "data", templateData)
	default:
		b.WriteSuccess(w, r, data)
	}
}

// writeTemplate is a placeholder for template rendering
// You'll need to integrate this with your existing template system
func (b *Builder) writeTemplate(w http.ResponseWriter, r *http.Request, templateName string, data *TemplateData) {
	// This should integrate with your existing template system
	// For now, fall back to JSON
	b.WriteSuccess(w, r, data)
}

// ===============================
// QUICK RESPONSE HELPERS
// ===============================

// QuickStatusResponse is a helper for simple status responses
func QuickStatusResponse(w http.ResponseWriter, r *http.Request, code int, message string) {
	if builder := GetBuilder(r.Context()); builder != nil {
		builder.WriteStatusResponse(w, r, code, message)
		return
	}

	// Fallback
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"status":%d,"message":"%s","success":%t}`,
		code, message, IsSuccessStatus(code))
}

// QuickNotFound is a helper for 404 responses
func QuickNotFound(w http.ResponseWriter, r *http.Request) {
	QuickStatusResponse(w, r, StatusNotFound, "Not found")
}

// QuickUnauthorized is a helper for 401 responses
func QuickUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	QuickStatusResponse(w, r, StatusUnauthorized, "Authentication required")
}

// QuickForbidden is a helper for 403 responses
func QuickForbidden(w http.ResponseWriter, r *http.Request) {
	QuickStatusResponse(w, r, StatusForbidden, "Access forbidden")
}

// QuickInternalError is a helper for 500 responses
func QuickInternalError(w http.ResponseWriter, r *http.Request) {
	QuickStatusResponse(w, r, StatusInternalServerError, "Internal server error")
}

// ===============================
// STATUS CODE VALIDATION
// ===============================

// ValidateStatusCode ensures status code is valid
func ValidateStatusCode(code int) error {
	if code < 100 || code > 599 {
		return fmt.Errorf("invalid HTTP status code: %d", code)
	}
	return nil
}

// IsValidStatusCode checks if status code is valid
func IsValidStatusCode(code int) bool {
	return code >= 100 && code <= 599
}

// NormalizeStatusCode ensures status code is valid, defaults to 500
func NormalizeStatusCode(code int) int {
	if IsValidStatusCode(code) {
		return code
	}
	return StatusInternalServerError
}
