// file: internal/middleware/validationMiddleware.go
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"evalhub/internal/models"
	"evalhub/internal/services"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// ValidationConfig holds validation middleware configuration
type ValidationConfig struct {
	// Request limits
	MaxRequestSize   int64         `json:"max_request_size"`   // 10MB default
	MaxMultipartSize int64         `json:"max_multipart_size"` // 32MB default
	MaxJSONDepth     int           `json:"max_json_depth"`     // 10 levels default
	RequestTimeout   time.Duration `json:"request_timeout"`    // 30s default

	// File upload limits
	MaxFileSize        int64    `json:"max_file_size"`  // 5MB default
	MaxFileCount       int      `json:"max_file_count"` // 10 files default
	AllowedMimeTypes   []string `json:"allowed_mime_types"`
	ForbiddenMimeTypes []string `json:"forbidden_mime_types"`

	// Validation settings
	EnableSanitization  bool     `json:"enable_sanitization"`
	EnableContentFilter bool     `json:"enable_content_filter"`
	StrictValidation    bool     `json:"strict_validation"`
	CaseSensitiveFields []string `json:"case_sensitive_fields"`

	// Security settings
	EnableXSSProtection bool `json:"enable_xss_protection"`
	EnableSQLInjection  bool `json:"enable_sql_injection_protection"`
	MaxStringLength     int  `json:"max_string_length"` // 10000 chars default

	// Performance
	EnableCaching bool          `json:"enable_caching"`
	CacheTTL      time.Duration `json:"cache_ttl"`

	// Endpoints configuration
	SkipValidationPaths []string `json:"skip_validation_paths"`
	RequireValidation   []string `json:"require_validation_paths"`

	// Error handling
	IncludeFieldDetails bool `json:"include_field_details"`
	MaxErrorsReturned   int  `json:"max_errors_returned"` // 20 default
}

// DefaultValidationConfig returns production-ready validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxRequestSize:   10 * 1024 * 1024, // 10MB
		MaxMultipartSize: 32 * 1024 * 1024, // 32MB
		MaxJSONDepth:     10,
		RequestTimeout:   30 * time.Second,
		MaxFileSize:      5 * 1024 * 1024, // 5MB
		MaxFileCount:     10,
		AllowedMimeTypes: []string{
			"image/jpeg", "image/png", "image/gif", "image/webp",
			"application/pdf", "application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"text/plain", "text/csv",
		},
		ForbiddenMimeTypes: []string{
			"application/x-executable", "application/x-dosexec",
			"application/x-msdownload", "application/x-sh",
			"text/x-script", "application/javascript",
		},
		EnableSanitization:  true,
		EnableContentFilter: true,
		StrictValidation:    true,
		CaseSensitiveFields: []string{"email", "username"},
		EnableXSSProtection: true,
		EnableSQLInjection:  true,
		MaxStringLength:     10000,
		EnableCaching:       true,
		CacheTTL:            5 * time.Minute,
		SkipValidationPaths: []string{
			"/health", "/metrics", "/static/", "/uploads/",
		},
		RequireValidation: []string{
			"/api/", "/auth/", "/admin/",
		},
		IncludeFieldDetails: true,
		MaxErrorsReturned:   20,
	}
}

// ValidationResult represents the result of request validation
type ValidationResult struct {
	IsValid     bool                   `json:"is_valid"`
	Errors      []ValidationFieldError `json:"errors,omitempty"`
	Sanitized   map[string]interface{} `json:"sanitized,omitempty"`
	Files       []ValidatedFile        `json:"files,omitempty"`
	RequestSize int64                  `json:"request_size"`
}

// ValidationFieldError represents a field-specific validation error
type ValidationFieldError struct {
	Field      string      `json:"field"`
	Message    string      `json:"message"`
	Code       string      `json:"code"`
	Value      interface{} `json:"value,omitempty"`
	Constraint string      `json:"constraint,omitempty"`
}

