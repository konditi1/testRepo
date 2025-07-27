// ===============================
// FILE: internal/handlers/api/v1/comments/comments_controller.go
// ===============================

package comments

import (
	"context"
	"encoding/json"
	"evalhub/internal/middleware"
	"evalhub/internal/models"
	"evalhub/internal/response"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CommentController handles comment API endpoints with enterprise features
type CommentController struct {
	serviceCollection *services.ServiceCollection
	logger            *zap.Logger
	responseBuilder   *response.Builder
	paginationParser  *response.PaginationParser
	paginationBuilder *response.PaginationBuilder
}

// NewCommentController creates a new enterprise comment controller
func NewCommentController(
	serviceCollection *services.ServiceCollection,
	logger *zap.Logger,
	responseBuilder *response.Builder,
) *CommentController {
	return &CommentController{
		serviceCollection: serviceCollection,
		logger:            logger,
		responseBuilder:   responseBuilder,
		paginationParser:  response.NewPaginationParser(response.DefaultPaginationConfig()),
		paginationBuilder: response.NewPaginationBuilder(response.DefaultPaginationConfig()),
	}
}

// ===============================
// CORE CRUD OPERATIONS
// ===============================

// CreateComment handles POST /api/v1/comments
func (c *CommentController) CreateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse request body
	var req services.CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.logger.Warn("Failed to decode create comment request", zap.Error(err))
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body format",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Set user ID from auth context
	req.UserID = authCtx.UserID

	// Validate request structure (business validation happens in service)
	if err := c.validateCreateCommentRequest(&req); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment data",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Content security validation (enhanced for MT-13)
	if err := c.validateContentSecurity(req.Content); err != nil {
		c.logger.Warn("Content security validation failed",
			zap.Error(err),
			zap.Int64("user_id", authCtx.UserID),
			zap.String("content_preview", c.truncateContent(req.Content, 50)),
		)
		businessErr := services.NewBusinessError(
			"Content validation failed",
			"CONTENT_REJECTED",
		)
		businessErr.StatusCode = response.StatusUnprocessableEntity
		c.responseBuilder.WriteError(w, r, businessErr)
		return
	}

	// Create comment using service
	commentService := c.serviceCollection.GetCommentService()
	comment, err := commentService.CreateComment(ctx, &req)
	if err != nil {
		c.handleServiceError(w, r, err, "create comment")
		return
	}

	// Log successful creation
	c.logger.Info("Comment created successfully via API",
		zap.Int64("comment_id", comment.ID),
		zap.Int64("user_id", authCtx.UserID),
		zap.Any("post_id", comment.PostID),
		zap.Any("question_id", comment.QuestionID),
		zap.Any("document_id", comment.DocumentID),
	)

	c.responseBuilder.WriteCreated(w, r, comment)
}

// GetComment handles GET /api/v1/comments/{id}
func (c *CommentController) GetComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Get comment using service
	commentService := c.serviceCollection.GetCommentService()
	var userID *int64
	if authCtx != nil {
		userID = &authCtx.UserID
	}

	comment, err := commentService.GetCommentByID(ctx, commentID, userID)
	if err != nil {
		c.handleServiceError(w, r, err, "get comment")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, comment)
}

// UpdateComment handles PUT /api/v1/comments/{id}
func (c *CommentController) UpdateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse request body
	var req services.UpdateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.logger.Warn("Failed to decode update comment request", zap.Error(err))
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body format",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Set IDs from context and URL
	req.CommentID = commentID
	req.UserID = authCtx.UserID

	// Content security validation
	if err := c.validateContentSecurity(req.Content); err != nil {
		c.logger.Warn("Content security validation failed on update",
			zap.Error(err),
			zap.Int64("comment_id", commentID),
			zap.Int64("user_id", authCtx.UserID),
		)
		businessErr := services.NewBusinessError(
			"Content validation failed",
			"CONTENT_REJECTED",
		)
		businessErr.StatusCode = response.StatusUnprocessableEntity
		c.responseBuilder.WriteError(w, r, businessErr)
		return
	}

	// Check permissions (owner can edit, admin/moderator can edit any)
	if !c.canUserModifyComment(ctx, commentID, authCtx) {
		authErr := &services.AuthorizationError{
			ServiceError: &services.ServiceError{
				Type:       "AUTHORIZATION_ERROR",
				Message:    "Insufficient permissions to update comment",
				StatusCode: response.StatusForbidden,
			},
		}
		c.responseBuilder.WriteError(w, r, authErr)
		return
	}

	// Update comment using service
	commentService := c.serviceCollection.GetCommentService()
	comment, err := commentService.UpdateComment(ctx, &req)
	if err != nil {
		c.handleServiceError(w, r, err, "update comment")
		return
	}

	c.logger.Info("Comment updated successfully via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", authCtx.UserID),
	)

	c.responseBuilder.WriteSuccess(w, r, comment)
}

