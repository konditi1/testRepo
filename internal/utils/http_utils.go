package utils

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// RespondWithJSON sends a JSON response with the given status code and data
func RespondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if data != nil {
		encoder := json.NewEncoder(w)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return
		}
	}
}

// RespondWithError sends a JSON error response with the given status code and message
func RespondWithError(w http.ResponseWriter, statusCode int, message string) {
	RespondWithJSON(w, statusCode, map[string]string{"error": message})
}

// GetPaginationParams extracts pagination parameters from the request query string
func GetPaginationParams(r *http.Request) (limit, offset int, err error) {
	// Default values
	limit = 20
	offset = 0	

	// Get limit from query params
	limitStr := r.URL.Query().Get("limit")
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, ErrInvalidPagination
		}
	}

	// Get offset from query params
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return 0, 0, ErrInvalidPagination
		}
	}

	return limit, offset, nil
}

// Error messages
var (
	ErrInvalidPagination = &ErrorResponse{
		Code:    http.StatusBadRequest,
		Message: "Invalid pagination parameters",
	}
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface for ErrorResponse
func (e *ErrorResponse) Error() string {
	return e.Message
}
