// ===============================
// FILE: internal/services/comment_service.go
// ===============================

package services

import (
	"context"
	"evalhub/internal/cache"
	"evalhub/internal/events"
	"evalhub/internal/models"
	"evalhub/internal/repositories"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// commentService implements CommentService with enterprise features
type commentService struct {
	commentRepo    repositories.CommentRepository
	postRepo       repositories.PostRepository
	userRepo       repositories.UserRepository
	cache          cache.Cache
	events         events.EventBus
	userService    UserService
	transactionSvc TransactionService
	logger         *zap.Logger
	config         *CommentServiceConfig
}

// CommentServiceConfig holds comment service configuration
type CommentServiceConfig struct {
	MaxContentLength      int           `json:"max_content_length"`
	MaxCommentsPerHour    int           `json:"max_comments_per_hour"`
	MaxDepthLevel         int           `json:"max_depth_level"`
	DefaultCacheTime      time.Duration `json:"default_cache_time"`
	EnableContentFilter   bool          `json:"enable_content_filter"`
	EnableAutoModeration  bool          `json:"enable_auto_moderation"`
	EnableThreading       bool          `json:"enable_threading"`
	EnableMentions        bool          `json:"enable_mentions"`
	RequireApproval       bool          `json:"require_approval"`
}

// NewCommentService creates a new enterprise comment service
func NewCommentService(
	commentRepo repositories.CommentRepository,
	postRepo repositories.PostRepository,
	userRepo repositories.UserRepository,
	cache cache.Cache,
	events events.EventBus,
	userService UserService,
	transactionSvc TransactionService,
	logger *zap.Logger,
	config *CommentServiceConfig,
) CommentService {
	if config == nil {
		config = DefaultCommentConfig()
	}

	return &commentService{
		commentRepo:    commentRepo,
		postRepo:       postRepo,
		userRepo:       userRepo,
		cache:          cache,
		events:         events,
		userService:    userService,
		transactionSvc: transactionSvc,
		logger:         logger,
		config:         config,
	}
}

// DefaultCommentConfig returns default comment service configuration
func DefaultCommentConfig() *CommentServiceConfig {
	return &CommentServiceConfig{
		MaxContentLength:     10000,
		MaxCommentsPerHour:   20,
		MaxDepthLevel:        5,
		DefaultCacheTime:     10 * time.Minute,
		EnableContentFilter:  true,
		EnableAutoModeration: true,
		EnableThreading:      true,
		EnableMentions:       true,
		RequireApproval:      false,
	}
}

// ===============================
// CORE CRUD OPERATIONS - FIXED SIGNATURES
// ===============================

// CreateComment creates a new comment with comprehensive validation
func (s *commentService) CreateComment(ctx context.Context, req *CreateCommentRequest) (*models.Comment, error) {
	// Validate request
	if err := s.validateCreateRequest(req); err != nil {
		return nil, NewValidationError("invalid create comment request", err)
	}

	// Check rate limiting
	if err := s.checkCommentRateLimit(ctx, req.UserID); err != nil {
		return nil, err
	}

	// Validate parent content exists
	if err := s.validateParentContent(ctx, req); err != nil {
		return nil, err
	}

	// Content moderation
	if s.config.EnableContentFilter {
		if err := s.moderateContent(req.Content); err != nil {
			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
		}
	}

	// Process mentions if enabled
	var mentions []string
	if s.config.EnableMentions {
		mentions = s.extractMentions(req.Content)
	}

	// Execute in transaction for consistency
	var comment *models.Comment
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "create",
			Service: "comment_service",
			Method:  "CreateComment",
		})

		// Create comment model
		comment = &models.Comment{
			UserID:              req.UserID,
			PostID:              req.PostID,
			QuestionID:          req.QuestionID,
			DocumentID:          req.DocumentID,
			ParentCommentID:     req.ParentID,
			Content:             strings.TrimSpace(req.Content),
			ThreadLevel:         0, // Will be calculated if parent exists
			LikesCount:          0,
			DislikesCount:       0,
			IsFlagged:           false,
			IsApproved:          !s.config.RequireApproval,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}

		// Calculate thread level if parent comment exists
		if req.ParentID != nil {
			parentComment, err := s.commentRepo.GetByID(ctx, *req.ParentID, nil)
			if err != nil {
				return NewNotFoundError("parent comment not found")
			}
			comment.ThreadLevel = parentComment.ThreadLevel + 1
			
			// Check max depth
			if comment.ThreadLevel > s.config.MaxDepthLevel {
				return NewBusinessError("maximum thread depth exceeded", "MAX_DEPTH_EXCEEDED")
			}
		}

		// Create comment in database
		if err := s.commentRepo.Create(ctx, comment); err != nil {
			s.logger.Error("Failed to create comment", zap.Error(err))
			return NewInternalError("failed to create comment")
		}

		// Process mentions
		if len(mentions) > 0 {
			s.processMentions(ctx, comment, mentions)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Invalidate relevant caches
	s.invalidateCommentCaches(ctx, comment)

	// Publish comment creation event
	if err := s.events.Publish(ctx, &events.CommentCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "comment.created",
			Timestamp: time.Now(),
			UserID:    &comment.UserID,
		},
		CommentID:  comment.ID,
		PostID:     comment.PostID,
		QuestionID: comment.QuestionID,
		DocumentID: comment.DocumentID,
		Content:    s.truncateContent(comment.Content, 100),
		Mentions:   mentions,
	}); err != nil {
		s.logger.Warn("Failed to publish comment created event", zap.Error(err))
	}

	// Send notifications for mentions
	if len(mentions) > 0 {
		go s.notifyMentionedUsers(ctx, comment, mentions)
	}

	// Notify parent content author
	go s.notifyParentAuthor(ctx, comment)

	s.logger.Info("Comment created successfully",
		zap.Int64("comment_id", comment.ID),
		zap.Int64("user_id", comment.UserID),
		zap.Int("mentions", len(mentions)),
	)

	return comment, nil
}

// GetCommentThread retrieves the entire thread for a comment - MISSING METHOD
func (s *commentService) GetCommentThread(ctx context.Context, commentID int64, userID *int64) ([]*models.Comment, error) {
	if commentID <= 0 {
		return nil, NewValidationError("invalid comment ID", nil)
	}

	// Check if comment exists
	comment, err := s.commentRepo.GetByID(ctx, commentID, userID)
	if err != nil {
		return nil, NewInternalError("failed to retrieve comment")
	}
	if comment == nil {
		return nil, NewNotFoundError("comment not found")
	}

	// Try cache first
	cacheKey := fmt.Sprintf("comment_thread:%d", commentID)
	if userID != nil {
		cacheKey = fmt.Sprintf("comment_thread:%d:user:%d", commentID, *userID)
	}
	
	if cachedThread, found := s.cache.Get(ctx, cacheKey); found {
		if thread, ok := cachedThread.([]*models.Comment); ok {
			s.logger.Debug("Comment thread retrieved from cache", 
				zap.Int64("comment_id", commentID),
				zap.Int("thread_size", len(thread)))
			return thread, nil
		}
	}

	// Get thread from repository
	thread, err := s.commentRepo.GetCommentThread(ctx, commentID, userID)
	if err != nil {
		s.logger.Error("Failed to get comment thread", 
			zap.Error(err), 
			zap.Int64("comment_id", commentID))
		return nil, NewInternalError("failed to retrieve comment thread")
	}

	// Enrich all comments in the thread with additional data
	for _, threadComment := range thread {
		if err := s.enrichComment(ctx, threadComment, userID); err != nil {
			s.logger.Warn("Failed to enrich thread comment", 
				zap.Error(err), 
				zap.Int64("comment_id", threadComment.ID))
		}
	}

	// Cache the result (threads don't change often)
	if err := s.cache.Set(ctx, cacheKey, thread, s.config.DefaultCacheTime); err != nil {
		s.logger.Warn("Failed to cache comment thread", zap.Error(err))
	}

	s.logger.Debug("Retrieved comment thread successfully",
		zap.Int64("comment_id", commentID),
		zap.Int("thread_size", len(thread)),
	)

	return thread, nil
}