// DeleteComment handles DELETE /api/v1/comments/{id}
func (c *CommentController) DeleteComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Check permissions (owner can delete, admin/moderator can delete any)
	if !c.canUserModifyComment(ctx, commentID, authCtx) {
		authErr := &services.AuthorizationError{
			ServiceError: &services.ServiceError{
				Type:       "AUTHORIZATION_ERROR",
				Message:    "Insufficient permissions to delete comment",
				StatusCode: response.StatusForbidden,
			},
		}
		c.responseBuilder.WriteError(w, r, authErr)
		return
	}

	// Delete comment using service
	commentService := c.serviceCollection.GetCommentService()
	err = commentService.DeleteComment(ctx, commentID, authCtx.UserID)
	if err != nil {
		c.handleServiceError(w, r, err, "delete comment")
		return
	}

	c.logger.Info("Comment deleted successfully via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", authCtx.UserID),
	)

	c.responseBuilder.WriteNoContent(w, r)
}

// ===============================
// LISTING OPERATIONS
// ===============================

// GetCommentsByPost handles GET /api/v1/comments/post/{postId}
func (c *CommentController) GetCommentsByPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Extract post ID from URL
	postID, err := c.extractIDFromPath(r.URL.Path, 5)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid post ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse pagination parameters using new pagination system
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams for service compatibility
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request
	req := &services.GetCommentsByPostRequest{
		PostID:     postID,
		Pagination: modelsPagination,
	}
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	// Get comments using service
	commentService := c.serviceCollection.GetCommentService()
	serviceResponse, err := commentService.GetCommentsByPost(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "get comments by post")
		return
	}

	// Extract pagination info and write paginated response
	c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
}

// GetCommentsByQuestion handles GET /api/v1/comments/question/{questionId}
func (c *CommentController) GetCommentsByQuestion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Extract question ID from URL
	questionID, err := c.extractIDFromPath(r.URL.Path, 5)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid question ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams for service compatibility
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request
	req := &services.GetCommentsByQuestionRequest{
		QuestionID: questionID,
		Pagination: modelsPagination,
	}
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	// Get comments using service
	commentService := c.serviceCollection.GetCommentService()
	serviceResponse, err := commentService.GetCommentsByQuestion(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "get comments by question")
		return
	}

	// Write paginated response
	c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
}

// GetCommentsByDocument handles GET /api/v1/comments/document/{docId}
func (c *CommentController) GetCommentsByDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Extract document ID from URL
	documentID, err := c.extractIDFromPath(r.URL.Path, 5)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid document ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams for service compatibility
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request
	req := &services.GetCommentsByDocumentRequest{
		DocumentID: documentID,
		Pagination: modelsPagination,
	}
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	// Get comments using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if documentService, ok := commentService.(interface {
		GetCommentsByDocument(context.Context, *services.GetCommentsByDocumentRequest) (*models.PaginatedResponse[*models.Comment], error)
	}); ok {
		serviceResponse, err := documentService.GetCommentsByDocument(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "get comments by document")
			return
		}
		c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
	} else {
		notFoundErr := &services.ServiceError{
			Type:       "NOT_IMPLEMENTED",
			Message:    "Document comments feature not implemented",
			StatusCode: response.StatusNotImplemented,
		}
		c.responseBuilder.WriteError(w, r, notFoundErr)
	}
}

