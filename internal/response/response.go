package response

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"evalhub/internal/contextutils"
	"evalhub/internal/responseutil"
	"evalhub/internal/services"

	"go.uber.org/zap"
)

// ===============================
// RESPONSE CONFIGURATION
// ===============================

// Config holds configuration for the response system
type Config struct {
	// JSON response settings
	PrettyJSON       bool   `json:"pretty_json"`
	IncludeRequestID bool   `json:"include_request_id"`
	IncludeTimestamp bool   `json:"include_timestamp"`
	IncludeVersion   bool   `json:"include_version"`
	APIVersion       string `json:"api_version"`

	// Error handling
	IncludeErrorStack  bool `json:"include_error_stack"`
	MaskInternalErrors bool `json:"mask_internal_errors"`

	// Performance
	EnableCompression bool `json:"enable_compression"`
	CacheHeaders      bool `json:"cache_headers"`

	// Template settings
	DefaultTemplate string `json:"default_template"`
	ErrorTemplate   string `json:"error_template"`
}

// DefaultConfig returns production-ready response configuration
func DefaultConfig() *Config {
	return &Config{
		PrettyJSON:         false, // Compact in production
		IncludeRequestID:   true,
		IncludeTimestamp:   true,
		IncludeVersion:     true,
		APIVersion:         "v1",
		IncludeErrorStack:  false, // Only in development
		MaskInternalErrors: true,  // Hide internal errors in production
		EnableCompression:  true,
		CacheHeaders:       true,
		DefaultTemplate:    "layout",
		ErrorTemplate:      "error",
	}
}

// ===============================
// RESPONSE TYPES
// ===============================

// APIResponse represents a standardized API response
type APIResponse struct {
	Success   bool          `json:"success"`
	Data      interface{}   `json:"data,omitempty"`
	Error     *ErrorDetail  `json:"error,omitempty"`
	Meta      *ResponseMeta `json:"meta,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
	Timestamp int64         `json:"timestamp,omitempty"`
	Version   string        `json:"version,omitempty"`
}

// ErrorDetail represents error information in API responses
type ErrorDetail struct {
	Type       string                 `json:"type"`
	Message    string                 `json:"message"`
	Code       string                 `json:"code,omitempty"`
	Fields     []FieldError           `json:"fields,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
}

// FieldError represents field-specific validation errors
type FieldError struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value,omitempty"`
	Message string      `json:"message"`
	Code    string      `json:"code"`
}

// ResponseMeta contains metadata about the response
type ResponseMeta struct {
	Pagination *PaginationMeta        `json:"pagination,omitempty"`
	Stats      *StatsMeta             `json:"stats,omitempty"`
	Timing     *TimingMeta            `json:"timing,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// PaginationMeta contains pagination information
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// StatsMeta contains statistics about the response
type StatsMeta struct {
	Count       int                    `json:"count"`
	Filtered    int                    `json:"filtered,omitempty"`
	ProcessTime float64                `json:"process_time_ms"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// TimingMeta contains timing information
type TimingMeta struct {
	StartTime   time.Time `json:"-"`
	ProcessTime float64   `json:"process_time_ms"`
	DBTime      float64   `json:"db_time_ms,omitempty"`
	CacheTime   float64   `json:"cache_time_ms,omitempty"`
}

// ===============================
// RESPONSE BUILDER
// ===============================

// Builder helps construct standardized responses
type Builder struct {
	config *Config
	logger *zap.Logger
}

// NewBuilder creates a new response builder
func NewBuilder(config *Config, logger *zap.Logger) *Builder {
	if config == nil {
		config = DefaultConfig()
	}
	return &Builder{
		config: config,
		logger: logger,
	}
}

// ===============================
// SUCCESS RESPONSES
// ===============================

// Success creates a successful API response
func (b *Builder) Success(ctx context.Context, data interface{}) *APIResponse {
	return &APIResponse{
		Success:   true,
		Data:      data,
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}
}

// SuccessWithMeta creates a successful API response with metadata
func (b *Builder) SuccessWithMeta(ctx context.Context, data interface{}, meta *ResponseMeta) *APIResponse {
	return &APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}
}

// Created creates a successful creation response
func (b *Builder) Created(ctx context.Context, data interface{}) *APIResponse {
	response := b.Success(ctx, data)
	return response
}

// NoContent creates a successful no-content response
func (b *Builder) NoContent(ctx context.Context) *APIResponse {
	return &APIResponse{
		Success:   true,
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}
}

// ===============================
// ERROR RESPONSES
// ===============================

// Error creates an error response from a service error
func (b *Builder) Error(ctx context.Context, err error) *APIResponse {
	errorDetail := b.convertError(err)

	response := &APIResponse{
		Success:   false,
		Error:     errorDetail,
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}

	// Log the error
	b.logError(ctx, err, errorDetail)

	return response
}

// ValidationError creates a validation error response
func (b *Builder) ValidationError(ctx context.Context, message string, fields []FieldError) *APIResponse {
	return &APIResponse{
		Success: false,
		Error: &ErrorDetail{
			Type:    "VALIDATION_ERROR",
			Message: message,
			Fields:  fields,
		},
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}
}

