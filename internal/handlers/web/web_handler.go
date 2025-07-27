// file: internal/handlers/web/web_handler.go
package web

import (
	"evalhub/internal/services"
	"go.uber.org/zap"
)

// WebHandler holds dependencies for web handlers
type WebHandler struct {
	serviceCollection *services.ServiceCollection
	logger           *zap.Logger
}

// NewWebHandler creates a new web handler with proper dependencies
func NewWebHandler(serviceCollection *services.ServiceCollection, logger *zap.Logger) *WebHandler {
	return &WebHandler{
		serviceCollection: serviceCollection,
		logger:           logger,
	}
}

// GetAuthService returns the auth service (same as API controllers)
func (h *WebHandler) GetAuthService() services.AuthService {
	return h.serviceCollection.GetAuthService()
}

// GetUserService returns the user service
func (h *WebHandler) GetUserService() services.UserService {
	return h.serviceCollection.GetUserService()
}

// PostService accessor
func (h *WebHandler) GetPostService() services.PostService {
	return h.serviceCollection.GetPostService()
}

// CommentService accessor
func (h *WebHandler) GetCommentService() services.CommentService {
	return h.serviceCollection.GetCommentService()
}

// JobService accessor
func (h *WebHandler) GetJobService() services.JobService {
	return h.serviceCollection.GetJobService()
}

// FileService accessor
func (h *WebHandler) GetFileService() services.FileService {
	return h.serviceCollection.GetFileService()
}

// CacheService accessor
func (h *WebHandler) GetCacheService() services.CacheService {
	return h.serviceCollection.GetCacheService()
}

// EventService accessor
func (h *WebHandler) GetEventService() services.EventService {
	return h.serviceCollection.GetEventService()
}

// TransactionService accessor
func (h *WebHandler) GetTransactionService() services.TransactionService {
	return h.serviceCollection.GetTransactionService()
}

// Logger accessor (optional but useful)
func (h *WebHandler) GetLogger() *zap.Logger {
	return h.logger
}


// Service Collection access for complex operations
func (h *WebHandler) GetServiceCollection() *services.ServiceCollection {
	return h.serviceCollection
}