// GetCommentsByUser handles GET /api/v1/comments/user/{userId}
func (c *CommentController) GetCommentsByUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Extract user ID from URL
	targetUserID, err := c.extractIDFromPath(r.URL.Path, 5)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid user ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams for service compatibility
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Add user ID to context if authenticated
	if authCtx != nil {
		ctx = context.WithValue(ctx, "user_id", authCtx.UserID)
	}

	// Build request
	req := &services.GetCommentsByUserRequest{
		TargetUserID: targetUserID,
		Pagination:   modelsPagination,
	}

	// Get comments using service
	commentService := c.serviceCollection.GetCommentService()
	serviceResponse, err := commentService.GetCommentsByUser(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "get comments by user")
		return
	}

	c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
}

// SearchComments handles GET /api/v1/comments/search
func (c *CommentController) SearchComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Parse search parameters
	query := r.URL.Query().Get("q")
	if query == "" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Search query parameter 'q' is required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams for service compatibility
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Additional search filters
	var postID, questionID, documentID *int64
	if postIDStr := r.URL.Query().Get("post_id"); postIDStr != "" {
		if id, err := strconv.ParseInt(postIDStr, 10, 64); err == nil {
			postID = &id
		}
	}
	if questionIDStr := r.URL.Query().Get("question_id"); questionIDStr != "" {
		if id, err := strconv.ParseInt(questionIDStr, 10, 64); err == nil {
			questionID = &id
		}
	}
	if documentIDStr := r.URL.Query().Get("document_id"); documentIDStr != "" {
		if id, err := strconv.ParseInt(documentIDStr, 10, 64); err == nil {
			documentID = &id
		}
	}

	// Add user ID to context if authenticated
	if authCtx != nil {
		ctx = context.WithValue(ctx, "user_id", authCtx.UserID)
	}

	// Build search request
	req := &services.SearchCommentsRequest{
		Query:      query,
		PostID:     postID,
		QuestionID: questionID,
		DocumentID: documentID,
		Pagination: modelsPagination,
	}

	// Search comments using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if searchService, ok := commentService.(interface {
		SearchComments(context.Context, *services.SearchCommentsRequest) (*models.PaginatedResponse[*models.Comment], error)
	}); ok {
		serviceResponse, err := searchService.SearchComments(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "search comments")
			return
		}
		c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
	} else {
		notFoundErr := &services.ServiceError{
			Type:       "NOT_IMPLEMENTED",
			Message:    "Comment search feature not implemented",
			StatusCode: response.StatusNotImplemented,
		}
		c.responseBuilder.WriteError(w, r, notFoundErr)
	}
}


// GetTrendingComments handles GET /api/v1/comments/trending
func (c *CommentController) GetTrendingComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse time range (default to 24h)
	timeRangeStr := r.URL.Query().Get("range")
	if timeRangeStr == "" {
		timeRangeStr = "24h"
	}

	// Parse time range string to TimeRange object
	timeRange, err := c.parseTimeRange(timeRangeStr, time.Now())
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "INVALID_TIME_RANGE",
				Message:    fmt.Sprintf("Invalid time range format: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request for trending comments
	req := &services.GetTrendingCommentsRequest{
		TimeRange:  timeRange,
		Pagination: modelsPagination,
	}
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	// Get trending comments using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if trendingService, ok := commentService.(interface {
		GetTrendingComments(context.Context, *services.GetTrendingCommentsRequest) (*models.PaginatedResponse[*models.Comment], error)
	}); ok {
		serviceResponse, err := trendingService.GetTrendingComments(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "get trending comments")
			return
		}
		c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
	} else {
		// Fallback implementation using existing methods
		c.getTrendingCommentsFallback(w, r, paginationParams)
	}
}

// GetRecentComments handles GET /api/v1/comments/recent
func (c *CommentController) GetRecentComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Convert to models.PaginationParams
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request for recent comments
	req := &services.GetRecentCommentsRequest{
		Pagination: modelsPagination,
	}
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	// Get recent comments using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if recentService, ok := commentService.(interface {
		GetRecentComments(context.Context, *services.GetRecentCommentsRequest) (*models.PaginatedResponse[*models.Comment], error)
	}); ok {
		serviceResponse, err := recentService.GetRecentComments(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "get recent comments")
			return
		}
		c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
	} else {
		// Fallback implementation
		c.getRecentCommentsFallback(w, r, paginationParams)
	}
}