// BusinessError creates a business logic error response
func (b *Builder) BusinessError(ctx context.Context, message, code string) *APIResponse {
	return &APIResponse{
		Success: false,
		Error: &ErrorDetail{
			Type:    "BUSINESS_ERROR",
			Message: message,
			Code:    code,
		},
		RequestID: b.getRequestID(ctx),
		Timestamp: b.getTimestamp(),
		Version:   b.getVersion(),
	}
}

// ===============================
// PAGINATION RESPONSES
// ===============================

// Paginated creates a paginated response
func (b *Builder) Paginated(ctx context.Context, data interface{}, page, pageSize int, total int64) *APIResponse {
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	meta := &ResponseMeta{
		Pagination: &PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
			HasNext:    page < totalPages,
			HasPrev:    page > 1,
		},
	}

	return b.SuccessWithMeta(ctx, data, meta)
}

// ===============================
// HTTP RESPONSE WRITERS
// ===============================

// WriteJSON writes a JSON response with appropriate headers
func (b *Builder) WriteJSON(w http.ResponseWriter, r *http.Request, response *APIResponse, statusCode int) {
	// Set headers
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Add cache headers if enabled
	if b.config.CacheHeaders {
		b.setCacheHeaders(w, statusCode)
	}

	// Add compression if enabled
	if b.config.EnableCompression {
		w.Header().Set("Vary", "Accept-Encoding")
	}

	// Set status code
	w.WriteHeader(statusCode)

	// Encode response
	encoder := json.NewEncoder(w)
	if b.config.PrettyJSON {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(response); err != nil {
		b.logger.Error("Failed to encode JSON response",
			zap.Error(err),
			zap.String("request_id", b.getRequestID(r.Context())),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// WriteSuccess writes a successful JSON response
func (b *Builder) WriteSuccess(w http.ResponseWriter, r *http.Request, data interface{}) {
	response := b.Success(r.Context(), data)
	b.WriteJSON(w, r, response, http.StatusOK)
}

// WriteCreated writes a successful creation response
func (b *Builder) WriteCreated(w http.ResponseWriter, r *http.Request, data interface{}) {
	response := b.Created(r.Context(), data)
	b.WriteJSON(w, r, response, http.StatusCreated)
}

// WriteNoContent writes a successful no-content response
func (b *Builder) WriteNoContent(w http.ResponseWriter, r *http.Request) {
	response := b.NoContent(r.Context())
	b.WriteJSON(w, r, response, http.StatusNoContent)
}

// WriteError writes an error response with appropriate status code
func (b *Builder) WriteError(w http.ResponseWriter, r *http.Request, err error) {
	response := b.Error(r.Context(), err)
	statusCode := b.getStatusCodeFromError(err)
	b.WriteJSON(w, r, response, statusCode)
}

// WritePaginated writes a paginated response
func (b *Builder) WritePaginated(w http.ResponseWriter, r *http.Request, data interface{}, page, pageSize int, total int64) {
	response := b.Paginated(r.Context(), data, page, pageSize, total)
	b.WriteJSON(w, r, response, http.StatusOK)
}

// ===============================
// TEMPLATE RESPONSES
// ===============================

// TemplateData represents data for template rendering
type TemplateData struct {
	Title      string                 `json:"title"`
	IsLoggedIn bool                   `json:"is_logged_in"`
	Username   string                 `json:"username,omitempty"`
	Data       interface{}            `json:"data,omitempty"`
	Error      *ErrorDetail           `json:"error,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// BuildTemplateData creates template data from API response
func (b *Builder) BuildTemplateData(ctx context.Context, title string, data interface{}, err error) *TemplateData {
	templateData := &TemplateData{
		Title:     title,
		RequestID: b.getRequestID(ctx),
		Extra:     make(map[string]interface{}),
	}

	// Check if user is authenticated
	if userID := contextutils.GetUserID(ctx); userID != 0 {
		templateData.IsLoggedIn = true
		// You might want to get username from context or user service
	}

	if err != nil {
		templateData.Error = b.convertError(err)
	} else {
		templateData.Data = data
	}

	return templateData
}

// ===============================
// UTILITY METHODS
// ===============================

// convertError converts various error types to ErrorDetail
func (b *Builder) convertError(err error) *ErrorDetail {
	if err == nil {
		return nil
	}

	// Handle ServiceError from services package
	if serviceErr, ok := err.(*services.ServiceError); ok {
		return &ErrorDetail{
			Type:    serviceErr.Type,
			Message: serviceErr.Message,
			Code:    serviceErr.Code,
			Details: serviceErr.Details,
		}
	}

	// Handle ValidationError from services package
	if valErr, ok := err.(*services.ValidationError); ok {
		fields := make([]FieldError, len(valErr.Fields))
		for i, field := range valErr.Fields {
			fields[i] = FieldError{
				Field:   field.Field,
				Value:   field.Value,
				Message: field.Message,
				Code:    field.Code,
			}
		}

		return &ErrorDetail{
			Type:    valErr.Type,
			Message: valErr.Message,
			Code:    valErr.Code,
			Fields:  fields,
			Details: valErr.Details,
		}
	}

	// Handle other service errors
	if serviceErr := services.GetServiceError(err); serviceErr != nil {
		detail := &ErrorDetail{
			Type:    serviceErr.Type,
			Message: serviceErr.Message,
			Code:    serviceErr.Code,
			Details: serviceErr.Details,
		}

		// Mask internal errors in production
		if b.config.MaskInternalErrors && serviceErr.Type == "INTERNAL_ERROR" {
			detail.Message = "An internal error occurred"
			detail.Details = nil
		}

		return detail
	}

	// Fallback for unknown errors
	message := err.Error()
	if b.config.MaskInternalErrors {
		message = "An unexpected error occurred"
	}

	return &ErrorDetail{
		Type:    "INTERNAL_ERROR",
		Message: message,
	}
}

// getStatusCodeFromError determines HTTP status code from error
func (b *Builder) getStatusCodeFromError(err error) int {
	if serviceErr := services.GetServiceError(err); serviceErr != nil {
		return serviceErr.GetStatusCode()
	}
	return http.StatusInternalServerError
}

// getRequestID extracts request ID from context
func (b *Builder) getRequestID(ctx context.Context) string {
	if !b.config.IncludeRequestID {
		return ""
	}
	return contextutils.GetRequestID(ctx)
}

// getTimestamp returns current timestamp if enabled
func (b *Builder) getTimestamp() int64 {
	if !b.config.IncludeTimestamp {
		return 0
	}
	return time.Now().Unix()
}

// getVersion returns API version if enabled
func (b *Builder) getVersion() string {
	if !b.config.IncludeVersion {
		return ""
	}
	return b.config.APIVersion
}

// setCacheHeaders sets appropriate cache headers
func (b *Builder) setCacheHeaders(w http.ResponseWriter, statusCode int) {
	if statusCode >= 200 && statusCode < 300 {
		// Cache successful responses briefly
		w.Header().Set("Cache-Control", "public, max-age=60")
	} else {
		// Don't cache error responses
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}
}

// logError logs error information
func (b *Builder) logError(ctx context.Context, err error, errorDetail *ErrorDetail) {
	requestID := b.getRequestID(ctx)

	// Determine log level based on error type
	switch errorDetail.Type {
	case "VALIDATION_ERROR", "BUSINESS_ERROR":
		b.logger.Warn("Request error",
			zap.String("request_id", requestID),
			zap.String("error_type", errorDetail.Type),
			zap.String("error_message", errorDetail.Message),
			zap.String("error_code", errorDetail.Code),
		)
	case "INTERNAL_ERROR":
		b.logger.Error("Internal error",
			zap.String("request_id", requestID),
			zap.String("error_type", errorDetail.Type),
			zap.String("error_message", errorDetail.Message),
			zap.Error(err),
		)
	default:
		b.logger.Info("Request completed with error",
			zap.String("request_id", requestID),
			zap.String("error_type", errorDetail.Type),
			zap.String("error_message", errorDetail.Message),
		)
	}
}

// ===============================
// CONTEXT HELPERS
// ===============================

// GetBuilder extracts response builder from context
func GetBuilder(ctx context.Context) *Builder {
	if builder, ok := responseutil.GetBuilder(ctx).(*Builder); ok {
		return builder
	}
	return nil
}

// SetBuilder stores response builder in context
func SetBuilder(ctx context.Context, builder *Builder) context.Context {
	return responseutil.SetBuilder(ctx, builder)
}

// ===============================
// HELPER FUNCTIONS
// ===============================

// QuickSuccess is a helper for simple success responses
func QuickSuccess(w http.ResponseWriter, r *http.Request, data interface{}) {
	if builder := GetBuilder(r.Context()); builder != nil {
		builder.WriteSuccess(w, r, data)
		return
	}

	// Fallback
	defaultBuilder := NewBuilder(DefaultConfig(), zap.NewNop())
	defaultBuilder.WriteSuccess(w, r, data)
}

// QuickError is a helper for simple error responses
func QuickError(w http.ResponseWriter, r *http.Request, err error) {
	if builder := GetBuilder(r.Context()); builder != nil {
		builder.WriteError(w, r, err)
		return
	}

	// Fallback
	defaultBuilder := NewBuilder(DefaultConfig(), zap.NewNop())
	defaultBuilder.WriteError(w, r, err)
}

// QuickJSON is a helper for manual JSON responses
func QuickJSON(w http.ResponseWriter, r *http.Request, data interface{}, statusCode int) {
	if builder := GetBuilder(r.Context()); builder != nil {
		response := builder.Success(r.Context(), data)
		builder.WriteJSON(w, r, response, statusCode)
		return
	}

	// Fallback to manual JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// ===============================
// RESPONSE MIDDLEWARE
// ===============================

// Middleware creates response builder middleware
func Middleware(builder *Builder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add builder to context
			ctx := SetBuilder(r.Context(), builder)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CreateResponseMiddlewareStack creates the complete response middleware
func CreateResponseMiddlewareStack(config *Config, logger *zap.Logger) func(http.Handler) http.Handler {
	builder := NewBuilder(config, logger)
	return Middleware(builder)
}