// GetCommentByID retrieves a comment by ID with comprehensive data loading - FIXED SIGNATURE
func (s *commentService) GetCommentByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error) {
	if id <= 0 {
		return nil, NewValidationError("invalid comment ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("comment:%d", id)
	if cachedComment, found := s.cache.Get(ctx, cacheKey); found {
		if comment, ok := cachedComment.(*models.Comment); ok {
			// Set user-specific data if userID provided
			if userID != nil {
				s.enrichCommentWithUserData(ctx, comment, *userID)
			}
			s.logger.Debug("Comment retrieved from cache", zap.Int64("comment_id", id))
			return comment, nil
		}
	}

	// Get from database - FIXED: Now matches repository interface
	comment, err := s.commentRepo.GetByID(ctx, id, userID)
	if err != nil {
		s.logger.Error("Failed to get comment by ID", zap.Error(err), zap.Int64("comment_id", id))
		return nil, NewInternalError("failed to retrieve comment")
	}

	if comment == nil {
		return nil, NewNotFoundError("comment not found")
	}

	// Enrich with additional data
	if err := s.enrichComment(ctx, comment, userID); err != nil {
		s.logger.Warn("Failed to enrich comment data", zap.Error(err), zap.Int64("comment_id", id))
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, comment, s.config.DefaultCacheTime); err != nil {
		s.logger.Warn("Failed to cache comment", zap.Error(err), zap.Int64("comment_id", id))
	}

	return comment, nil
}

// UpdateComment updates an existing comment with validation and authorization - FIXED SIGNATURE
func (s *commentService) UpdateComment(ctx context.Context, req *UpdateCommentRequest) (*models.Comment, error) {
	// Validate request
	if err := s.validateUpdateRequest(req); err != nil {
		return nil, NewValidationError("invalid update comment request", err)
	}

	// Get current comment for authorization check
	currentComment, err := s.commentRepo.GetByID(ctx, req.CommentID, nil)
	if err != nil {
		return nil, NewInternalError("failed to retrieve current comment")
	}
	if currentComment == nil {
		return nil, NewNotFoundError("comment not found")
	}

	// Authorization check
	if currentComment.UserID != req.UserID {
		return nil, NewAuthorizationError("insufficient permissions to update comment", "comment", "update", req.UserID)
	}

	// Check edit time window (e.g., can only edit within 1 hour)
	if time.Since(currentComment.CreatedAt) > 1*time.Hour {
		return nil, NewBusinessError("comment edit time window has expired", "EDIT_WINDOW_EXPIRED")
	}

	// Content moderation for updates
	if s.config.EnableContentFilter {
		if err := s.moderateContent(req.Content); err != nil {
			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
		}
	}

	// Process mentions
	var mentions []string
	if s.config.EnableMentions {
		mentions = s.extractMentions(req.Content)
	}

	// Execute update in transaction
	var updatedComment *models.Comment
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "update",
			Service: "comment_service",
			Method:  "UpdateComment",
		})

		// Update fields
		currentComment.Content = strings.TrimSpace(req.Content)
		currentComment.UpdatedAt = time.Now()

		// Update in database
		if err := s.commentRepo.Update(ctx, currentComment); err != nil {
			s.logger.Error("Failed to update comment", zap.Error(err), zap.Int64("comment_id", req.CommentID))
			return NewInternalError("failed to update comment")
		}

		updatedComment = currentComment
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Invalidate caches
	s.invalidateCommentCaches(ctx, updatedComment)
	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", updatedComment.ID))

	// Publish comment updated event
	if err := s.events.Publish(ctx, &events.CommentUpdatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "comment.updated",
			Timestamp: time.Now(),
			UserID:    &updatedComment.UserID,
		},
		CommentID: updatedComment.ID,
		Content:   s.truncateContent(updatedComment.Content, 100),
		Mentions:  mentions,
	}); err != nil {
		s.logger.Warn("Failed to publish comment updated event", zap.Error(err))
	}

	s.logger.Info("Comment updated successfully",
		zap.Int64("comment_id", updatedComment.ID),
		zap.Int64("user_id", updatedComment.UserID),
	)

	return updatedComment, nil
}