// GetModerationQueue handles GET /api/v1/comments/moderation/queue (Admin/Moderator only)
func (c *CommentController) GetModerationQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Verify moderator/admin role
	if authCtx.Role != "admin" && authCtx.Role != "moderator" {
		c.responseBuilder.WriteForbidden(w, r, "Moderator or admin role required")
		return
	}

	// Parse pagination parameters
	paginationParams, err := c.paginationParser.ParseFromRequest(r)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse filter parameters
	status := r.URL.Query().Get("status") // pending, reviewed, approved, rejected
	priority := r.URL.Query().Get("priority") // high, medium, low

	// Convert to models.PaginationParams
	modelsPagination := c.convertToModelsPagination(paginationParams)

	// Build request for moderation queue
	req := &services.GetModerationQueueRequest{
		ModeratorID: authCtx.UserID,
		Status:      &status,
		Priority:    &priority,
		Pagination:  modelsPagination,
	}

	// Get moderation queue using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if queueService, ok := commentService.(interface {
		GetModerationQueue(context.Context, *services.GetModerationQueueRequest) (*models.PaginatedResponse[*models.Comment], error)
	}); ok {
		serviceResponse, err := queueService.GetModerationQueue(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "get moderation queue")
			return
		}
		c.writePaginatedResponse(w, r, serviceResponse, paginationParams)
	} else {
		// Fallback implementation
		c.getModerationQueueFallback(w, r, paginationParams)
	}
}

// ===============================
// FALLBACK IMPLEMENTATIONS
// ===============================

// getTrendingCommentsFallback provides fallback when service method doesn't exist
func (c *CommentController) getTrendingCommentsFallback(w http.ResponseWriter, r *http.Request, paginationParams *response.PaginationParams) {
	// Simple fallback: return recent comments with high engagement
	// This is a basic implementation - you can enhance it later
	fallbackData := map[string]interface{}{
		"message": "Trending comments feature is coming soon",
		"data":    []interface{}{},
		"meta": map[string]interface{}{
			"total":        0,
			"page":         paginationParams.Page,
			"per_page":     paginationParams.PageSize,
			"total_pages":  0,
			"has_next":     false,
			"has_previous": false,
		},
	}
	c.responseBuilder.WriteSuccess(w, r, fallbackData)
}

// getRecentCommentsFallback provides fallback when service method doesn't exist
func (c *CommentController) getRecentCommentsFallback(w http.ResponseWriter, r *http.Request, paginationParams *response.PaginationParams) {
	// Simple fallback: return message about feature development
	fallbackData := map[string]interface{}{
		"message": "Recent comments feature is coming soon",
		"data":    []interface{}{},
		"meta": map[string]interface{}{
			"total":        0,
			"page":         paginationParams.Page,
			"per_page":     paginationParams.PageSize,
			"total_pages":  0,
			"has_next":     false,
			"has_previous": false,
		},
	}
	c.responseBuilder.WriteSuccess(w, r, fallbackData)
}

// getModerationQueueFallback provides fallback when service method doesn't exist
func (c *CommentController) getModerationQueueFallback(w http.ResponseWriter, r *http.Request, paginationParams *response.PaginationParams) {
	// Simple fallback: return message about feature development
	fallbackData := map[string]interface{}{
		"message": "Moderation queue feature is coming soon",
		"data":    []interface{}{},
		"meta": map[string]interface{}{
			"total":        0,
			"page":         paginationParams.Page,
			"per_page":     paginationParams.PageSize,
			"total_pages":  0,
			"has_next":     false,
			"has_previous": false,
		},
	}
	c.responseBuilder.WriteSuccess(w, r, fallbackData)
}

// ===============================
// ENGAGEMENT OPERATIONS
// ===============================

