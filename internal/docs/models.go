package docs

import "time"

// APIResponse is the standard response format for all API responses
type APIResponse struct {
    Success   bool        `json:"success" example:"true"`
    Message   string      `json:"message" example:"Operation successful"`
    Data      interface{} `json:"data,omitempty"`
    Error     string      `json:"error,omitempty"`
    RequestID string      `json:"request_id" example:"req_123456789"`
}

// PaginationResponse is the standard response format for paginated data
type PaginationResponse struct {
    APIResponse
    Pagination PaginationMeta `json:"pagination"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
    CurrentPage int64 `json:"current_page" example:"1"`
    PerPage     int64 `json:"per_page" example:"20"`
    TotalPages  int64 `json:"total_pages" example:"5"`
    TotalCount  int64 `json:"total_count" example:"95"`
    HasNext     bool  `json:"has_next" example:"true"`
    HasPrev     bool  `json:"has_prev" example:"false"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
    Code    int    `json:"code" example:"400"`
    Message string `json:"message" example:"Invalid request parameters"`
    Details string `json:"details,omitempty" example:"Field 'email' is required"`
}

// ValidationError represents a validation error response
type ValidationError struct {
    Field   string `json:"field" example:"email"`
    Message string `json:"message" example:"must be a valid email address"`
    Tag     string `json:"tag,omitempty" example:"email"`
}

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
    Status    string            `json:"status" example:"healthy"`
    Timestamp time.Time         `json:"timestamp" example:"2024-01-15T10:30:00Z"`
    Services  map[string]string `json:"services,omitempty"`
}

// EmptyResponse represents an empty successful response
type EmptyResponse struct{}
