package services

import (
	"fmt"
	"net/http"
)

// ===============================
// ERROR TYPES
// ===============================

// ServiceError represents a structured service error
type ServiceError struct {
	Type       string                 `json:"type"`
	Message    string                 `json:"message"`
	Code       string                 `json:"code,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	StatusCode int                    `json:"-"`
	Cause      error                  `json:"-"`
}

// Error implements the error interface
func (e *ServiceError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error
func (e *ServiceError) Unwrap() error {
	return e.Cause
}

// GetStatusCode returns the HTTP status code for this error
func (e *ServiceError) GetStatusCode() int {
	if e.StatusCode > 0 {
		return e.StatusCode
	}
	return http.StatusInternalServerError
}

// ===============================
// ERROR CONSTRUCTORS
// ===============================

// NewValidationError creates a validation error
func NewValidationError(message string, cause error) *ServiceError {
	return &ServiceError{
		Type:       "VALIDATION_ERROR",
		Message:    message,
		StatusCode: http.StatusBadRequest,
		Cause:      cause,
	}
}

// NewBusinessError creates a business logic error
func NewBusinessError(message, code string) *ServiceError {
	return &ServiceError{
		Type:       "BUSINESS_ERROR",
		Message:    message,
		Code:       code,
		StatusCode: http.StatusUnprocessableEntity,
	}
}

// NewNotFoundError creates a not found error
func NewNotFoundError(message string) *ServiceError {
	return &ServiceError{
		Type:       "NOT_FOUND",
		Message:    message,
		StatusCode: http.StatusNotFound,
	}
}

// NewUnauthorizedError creates an unauthorized error
func NewUnauthorizedError(message string) *ServiceError {
	return &ServiceError{
		Type:       "UNAUTHORIZED",
		Message:    message,
		StatusCode: http.StatusUnauthorized,
	}
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(message string) *ServiceError {
	return &ServiceError{
		Type:       "FORBIDDEN",
		Message:    message,
		StatusCode: http.StatusForbidden,
	}
}

// NewConflictError creates a conflict error
func NewConflictError(message, code string) *ServiceError {
	return &ServiceError{
		Type:       "CONFLICT",
		Message:    message,
		Code:       code,
		StatusCode: http.StatusConflict,
	}
}

// NewRateLimitError creates a rate limit error
func NewRateLimitError(message string, details map[string]interface{}) *ServiceError {
	return &ServiceError{
		Type:       "RATE_LIMIT",
		Message:    message,
		Details:    details,
		StatusCode: http.StatusTooManyRequests,
	}
}

// NewInternalError creates an internal server error
func NewInternalError(message string) *ServiceError {
	return &ServiceError{
		Type:       "INTERNAL_ERROR",
		Message:    message,
		StatusCode: http.StatusInternalServerError,
	}
}

// NewNotImplementedError creates a not implemented error
func NewNotImplementedError(message string) *ServiceError {
	return &ServiceError{
		Type:       "NOT_IMPLEMENTED",
		Message:    message,
		StatusCode: http.StatusNotImplemented,
	}
}

// NewServiceUnavailableError creates a service unavailable error
func NewServiceUnavailableError(message string) *ServiceError {
	return &ServiceError{
		Type:       "SERVICE_UNAVAILABLE",
		Message:    message,
		StatusCode: http.StatusServiceUnavailable,
	}
}

// ===============================
// SPECIALIZED ERRORS
// ===============================

// AuthenticationError represents authentication-related errors
type AuthenticationError struct {
	*ServiceError
	UserID   *int64 `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// NewAuthenticationError creates an authentication error
func NewAuthenticationError(message, reason string, userID *int64, username string) *AuthenticationError {
	return &AuthenticationError{
		ServiceError: &ServiceError{
			Type:       "AUTHENTICATION_ERROR",
			Message:    message,
			StatusCode: http.StatusUnauthorized,
		},
		UserID:   userID,
		Username: username,
		Reason:   reason,
	}
}

// AuthorizationError represents authorization-related errors
type AuthorizationError struct {
	*ServiceError
	UserID       int64    `json:"user_id"`
	Resource     string   `json:"resource"`
	Action       string   `json:"action"`
	RequiredRole string   `json:"required_role,omitempty"`
	UserRoles    []string `json:"user_roles,omitempty"`
}

// NewAuthorizationError creates an authorization error
func NewAuthorizationError(message, resource, action string, userID int64) *AuthorizationError {
	return &AuthorizationError{
		ServiceError: &ServiceError{
			Type:       "AUTHORIZATION_ERROR",
			Message:    message,
			StatusCode: http.StatusForbidden,
		},
		UserID:   userID,
		Resource: resource,
		Action:   action,
	}
}

// ValidationError represents detailed validation errors
type ValidationError struct {
	*ServiceError
	Fields []FieldError `json:"fields,omitempty"`
}

// FieldError represents a single field validation error
type FieldError struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value,omitempty"`
	Message string      `json:"message"`
	Code    string      `json:"code"`
}

