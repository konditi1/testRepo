// Package responseutil provides utility functions for response handling.
package responseutil

import (
	"context"
	"net/http"
)

// ResponseBuilder defines the interface for response building functionality
type ResponseBuilder interface {
	// WriteError writes an error response with appropriate status code
	WriteError(w http.ResponseWriter, r *http.Request, err error)
}

type contextKey string

const (
	// ResponseBuilderKey is the key used to store the response builder in the context.
	ResponseBuilderKey contextKey = "response_builder"
)

// GetBuilder extracts the response builder from the context.
// Returns nil if no builder is found in the context.
func GetBuilder(ctx context.Context) interface{} {
	if builder := ctx.Value(ResponseBuilderKey); builder != nil {
		return builder
	}
	return nil
}

// SetBuilder stores a response builder in the context.
// Returns a new context with the builder set.
func SetBuilder(ctx context.Context, builder interface{}) context.Context {
	return context.WithValue(ctx, ResponseBuilderKey, builder)
}
