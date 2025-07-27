package contextutils

import "context"

type contextKey string

const (
    requestIDKey contextKey = "request_id"
    userIDKey   contextKey = "user_id"
)

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}

// WithRequestID adds the request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
    return context.WithValue(ctx, requestIDKey, requestID)
}

// GetUserID retrieves the user ID from the context
func GetUserID(ctx context.Context) int64 {
    if id, ok := ctx.Value(userIDKey).(int64); ok {
        return id
    }
    return 0
}

// WithUserID adds the user ID to the context
func WithUserID(ctx context.Context, userID int64) context.Context {
    return context.WithValue(ctx, userIDKey, userID)
}