// ValidatedFile represents a validated file upload
type ValidatedFile struct {
	FieldName string `json:"field_name"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
	IsValid   bool   `json:"is_valid"`
	Error     string `json:"error,omitempty"`
}

// RequestValidator provides comprehensive request validation
type RequestValidator struct {
	config    *ValidationConfig
	validator *validator.Validate
	logger    *zap.Logger
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(config *ValidationConfig, logger *zap.Logger) *RequestValidator {
	if config == nil {
		config = DefaultValidationConfig()
	}

	// Initialize go-playground validator
	validate := validator.New()

	// Register custom validators
	registerCustomValidators(validate)

	return &RequestValidator{
		config:    config,
		validator: validate,
		logger:    logger,
	}
}

// ===============================
// MAIN VALIDATION MIDDLEWARE
// ===============================

// ValidateRequest creates the main validation middleware
func ValidateRequest(validator *RequestValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestLogger := GetRequestLogger(ctx)
			requestID := GetRequestID(ctx)

			// Check if validation should be skipped for this path
			if validator.shouldSkipValidation(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Create timeout context for validation
			validationCtx, cancel := context.WithTimeout(ctx, validator.config.RequestTimeout)
			defer cancel()

			// Perform comprehensive validation
			result, err := validator.validateRequest(validationCtx, r)
			if err != nil {
				requestLogger.Error("Request validation failed",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				validator.writeValidationError(w, "Internal validation error", http.StatusInternalServerError)
				return
			}

			// Check validation result
			if !result.IsValid {
				requestLogger.Warn("Request validation failed",
					zap.String("request_id", requestID),
					zap.Int("error_count", len(result.Errors)),
					zap.Any("errors", result.Errors),
				)
				validator.writeValidationErrors(w, result.Errors)
				return
			}

			// Log successful validation
			requestLogger.Debug("Request validation passed",
				zap.String("request_id", requestID),
				zap.Int64("request_size", result.RequestSize),
				zap.Int("file_count", len(result.Files)),
			)

			// Inject validated and sanitized data into context
			if len(result.Sanitized) > 0 {
				ctx = context.WithValue(ctx, SanitizedDataKey, result.Sanitized)
			}
			if len(result.Files) > 0 {
				ctx = context.WithValue(ctx, ValidatedFilesKey, result.Files)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ===============================
// CORE VALIDATION LOGIC
// ===============================

// validateRequest performs comprehensive request validation
func (rv *RequestValidator) validateRequest(ctx context.Context, r *http.Request) (*ValidationResult, error) {
	result := &ValidationResult{
		IsValid:     true,
		Errors:      []ValidationFieldError{},
		Sanitized:   make(map[string]interface{}),
		Files:       []ValidatedFile{},
		RequestSize: r.ContentLength,
	}

	// 1. Validate request size
	if err := rv.validateRequestSize(r); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "request",
			Message: err.Error(),
			Code:    "REQUEST_TOO_LARGE",
		})
		return result, nil
	}

	// 2. Validate content type and parse request
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		return rv.validateJSONRequest(ctx, r, result)
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		return rv.validateMultipartRequest(ctx, r, result)
	} else if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		return rv.validateFormRequest(ctx, r, result)
	}

	// 3. For other content types, perform basic validation
	return rv.validateBasicRequest(ctx, r, result)
}

// validateJSONRequest validates JSON requests
func (rv *RequestValidator) validateJSONRequest(ctx context.Context, r *http.Request, result *ValidationResult) (*ValidationResult, error) {
	// Read and validate JSON body
	body, err := rv.readLimitedBody(r)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "body",
			Message: err.Error(),
			Code:    "INVALID_BODY",
		})
		return result, nil
	}

	// Parse JSON with depth protection
	var data interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields() // Strict mode if enabled

	if err := decoder.Decode(&data); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "body",
			Message: "Invalid JSON format",
			Code:    "INVALID_JSON",
			Value:   string(body),
		})
		return result, nil
	}

	// Validate JSON depth
	if err := rv.validateJSONDepth(data, 0); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "body",
			Message: err.Error(),
			Code:    "JSON_TOO_DEEP",
		})
		return result, nil
	}

	// Sanitize and validate data
	sanitizedData, validationErrors := rv.sanitizeAndValidateData(data)
	result.Sanitized = sanitizedData
	result.Errors = append(result.Errors, validationErrors...)

	if len(validationErrors) > 0 {
		result.IsValid = false
	}

	// Restore sanitized body to request
	sanitizedBody, _ := json.Marshal(sanitizedData)
	r.Body = io.NopCloser(bytes.NewReader(sanitizedBody))
	r.ContentLength = int64(len(sanitizedBody))

	return result, nil
}

// validateMultipartRequest validates multipart form requests
func (rv *RequestValidator) validateMultipartRequest(ctx context.Context, r *http.Request, result *ValidationResult) (*ValidationResult, error) {
	// Parse multipart form with size limit
	if err := r.ParseMultipartForm(rv.config.MaxMultipartSize); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "form",
			Message: "Failed to parse multipart form",
			Code:    "INVALID_MULTIPART",
		})
		return result, nil
	}

	// Validate form fields
	if r.MultipartForm != nil && r.MultipartForm.Value != nil {
		for fieldName, values := range r.MultipartForm.Value {
			for _, value := range values {
				if sanitized, err := rv.sanitizeString(value); err != nil {
					result.IsValid = false
					result.Errors = append(result.Errors, ValidationFieldError{
						Field:   fieldName,
						Message: err.Error(),
						Code:    "INVALID_CONTENT",
						Value:   value,
					})
				} else {
					result.Sanitized[fieldName] = sanitized
				}
			}
		}

		// Validate files
		if r.MultipartForm.File != nil {
			for fieldName, files := range r.MultipartForm.File {
				for _, fileHeader := range files {
					validatedFile := rv.validateFile(fieldName, fileHeader)
					result.Files = append(result.Files, validatedFile)

					if !validatedFile.IsValid {
						result.IsValid = false
						result.Errors = append(result.Errors, ValidationFieldError{
							Field:   fieldName,
							Message: validatedFile.Error,
							Code:    "INVALID_FILE",
							Value:   fileHeader.Filename,
						})
					}
				}
			}
		}
	}

	return result, nil
}

// validateFormRequest validates URL-encoded form requests
func (rv *RequestValidator) validateFormRequest(ctx context.Context, r *http.Request, result *ValidationResult) (*ValidationResult, error) {
	if err := r.ParseForm(); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationFieldError{
			Field:   "form",
			Message: "Failed to parse form data",
			Code:    "INVALID_FORM",
		})
		return result, nil
	}

	// Validate and sanitize form values
	for fieldName, values := range r.Form {
		for _, value := range values {
			if sanitized, err := rv.sanitizeString(value); err != nil {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationFieldError{
					Field:   fieldName,
					Message: err.Error(),
					Code:    "INVALID_CONTENT",
					Value:   value,
				})
			} else {
				result.Sanitized[fieldName] = sanitized
			}
		}
	}

	return result, nil
}

// validateBasicRequest validates other request types
func (rv *RequestValidator) validateBasicRequest(ctx context.Context, r *http.Request, result *ValidationResult) (*ValidationResult, error) {
	// Validate query parameters
	for key, values := range r.URL.Query() {
		for _, value := range values {
			if sanitized, err := rv.sanitizeString(value); err != nil {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationFieldError{
					Field:   key,
					Message: err.Error(),
					Code:    "INVALID_QUERY_PARAM",
					Value:   value,
				})
			} else {
				result.Sanitized[key] = sanitized
			}
		}
	}

	return result, nil
}

// ===============================
// VALIDATION HELPERS
// ===============================

// validateRequestSize validates overall request size
func (rv *RequestValidator) validateRequestSize(r *http.Request) error {
	if r.ContentLength > rv.config.MaxRequestSize {
		return fmt.Errorf("request size %d exceeds maximum %d bytes",
			r.ContentLength, rv.config.MaxRequestSize)
	}
	return nil
}

// validateJSONDepth validates JSON nesting depth
func (rv *RequestValidator) validateJSONDepth(data interface{}, depth int) error {
	if depth > rv.config.MaxJSONDepth {
		return fmt.Errorf("JSON nesting too deep (max %d levels)", rv.config.MaxJSONDepth)
	}

	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			if err := rv.validateJSONDepth(value, depth+1); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, value := range v {
			if err := rv.validateJSONDepth(value, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateFile validates individual file uploads
func (rv *RequestValidator) validateFile(fieldName string, fileHeader *multipart.FileHeader) ValidatedFile {
	result := ValidatedFile{
		FieldName: fieldName,
		Filename:  fileHeader.Filename,
		Size:      fileHeader.Size,
		IsValid:   true,
	}

	// Check file size
	if fileHeader.Size > rv.config.MaxFileSize {
		result.IsValid = false
		result.Error = fmt.Sprintf("file size %d exceeds maximum %d bytes",
			fileHeader.Size, rv.config.MaxFileSize)
		return result
	}

	// Check MIME type
	file, err := fileHeader.Open()
	if err != nil {
		result.IsValid = false
		result.Error = "failed to open file"
		return result
	}
	defer file.Close()

	// Read first 512 bytes to detect MIME type
	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)
	mimeType := http.DetectContentType(buffer[:n])
	result.MimeType = mimeType

	// Validate MIME type
	if err := rv.validateMimeType(mimeType); err != nil {
		result.IsValid = false
		result.Error = err.Error()
		return result
	}

	// Additional file security checks
	if err := rv.validateFileContent(fileHeader.Filename, buffer[:n]); err != nil {
		result.IsValid = false
		result.Error = err.Error()
		return result
	}

	return result
}

// validateMimeType validates file MIME type
func (rv *RequestValidator) validateMimeType(mimeType string) error {
	// Check forbidden types first
	for _, forbidden := range rv.config.ForbiddenMimeTypes {
		if strings.Contains(mimeType, forbidden) {
			return fmt.Errorf("forbidden file type: %s", mimeType)
		}
	}

	// Check allowed types
	if len(rv.config.AllowedMimeTypes) > 0 {
		for _, allowed := range rv.config.AllowedMimeTypes {
			if strings.Contains(mimeType, allowed) {
				return nil
			}
		}
		return fmt.Errorf("file type not allowed: %s", mimeType)
	}

	return nil
}

// validateFileContent performs additional file content validation
func (rv *RequestValidator) validateFileContent(filename string, content []byte) error {
	// Check for executable file signatures
	executableSignatures := [][]byte{
		{0x4D, 0x5A},             // PE executable
		{0x7F, 0x45, 0x4C, 0x46}, // ELF executable
		{0xCA, 0xFE, 0xBA, 0xBE}, // Mach-O executable
	}

	for _, signature := range executableSignatures {
		if bytes.HasPrefix(content, signature) {
			return fmt.Errorf("executable files not allowed")
		}
	}

	// Check for script content in filename extensions
	dangerousExtensions := []string{
		".exe", ".bat", ".cmd", ".com", ".scr", ".sh",
		".ps1", ".vbs", ".js", ".jar",
	}

	lowerFilename := strings.ToLower(filename)
	for _, ext := range dangerousExtensions {
		if strings.HasSuffix(lowerFilename, ext) {
			return fmt.Errorf("dangerous file extension: %s", ext)
		}
	}

	return nil
}

// ===============================
// SANITIZATION AND VALIDATION
// ===============================

// sanitizeAndValidateData sanitizes and validates data structure
func (rv *RequestValidator) sanitizeAndValidateData(data interface{}) (map[string]interface{}, []ValidationFieldError) {
	sanitized := make(map[string]interface{})
	var errors []ValidationFieldError

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if sanitizedValue, err := rv.sanitizeValue(key, value); err != nil {
				errors = append(errors, ValidationFieldError{
					Field:   key,
					Message: err.Error(),
					Code:    "INVALID_VALUE",
					Value:   value,
				})
			} else {
				sanitized[key] = sanitizedValue
			}
		}
	}

	return sanitized, errors
}

// sanitizeValue sanitizes individual values
func (rv *RequestValidator) sanitizeValue(key string, value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return rv.sanitizeString(v)
	case map[string]interface{}:
		sanitized, errors := rv.sanitizeAndValidateData(v)
		if len(errors) > 0 {
			return nil, fmt.Errorf("nested validation failed")
		}
		return sanitized, nil
	case []interface{}:
		var sanitizedArray []interface{}
		for _, item := range v {
			sanitizedItem, err := rv.sanitizeValue(key, item)
			if err != nil {
				return nil, err
			}
			sanitizedArray = append(sanitizedArray, sanitizedItem)
		}
		return sanitizedArray, nil
	default:
		return value, nil
	}
}

// sanitizeString sanitizes string values
func (rv *RequestValidator) sanitizeString(s string) (string, error) {
	if !rv.config.EnableSanitization {
		return s, nil
	}

	// Check string length
	if len(s) > rv.config.MaxStringLength {
		return "", fmt.Errorf("string too long (max %d characters)", rv.config.MaxStringLength)
	}

	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// XSS protection
	if rv.config.EnableXSSProtection {
		if err := rv.checkXSS(s); err != nil {
			return "", err
		}
	}

	// SQL injection protection
	if rv.config.EnableSQLInjection {
		if err := rv.checkSQLInjection(s); err != nil {
			return "", err
		}
	}

	// Content filtering
	if rv.config.EnableContentFilter {
		if err := rv.checkContentSafety(s); err != nil {
			return "", err
		}
	}

	// Trim whitespace
	s = strings.TrimSpace(s)

	return s, nil
}

// checkXSS checks for XSS patterns
func (rv *RequestValidator) checkXSS(s string) error {
	xssPatterns := []string{
		"<script", "javascript:", "onload=", "onerror=", "onclick=",
		"eval(", "alert(", "confirm(", "prompt(",
	}

	lower := strings.ToLower(s)
	for _, pattern := range xssPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potentially malicious content detected")
		}
	}

	return nil
}

// checkSQLInjection checks for SQL injection patterns
func (rv *RequestValidator) checkSQLInjection(s string) error {
	sqlPatterns := []string{
		"union select", "drop table", "insert into", "delete from",
		"update set", "exec(", "sp_", "xp_", "'; --", "' or '1'='1",
	}

	lower := strings.ToLower(s)
	for _, pattern := range sqlPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potentially malicious SQL detected")
		}
	}

	return nil
}

// checkContentSafety checks for potentially harmful content in the input string
func (rv *RequestValidator) checkContentSafety(s string) error {
	// Basic content safety without using models that require user context
	
	// Check for common dangerous patterns
	dangerousPatterns := []string{
		"<script", "javascript:", "data:text/html", 
		"vbscript:", "onload=", "onerror=", "onclick=",
	}
	
	lower := strings.ToLower(s)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potentially unsafe content detected")
		}
	}
	
	// Check for excessive length
	if len(s) > rv.config.MaxStringLength {
		return fmt.Errorf("content too long")
	}
	
	return nil
}

// // checkContentSafety checks for potentially harmful content in the input string
// func (rv *RequestValidator) checkContentSafety(s string) error {
// 	// Create a temporary model that implements the Validator interface
// 	// We'll use the Comment model as it's lightweight and implements the interface
// 	comment := &models.Comment{
// 		Content: s,
// 	}

// 	// Use the existing Validate method
// 	errs := comment.Validate()
// 	if len(errs) > 0 {
// 		// Return the first validation error
// 		return fmt.Errorf("content validation failed: %v", errs[0].Message)
// 	}

// 	return nil
// }

// ===============================
// UTILITY FUNCTIONS
// ===============================

// readLimitedBody reads request body with size limits
func (rv *RequestValidator) readLimitedBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("empty request body")
	}

	limitedReader := io.LimitReader(r.Body, rv.config.MaxRequestSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Restore body for further processing
	r.Body = io.NopCloser(bytes.NewReader(body))

	return body, nil
}

// shouldSkipValidation checks if validation should be skipped for a path
func (rv *RequestValidator) shouldSkipValidation(path string) bool {
	for _, skipPath := range rv.config.SkipValidationPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// writeValidationErrors writes structured validation errors
func (rv *RequestValidator) writeValidationErrors(w http.ResponseWriter, errors []ValidationFieldError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	// Limit number of errors returned
	if len(errors) > rv.config.MaxErrorsReturned {
		errors = errors[:rv.config.MaxErrorsReturned]
	}

	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "VALIDATION_ERROR",
			"message": "Request validation failed",
			"fields":  errors,
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(errorResponse)
}

// writeValidationError writes a single validation error
func (rv *RequestValidator) writeValidationError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "VALIDATION_ERROR",
			"message": message,
		},
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(errorResponse)
}

// ===============================
// CUSTOM VALIDATORS
// ===============================

// registerCustomValidators registers custom validation rules
func registerCustomValidators(validate *validator.Validate) {
	// Email validation
	validate.RegisterValidation("evalhub_email", validateEmail)

	// Username validation
	validate.RegisterValidation("evalhub_username", validateUsername)

	// Password strength validation
	validate.RegisterValidation("evalhub_password", validatePassword)

	// Content validation
	validate.RegisterValidation("evalhub_content", validateContent)
}

// Custom validator functions
func validateEmail(fl validator.FieldLevel) bool {
	email := fl.Field().String()
	return models.EmailValidator("email", email) == nil
}

func validateUsername(fl validator.FieldLevel) bool {
	username := fl.Field().String()
	return models.UsernameValidator("username", username) == nil
}

func validatePassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	return models.PasswordValidator("password", password) == nil
}

func validateContent(fl validator.FieldLevel) bool {
	content := fl.Field().String()
	return models.ContentValidator("content", content, 1, 10000) == nil
}

// ===============================
// CONTEXT HELPERS
// ===============================
// Note: GetSanitizedData and GetValidatedFiles are now defined in context.go

// ===============================
// INTEGRATION HELPERS
// ===============================

// CreateValidationMiddlewareStack creates a complete validation middleware stack
func CreateValidationMiddlewareStack(
	config *ValidationConfig,
	logger *zap.Logger,
) func(http.Handler) http.Handler {
	validator := NewRequestValidator(config, logger)

	return func(next http.Handler) http.Handler {
		// Stack validation middleware
		handler := next
		handler = ValidateRequest(validator)(handler)
		return handler
	}
}

// ValidateStruct validates a struct using the existing validation system
func ValidateStruct(s interface{}) error {
	if validator, ok := s.(models.Validator); ok {
		if errors := validator.Validate(); errors.HasErrors() {
			return errors
		}
	}
	return nil
}

// Integration with existing ServiceError system
func (rv *RequestValidator) convertToServiceError(errors []ValidationFieldError) *services.ValidationError {
	var fieldErrors []services.FieldError

	for _, err := range errors {
		fieldErrors = append(fieldErrors, services.FieldError{
			Field:   err.Field,
			Value:   err.Value,
			Message: err.Message,
			Code:    err.Code,
		})
	}

	return services.NewDetailedValidationError("Request validation failed", fieldErrors)
}

// ===============================
// PERFORMANCE OPTIMIZATIONS
// ===============================

// ValidationCache for caching validation results
type ValidationCache struct {
	cache map[string]*ValidationResult
	ttl   time.Duration
}

// NewValidationCache creates a new validation cache
func NewValidationCache(ttl time.Duration) *ValidationCache {
	return &ValidationCache{
		cache: make(map[string]*ValidationResult),
		ttl:   ttl,
	}
}

// Advanced validation middleware with caching
func ValidateRequestWithCache(validator *RequestValidator, cache *ValidationCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validator.config.EnableCaching {
				// Generate cache key based on request content
				cacheKey := generateValidationCacheKey(r)

				// Check cache first
				if result, found := cache.cache[cacheKey]; found {
					if !result.IsValid {
						validator.writeValidationErrors(w, result.Errors)
						return
					}
					// Continue with cached valid result
					next.ServeHTTP(w, r)
					return
				}
			}

			// Fallback to regular validation
			ValidateRequest(validator)(next).ServeHTTP(w, r)
		})
	}
}

// generateValidationCacheKey generates a cache key for validation results
func generateValidationCacheKey(r *http.Request) string {
	// Simple implementation - could be enhanced with better hashing
	return fmt.Sprintf("%s:%s:%s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
}