// NewDetailedValidationError creates a validation error with field details
func NewDetailedValidationError(message string, fields []FieldError) *ValidationError {
	return &ValidationError{
		ServiceError: &ServiceError{
			Type:       "VALIDATION_ERROR",
			Message:    message,
			StatusCode: http.StatusBadRequest,
		},
		Fields: fields,
	}
}

// ===============================
// ERROR UTILITIES
// ===============================

// IsServiceError checks if an error is a ServiceError
func IsServiceError(err error) bool {
	_, ok := err.(*ServiceError)
	return ok
}

// GetServiceError extracts a ServiceError from an error, or creates a generic one
func GetServiceError(err error) *ServiceError {
	if serviceErr, ok := err.(*ServiceError); ok {
		return serviceErr
	}
	
	if authErr, ok := err.(*AuthenticationError); ok {
		return authErr.ServiceError
	}
	
	if authzErr, ok := err.(*AuthorizationError); ok {
		return authzErr.ServiceError
	}
	
	if valErr, ok := err.(*ValidationError); ok {
		return valErr.ServiceError
	}
	
	// Create a generic internal error
	return NewInternalError(err.Error())
}

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errorType string) bool {
	if serviceErr := GetServiceError(err); serviceErr != nil {
		return serviceErr.Type == errorType
	}
	return false
}

// IsNotFoundError checks if an error is a not found error
func IsNotFoundError(err error) bool {
	return IsErrorType(err, "NOT_FOUND")
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	return IsErrorType(err, "VALIDATION_ERROR")
}

// IsAuthenticationError checks if an error is an authentication error
func IsAuthenticationError(err error) bool {
	return IsErrorType(err, "AUTHENTICATION_ERROR")
}

// IsAuthorizationError checks if an error is an authorization error
func IsAuthorizationError(err error) bool {
	return IsErrorType(err, "AUTHORIZATION_ERROR")
}

// IsBusinessError checks if an error is a business logic error
func IsBusinessError(err error) bool {
	return IsErrorType(err, "BUSINESS_ERROR")
}

// ===============================
// ERROR RESPONSE BUILDERS
// ===============================

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error     *ServiceError `json:"error"`
	RequestID string        `json:"request_id,omitempty"`
	Timestamp string        `json:"timestamp"`
	Path      string        `json:"path,omitempty"`
}

// BuildErrorResponse creates a standardized error response
func BuildErrorResponse(err error, requestID, path string) *ErrorResponse {
	return &ErrorResponse{
		Error:     GetServiceError(err),
		RequestID: requestID,
		Timestamp: fmt.Sprintf("%d", GetCurrentTimestamp()),
		Path:      path,
	}
}

// GetCurrentTimestamp returns the current Unix timestamp
func GetCurrentTimestamp() int64 {
	return GetCurrentTime().Unix()
}

// GetCurrentTime returns the current time (mockable for testing)
var GetCurrentTime = func() TimeInterface {
	return &RealTime{}
}

// TimeInterface allows for time mocking in tests
type TimeInterface interface {
	Unix() int64
}

// RealTime implements TimeInterface using real time
type RealTime struct{}

// Unix returns the current Unix timestamp
func (r *RealTime) Unix() int64 {
	return GetCurrentTimestamp()
}

// ===============================
// ERROR AGGREGATION
// ===============================