// ReactToComment handles POST /api/v1/comments/{id}/react
func (c *CommentController) ReactToComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse reaction type from request body
	var reactionReq struct {
		ReactionType string `json:"reaction_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reactionReq); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body format",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Validate reaction type
	if reactionReq.ReactionType != "like" && reactionReq.ReactionType != "dislike" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Reaction type must be 'like' or 'dislike'",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build request
	req := &services.ReactToCommentRequest{
		CommentID:    commentID,
		UserID:       authCtx.UserID,
		ReactionType: reactionReq.ReactionType,
	}

	// React to comment using service
	commentService := c.serviceCollection.GetCommentService()
	err = commentService.ReactToComment(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "react to comment")
		return
	}

	c.logger.Info("User reacted to comment via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", authCtx.UserID),
		zap.String("reaction_type", reactionReq.ReactionType),
	)

	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":       "Reaction added successfully",
		"comment_id":    commentID,
		"reaction_type": reactionReq.ReactionType,
	})
}

// RemoveCommentReaction handles DELETE /api/v1/comments/{id}/react
func (c *CommentController) RemoveCommentReaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Remove reaction using service
	commentService := c.serviceCollection.GetCommentService()
	err = commentService.RemoveCommentReaction(ctx, commentID, authCtx.UserID)
	if err != nil {
		c.handleServiceError(w, r, err, "remove comment reaction")
		return
	}

	c.logger.Info("User removed reaction from comment via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", authCtx.UserID),
	)

	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":    "Reaction removed successfully",
		"comment_id": commentID,
	})
}

// ===============================
// MODERATION OPERATIONS
// ===============================

// ReportComment handles POST /api/v1/comments/{id}/report
func (c *CommentController) ReportComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse report request
	var reportReq struct {
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reportReq); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body format",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Validate report reason
	if reportReq.Reason == "" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Report reason is required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build request
	req := &services.ReportContentRequest{
		ContentID:   commentID,
		ReporterID:  authCtx.UserID,
		Reason:      reportReq.Reason,
		Description: reportReq.Description,
	}

	// Report comment using service
	commentService := c.serviceCollection.GetCommentService()
	err = commentService.ReportComment(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "report comment")
		return
	}

	c.logger.Info("Comment reported via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("reporter_id", authCtx.UserID),
		zap.String("reason", reportReq.Reason),
	)

	c.responseBuilder.WriteAccepted(w, r, "Comment reported successfully and will be reviewed")
}

// ModerateComment handles POST /api/v1/comments/{id}/moderate (Admin/Moderator only)
func (c *CommentController) ModerateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Verify moderator/admin role
	if authCtx.Role != "admin" && authCtx.Role != "moderator" {
		c.responseBuilder.WriteForbidden(w, r, "Moderator or admin role required")
		return
	}

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Parse moderation request
	var moderationReq struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&moderationReq); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body format",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Validate moderation action
	validActions := []string{"approve", "reject", "delete", "flag", "warn"}
	isValidAction := false
	for _, action := range validActions {
		if moderationReq.Action == action {
			isValidAction = true
			break
		}
	}
	if !isValidAction {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid moderation action. Must be one of: approve, reject, delete, flag, warn",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build request
	req := &services.ModerateContentRequest{
		ContentID:   commentID,
		ModeratorID: authCtx.UserID,
		Action:      moderationReq.Action,
		Reason:      moderationReq.Reason,
	}

	// Moderate comment using service
	commentService := c.serviceCollection.GetCommentService()
	err = commentService.ModerateComment(ctx, req)
	if err != nil {
		c.handleServiceError(w, r, err, "moderate comment")
		return
	}

	c.logger.Info("Comment moderated via API",
		zap.Int64("comment_id", commentID),
		zap.Int64("moderator_id", authCtx.UserID),
		zap.String("action", moderationReq.Action),
		zap.String("moderator_role", authCtx.Role),
	)

	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":    "Comment moderated successfully",
		"comment_id": commentID,
		"action":     moderationReq.Action,
	})
}

// ===============================
// ANALYTICS OPERATIONS
// ===============================

// GetCommentStats handles GET /api/v1/comments/{id}/stats
func (c *CommentController) GetCommentStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract comment ID from URL
	commentID, err := c.extractIDFromPath(r.URL.Path, 4)
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid comment ID",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Get comment stats using service
	commentService := c.serviceCollection.GetCommentService()
	stats, err := commentService.GetCommentStats(ctx, commentID)
	if err != nil {
		c.handleServiceError(w, r, err, "get comment stats")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, stats)
}

// GetCommentAnalytics handles GET /api/v1/comments/analytics
func (c *CommentController) GetCommentAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse time range parameters
	timeRangeStr := r.URL.Query().Get("range")
	if timeRangeStr == "" {
		timeRangeStr = "30d" // Default to 30 days
	}

	// Convert string time range to TimeRange
	timeRange, err := c.parseTimeRange(timeRangeStr, time.Now())
	if err != nil {
		errMsg := fmt.Errorf("invalid time range format. Use format like '7d', '30d', '90d'")
		c.responseBuilder.WriteError(w, r, errMsg)
		return
	}

	// Build analytics request
	req := &services.GetCommentAnalyticsRequest{
		UserID:    authCtx.UserID,
		TimeRange: timeRange,
	}

	// Get analytics using service (check if method exists)
	commentService := c.serviceCollection.GetCommentService()
	if analyticsService, ok := commentService.(interface {
		GetCommentAnalytics(context.Context, *services.GetCommentAnalyticsRequest) (*services.CommentAnalyticsResponse, error)
	}); ok {
		analytics, err := analyticsService.GetCommentAnalytics(ctx, req)
		if err != nil {
			c.handleServiceError(w, r, err, "get comment analytics")
			return
		}
		c.responseBuilder.WriteSuccess(w, r, analytics)
	} else {
		notFoundErr := &services.ServiceError{
			Type:       "NOT_IMPLEMENTED",
			Message:    "Comment analytics feature not implemented",
			StatusCode: response.StatusNotImplemented,
		}
		c.responseBuilder.WriteError(w, r, notFoundErr)
	}
}

// ===============================
// HELPER METHODS
// ===============================

// convertToModelsPagination converts response.PaginationParams to models.PaginationParams
func (c *CommentController) convertToModelsPagination(params *response.PaginationParams) models.PaginationParams {
	return models.PaginationParams{
		Limit:  params.PageSize,
		Offset: params.Offset,
		Sort:   params.Sort,
		Order:  params.Order,
	}
}

// writePaginatedResponse writes a paginated response using the integrated systems
func (c *CommentController) writePaginatedResponse(
	w http.ResponseWriter,
	r *http.Request,
	serviceResponse interface{},
	paginationParams *response.PaginationParams,
) {
	// Extract pagination info from service response
	items, _, total, err := response.ExtractPaginationFromModels(serviceResponse)
	if err != nil {
		c.logger.Warn("Failed to extract pagination from service response", zap.Error(err))
		// Fallback to simple success response
		c.responseBuilder.WriteSuccess(w, r, serviceResponse)
		return
	}

	// Write paginated response using the response builder
	c.responseBuilder.WritePaginatedResponse(w, r, items, paginationParams, total)
}

// canUserModifyComment checks if user can modify (edit/delete) a comment
func (c *CommentController) canUserModifyComment(ctx context.Context, commentID int64, authCtx *middleware.AuthContext) bool {
	// Admin and moderator can modify any comment
	if authCtx.Role == "admin" || authCtx.Role == "moderator" {
		return true
	}

	// Get comment to check ownership
	commentService := c.serviceCollection.GetCommentService()
	comment, err := commentService.GetCommentByID(ctx, commentID, &authCtx.UserID)
	if err != nil {
		c.logger.Warn("Failed to get comment for permission check",
			zap.Error(err),
			zap.Int64("comment_id", commentID),
			zap.Int64("user_id", authCtx.UserID),
		)
		return false
	}

	// Owner can modify their own comment
	return comment.UserID == authCtx.UserID
}

// validateContentSecurity performs content security validation
func (c *CommentController) validateContentSecurity(content string) error {
	// Use existing validation from models package
	if err := models.ContentValidator("content", content, 1, 10000); err != nil {
		return err
	}

	// Additional security checks
	if c.containsSpamPatterns(content) {
		return fmt.Errorf("content contains spam patterns")
	}

	if c.checkXSS(content) {
		return fmt.Errorf("content contains potential XSS")
	}

	if c.checkSQLInjection(content) {
		return fmt.Errorf("content contains potential SQL injection")
	}

	return nil
}

// containsSpamPatterns checks for spam patterns
func (c *CommentController) containsSpamPatterns(content string) bool {
	spamPatterns := []string{
		"click here", "buy now", "limited time", "urgent",
		"money back guarantee", "100% free", "no risk", "act now",
	}

	lowerContent := strings.ToLower(content)
	for _, pattern := range spamPatterns {
		if strings.Contains(lowerContent, pattern) {
			return true
		}
	}
	return false
}

// checkXSS checks for potential XSS attacks
func (c *CommentController) checkXSS(content string) bool {
	xssPatterns := []string{
		"<script", "javascript:", "onerror=", "onload=", "onclick=",
		"eval(", "document.cookie", "window.location",
	}

	lowerContent := strings.ToLower(content)
	for _, pattern := range xssPatterns {
		if strings.Contains(lowerContent, pattern) {
			return true
		}
	}
	return false
}

// checkSQLInjection checks for potential SQL injection
func (c *CommentController) checkSQLInjection(content string) bool {
	sqlPatterns := []string{
		"union select", "or 1=1", "drop table", "delete from",
		"insert into", "update set", "exec(", "execute(",
		"--", "/*", "xp_", "sp_",
	}

	lowerContent := strings.ToLower(content)
	for _, pattern := range sqlPatterns {
		if strings.Contains(lowerContent, pattern) {
			return true
		}
	}
	return false
}

// validateCreateCommentRequest validates create comment request structure
func (c *CommentController) validateCreateCommentRequest(req *services.CreateCommentRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return fmt.Errorf("content is required")
	}

	// Must have exactly one parent
	parentCount := 0
	if req.PostID != nil {
		parentCount++
	}
	if req.QuestionID != nil {
		parentCount++
	}
	if req.DocumentID != nil {
		parentCount++
	}
	if parentCount != 1 {
		return fmt.Errorf("comment must have exactly one parent (post, question, or document)")
	}

	return nil
}

// handleServiceError handles service errors with proper logging and response
func (c *CommentController) handleServiceError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	// Log the error with context
	c.logger.Error("Comment service error",
		zap.Error(err),
		zap.String("operation", operation),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method),
	)

	// Handle service error using the response builder
	c.responseBuilder.WriteError(w, r, err)
}

// extractIDFromPath extracts an ID from URL path at specified position
func (c *CommentController) extractIDFromPath(path string, position int) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) <= position {
		return 0, fmt.Errorf("missing ID in path")
	}

	id, err := strconv.ParseInt(parts[position], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ID format")
	}

	return id, nil
}

// truncateContent safely truncates content for logging
func (c *CommentController) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// parseTimeRange converts a string like "7d", "30d" to a TimeRange
func (c *CommentController) parseTimeRange(timeRangeStr string, now time.Time) (*services.TimeRange, error) {
	if len(timeRangeStr) < 1 {
		return nil, fmt.Errorf("invalid time range format")
	}

	// Get the number part
	numStr := timeRangeStr[:len(timeRangeStr)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return nil, fmt.Errorf("invalid number in time range")
	}

	// Get the unit part (last character)
	unit := timeRangeStr[len(timeRangeStr)-1:]

	var duration time.Duration
	switch strings.ToLower(unit) {
	case "d":
		duration = time.Duration(num*24) * time.Hour
	case "h":
		duration = time.Duration(num) * time.Hour
	case "m":
		duration = time.Duration(num) * time.Minute
	default:
		return nil, fmt.Errorf("invalid time unit. Use 'd' for days, 'h' for hours, 'm' for minutes")
	}

	return &services.TimeRange{
		StartTime: now.Add(-duration),
		EndTime:   now,
	}, nil
}