// DeleteComment soft deletes a comment with authorization
func (s *commentService) DeleteComment(ctx context.Context, commentID, userID int64) error {
	if commentID <= 0 {
		return NewValidationError("invalid comment ID", nil)
	}

	// Get comment for authorization
	comment, err := s.commentRepo.GetByID(ctx, commentID, nil)
	if err != nil {
		return NewInternalError("failed to retrieve comment")
	}
	if comment == nil {
		return NewNotFoundError("comment not found")
	}

	// Authorization check
	if comment.UserID != userID {
		return NewAuthorizationError("insufficient permissions to delete comment", "comment", "delete", userID)
	}

	// Execute deletion in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "delete",
			Service: "comment_service",
			Method:  "DeleteComment",
		})

		// Delete comment (soft delete)
		if err := s.commentRepo.Delete(ctx, commentID); err != nil {
			s.logger.Error("Failed to delete comment", zap.Error(err), zap.Int64("comment_id", commentID))
			return NewInternalError("failed to delete comment")
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate caches
	s.invalidateCommentCaches(ctx, comment)
	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", commentID))

	// Publish comment deleted event
	if err := s.events.Publish(ctx, &events.CommentDeletedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "comment.deleted",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		CommentID: commentID,
	}); err != nil {
		s.logger.Warn("Failed to publish comment deleted event", zap.Error(err))
	}

	s.logger.Info("Comment deleted successfully",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// ===============================
// LISTING OPERATIONS - FIXED SIGNATURES
// ===============================

// GetCommentsByPost retrieves comments for a specific post - FIXED SIGNATURE
func (s *commentService) GetCommentsByPost(ctx context.Context, req *GetCommentsByPostRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Validate request
	if err := s.validateGetByPostRequest(req); err != nil {
		return nil, NewValidationError("invalid get comments request", err)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Try cache for recent comments
	var cacheKey string
	if req.Pagination.Offset == 0 {
		cacheKey = fmt.Sprintf("comments:post:%d:limit:%d", req.PostID, req.Pagination.Limit)
		if cachedComments, found := s.cache.Get(ctx, cacheKey); found {
			if response, ok := cachedComments.(*models.PaginatedResponse[*models.Comment]); ok {
				// Enrich with user-specific data if needed
				if req.UserID != nil {
					for _, comment := range response.Data {
						s.enrichCommentWithUserData(ctx, comment, *req.UserID)
					}
				}
				return response, nil
			}
		}
	}

	// Get comments from repository - FIXED: Now matches repository interface
	response, err := s.commentRepo.GetByPostID(ctx, req.PostID, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get comments by post", zap.Error(err), zap.Int64("post_id", req.PostID))
		return nil, NewInternalError("failed to retrieve comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
		}
	}

	// Cache the result if appropriate
	if cacheKey != "" {
		if err := s.cache.Set(ctx, cacheKey, response, s.config.DefaultCacheTime); err != nil {
			s.logger.Warn("Failed to cache comments", zap.Error(err))
		}
	}

	return response, nil
}

// GetCommentsByQuestion retrieves comments for a specific question - FIXED SIGNATURE
func (s *commentService) GetCommentsByQuestion(ctx context.Context, req *GetCommentsByQuestionRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Validate request
	if req.QuestionID <= 0 {
		return nil, NewValidationError("invalid question ID", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Get comments from repository - FIXED: Now matches repository interface
	response, err := s.commentRepo.GetByQuestionID(ctx, req.QuestionID, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get comments by question", zap.Error(err), zap.Int64("question_id", req.QuestionID))
		return nil, NewInternalError("failed to retrieve comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
		}
	}

	return response, nil
}

// 🆕 NEW METHOD - GetCommentsByDocument retrieves comments for a specific document
func (s *commentService) GetCommentsByDocument(ctx context.Context, req *GetCommentsByDocumentRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Validate request
	if req.DocumentID <= 0 {
		return nil, NewValidationError("invalid document ID", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Get comments from repository
	response, err := s.commentRepo.GetByDocumentID(ctx, req.DocumentID, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get comments by document", zap.Error(err), zap.Int64("document_id", req.DocumentID))
		return nil, NewInternalError("failed to retrieve comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
		}
	}

	return response, nil
}

// GetCommentsByUser retrieves comments by a specific user - FIXED SIGNATURE
func (s *commentService) GetCommentsByUser(ctx context.Context, req *GetCommentsByUserRequest) (*models.PaginatedResponse[*models.Comment], error) {
	if req.TargetUserID <= 0 {
		return nil, NewValidationError("invalid target user ID", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Get comments by user - FIXED: Now matches repository interface
	response, err := s.commentRepo.GetByUserID(ctx, req.TargetUserID, req.Pagination)
	if err != nil {
		s.logger.Error("Failed to get comments by user", zap.Error(err), zap.Int64("user_id", req.TargetUserID))
		return nil, NewInternalError("failed to retrieve user comments")
	}

	// Enrich comments with requesting user's context
	requestingUserID := s.getRequestingUserID(ctx)
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, requestingUserID); err != nil {
			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
		}
	}

	return response, nil
}

// ===============================
// NEW METHODS - MISSING IMPLEMENTATIONS
// ===============================

// 🆕 NEW METHOD - SearchComments searches comments across all content types
func (s *commentService) SearchComments(ctx context.Context, req *SearchCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Validate request
	if strings.TrimSpace(req.Query) == "" {
		return nil, NewValidationError("search query is required", nil)
	}
	if len(req.Query) < 2 {
		return nil, NewValidationError("search query must be at least 2 characters", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Search comments using repository
	response, err := s.commentRepo.Search(ctx, req.Query, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to search comments", zap.Error(err), zap.String("query", req.Query))
		return nil, NewInternalError("failed to search comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
		}
	}

	return response, nil
}

// 🆕 NEW METHOD - GetTrendingComments retrieves trending comments - FIXED SIGNATURE
func (s *commentService) GetTrendingComments(ctx context.Context, req *GetTrendingCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Default to last 7 days if no time range is provided
	if req.TimeRange == nil {
		endTime := time.Now()
		startTime := endTime.Add(-7 * 24 * time.Hour)
		req.TimeRange = &TimeRange{
			StartTime: startTime,
			EndTime:   endTime,
		}
	}

	// Set default pagination values if not provided
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 10
	}

	// Get trending comments from repository
	response, err := s.commentRepo.GetTrendingComments(
		ctx,
		req.TimeRange.StartTime,
		req.TimeRange.EndTime,
		req.Pagination,
		req.UserID,
	)

	if err != nil {
		s.logger.Error("failed to get trending comments",
			zap.Error(err),
			zap.Time("start_time", req.TimeRange.StartTime),
			zap.Time("end_time", req.TimeRange.EndTime),
		)
		return nil, NewInternalError("failed to get trending comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("failed to enrich comment",
				zap.Int64("comment_id", comment.ID),
				zap.Error(err),
			)
		}
	}

	return response, nil
}

// 🆕 NEW METHOD - GetRecentComments retrieves the most recent comments - FIXED SIGNATURE
func (s *commentService) GetRecentComments(ctx context.Context, req *GetRecentCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Set default pagination values if not provided
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 10
	}

	// Get recent comments from repository
	response, err := s.commentRepo.GetRecentComments(
		ctx,
		req.Pagination,
		req.UserID,
	)

	if err != nil {
		s.logger.Error("failed to get recent comments", zap.Error(err))
		return nil, NewInternalError("failed to get recent comments")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("failed to enrich comment",
				zap.Int64("comment_id", comment.ID),
				zap.Error(err),
			)
		}
	}

	return response, nil
}

// 🆕 NEW METHOD - GetModerationQueue retrieves comments for moderation
func (s *commentService) GetModerationQueue(ctx context.Context, req *GetModerationQueueRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Get comments for moderation from repository
	response, err := s.commentRepo.GetCommentsForModeration(ctx, req.Status, req.Priority, req.Pagination)
	if err != nil {
		s.logger.Error("Failed to get moderation queue", zap.Error(err))
		return nil, NewInternalError("failed to retrieve moderation queue")
	}

	// Don't enrich with user-specific data for moderation queue
	// Moderators should see objective data
	return response, nil
}

// 🆕 NEW METHOD - GetCommentAnalytics retrieves comment analytics
func (s *commentService) GetCommentAnalytics(ctx context.Context, req *GetCommentAnalyticsRequest) (*CommentAnalyticsResponse, error) {
	// Default time range if not provided
	if req.TimeRange == nil {
		endTime := time.Now()
		startTime := endTime.Add(-30 * 24 * time.Hour) // Last 30 days
		req.TimeRange = &TimeRange{
			StartTime: startTime,
			EndTime:   endTime,
		}
	}

	// Get analytics data
	analytics := &CommentAnalyticsResponse{
		TotalComments:   0,
		CommentsByDay:   make(map[string]int),
		CommentsByType:  make(map[string]int),
		AvgResponseTime: 0.0,
		EngagementRate:  0.0,
		TopComments:     []*models.Comment{},
		DailyStats:      []DailyCommentStats{},
	}

	// Get user's comments in time range
	userComments, err := s.commentRepo.GetByUserID(ctx, req.UserID, models.PaginationParams{
		Limit:  1000, // Large limit for analytics
		Offset: 0,
	})
	if err == nil && userComments != nil {
		analytics.TotalComments = len(userComments.Data)
		
		// Process comments for analytics
		for _, comment := range userComments.Data {
			// Filter by time range
			if comment.CreatedAt.After(req.TimeRange.StartTime) && comment.CreatedAt.Before(req.TimeRange.EndTime) {
				dayKey := comment.CreatedAt.Format("2006-01-02")
				analytics.CommentsByDay[dayKey]++
				
				// Categorize by context type
				contextType := comment.GetParentType()
				analytics.CommentsByType[contextType]++
			}
		}
	}

	return analytics, nil
}

// ===============================
// ENGAGEMENT OPERATIONS
// ===============================

// ReactToComment handles comment reactions (like/dislike)
func (s *commentService) ReactToComment(ctx context.Context, req *ReactToCommentRequest) error {
	// Validate request
	if err := s.validateReactionRequest(req); err != nil {
		return NewValidationError("invalid reaction request", err)
	}

	// Check if comment exists
	comment, err := s.commentRepo.GetByID(ctx, req.CommentID, nil)
	if err != nil {
		return NewInternalError("failed to retrieve comment")
	}
	if comment == nil {
		return NewNotFoundError("comment not found")
	}

	// Execute reaction in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Add reaction
		if err := s.commentRepo.AddReaction(ctx, req.CommentID, req.UserID, req.ReactionType); err != nil {
			return NewInternalError("failed to add reaction")
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate comment cache
	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", req.CommentID))

	// Publish reaction event
	if err := s.events.Publish(ctx, &events.CommentReactionEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "comment.reacted",
			Timestamp: time.Now(),
			UserID:    &req.UserID,
		},
		CommentID:    req.CommentID,
		ReactionType: req.ReactionType,
	}); err != nil {
		s.logger.Warn("Failed to publish reaction event", zap.Error(err))
	}

	s.logger.Info("User reacted to comment",
		zap.Int64("comment_id", req.CommentID),
		zap.Int64("user_id", req.UserID),
		zap.String("reaction", req.ReactionType),
	)

	return nil
}

// RemoveCommentReaction removes a user's reaction from a comment
func (s *commentService) RemoveCommentReaction(ctx context.Context, commentID, userID int64) error {
	if commentID <= 0 || userID <= 0 {
		return NewValidationError("invalid comment or user ID", nil)
	}

	// Execute removal in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := s.commentRepo.RemoveReaction(ctx, commentID, userID); err != nil {
			return NewInternalError("failed to remove reaction")
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate comment cache
	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", commentID))

	s.logger.Info("User removed reaction from comment",
		zap.Int64("comment_id", commentID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// ===============================
// MODERATION
// ===============================

// ReportComment reports a comment for moderation
func (s *commentService) ReportComment(ctx context.Context, req *ReportContentRequest) error {
	if req.ContentID <= 0 || req.ReporterID <= 0 {
		return NewValidationError("invalid content or reporter ID", nil)
	}

	// Check if comment exists
	comment, err := s.commentRepo.GetByID(ctx, req.ContentID, nil)
	if err != nil {
		return NewInternalError("failed to retrieve comment")
	}
	if comment == nil {
		return NewNotFoundError("comment not found")
	}

	// Execute report in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.ReporterID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Check if user already reported this comment
		// This would be implemented in the repository layer
		return nil
	})

	if err != nil {
		return err
	}

	// Publish report event
	if err := s.events.Publish(ctx, &events.ContentReportedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "content.reported",
			Timestamp: time.Now(),
			UserID:    &req.ReporterID,
		},
		ContentType: "comment",
		ContentID:   req.ContentID,
		Reason:      req.Reason,
	}); err != nil {
		s.logger.Warn("Failed to publish report event", zap.Error(err))
	}

	s.logger.Info("Comment reported for moderation",
		zap.Int64("comment_id", req.ContentID),
		zap.Int64("reporter_id", req.ReporterID),
		zap.String("reason", req.Reason),
	)

	return nil
}

// ModerateComment handles moderation actions on comments
func (s *commentService) ModerateComment(ctx context.Context, req *ModerateContentRequest) error {
	if req.ContentID <= 0 || req.ModeratorID <= 0 {
		return NewValidationError("invalid content or moderator ID", nil)
	}

	// Execute moderation in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.ModeratorID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// This would be implemented based on your moderation system
		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate comment cache
	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", req.ContentID))

	s.logger.Info("Comment moderated",
		zap.Int64("comment_id", req.ContentID),
		zap.Int64("moderator_id", req.ModeratorID),
		zap.String("action", req.Action),
	)

	return nil
}

// ===============================
// ANALYTICS
// ===============================

// GetCommentStats retrieves comment statistics - FIXED SIGNATURE
func (s *commentService) GetCommentStats(ctx context.Context, commentID int64) (*CommentStatsResponse, error) {
	if commentID <= 0 {
		return nil, NewValidationError("invalid comment ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("comment_stats:%d", commentID)
	if cachedStats, found := s.cache.Get(ctx, cacheKey); found {
		if stats, ok := cachedStats.(*CommentStatsResponse); ok {
			return stats, nil
		}
	}

	// Get stats from repository
	repoStats, err := s.commentRepo.GetCommentStats(ctx, commentID)
	if err != nil {
		s.logger.Error("Failed to get comment stats", zap.Error(err), zap.Int64("comment_id", commentID))
		return nil, NewInternalError("failed to retrieve comment statistics")
	}

	// Convert to service response type
	stats := &CommentStatsResponse{
		CommentID:     repoStats.CommentID,
		LikesCount:    repoStats.LikesCount,
		DislikesCount: repoStats.DislikesCount,
		RepliesCount:  repoStats.RepliesCount,
		IsAccepted:    repoStats.IsAccepted,
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, stats, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to cache comment stats", zap.Error(err))
	}

	return stats, nil
}

// ===============================
// HELPER METHODS - FIXED AND IMPROVED
// ===============================

// validateCreateRequest validates create comment request
func (s *commentService) validateCreateRequest(req *CreateCommentRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if len(strings.TrimSpace(req.Content)) == 0 {
		return fmt.Errorf("content is required")
	}
	if len(req.Content) > s.config.MaxContentLength {
		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
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

// validateUpdateRequest validates update comment request
func (s *commentService) validateUpdateRequest(req *UpdateCommentRequest) error {
	if req.CommentID <= 0 {
		return fmt.Errorf("comment ID is required")
	}
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if len(strings.TrimSpace(req.Content)) == 0 {
		return fmt.Errorf("content cannot be empty")
	}
	if len(req.Content) > s.config.MaxContentLength {
		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
	}

	return nil
}

// validateGetByPostRequest validates get comments by post request
func (s *commentService) validateGetByPostRequest(req *GetCommentsByPostRequest) error {
	if req.PostID <= 0 {
		return fmt.Errorf("post ID is required")
	}
	if req.Pagination.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if req.Pagination.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}

	return nil
}

// validateReactionRequest validates reaction request
func (s *commentService) validateReactionRequest(req *ReactToCommentRequest) error {
	if req.CommentID <= 0 {
		return fmt.Errorf("comment ID is required")
	}
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if req.ReactionType != "like" && req.ReactionType != "dislike" {
		return fmt.Errorf("invalid reaction type")
	}

	return nil
}

// validateParentContent validates that the parent content exists
func (s *commentService) validateParentContent(ctx context.Context, req *CreateCommentRequest) error {
	if req.PostID != nil {
		post, err := s.postRepo.GetByID(ctx, *req.PostID, nil)
		if err != nil {
			return NewInternalError("failed to validate parent post")
		}
		if post == nil {
			return NewNotFoundError("parent post not found")
		}
	}

	// Similar validation for QuestionID and DocumentID would go here
	return nil
}

// moderateContent performs basic content moderation
func (s *commentService) moderateContent(content string) error {
	// Basic content filtering
	bannedWords := []string{"spam", "scam", "illegal"}
	
	lowerContent := strings.ToLower(content)
	for _, word := range bannedWords {
		if strings.Contains(lowerContent, word) {
			return fmt.Errorf("content contains prohibited words")
		}
	}

	return nil
}

// checkCommentRateLimit checks if user is commenting too frequently
func (s *commentService) checkCommentRateLimit(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("comment_rate_limit:%d", userID)
	count, _ := s.cache.Increment(ctx, key, 1)
	
	if count == 1 {
		s.cache.SetTTL(ctx, key, 1*time.Hour)
	}

	if count > int64(s.config.MaxCommentsPerHour) {
		return NewRateLimitError("commenting rate limit exceeded", map[string]interface{}{
			"limit":      s.config.MaxCommentsPerHour,
			"reset_time": "1 hour",
		})
	}

	return nil
}

// getInitialStatus returns the initial status for new comments
func (s *commentService) getInitialStatus() string {
	if s.config.RequireApproval {
		return "pending"
	}
	return "published"
}

// extractMentions extracts @mentions from content
func (s *commentService) extractMentions(content string) []string {
	words := strings.Fields(content)
	var mentions []string
	
	for _, word := range words {
		if strings.HasPrefix(word, "@") && len(word) > 1 {
			username := strings.TrimPrefix(word, "@")
			username = strings.TrimRight(username, ".,!?;:")
			if len(username) > 0 {
				mentions = append(mentions, username)
			}
		}
	}
	
	return mentions
}

// enrichComment adds additional data to a comment
func (s *commentService) enrichComment(ctx context.Context, comment *models.Comment, userID *int64) error {
	// Get author information
	author, err := s.userService.GetUserByID(ctx, comment.UserID)
	if err == nil && author != nil {
		comment.Username = author.Username
		comment.DisplayName = author.DisplayName
		comment.AuthorProfileURL = author.ProfileURL
	}

	// Add user-specific data if userID provided
	if userID != nil {
		s.enrichCommentWithUserData(ctx, comment, *userID)
	}

	return nil
}

// enrichCommentWithUserData adds user-specific data to a comment
func (s *commentService) enrichCommentWithUserData(ctx context.Context, comment *models.Comment, userID int64) {
	// Check if user has reacted
	if reaction, err := s.commentRepo.GetUserReaction(ctx, comment.ID, userID); err == nil && reaction != nil {
		comment.UserReaction = reaction
	}

	// Check ownership
	comment.IsOwner = (comment.UserID == userID)
}

// truncateContent safely truncates content for logging
func (s *commentService) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// invalidateCommentCaches invalidates relevant caches
func (s *commentService) invalidateCommentCaches(ctx context.Context, comment *models.Comment) {
	// Invalidate post comments cache
	if comment.PostID != nil {
		s.cache.DeletePattern(ctx, fmt.Sprintf("comments:post:%d:*", *comment.PostID))
	}
	
	// Invalidate question comments cache
	if comment.QuestionID != nil {
		s.cache.DeletePattern(ctx, fmt.Sprintf("comments:question:%d:*", *comment.QuestionID))
	}
	
	// Invalidate user comments cache
	s.cache.DeletePattern(ctx, fmt.Sprintf("comments:user:%d:*", comment.UserID))
}

// processMentions processes user mentions in comments
func (s *commentService) processMentions(ctx context.Context, comment *models.Comment, mentions []string) {
	for _, username := range mentions {
		user, err := s.userService.GetUserByUsername(ctx, username)
		if err == nil && user != nil {
			s.logger.Debug("Processed mention",
				zap.String("username", username),
				zap.Int64("comment_id", comment.ID),
			)
		}
	}
}

// notifyMentionedUsers sends notifications to mentioned users
func (s *commentService) notifyMentionedUsers(ctx context.Context, comment *models.Comment, mentions []string) {
	for _, username := range mentions {
		user, err := s.userService.GetUserByUsername(ctx, username)
		if err == nil && user != nil {
			if err := s.events.Publish(ctx, &events.UserMentionedEvent{
				BaseEvent: events.BaseEvent{
					EventID:   events.GenerateEventID(),
					EventType: "user.mentioned",
					Timestamp: time.Now(),
					UserID:    &user.ID,
				},
				MentionedByUserID: comment.UserID,
				CommentID:        comment.ID,
				PostID:           comment.PostID,
				QuestionID:       comment.QuestionID,
			}); err != nil {
				s.logger.Warn("Failed to publish mention event", zap.Error(err))
			}
		}
	}
}

// notifyParentAuthor notifies the author of the parent content
func (s *commentService) notifyParentAuthor(ctx context.Context, comment *models.Comment) {
	if comment.PostID != nil {
		post, err := s.postRepo.GetByID(ctx, *comment.PostID, nil)
		if err == nil && post != nil && post.UserID != comment.UserID {
			if err := s.events.Publish(ctx, &events.CommentNotificationEvent{
				BaseEvent: events.BaseEvent{
					EventID:   events.GenerateEventID(),
					EventType: "comment.notification",
					Timestamp: time.Now(),
					UserID:    &post.UserID,
				},
				CommentID:      comment.ID,
				CommenterID:    comment.UserID,
				PostID:         comment.PostID,
				CommentPreview: s.truncateContent(comment.Content, 100),
			}); err != nil {
				s.logger.Warn("Failed to publish comment notification event", zap.Error(err))
			}
		}
	}
}

// getRequestingUserID extracts user ID from context
func (s *commentService) getRequestingUserID(ctx context.Context) *int64 {
	if userID, ok := ctx.Value("user_id").(int64); ok {
		return &userID
	}
	return nil
}

// GetCommentReplies retrieves replies to a specific comment - MISSING METHOD
func (s *commentService) GetCommentReplies(ctx context.Context, req *GetCommentRepliesRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Validate request
	if req.ParentCommentID <= 0 {
		return nil, NewValidationError("invalid parent comment ID", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Check if parent comment exists
	parentComment, err := s.commentRepo.GetByID(ctx, req.ParentCommentID, nil)
	if err != nil {
		return nil, NewInternalError("failed to retrieve parent comment")
	}
	if parentComment == nil {
		return nil, NewNotFoundError("parent comment not found")
	}

	// Try cache first for recent replies
	var cacheKey string
	if req.Pagination.Offset == 0 {
		cacheKey = fmt.Sprintf("comment_replies:%d:limit:%d", req.ParentCommentID, req.Pagination.Limit)
		if cachedReplies, found := s.cache.Get(ctx, cacheKey); found {
			if response, ok := cachedReplies.(*models.PaginatedResponse[*models.Comment]); ok {
				// Enrich with user-specific data if needed
				if req.UserID != nil {
					for _, comment := range response.Data {
						s.enrichCommentWithUserData(ctx, comment, *req.UserID)
					}
				}
				return response, nil
			}
		}
	}

	// Get replies from repository
	response, err := s.commentRepo.GetReplies(ctx, req.ParentCommentID, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get comment replies", 
			zap.Error(err), 
			zap.Int64("parent_comment_id", req.ParentCommentID))
		return nil, NewInternalError("failed to retrieve comment replies")
	}

	// Enrich comments with additional data
	for _, comment := range response.Data {
		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich reply comment", 
				zap.Error(err), 
				zap.Int64("comment_id", comment.ID))
		}
	}

	// Cache the result if appropriate
	if cacheKey != "" {
		if err := s.cache.Set(ctx, cacheKey, response, s.config.DefaultCacheTime); err != nil {
			s.logger.Warn("Failed to cache comment replies", zap.Error(err))
		}
	}

	s.logger.Debug("Retrieved comment replies successfully",
		zap.Int64("parent_comment_id", req.ParentCommentID),
		zap.Int("replies_count", len(response.Data)),
	)

	return response, nil
}




// // internal/services/comment_service.go
// package services

// import (
// 	"context"
// 	"evalhub/internal/cache"
// 	"evalhub/internal/events"
// 	"evalhub/internal/models"
// 	"evalhub/internal/repositories"
// 	"fmt"
// 	"strings"
// 	"time"

// 	"go.uber.org/zap"
// )

// // commentService implements CommentService with enterprise features
// type commentService struct {
// 	commentRepo    repositories.CommentRepository
// 	postRepo       repositories.PostRepository
// 	userRepo       repositories.UserRepository
// 	cache          cache.Cache
// 	events         events.EventBus
// 	userService    UserService
// 	transactionSvc TransactionService
// 	logger         *zap.Logger
// 	config         *CommentServiceConfig
// }

// // CommentServiceConfig holds comment service configuration
// type CommentServiceConfig struct {
// 	MaxContentLength      int           `json:"max_content_length"`
// 	MaxCommentsPerHour    int           `json:"max_comments_per_hour"`
// 	MaxDepthLevel         int           `json:"max_depth_level"`
// 	DefaultCacheTime      time.Duration `json:"default_cache_time"`
// 	EnableContentFilter   bool          `json:"enable_content_filter"`
// 	EnableAutoModeration  bool          `json:"enable_auto_moderation"`
// 	EnableThreading       bool          `json:"enable_threading"`
// 	EnableMentions        bool          `json:"enable_mentions"`
// 	RequireApproval       bool          `json:"require_approval"`
// }

// // NewCommentService creates a new enterprise comment service
// func NewCommentService(
// 	commentRepo repositories.CommentRepository,
// 	postRepo repositories.PostRepository,
// 	userRepo repositories.UserRepository,
// 	cache cache.Cache,
// 	events events.EventBus,
// 	userService UserService,
// 	transactionSvc TransactionService,
// 	logger *zap.Logger,
// 	config *CommentServiceConfig,
// ) CommentService {
// 	if config == nil {
// 		config = DefaultCommentConfig()
// 	}

// 	return &commentService{
// 		commentRepo:    commentRepo,
// 		postRepo:       postRepo,
// 		userRepo:       userRepo,
// 		cache:          cache,
// 		events:         events,
// 		userService:    userService,
// 		transactionSvc: transactionSvc,
// 		logger:         logger,
// 		config:         config,
// 	}
// }

// // DefaultCommentConfig returns default comment service configuration
// func DefaultCommentConfig() *CommentServiceConfig {
// 	return &CommentServiceConfig{
// 		MaxContentLength:     10000,
// 		MaxCommentsPerHour:   20,
// 		MaxDepthLevel:        5,
// 		DefaultCacheTime:     10 * time.Minute,
// 		EnableContentFilter:  true,
// 		EnableAutoModeration: true,
// 		EnableThreading:      true,
// 		EnableMentions:       true,
// 		RequireApproval:      false,
// 	}
// }

// // ===============================
// // CORE CRUD OPERATIONS
// // ===============================

// // CreateComment creates a new comment with comprehensive validation
// func (s *commentService) CreateComment(ctx context.Context, req *CreateCommentRequest) (*models.Comment, error) {
// 	// Validate request
// 	if err := s.validateCreateRequest(req); err != nil {
// 		return nil, NewValidationError("invalid create comment request", err)
// 	}

// 	// Check rate limiting
// 	if err := s.checkCommentRateLimit(ctx, req.UserID); err != nil {
// 		return nil, err
// 	}

// 	// Validate parent content exists
// 	if err := s.validateParentContent(ctx, req); err != nil {
// 		return nil, err
// 	}

// 	// Content moderation
// 	if s.config.EnableContentFilter {
// 		if err := s.moderateContent(req.Content); err != nil {
// 			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
// 		}
// 	}

// 	// Process mentions if enabled
// 	var mentions []string
// 	if s.config.EnableMentions {
// 		mentions = s.extractMentions(req.Content)
// 	}

// 	// Execute in transaction for consistency
// 	var comment *models.Comment
// 	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &req.UserID,
// 		Timeout: 30 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		// Track operation
// 		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
// 			Type:    "create",
// 			Service: "comment_service",
// 			Method:  "CreateComment",
// 		})

// 		// Create comment model
// 		comment = &models.Comment{
// 			UserID:     req.UserID,
// 			PostID:     req.PostID,
// 			QuestionID: req.QuestionID,
// 			DocumentID: req.DocumentID,
// 			Content:    strings.TrimSpace(req.Content),
// 			Status:     s.getInitialStatus(),
// 			CreatedAt:  time.Now(),
// 			UpdatedAt:  time.Now(),
// 		}

// 		// Create comment in database
// 		if err := s.commentRepo.Create(ctx, comment); err != nil {
// 			s.logger.Error("Failed to create comment", zap.Error(err))
// 			return NewInternalError("failed to create comment")
// 		}

// 		// Process mentions
// 		if len(mentions) > 0 {
// 			s.processMentions(ctx, comment, mentions)
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		return nil, err
// 	}

// 	// Invalidate relevant caches
// 	s.invalidateCommentCaches(ctx, comment)

// 	// Publish comment creation event
// 	if err := s.events.Publish(ctx, &events.CommentCreatedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "comment.created",
// 			Timestamp: time.Now(),
// 			UserID:    &comment.UserID,
// 		},
// 		CommentID:  comment.ID,
// 		PostID:     comment.PostID,
// 		QuestionID: comment.QuestionID,
// 		DocumentID: comment.DocumentID,
// 		Content:    s.truncateContent(comment.Content, 100),
// 		Mentions:   mentions,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish comment created event", zap.Error(err))
// 	}

// 	// Send notifications for mentions
// 	if len(mentions) > 0 {
// 		go s.notifyMentionedUsers(ctx, comment, mentions)
// 	}

// 	// Notify parent content author
// 	go s.notifyParentAuthor(ctx, comment)

// 	s.logger.Info("Comment created successfully",
// 		zap.Int64("comment_id", comment.ID),
// 		zap.Int64("user_id", comment.UserID),
// 		zap.Int("mentions", len(mentions)),
// 	)

// 	return comment, nil
// }

// // GetCommentByID retrieves a comment by ID with comprehensive data loading
// func (s *commentService) GetCommentByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error) {
// 	if id <= 0 {
// 		return nil, NewValidationError("invalid comment ID", nil)
// 	}

// 	// Try cache first
// 	cacheKey := fmt.Sprintf("comment:%d", id)
// 	if cachedComment, found := s.cache.Get(ctx, cacheKey); found {
// 		if comment, ok := cachedComment.(*models.Comment); ok {
// 			// Set user-specific data if userID provided
// 			if userID != nil {
// 				s.enrichCommentWithUserData(ctx, comment, *userID)
// 			}
// 			s.logger.Debug("Comment retrieved from cache", zap.Int64("comment_id", id))
// 			return comment, nil
// 		}
// 	}

// 	// Get from database
// 	comment, err := s.commentRepo.GetByID(ctx, id)
// 	if err != nil {
// 		s.logger.Error("Failed to get comment by ID", zap.Error(err), zap.Int64("comment_id", id))
// 		return nil, NewInternalError("failed to retrieve comment")
// 	}

// 	if comment == nil {
// 		return nil, NewNotFoundError("comment not found")
// 	}

// 	// Enrich with additional data
// 	if err := s.enrichComment(ctx, comment, userID); err != nil {
// 		s.logger.Warn("Failed to enrich comment data", zap.Error(err), zap.Int64("comment_id", id))
// 	}

// 	// Cache the result
// 	if err := s.cache.Set(ctx, cacheKey, comment, s.config.DefaultCacheTime); err != nil {
// 		s.logger.Warn("Failed to cache comment", zap.Error(err), zap.Int64("comment_id", id))
// 	}

// 	return comment, nil
// }

// // UpdateComment updates an existing comment with validation and authorization
// func (s *commentService) UpdateComment(ctx context.Context, req *UpdateCommentRequest) (*models.Comment, error) {
// 	// Validate request
// 	if err := s.validateUpdateRequest(req); err != nil {
// 		return nil, NewValidationError("invalid update comment request", err)
// 	}

// 	// Get current comment for authorization check
// 	currentComment, err := s.commentRepo.GetByID(ctx, req.CommentID)
// 	if err != nil {
// 		return nil, NewInternalError("failed to retrieve current comment")
// 	}
// 	if currentComment == nil {
// 		return nil, NewNotFoundError("comment not found")
// 	}

// 	// Authorization check
// 	if currentComment.UserID != req.UserID {
// 		return nil, NewAuthorizationError("insufficient permissions to update comment", "comment", "update", req.UserID)
// 	}

// 	// Check edit time window (e.g., can only edit within 1 hour)
// 	if time.Since(currentComment.CreatedAt) > 1*time.Hour {
// 		return nil, NewBusinessError("comment edit time window has expired", "EDIT_WINDOW_EXPIRED")
// 	}

// 	// Content moderation for updates
// 	if s.config.EnableContentFilter {
// 		if err := s.moderateContent(req.Content); err != nil {
// 			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
// 		}
// 	}

// 	// Process mentions
// 	var mentions []string
// 	if s.config.EnableMentions {
// 		mentions = s.extractMentions(req.Content)
// 	}

// 	// Execute update in transaction
// 	var updatedComment *models.Comment
// 	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &req.UserID,
// 		Timeout: 30 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		// Track operation
// 		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
// 			Type:    "update",
// 			Service: "comment_service",
// 			Method:  "UpdateComment",
// 		})

// 		// Update fields
// 		currentComment.Content = strings.TrimSpace(req.Content)
// 		currentComment.UpdatedAt = time.Now()
// 		currentComment.IsEdited = true

// 		// Update in database
// 		if err := s.commentRepo.Update(ctx, currentComment); err != nil {
// 			s.logger.Error("Failed to update comment", zap.Error(err), zap.Int64("comment_id", req.CommentID))
// 			return NewInternalError("failed to update comment")
// 		}

// 		updatedComment = currentComment
// 		return nil
// 	})

// 	if err != nil {
// 		return nil, err
// 	}

// 	// Invalidate caches
// 	s.invalidateCommentCaches(ctx, updatedComment)
// 	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", updatedComment.ID))

// 	// Publish comment updated event
// 	if err := s.events.Publish(ctx, &events.CommentUpdatedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "comment.updated",
// 			Timestamp: time.Now(),
// 			UserID:    &updatedComment.UserID,
// 		},
// 		CommentID: updatedComment.ID,
// 		Content:   s.truncateContent(updatedComment.Content, 100),
// 		Mentions:  mentions,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish comment updated event", zap.Error(err))
// 	}

// 	s.logger.Info("Comment updated successfully",
// 		zap.Int64("comment_id", updatedComment.ID),
// 		zap.Int64("user_id", updatedComment.UserID),
// 	)

// 	return updatedComment, nil
// }

// // DeleteComment soft deletes a comment with authorization
// func (s *commentService) DeleteComment(ctx context.Context, commentID, userID int64) error {
// 	if commentID <= 0 {
// 		return NewValidationError("invalid comment ID", nil)
// 	}

// 	// Get comment for authorization
// 	comment, err := s.commentRepo.GetByID(ctx, commentID)
// 	if err != nil {
// 		return NewInternalError("failed to retrieve comment")
// 	}
// 	if comment == nil {
// 		return NewNotFoundError("comment not found")
// 	}

// 	// Authorization check
// 	if comment.UserID != userID {
// 		return NewAuthorizationError("insufficient permissions to delete comment", "comment", "delete", userID)
// 	}

// 	// Execute deletion in transaction
// 	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &userID,
// 		Timeout: 30 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		// Track operation
// 		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
// 			Type:    "delete",
// 			Service: "comment_service",
// 			Method:  "DeleteComment",
// 		})

// 		// Delete comment (soft delete)
// 		if err := s.commentRepo.Delete(ctx, commentID); err != nil {
// 			s.logger.Error("Failed to delete comment", zap.Error(err), zap.Int64("comment_id", commentID))
// 			return NewInternalError("failed to delete comment")
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	// Invalidate caches
// 	s.invalidateCommentCaches(ctx, comment)
// 	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", commentID))

// 	// Publish comment deleted event
// 	if err := s.events.Publish(ctx, &events.CommentDeletedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "comment.deleted",
// 			Timestamp: time.Now(),
// 			UserID:    &userID,
// 		},
// 		CommentID: commentID,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish comment deleted event", zap.Error(err))
// 	}

// 	s.logger.Info("Comment deleted successfully",
// 		zap.Int64("comment_id", commentID),
// 		zap.Int64("user_id", userID),
// 	)

// 	return nil
// }

// // ===============================
// // LISTING OPERATIONS
// // ===============================

// // GetCommentsByPost retrieves comments for a specific post
// func (s *commentService) GetCommentsByPost(ctx context.Context, req *GetCommentsByPostRequest) (*models.PaginatedResponse[*models.Comment], error) {
// 	// Validate request
// 	if err := s.validateGetByPostRequest(req); err != nil {
// 		return nil, NewValidationError("invalid get comments request", err)
// 	}

// 	// Set default pagination
// 	if req.Pagination.Limit == 0 {
// 		req.Pagination.Limit = 20
// 	}
// 	if req.Pagination.Limit > 100 {
// 		req.Pagination.Limit = 100
// 	}

// 	// Try cache for recent comments
// 	var cacheKey string
// 	if req.Pagination.Offset == 0 {
// 		cacheKey = fmt.Sprintf("comments:post:%d:limit:%d", req.PostID, req.Pagination.Limit)
// 		if cachedComments, found := s.cache.Get(ctx, cacheKey); found {
// 			if response, ok := cachedComments.(*models.PaginatedResponse[*models.Comment]); ok {
// 				// Enrich with user-specific data if needed
// 				if req.UserID != nil {
// 					for _, comment := range response.Data {
// 						s.enrichCommentWithUserData(ctx, comment, *req.UserID)
// 					}
// 				}
// 				return response, nil
// 			}
// 		}
// 	}

// 	// Get comments from repository
// 	response, err := s.commentRepo.GetByPostID(ctx, req.PostID, req.Pagination)
// 	if err != nil {
// 		s.logger.Error("Failed to get comments by post", zap.Error(err), zap.Int64("post_id", req.PostID))
// 		return nil, NewInternalError("failed to retrieve comments")
// 	}

// 	// Enrich comments with additional data
// 	for _, comment := range response.Data {
// 		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
// 			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
// 		}
// 	}

// 	// Cache the result if appropriate
// 	if cacheKey != "" {
// 		if err := s.cache.Set(ctx, cacheKey, response, s.config.DefaultCacheTime); err != nil {
// 			s.logger.Warn("Failed to cache comments", zap.Error(err))
// 		}
// 	}

// 	return response, nil
// }

// // GetCommentsByQuestion retrieves comments for a specific question
// func (s *commentService) GetCommentsByQuestion(ctx context.Context, req *GetCommentsByQuestionRequest) (*models.PaginatedResponse[*models.Comment], error) {
// 	// Validate request
// 	if req.QuestionID <= 0 {
// 		return nil, NewValidationError("invalid question ID", nil)
// 	}

// 	// Set default pagination
// 	if req.Pagination.Limit == 0 {
// 		req.Pagination.Limit = 20
// 	}
// 	if req.Pagination.Limit > 100 {
// 		req.Pagination.Limit = 100
// 	}

// 	// Get comments from repository
// 	response, err := s.commentRepo.GetByQuestionID(ctx, req.QuestionID, req.Pagination)
// 	if err != nil {
// 		s.logger.Error("Failed to get comments by question", zap.Error(err), zap.Int64("question_id", req.QuestionID))
// 		return nil, NewInternalError("failed to retrieve comments")
// 	}

// 	// Enrich comments with additional data
// 	for _, comment := range response.Data {
// 		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
// 			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
// 		}
// 	}

// 	return response, nil
// }

// // GetCommentsByUser retrieves comments by a specific user
// func (s *commentService) GetCommentsByUser(ctx context.Context, req *GetCommentsByUserRequest) (*models.PaginatedResponse[*models.Comment], error) {
// 	if req.TargetUserID <= 0 {
// 		return nil, NewValidationError("invalid target user ID", nil)
// 	}

// 	// Set default pagination
// 	if req.Pagination.Limit == 0 {
// 		req.Pagination.Limit = 20
// 	}
// 	if req.Pagination.Limit > 100 {
// 		req.Pagination.Limit = 100
// 	}

// 	// Get requesting user ID from context if available
// 	var requestingUserID *int64
// 	if userID, ok := ctx.Value("user_id").(int64); ok {
// 		requestingUserID = &userID
// 	}

// 	// Get comments by user
// 	response, err := s.commentRepo.GetByUserID(ctx, req.TargetUserID, req.Pagination)
// 	if err != nil {
// 		s.logger.Error("Failed to get comments by user", zap.Error(err), zap.Int64("user_id", req.TargetUserID))
// 		return nil, NewInternalError("failed to retrieve user comments")
// 	}

// 	// Enrich comments with requesting user's context
// 	for _, comment := range response.Data {
// 		if err := s.enrichComment(ctx, comment, requestingUserID); err != nil {
// 			s.logger.Warn("Failed to enrich comment", zap.Error(err), zap.Int64("comment_id", comment.ID))
// 		}
// 	}

// 	return response, nil
// }

// // ===============================
// // ENGAGEMENT OPERATIONS
// // ===============================

// // ReactToComment handles comment reactions (like/dislike)
// func (s *commentService) ReactToComment(ctx context.Context, req *ReactToCommentRequest) error {
// 	// Validate request
// 	if err := s.validateReactionRequest(req); err != nil {
// 		return NewValidationError("invalid reaction request", err)
// 	}

// 	// Check if comment exists
// 	comment, err := s.commentRepo.GetByID(ctx, req.CommentID)
// 	if err != nil {
// 		return NewInternalError("failed to retrieve comment")
// 	}
// 	if comment == nil {
// 		return NewNotFoundError("comment not found")
// 	}

// 	// Execute reaction in transaction
// 	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &req.UserID,
// 		Timeout: 15 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		// Add reaction
// 		if err := s.commentRepo.AddReaction(ctx, req.CommentID, req.UserID, req.ReactionType); err != nil {
// 			return NewInternalError("failed to add reaction")
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	// Invalidate comment cache
// 	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", req.CommentID))

// 	// Publish reaction event
// 	if err := s.events.Publish(ctx, &events.CommentReactionEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "comment.reacted",
// 			Timestamp: time.Now(),
// 			UserID:    &req.UserID,
// 		},
// 		CommentID:    req.CommentID,
// 		ReactionType: req.ReactionType,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish reaction event", zap.Error(err))
// 	}

// 	s.logger.Info("User reacted to comment",
// 		zap.Int64("comment_id", req.CommentID),
// 		zap.Int64("user_id", req.UserID),
// 		zap.String("reaction", req.ReactionType),
// 	)

// 	return nil
// }

// // RemoveCommentReaction removes a user's reaction from a comment
// func (s *commentService) RemoveCommentReaction(ctx context.Context, commentID, userID int64) error {
// 	if commentID <= 0 || userID <= 0 {
// 		return NewValidationError("invalid comment or user ID", nil)
// 	}

// 	// Execute removal in transaction
// 	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &userID,
// 		Timeout: 15 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		if err := s.commentRepo.RemoveReaction(ctx, commentID, userID); err != nil {
// 			return NewInternalError("failed to remove reaction")
// 		}
// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	// Invalidate comment cache
// 	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", commentID))

// 	s.logger.Info("User removed reaction from comment",
// 		zap.Int64("comment_id", commentID),
// 		zap.Int64("user_id", userID),
// 	)

// 	return nil
// }

// // ===============================
// // MODERATION
// // ===============================

// // ReportComment reports a comment for moderation
// func (s *commentService) ReportComment(ctx context.Context, req *ReportContentRequest) error {
// 	if req.ContentID <= 0 || req.ReporterID <= 0 {
// 		return NewValidationError("invalid content or reporter ID", nil)
// 	}

// 	// Execute report in transaction
// 	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &req.ReporterID,
// 		Timeout: 15 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		if err := s.commentRepo.AddReport(ctx, req.ContentID, req.ReporterID, req.Reason, req.Description); err != nil {
// 			return NewInternalError("failed to report comment")
// 		}
// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	// Publish report event
// 	if err := s.events.Publish(ctx, &events.ContentReportedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "content.reported",
// 			Timestamp: time.Now(),
// 			UserID:    &req.ReporterID,
// 		},
// 		ContentType: "comment",
// 		ContentID:   req.ContentID,
// 		Reason:      req.Reason,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish report event", zap.Error(err))
// 	}

// 	s.logger.Info("Comment reported for moderation",
// 		zap.Int64("comment_id", req.ContentID),
// 		zap.Int64("reporter_id", req.ReporterID),
// 		zap.String("reason", req.Reason),
// 	)

// 	return nil
// }

// // ModerateComment handles moderation actions on comments
// func (s *commentService) ModerateComment(ctx context.Context, req *ModerateContentRequest) error {
// 	if req.ContentID <= 0 || req.ModeratorID <= 0 {
// 		return NewValidationError("invalid content or moderator ID", nil)
// 	}

// 	// Execute moderation in transaction
// 	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
// 		UserID:  &req.ModeratorID,
// 		Timeout: 30 * time.Second,
// 	}, func(ctx context.Context, txCtx *TransactionContext) error {
// 		if err := s.commentRepo.ModerateComment(ctx, req.ContentID, req.ModeratorID, req.Action, req.Reason); err != nil {
// 			return NewInternalError("failed to moderate comment")
// 		}
// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	// Invalidate comment cache
// 	s.cache.Delete(ctx, fmt.Sprintf("comment:%d", req.ContentID))

// 	s.logger.Info("Comment moderated",
// 		zap.Int64("comment_id", req.ContentID),
// 		zap.Int64("moderator_id", req.ModeratorID),
// 		zap.String("action", req.Action),
// 	)

// 	return nil
// }

// // ===============================
// // ANALYTICS
// // ===============================

// // GetCommentStats retrieves comment statistics
// func (s *commentService) GetCommentStats(ctx context.Context, commentID int64) (*CommentStatsResponse, error) {
// 	if commentID <= 0 {
// 		return nil, NewValidationError("invalid comment ID", nil)
// 	}

// 	// Try cache first
// 	cacheKey := fmt.Sprintf("comment_stats:%d", commentID)
// 	if cachedStats, found := s.cache.Get(ctx, cacheKey); found {
// 		if stats, ok := cachedStats.(*CommentStatsResponse); ok {
// 			return stats, nil
// 		}
// 	}

// 	// Get stats from repository
// 	stats, err := s.commentRepo.GetCommentStats(ctx, commentID)
// 	if err != nil {
// 		s.logger.Error("Failed to get comment stats", zap.Error(err), zap.Int64("comment_id", commentID))
// 		return nil, NewInternalError("failed to retrieve comment statistics")
// 	}

// 	// Cache the result
// 	if err := s.cache.Set(ctx, cacheKey, stats, 5*time.Minute); err != nil {
// 		s.logger.Warn("Failed to cache comment stats", zap.Error(err))
// 	}

// 	return stats, nil
// }

// // GetTrendingComments retrieves trending comments based on engagement metrics
// func (s *commentService) GetTrendingComments(ctx context.Context, req *GetTrendingCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
// 	// Default to last 7 days if no time range is provided
// 	if req.TimeRange == nil {
// 		endTime := time.Now()
// 		startTime := endTime.Add(-7 * 24 * time.Hour) // Default to last 7 days
// 		req.TimeRange = &TimeRange{
// 			StartTime: startTime,
// 			EndTime:   endTime,
// 		}
// 	}

// 	// Set default pagination values if not provided
// 	if req.Pagination.Limit == 0 {
// 		req.Pagination.Limit = 10
// 	}

// 	// Get trending comments from repository
// 	response, err := s.commentRepo.GetTrendingComments(
// 		ctx,
// 		req.TimeRange.StartTime,
// 		req.TimeRange.EndTime,
// 		req.Pagination,
// 		req.UserID,
// 	)

// 	if err != nil {
// 		s.logger.Error("failed to get trending comments",
// 			zap.Error(err),
// 			zap.Time("start_time", req.TimeRange.StartTime),
// 			zap.Time("end_time", req.TimeRange.EndTime),
// 		)
// 		return nil, fmt.Errorf("failed to get trending comments: %w", err)
// 	}

// 	// Enrich comments with additional data
// 	enrichedComments := make([]*models.Comment, 0, len(response.Data))
// 	for _, comment := range response.Data {
// 		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
// 			s.logger.Warn("failed to enrich comment",
// 				zap.Int64("comment_id", comment.ID),
// 				zap.Error(err),
// 			)
// 			continue
// 		}
// 		enrichedComments = append(enrichedComments, comment)
// 	}

// 	// Update the response with enriched comments
// 	response.Data = enrichedComments

// 	return response, nil
// }

// // GetRecentComments retrieves the most recent comments
// func (s *commentService) GetRecentComments(ctx context.Context, req *GetRecentCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
// 	// Set default pagination values if not provided
// 	if req.Pagination.Limit == 0 {
// 		req.Pagination.Limit = 10
// 	}

// 	// Get recent comments from repository
// 	response, err := s.commentRepo.GetRecentComments(
// 		ctx,
// 		req.Pagination,
// 		req.UserID,
// 	)

// 	if err != nil {
// 		s.logger.Error("failed to get recent comments",
// 			zap.Error(err),
// 		)
// 		return nil, fmt.Errorf("failed to get recent comments: %w", err)
// 	}

// 	// Enrich comments with additional data
// 	enrichedComments := make([]*models.Comment, 0, len(response.Data))
// 	for _, comment := range response.Data {
// 		if err := s.enrichComment(ctx, comment, req.UserID); err != nil {
// 			s.logger.Warn("failed to enrich comment",
// 				zap.Int64("comment_id", comment.ID),
// 				zap.Error(err),
// 			)
// 			continue
// 		}
// 		enrichedComments = append(enrichedComments, comment)
// 	}

// 	// Update the response with enriched comments
// 	response.Data = enrichedComments

// 	return response, nil
// }

// // ===============================
// // HELPER METHODS
// // ===============================

// // validateCreateRequest validates create comment request
// func (s *commentService) validateCreateRequest(req *CreateCommentRequest) error {
// 	if req.UserID <= 0 {
// 		return fmt.Errorf("user ID is required")
// 	}
// 	if len(strings.TrimSpace(req.Content)) == 0 {
// 		return fmt.Errorf("content is required")
// 	}
// 	if len(req.Content) > s.config.MaxContentLength {
// 		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
// 	}

// 	// Must have exactly one parent
// 	parentCount := 0
// 	if req.PostID != nil {
// 		parentCount++
// 	}
// 	if req.QuestionID != nil {
// 		parentCount++
// 	}
// 	if req.DocumentID != nil {
// 		parentCount++
// 	}
// 	if parentCount != 1 {
// 		return fmt.Errorf("comment must have exactly one parent (post, question, or document)")
// 	}

// 	return nil
// }

// // validateUpdateRequest validates update comment request
// func (s *commentService) validateUpdateRequest(req *UpdateCommentRequest) error {
// 	if req.CommentID <= 0 {
// 		return fmt.Errorf("comment ID is required")
// 	}
// 	if req.UserID <= 0 {
// 		return fmt.Errorf("user ID is required")
// 	}
// 	if len(strings.TrimSpace(req.Content)) == 0 {
// 		return fmt.Errorf("content cannot be empty")
// 	}
// 	if len(req.Content) > s.config.MaxContentLength {
// 		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
// 	}

// 	return nil
// }

// // validateGetByPostRequest validates get comments by post request
// func (s *commentService) validateGetByPostRequest(req *GetCommentsByPostRequest) error {
// 	if req.PostID <= 0 {
// 		return fmt.Errorf("post ID is required")
// 	}
// 	if req.Pagination.Limit < 0 {
// 		return fmt.Errorf("limit cannot be negative")
// 	}
// 	if req.Pagination.Offset < 0 {
// 		return fmt.Errorf("offset cannot be negative")
// 	}

// 	return nil
// }

// // validateReactionRequest validates reaction request
// func (s *commentService) validateReactionRequest(req *ReactToCommentRequest) error {
// 	if req.CommentID <= 0 {
// 		return fmt.Errorf("comment ID is required")
// 	}
// 	if req.UserID <= 0 {
// 		return fmt.Errorf("user ID is required")
// 	}
// 	if req.ReactionType != "like" && req.ReactionType != "dislike" {
// 		return fmt.Errorf("invalid reaction type")
// 	}

// 	return nil
// }

// // validateParentContent validates that the parent content exists
// func (s *commentService) validateParentContent(ctx context.Context, req *CreateCommentRequest) error {
// 	if req.PostID != nil {
// 		post, err := s.postRepo.GetByID(ctx, *req.PostID)
// 		if err != nil {
// 			return NewInternalError("failed to validate parent post")
// 		}
// 		if post == nil {
// 			return NewNotFoundError("parent post not found")
// 		}
// 	}

// 	// Similar validation for QuestionID and DocumentID would go here
// 	// if req.QuestionID != nil { ... }
// 	// if req.DocumentID != nil { ... }

// 	return nil
// }

// // moderateContent performs basic content moderation
// func (s *commentService) moderateContent(content string) error {
// 	// Basic content filtering
// 	bannedWords := []string{"spam", "scam", "illegal"}
	
// 	lowerContent := strings.ToLower(content)
// 	for _, word := range bannedWords {
// 		if strings.Contains(lowerContent, word) {
// 			return fmt.Errorf("content contains prohibited words")
// 		}
// 	}

// 	return nil
// }

// // checkCommentRateLimit checks if user is commenting too frequently
// func (s *commentService) checkCommentRateLimit(ctx context.Context, userID int64) error {
// 	key := fmt.Sprintf("comment_rate_limit:%d", userID)
// 	count, _ := s.cache.Increment(ctx, key, 1)
	
// 	if count == 1 {
// 		s.cache.SetTTL(ctx, key, 1*time.Hour)
// 	}

// 	if count > int64(s.config.MaxCommentsPerHour) {
// 		return NewRateLimitError("commenting rate limit exceeded", map[string]interface{}{
// 			"limit":      s.config.MaxCommentsPerHour,
// 			"reset_time": "1 hour",
// 		})
// 	}

// 	return nil
// }

// // getInitialStatus returns the initial status for new comments
// func (s *commentService) getInitialStatus() string {
// 	if s.config.RequireApproval {
// 		return "pending"
// 	}
// 	return "published"
// }

// // extractMentions extracts @mentions from content
// func (s *commentService) extractMentions(content string) []string {
// 	// Simple regex to find @username mentions
// 	// This would be more sophisticated in a real implementation
// 	words := strings.Fields(content)
// 	var mentions []string
	
// 	for _, word := range words {
// 		if strings.HasPrefix(word, "@") && len(word) > 1 {
// 			username := strings.TrimPrefix(word, "@")
// 			// Remove common punctuation
// 			username = strings.TrimRight(username, ".,!?;:")
// 			if len(username) > 0 {
// 				mentions = append(mentions, username)
// 			}
// 		}
// 	}
	
// 	return mentions
// }

// // enrichComment adds additional data to a comment
// func (s *commentService) enrichComment(ctx context.Context, comment *models.Comment, userID *int64) error {
// 	// Get author information
// 	author, err := s.userService.GetUserByID(ctx, comment.UserID)
// 	if err == nil && author != nil {
// 		comment.AuthorUsername = author.Username
// 		comment.AuthorProfileURL = author.ProfileURL
// 	}

// 	// Get engagement counts
// 	if stats, err := s.commentRepo.GetCommentStats(ctx, comment.ID); err == nil {
// 		comment.LikesCount = stats.LikesCount
// 		comment.DislikesCount = stats.DislikesCount
// 	}

// 	// Add user-specific data if userID provided
// 	if userID != nil {
// 		s.enrichCommentWithUserData(ctx, comment, *userID)
// 	}

// 	return nil
// }

// // enrichCommentWithUserData adds user-specific data to a comment
// func (s *commentService) enrichCommentWithUserData(ctx context.Context, comment *models.Comment, userID int64) {
// 	// Check if user has reacted
// 	if reaction, err := s.commentRepo.GetUserReaction(ctx, comment.ID, userID); err == nil {
// 		comment.UserReaction = &reaction
// 	}

// 	// Check ownership
// 	comment.IsOwner = (comment.UserID == userID)
// }

// // truncateContent safely truncates content for logging
// func (s *commentService) truncateContent(content string, maxLen int) string {
// 	if len(content) <= maxLen {
// 		return content
// 	}
// 	return content[:maxLen] + "..."
// }

// // invalidateCommentCaches invalidates relevant caches
// func (s *commentService) invalidateCommentCaches(ctx context.Context, comment *models.Comment) {
// 	// Invalidate post comments cache
// 	if comment.PostID != nil {
// 		s.cache.DeletePattern(ctx, fmt.Sprintf("comments:post:%d:*", *comment.PostID))
// 	}
	
// 	// Invalidate question comments cache
// 	if comment.QuestionID != nil {
// 		s.cache.DeletePattern(ctx, fmt.Sprintf("comments:question:%d:*", *comment.QuestionID))
// 	}
	
// 	// Invalidate user comments cache
// 	s.cache.DeletePattern(ctx, fmt.Sprintf("comments:user:%d:*", comment.UserID))
// }

// // processMentions processes user mentions in comments
// func (s *commentService) processMentions(ctx context.Context, comment *models.Comment, mentions []string) {
// 	// This would validate mentioned users exist and store mention relationships
// 	for _, username := range mentions {
// 		// Get user by username and create mention record
// 		user, err := s.userService.GetUserByUsername(ctx, username)
// 		if err == nil && user != nil {
// 			// Store mention relationship
// 			s.logger.Debug("Processed mention",
// 				zap.String("username", username),
// 				zap.Int64("comment_id", comment.ID),
// 			)
// 		}
// 	}
// }

// // notifyMentionedUsers sends notifications to mentioned users
// func (s *commentService) notifyMentionedUsers(ctx context.Context, comment *models.Comment, mentions []string) {
// 	for _, username := range mentions {
// 		user, err := s.userService.GetUserByUsername(ctx, username)
// 		if err == nil && user != nil {
// 			// Publish mention notification event
// 			if err := s.events.Publish(ctx, &events.UserMentionedEvent{
// 				BaseEvent: events.BaseEvent{
// 					EventID:   events.GenerateEventID(),
// 					EventType: "user.mentioned",
// 					Timestamp: time.Now(),
// 					UserID:    &user.ID,
// 				},
// 				MentionedByUserID: comment.UserID,
// 				CommentID:        comment.ID,
// 				PostID:           comment.PostID,
// 				QuestionID:       comment.QuestionID,
// 			}); err != nil {
// 				s.logger.Warn("Failed to publish mention event", zap.Error(err))
// 			}
// 		}
// 	}
// }

// // notifyParentAuthor notifies the author of the parent content
// func (s *commentService) notifyParentAuthor(ctx context.Context, comment *models.Comment) {
// 	if comment.PostID != nil {
// 		// Get post and notify author
// 		post, err := s.postRepo.GetByID(ctx, *comment.PostID)
// 		if err == nil && post != nil && post.UserID != comment.UserID {
// 			// Publish comment notification event
// 			if err := s.events.Publish(ctx, &events.CommentNotificationEvent{
// 				BaseEvent: events.BaseEvent{
// 					EventID:   events.GenerateEventID(),
// 					EventType: "comment.notification",
// 					Timestamp: time.Now(),
// 					UserID:    &post.UserID,
// 				},
// 				CommentID:     comment.ID,
// 				CommenterID:   comment.UserID,
// 				PostID:        comment.PostID,
// 				CommentPreview: s.truncateContent(comment.Content, 100),
// 			}); err != nil {
// 				s.logger.Warn("Failed to publish comment notification event", zap.Error(err))
// 			}
// 		}
// 	}
	
// 	// Similar logic for questions and documents would go here
// }