// ErrorGroup represents multiple errors
type ErrorGroup struct {
	Errors []error `json:"errors"`
	Type   string  `json:"type"`
}

// Error implements the error interface
func (eg *ErrorGroup) Error() string {
	if len(eg.Errors) == 0 {
		return "no errors"
	}
	if len(eg.Errors) == 1 {
		return eg.Errors[0].Error()
	}
	return fmt.Sprintf("multiple errors (%d total)", len(eg.Errors))
}

// Add adds an error to the group
func (eg *ErrorGroup) Add(err error) {
	if err != nil {
		eg.Errors = append(eg.Errors, err)
	}
}

// HasErrors returns true if there are any errors
func (eg *ErrorGroup) HasErrors() bool {
	return len(eg.Errors) > 0
}

// GetFirst returns the first error or nil
func (eg *ErrorGroup) GetFirst() error {
	if len(eg.Errors) > 0 {
		return eg.Errors[0]
	}
	return nil
}

// ToServiceError converts the error group to a service error
func (eg *ErrorGroup) ToServiceError() *ServiceError {
	if !eg.HasErrors() {
		return nil
	}
	
	if len(eg.Errors) == 1 {
		return GetServiceError(eg.Errors[0])
	}
	
	details := make(map[string]interface{})
	details["error_count"] = len(eg.Errors)
	details["errors"] = eg.Errors
	
	return &ServiceError{
		Type:       "MULTIPLE_ERRORS",
		Message:    fmt.Sprintf("Multiple errors occurred (%d total)", len(eg.Errors)),
		Details:    details,
		StatusCode: http.StatusBadRequest,
	}
}

// NewErrorGroup creates a new error group
func NewErrorGroup(errorType string) *ErrorGroup {
	return &ErrorGroup{
		Errors: make([]error, 0),
		Type:   errorType,
	}
}

// ===============================
// ERROR CONTEXT
// ===============================

// ErrorContext provides additional context for errors
type ErrorContext struct {
	UserID    *int64                 `json:"user_id,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Operation string                 `json:"operation,omitempty"`
	Resource  string                 `json:"resource,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WithContext adds context to a service error
func (e *ServiceError) WithContext(ctx *ErrorContext) *ServiceError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	
	if ctx.UserID != nil {
		e.Details["user_id"] = *ctx.UserID
	}
	if ctx.RequestID != "" {
		e.Details["request_id"] = ctx.RequestID
	}
	if ctx.Operation != "" {
		e.Details["operation"] = ctx.Operation
	}
	if ctx.Resource != "" {
		e.Details["resource"] = ctx.Resource
	}
	if ctx.Metadata != nil {
		for k, v := range ctx.Metadata {
			e.Details[k] = v
		}
	}
	
	return e
}

// ===============================
// COMMON ERROR PATTERNS
// ===============================

// EntityNotFoundError creates a standard entity not found error
func EntityNotFoundError(entityType string, id interface{}) *ServiceError {
	return NewNotFoundError(fmt.Sprintf("%s not found", entityType)).WithContext(&ErrorContext{
		Resource: entityType,
		Metadata: map[string]interface{}{
			"id": id,
		},
	})
}

// EntityAlreadyExistsError creates a standard entity already exists error
func EntityAlreadyExistsError(entityType string, field, value string) *ServiceError {
	return NewConflictError(
		fmt.Sprintf("%s already exists", entityType),
		"ENTITY_ALREADY_EXISTS",
	).WithContext(&ErrorContext{
		Resource: entityType,
		Metadata: map[string]interface{}{
			"field": field,
			"value": value,
		},
	})
}

// InsufficientPermissionsError creates a standard permissions error
func InsufficientPermissionsError(action, resource string) *ServiceError {
	return NewForbiddenError(fmt.Sprintf("Insufficient permissions to %s %s", action, resource)).WithContext(&ErrorContext{
		Operation: action,
		Resource:  resource,
	})
}

// InvalidInputError creates a standard invalid input error
func InvalidInputError(field, reason string) *ServiceError {
	return NewValidationError(fmt.Sprintf("Invalid input for field '%s': %s", field, reason), nil).WithContext(&ErrorContext{
		Metadata: map[string]interface{}{
			"field":  field,
			"reason": reason,
		},
	})
}
