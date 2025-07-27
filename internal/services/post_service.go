// file: internal/services/post_service.go
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

// postService implements PostService with enterprise features
type postService struct {
	postRepo       repositories.PostRepository
	userRepo       repositories.UserRepository
	commentRepo    repositories.CommentRepository
	cache          cache.Cache
	events         events.EventBus
	fileService    FileService  // Changed from repositories.FileService
	userService    UserService
	transactionSvc TransactionService  // Changed from repositories.TransactionService
	logger         *zap.Logger
	config         *PostServiceConfig
}

// PostServiceConfig holds post service configuration
type PostServiceConfig struct {
	MaxTitleLength       int           `json:"max_title_length"`
	MaxContentLength     int           `json:"max_content_length"`
	MaxImageSize         int64         `json:"max_image_size"`
	AllowedCategories    []string      `json:"allowed_categories"`
	DefaultCacheTime     time.Duration `json:"default_cache_time"`
	TrendingCacheTime    time.Duration `json:"trending_cache_time"`
	EnableContentFilter  bool          `json:"enable_content_filter"`
	EnableAutoModeration bool          `json:"enable_auto_moderation"`
	MaxPostsPerHour      int           `json:"max_posts_per_hour"`
}

// NewPostService creates a new enterprise post service
func NewPostService(
	postRepo repositories.PostRepository,
	userRepo repositories.UserRepository,
	commentRepo repositories.CommentRepository,
	cache cache.Cache,
	events events.EventBus,
	fileService FileService,  // Changed type
	userService UserService,
	transactionSvc TransactionService,  // Changed type
	logger *zap.Logger,
	config *PostServiceConfig,
) PostService {
	if config == nil {
		config = DefaultPostConfig()
	}

	return &postService{
		postRepo:       postRepo,
		userRepo:       userRepo,
		commentRepo:    commentRepo,
		cache:          cache,
		events:         events,
		fileService:    fileService,
		userService:    userService,
		transactionSvc: transactionSvc,
		logger:         logger,
		config:         config,
	}
}

// DefaultPostConfig returns default post service configuration
func DefaultPostConfig() *PostServiceConfig {
	return &PostServiceConfig{
		MaxTitleLength:       255,
		MaxContentLength:     50000,
		MaxImageSize:         10 * 1024 * 1024, // 10MB
		AllowedCategories:    []string{"general", "technology", "science", "education"},
		DefaultCacheTime:     15 * time.Minute,
		TrendingCacheTime:    5 * time.Minute,
		EnableContentFilter:  true,
		EnableAutoModeration: true,
		MaxPostsPerHour:      10,
	}
}

// ===============================
// CORE CRUD OPERATIONS
// ===============================

// CreatePost creates a new post with comprehensive validation and side effects
func (s *postService) CreatePost(ctx context.Context, req *CreatePostRequest) (*models.Post, error) {
	// Validate request
	if err := s.validateCreateRequest(req); err != nil {
		return nil, NewValidationError("invalid create post request", err)
	}

	// Check rate limiting
	if err := s.checkPostRateLimit(ctx, req.UserID); err != nil {
		return nil, err
	}

	// Content moderation
	if s.config.EnableContentFilter {
		if err := s.moderateContent(req.Title, req.Content); err != nil {
			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
		}
	}

	// Execute in transaction for consistency
	var post *models.Post
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "create",
			Service: "post_service",
			Method:  "CreatePost",
		})

		// Create post model
		post = &models.Post{
			UserID:        req.UserID,
			Title:         strings.TrimSpace(req.Title),
			Content:       strings.TrimSpace(req.Content),
			Category:      req.Category,
			Status:        "published",
			ImageURL:      req.ImageURL,
			ImagePublicID: req.ImagePublicID,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		// Create post in database
		if err := s.postRepo.Create(ctx, post); err != nil {
			s.logger.Error("Failed to create post", zap.Error(err))
			return NewInternalError("failed to create post")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Invalidate relevant caches
	s.invalidatePostCaches(ctx, post.UserID, post.Category)

	// Publish post creation event
	if err := s.events.Publish(ctx, &events.PostCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "post.created",
			Timestamp: time.Now(),
			UserID:    &post.UserID,
		},
		PostID:   post.ID,
		Title:    post.Title,
		Category: post.Category,
	}); err != nil {
		s.logger.Warn("Failed to publish post created event", zap.Error(err))
	}

	s.logger.Info("Post created successfully",
		zap.Int64("post_id", post.ID),
		zap.Int64("user_id", post.UserID),
		zap.String("title", post.Title),
		zap.String("category", post.Category),
	)

	return post, nil
}

// GetPostByID retrieves a post by ID with comprehensive data loading
func (s *postService) GetPostByID(ctx context.Context, id int64, userID *int64) (*models.Post, error) {
	if id <= 0 {
		return nil, NewValidationError("invalid post ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("post:%d", id)
	if cachedPost, found := s.cache.Get(ctx, cacheKey); found {
		if post, ok := cachedPost.(*models.Post); ok {
			// Set user-specific data if userID provided
			if userID != nil {
				s.enrichPostWithUserData(ctx, post, *userID)
			}
			s.logger.Debug("Post retrieved from cache", zap.Int64("post_id", id))
			return post, nil
		}
	}

	// Get from database
	post, err := s.postRepo.GetByID(ctx, id, userID)
	if err != nil {
		s.logger.Error("Failed to get post by ID", zap.Error(err), zap.Int64("post_id", id))
		return nil, NewInternalError("failed to retrieve post")
	}

	if post == nil {
		return nil, NewNotFoundError("post not found")
	}

	// Enrich with additional data
	if err := s.enrichPost(ctx, post, userID); err != nil {
		s.logger.Warn("Failed to enrich post data", zap.Error(err), zap.Int64("post_id", id))
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, post, s.config.DefaultCacheTime); err != nil {
		s.logger.Warn("Failed to cache post", zap.Error(err), zap.Int64("post_id", id))
	}

	// Track view
	go s.trackPostView(ctx, id, userID)

	return post, nil
}

// UpdatePost updates an existing post with validation and authorization
func (s *postService) UpdatePost(ctx context.Context, req *UpdatePostRequest) (*models.Post, error) {
	// Validate request
	if err := s.validateUpdateRequest(req); err != nil {
		return nil, NewValidationError("invalid update post request", err)
	}

	// Get current post for authorization check
	currentPost, err := s.postRepo.GetByID(ctx, req.PostID, &req.UserID)
	if err != nil {
		return nil, NewInternalError("failed to retrieve current post")
	}
	if currentPost == nil {
		return nil, NewNotFoundError("post not found")
	}

	// Authorization check
	if currentPost.UserID != req.UserID {
		return nil, NewAuthorizationError("insufficient permissions to update post", "post", "update", req.UserID)
	}

	// Content moderation for updates
	if s.config.EnableContentFilter {
		title := req.Title
		content := req.Content
		if title == nil {
			title = &currentPost.Title
		}
		if content == nil {
			content = &currentPost.Content
		}
		if err := s.moderateContent(*title, *content); err != nil {
			return nil, NewBusinessError("content moderation failed", "CONTENT_REJECTED")
		}
	}

	// Execute update in transaction
	var updatedPost *models.Post
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "update",
			Service: "post_service",
			Method:  "UpdatePost",
		})

		// Update fields
		if req.Title != nil {
			currentPost.Title = strings.TrimSpace(*req.Title)
		}
		if req.Content != nil {
			currentPost.Content = strings.TrimSpace(*req.Content)
		}
		if req.Category != nil {
			currentPost.Category = *req.Category
		}
		if req.ImageURL != nil {
			currentPost.ImageURL = req.ImageURL
		}
		if req.ImagePublicID != nil {
			currentPost.ImagePublicID = req.ImagePublicID
		}
		currentPost.UpdatedAt = time.Now()

		// Update in database
		if err := s.postRepo.Update(ctx, currentPost); err != nil {
			s.logger.Error("Failed to update post", zap.Error(err), zap.Int64("post_id", req.PostID))
			return NewInternalError("failed to update post")
		}

		updatedPost = currentPost
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Invalidate caches
	s.invalidatePostCaches(ctx, updatedPost.UserID, updatedPost.Category)
	s.cache.Delete(ctx, fmt.Sprintf("post:%d", updatedPost.ID))

	// Publish post updated event
	if err := s.events.Publish(ctx, &events.PostUpdatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "post.updated",
			Timestamp: time.Now(),
			UserID:    &updatedPost.UserID,
		},
		PostID:  updatedPost.ID,
		UpdatedAt: time.Now(),
		Changes: s.getChangedFields(req),
	}); err != nil {
		s.logger.Warn("Failed to publish post updated event", zap.Error(err))
	}

	s.logger.Info("Post updated successfully",
		zap.Int64("post_id", updatedPost.ID),
		zap.Int64("user_id", updatedPost.UserID),
	)

	return updatedPost, nil
}

// DeletePost soft deletes a post with authorization
func (s *postService) DeletePost(ctx context.Context, postID, userID int64) error {
	if postID <= 0 {
		return NewValidationError("invalid post ID", nil)
	}

	// Get post for authorization
	post, err := s.postRepo.GetByID(ctx, postID, &userID)
	if err != nil {
		return NewInternalError("failed to retrieve post")
	}
	if post == nil {
		return NewNotFoundError("post not found")
	}

	// Authorization check
	if post.UserID != userID {
		return NewAuthorizationError("insufficient permissions to delete post", "post", "delete", userID)
	}

	// Execute deletion in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Track operation
		s.transactionSvc.AddOperation(ctx, txCtx.ID, &AddOperationRequest{
			Type:    "delete",
			Service: "post_service",
			Method:  "DeletePost",
		})

		// Delete post
		if err := s.postRepo.Delete(ctx, postID); err != nil {
			s.logger.Error("Failed to delete post", zap.Error(err), zap.Int64("post_id", postID))
			return NewInternalError("failed to delete post")
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Clean up associated resources
	go s.cleanupPostResources(ctx, post)

	// Invalidate caches
	s.invalidatePostCaches(ctx, post.UserID, post.Category)
	s.cache.Delete(ctx, fmt.Sprintf("post:%d", postID))

	// Publish post deleted event
	if err := s.events.Publish(ctx, &events.PostDeletedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "post.deleted",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		PostID: postID,
		DeletedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish post deleted event", zap.Error(err))
	}

	s.logger.Info("Post deleted successfully",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// ===============================
// LISTING AND FILTERING
// ===============================

// ListPosts retrieves paginated list of posts with filtering
func (s *postService) ListPosts(ctx context.Context, req *ListPostsRequest) (*models.PaginatedResponse[*models.Post], error) {
	// Validate request
	if err := s.validateListRequest(req); err != nil {
		return nil, NewValidationError("invalid list posts request", err)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Try cache for certain queries
	var cacheKey string
	if req.Category == nil && req.Status == nil {
		cacheKey = fmt.Sprintf("posts:list:%d:%d", req.Pagination.Limit, req.Pagination.Offset)
		if cachedPosts, found := s.cache.Get(ctx, cacheKey); found {
			if response, ok := cachedPosts.(*models.PaginatedResponse[*models.Post]); ok {
				// Enrich with user-specific data if needed
				if req.UserID != nil {
					for _, post := range response.Data {
						s.enrichPostWithUserData(ctx, post, *req.UserID)
					}
				}
				return response, nil
			}
		}
	}

	// Get posts from repository
	var response *models.PaginatedResponse[*models.Post]
	var err error

	if req.Category != nil {
		response, err = s.postRepo.GetByCategory(ctx, *req.Category, req.Pagination, req.UserID)
	} else if req.Status != nil {
		response, err = s.postRepo.GetByStatus(ctx, *req.Status, req.Pagination, req.UserID)
	} else {
		response, err = s.postRepo.List(ctx, req.Pagination, req.UserID)
	}

	if err != nil {
		s.logger.Error("Failed to list posts", zap.Error(err))
		return nil, NewInternalError("failed to retrieve posts")
	}

	// Enrich posts with additional data
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	// Cache the result if appropriate
	if cacheKey != "" {
		if err := s.cache.Set(ctx, cacheKey, response, s.config.DefaultCacheTime); err != nil {
			s.logger.Warn("Failed to cache posts list", zap.Error(err))
		}
	}

	return response, nil
}

// GetPostsByUser retrieves posts by a specific user
func (s *postService) GetPostsByUser(ctx context.Context, req *GetPostsByUserRequest) (*models.PaginatedResponse[*models.Post], error) {
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

	// Get posts by user
	response, err := s.postRepo.GetByUserID(ctx, req.TargetUserID, req.Pagination)
	if err != nil {
		s.logger.Error("Failed to get posts by user", zap.Error(err), zap.Int64("user_id", req.TargetUserID))
		return nil, NewInternalError("failed to retrieve user posts")
	}

	// Enrich posts
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, req.ViewerID); err != nil {
			s.logger.Warn("Failed to enrich post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	return response, nil
}

// GetPostsByCategory retrieves posts by category
func (s *postService) GetPostsByCategory(ctx context.Context, req *GetPostsByCategoryRequest) (*models.PaginatedResponse[*models.Post], error) {
	if req.Category == "" {
		return nil, NewValidationError("category is required", nil)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Get posts by category
	response, err := s.postRepo.GetByCategory(ctx, req.Category, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get posts by category", zap.Error(err), zap.String("category", req.Category))
		return nil, NewInternalError("failed to retrieve category posts")
	}

	// Enrich posts
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	return response, nil
}

// GetTrendingPosts retrieves trending posts based on engagement
func (s *postService) GetTrendingPosts(ctx context.Context, limit int, userID *int64) ([]*models.Post, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Try cache first
	cacheKey := fmt.Sprintf("posts:trending:%d", limit)
	if cachedPosts, found := s.cache.Get(ctx, cacheKey); found {
		if posts, ok := cachedPosts.([]*models.Post); ok {
			// Enrich with user-specific data
			if userID != nil {
				for _, post := range posts {
					s.enrichPostWithUserData(ctx, post, *userID)
				}
			}
			return posts, nil
		}
	}

	// Get trending posts from repository
	posts, err := s.postRepo.GetTrending(ctx, limit, userID)
	if err != nil {
		s.logger.Error("Failed to get trending posts", zap.Error(err))
		return nil, NewInternalError("failed to retrieve trending posts")
	}

	// Enrich posts
	for _, post := range posts {
		if err := s.enrichPost(ctx, post, userID); err != nil {
			s.logger.Warn("Failed to enrich trending post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, posts, s.config.TrendingCacheTime); err != nil {
		s.logger.Warn("Failed to cache trending posts", zap.Error(err))
	}

	return posts, nil
}

// GetFeaturedPosts retrieves featured posts
func (s *postService) GetFeaturedPosts(ctx context.Context, limit int, userID *int64) ([]*models.Post, error) {
	if limit <= 0 || limit > 100 {
		limit = 5
	}

	// Try cache first
	cacheKey := fmt.Sprintf("posts:featured:%d", limit)
	if cachedPosts, found := s.cache.Get(ctx, cacheKey); found {
		if posts, ok := cachedPosts.([]*models.Post); ok {
			if userID != nil {
				for _, post := range posts {
					s.enrichPostWithUserData(ctx, post, *userID)
				}
			}
			return posts, nil
		}
	}

	// Get featured posts (implementation depends on your featured logic)
	posts, err := s.postRepo.GetFeatured(ctx, limit, userID)
	if err != nil {
		s.logger.Error("Failed to get featured posts", zap.Error(err))
		return nil, NewInternalError("failed to retrieve featured posts")
	}

	// Enrich posts
	for _, post := range posts {
		if err := s.enrichPost(ctx, post, userID); err != nil {
			s.logger.Warn("Failed to enrich featured post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, posts, s.config.DefaultCacheTime); err != nil {
		s.logger.Warn("Failed to cache featured posts", zap.Error(err))
	}

	return posts, nil
}

// GetDraftPosts retrieves user's draft posts - MISSING METHOD IMPLEMENTATION
func (s *postService) GetDraftPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}

	// Set default pagination
	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}

	// Try cache first
	cacheKey := fmt.Sprintf("posts:drafts:%d:%d:%d", userID, params.Limit, params.Offset)
	if cachedPosts, found := s.cache.Get(ctx, cacheKey); found {
		if response, ok := cachedPosts.(*models.PaginatedResponse[*models.Post]); ok {
			// Enrich with user-specific data
			for _, post := range response.Data {
				s.enrichPostWithUserData(ctx, post, userID)
			}
			return response, nil
		}
	}

	// Get draft posts from repository
	response, err := s.postRepo.GetDrafts(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to get draft posts", zap.Error(err), zap.Int64("user_id", userID))
		return nil, NewInternalError("failed to retrieve draft posts")
	}

	// Enrich posts with additional data
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, &userID); err != nil {
			s.logger.Warn("Failed to enrich draft post", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, response, s.config.DefaultCacheTime); err != nil {
		s.logger.Warn("Failed to cache draft posts", zap.Error(err))
	}

	return response, nil
}

// ===============================
// SEARCH OPERATIONS
// ===============================

// SearchPosts performs full-text search on posts
func (s *postService) SearchPosts(ctx context.Context, req *SearchPostsRequest) (*models.PaginatedResponse[*models.Post], error) {
	// Validate request
	if err := s.validateSearchRequest(req); err != nil {
		return nil, NewValidationError("invalid search request", err)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	// Perform search
	response, err := s.postRepo.Search(ctx, req.Query, req.Pagination, req.UserID)
	if err != nil {
		s.logger.Error("Failed to search posts", zap.Error(err), zap.String("query", req.Query))
		return nil, NewInternalError("failed to search posts")
	}

	// Enrich posts
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, req.UserID); err != nil {
			s.logger.Warn("Failed to enrich search result", zap.Error(err), zap.Int64("post_id", post.ID))
		}
	}

	return response, nil
}

// ===============================
// ENGAGEMENT OPERATIONS
// ===============================

// ReactToPost handles user reactions to posts
func (s *postService) ReactToPost(ctx context.Context, req *ReactToPostRequest) error {
	// Validate request
	if err := s.validateReactionRequest(req); err != nil {
		return NewValidationError("invalid reaction request", err)
	}

	// Check if post exists
	post, err := s.postRepo.GetByID(ctx, req.PostID, &req.UserID)
	if err != nil {
		return NewInternalError("failed to retrieve post")
	}
	if post == nil {
		return NewNotFoundError("post not found")
	}

	// Execute reaction in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.UserID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Add reaction (repository handles add or update logic)
		if err := s.postRepo.AddReaction(ctx, req.PostID, req.UserID, req.ReactionType); err != nil {
			return NewInternalError("failed to add reaction")
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate post cache
	s.cache.Delete(ctx, fmt.Sprintf("post:%d", req.PostID))

	// Publish reaction event
	if err := s.events.Publish(ctx, &events.PostReactionEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "post.reacted",
			Timestamp: time.Now(),
			UserID:    &req.UserID,
		},
		PostID:       req.PostID,
		ReactionType: req.ReactionType,
	}); err != nil {
		s.logger.Warn("Failed to publish reaction event", zap.Error(err))
	}

	s.logger.Info("User reacted to post",
		zap.Int64("post_id", req.PostID),
		zap.Int64("user_id", req.UserID),
		zap.String("reaction", req.ReactionType),
	)

	return nil
}

// RemoveReaction removes a user's reaction from a post
func (s *postService) RemoveReaction(ctx context.Context, postID, userID int64) error {
	if postID <= 0 || userID <= 0 {
		return NewValidationError("invalid post or user ID", nil)
	}

	// Execute removal in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := s.postRepo.RemoveReaction(ctx, postID, userID); err != nil {
			return NewInternalError("failed to remove reaction")
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate post cache
	s.cache.Delete(ctx, fmt.Sprintf("post:%d", postID))

	s.logger.Info("User removed reaction from post",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// GetBookmarkedPosts retrieves a user's bookmarked posts
func (s *postService) GetBookmarkedPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}

	// Set default pagination if not provided
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}

	// Get bookmarked posts from repository
	response, err := s.postRepo.GetBookmarkedPosts(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to get bookmarked posts", 
			zap.Error(err), 
			zap.Int64("user_id", userID))
		return nil, NewInternalError("failed to retrieve bookmarked posts")
	}

	// Enrich posts with additional data
	for _, post := range response.Data {
		if err := s.enrichPost(ctx, post, &userID); err != nil {
			s.logger.Warn("Failed to enrich bookmarked post", 
				zap.Error(err), 
				zap.Int64("post_id", post.ID))
		}
	}

	return response, nil
}

// BookmarkPost bookmarks a post for a user
func (s *postService) BookmarkPost(ctx context.Context, userID, postID int64) error {
	if userID <= 0 || postID <= 0 {
		return NewValidationError("invalid user or post ID", nil)
	}

	// Execute bookmark in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := s.postRepo.AddBookmark(ctx, userID, postID); err != nil {
			return NewInternalError("failed to bookmark post")
		}
		return nil
	})

	if err != nil {
		return err
	}

	s.logger.Info("User bookmarked post",
		zap.Int64("user_id", userID),
		zap.Int64("post_id", postID),
	)

	return nil
}

// UnbookmarkPost removes a bookmark
func (s *postService) UnbookmarkPost(ctx context.Context, userID, postID int64) error {
	if userID <= 0 || postID <= 0 {
		return NewValidationError("invalid user or post ID", nil)
	}

	// Execute unbookmark in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &userID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := s.postRepo.RemoveBookmark(ctx, userID, postID); err != nil {
			return NewInternalError("failed to remove bookmark")
		}
		return nil
	})

	if err != nil {
		return err
	}

	s.logger.Info("User removed bookmark from post",
		zap.Int64("user_id", userID),
		zap.Int64("post_id", postID),
	)

	return nil
}

// SharePost handles post sharing
func (s *postService) SharePost(ctx context.Context, req *SharePostRequest) error {
	// Validate request
	if req.PostID <= 0 || req.UserID <= 0 {
		return NewValidationError("invalid post or user ID", nil)
	}

	// Check if post exists
	post, err := s.postRepo.GetByID(ctx, req.PostID, &req.UserID)
	if err != nil {
		return NewInternalError("failed to retrieve post")
	}
	if post == nil {
		return NewNotFoundError("post not found")
	}

	// Track share
	if err := s.postRepo.IncrementShareCount(ctx, req.PostID); err != nil {
		s.logger.Warn("Failed to increment share count", zap.Error(err))
	}

	// Publish share event
	shareEvent := events.NewPostSharedEvent(req.PostID, req.UserID, req.Platform)
	if err := s.events.Publish(ctx, shareEvent); err != nil {
		s.logger.Warn("Failed to publish share event", zap.Error(err))
	}

	s.logger.Info("Post shared",
		zap.Int64("post_id", req.PostID),
		zap.Int64("user_id", req.UserID),
		zap.String("platform", req.Platform),
	)

	return nil
}

// ===============================
// CONTENT MODERATION
// ===============================

// ReportPost reports a post for moderation
func (s *postService) ReportPost(ctx context.Context, req *ReportContentRequest) error {
	if req.ContentID <= 0 || req.ReporterID <= 0 {
		return NewValidationError("invalid content or reporter ID", nil)
	}

	// Execute report in transaction
	err := s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.ReporterID,
		Timeout: 15 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		if err := s.postRepo.AddReport(ctx, req.ContentID, req.ReporterID, req.Reason, req.Description); err != nil {
			return NewInternalError("failed to report post")
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Publish report event
	if err := s.events.Publish(ctx, events.NewContentReportedEvent(
		"post",
		req.ContentID,
		req.Reason,
		&req.ReporterID,
	)); err != nil {
		s.logger.Warn("Failed to publish report event", zap.Error(err))
	}

	s.logger.Info("Post reported for moderation",
		zap.Int64("post_id", req.ContentID),
		zap.Int64("reporter_id", req.ReporterID),
		zap.String("reason", req.Reason),
	)

	return nil
}

// ModeratePost handles moderation actions on posts
func (s *postService) ModeratePost(ctx context.Context, req *ModerateContentRequest) error {
	// Input validation
	if req.ContentID <= 0 {
		return NewValidationError("invalid content ID", nil)
	}

	if req.ModeratorID <= 0 {
		return NewValidationError("invalid moderator ID", nil)
	}

	// Ensure content type is 'post'
	req.ContentType = "post"

	// Validate required fields
	if req.Action == "" {
		return NewValidationError("action is required", nil)
	}

	// Validate action against allowed values from the struct tag
	validActions := map[string]bool{
		"approve": true,
		"reject":  true,
		"hide":    true,
		"warn":    true, // Note: Changed from 'delete' to 'warn' to match struct definition
	}

	if !validActions[req.Action] {
		return NewValidationError("invalid moderation action", nil)
	}

	// Get moderator info for logging
	moderator, err := s.userRepo.GetByID(ctx, req.ModeratorID)
	if err != nil {
		s.logger.Error("Failed to fetch moderator information",
			zap.Error(err),
			zap.Int64("moderator_id", req.ModeratorID),
		)
		return fmt.Errorf("failed to verify moderator: %w", err)
	}

	if moderator == nil {
		s.logger.Warn("Moderator not found",
			zap.Int64("moderator_id", req.ModeratorID),
		)
		return NewNotFoundError("moderator not found")
	}

	// Log moderation attempt
	s.logger.Info("Processing post moderation",
		zap.String("content_type", req.ContentType),
		zap.Int64("content_id", req.ContentID),
		zap.Int64("moderator_id", req.ModeratorID),
		zap.String("action", req.Action),
		zap.String("reason", req.Reason),
	)

	// Execute moderation in transaction
	err = s.transactionSvc.ExecuteInTransaction(ctx, &ExecuteInTransactionRequest{
		UserID:  &req.ModeratorID,
		Timeout: 30 * time.Second,
	}, func(ctx context.Context, txCtx *TransactionContext) error {
		// Combine reason and notes for the repository
		reason := req.Reason
		if req.Notes != "" {
			reason = fmt.Sprintf("%s\nNotes: %s", reason, req.Notes)
		}

		// Map 'warn' action to 'delete' if needed (repository uses 'delete' instead of 'warn')
		action := req.Action
		if action == "warn" {
			action = "delete"
		}

		// Use the repository's ModeratePost method which handles both the update and logging
		if err := s.postRepo.ModeratePost(ctx, req.ContentID, req.ModeratorID, action, reason); err != nil {
			s.logger.Error("Failed to moderate post",
				zap.Error(err),
				zap.String("content_type", req.ContentType),
				zap.Int64("content_id", req.ContentID),
				zap.Int64("moderator_id", req.ModeratorID),
				zap.String("action", action),
				zap.String("reason", reason),
			)
			return fmt.Errorf("failed to moderate post: %w", err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("moderation transaction failed: %w", err)
	}

	// Invalidate relevant caches
	post, err := s.postRepo.GetByID(ctx, req.ContentID, &req.ModeratorID)
	if err != nil {
		s.logger.Error("Failed to get post for cache invalidation", zap.Error(err))
		return nil // or handle error
	}
	if err := s.invalidatePostCaches(ctx, req.ContentID, post.Category); err != nil {
		s.logger.Warn("Failed to invalidate post caches",
			zap.Error(err),
			zap.Int64("post_id", req.ContentID),
			zap.String("category", post.Category),
		)
		// Continue execution even if cache invalidation fails
	}

	// Publish moderation event
	if s.events != nil {
		event := events.NewContentModeratedEvent(
			req.ContentType,
			req.ContentID,
			req.Action,
			req.Reason,
			&req.ModeratorID,
		)

		if err := s.events.Publish(ctx, event); err != nil {
			s.logger.Warn("Failed to publish moderation event",
				zap.Error(err),
				zap.String("content_type", req.ContentType),
				zap.Int64("content_id", req.ContentID),
			)
			// Continue execution even if event publishing fails
		}
	}

	// Log the successful moderation
	s.logger.Info("Content moderated successfully",
		zap.String("content_type", req.ContentType),
		zap.Int64("content_id", req.ContentID),
		zap.Int64("moderator_id", req.ModeratorID),
		zap.String("moderator_username", moderator.Username),
		zap.String("action", req.Action),
		zap.String("reason", req.Reason),
		zap.String("notes", req.Notes),
	)

	return nil
}

// ===============================
// ANALYTICS
// ===============================

// GetPostStats retrieves comprehensive post statistics
func (s *postService) GetPostStats(ctx context.Context, postID int64) (*PostStatsResponse, error) {
	if postID <= 0 {
		return nil, NewValidationError("invalid post ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("post_stats:%d", postID)
	if cachedStats, found := s.cache.Get(ctx, cacheKey); found {
		if stats, ok := cachedStats.(*PostStatsResponse); ok {
			return stats, nil
		}
	}

	// Get stats from repository
	repoStats, err := s.postRepo.GetPostStats(ctx, postID)
	if err != nil {
		s.logger.Error("Failed to get post stats", zap.Error(err), zap.Int64("post_id", postID))
		return nil, NewInternalError("failed to retrieve post statistics")
	}

	// Map repository stats to response
	stats := &PostStatsResponse{
		PostID:         repoStats.PostID,
		ViewsCount:     repoStats.ViewsCount,
		LikesCount:     repoStats.LikesCount,
		DislikesCount:  repoStats.DislikesCount,
		CommentsCount:  repoStats.CommentsCount,
		SharesCount:    repoStats.SharesCount,
		BookmarksCount: repoStats.BookmarksCount,
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, stats, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to cache post stats", zap.Error(err))
	}

	return stats, nil
}

// GetPostAnalytics retrieves analytics for user's posts
func (s *postService) GetPostAnalytics(ctx context.Context, userID int64, days int) (*PostAnalyticsResponse, error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}
	if days <= 0 || days > 365 {
		days = 30
	}

	// Get analytics from repository
	repoAnalytics, err := s.postRepo.GetPostAnalytics(ctx, userID, days)
	if err != nil {
		s.logger.Error("Failed to get post analytics", zap.Error(err), zap.Int64("user_id", userID))
		return nil, NewInternalError("failed to retrieve post analytics")
	}

	// Convert repository analytics to service response
	// Note: We need to fetch the actual Post models for TopPosts
	var topPostModels []*models.Post
	if len(repoAnalytics.TopPosts) > 0 {
		postIDs := make([]int64, 0, len(repoAnalytics.TopPosts))
		for _, p := range repoAnalytics.TopPosts {
			postIDs = append(postIDs, p.PostID)
		}
		
		topPostModels, err = s.postRepo.GetByIDs(ctx, postIDs, &userID)
		if err != nil {
			s.logger.Error("Failed to fetch top posts", zap.Error(err))
			topPostModels = nil // We'll return empty slice instead of failing the whole request
		}
	}

	// Convert repository daily stats to service layer stats
	dailyStats := make([]DailyPostStats, len(repoAnalytics.DailyStats))
	for i, stat := range repoAnalytics.DailyStats {
		dailyStats[i] = DailyPostStats{
			Date:       stat.Date,
			PostsCount: stat.PostsCount,
			TotalViews: stat.TotalViews,
			TotalLikes: stat.TotalLikes,
		}
	}

	return &PostAnalyticsResponse{
		UserID:     repoAnalytics.UserID,
		Days:       repoAnalytics.Days,
		TotalPosts: repoAnalytics.TotalPosts,
		TotalViews: repoAnalytics.TotalViews,
		TotalLikes: repoAnalytics.TotalLikes,
		DailyStats: dailyStats,
		TopPosts:   topPostModels,
	}, nil
}

// ===============================
// HELPER METHODS
// ===============================

// validateCreateRequest validates create post request
func (s *postService) validateCreateRequest(req *CreatePostRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if len(strings.TrimSpace(req.Title)) == 0 {
		return fmt.Errorf("title is required")
	}
	if len(req.Title) > s.config.MaxTitleLength {
		return fmt.Errorf("title too long (max %d characters)", s.config.MaxTitleLength)
	}
	if len(strings.TrimSpace(req.Content)) == 0 {
		return fmt.Errorf("content is required")
	}
	if len(req.Content) > s.config.MaxContentLength {
		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
	}
	if req.Category == "" {
		return fmt.Errorf("category is required")
	}

	// Validate category
	if !s.isValidCategory(req.Category) {
		return fmt.Errorf("invalid category")
	}

	return nil
}

// validateUpdateRequest validates update post request
func (s *postService) validateUpdateRequest(req *UpdatePostRequest) error {
	if req.PostID <= 0 {
		return fmt.Errorf("post ID is required")
	}
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if req.Title != nil && len(strings.TrimSpace(*req.Title)) == 0 {
		return fmt.Errorf("title cannot be empty")
	}
	if req.Title != nil && len(*req.Title) > s.config.MaxTitleLength {
		return fmt.Errorf("title too long (max %d characters)", s.config.MaxTitleLength)
	}
	if req.Content != nil && len(*req.Content) > s.config.MaxContentLength {
		return fmt.Errorf("content too long (max %d characters)", s.config.MaxContentLength)
	}
	if req.Category != nil && !s.isValidCategory(*req.Category) {
		return fmt.Errorf("invalid category")
	}

	return nil
}

// validateListRequest validates list posts request
func (s *postService) validateListRequest(req *ListPostsRequest) error {
	if req.Pagination.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if req.Pagination.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}
	if req.Category != nil && !s.isValidCategory(*req.Category) {
		return fmt.Errorf("invalid category")
	}

	return nil
}

// validateSearchRequest validates search request
func (s *postService) validateSearchRequest(req *SearchPostsRequest) error {
	if len(strings.TrimSpace(req.Query)) < 2 {
		return fmt.Errorf("search query must be at least 2 characters")
	}
	if len(req.Query) > 200 {
		return fmt.Errorf("search query too long (max 200 characters)")
	}
	if req.Category != nil && !s.isValidCategory(*req.Category) {
		return fmt.Errorf("invalid category")
	}

	return nil
}

// validateReactionRequest validates reaction request
func (s *postService) validateReactionRequest(req *ReactToPostRequest) error {
	if req.PostID <= 0 {
		return fmt.Errorf("post ID is required")
	}
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if req.ReactionType != "like" && req.ReactionType != "dislike" {
		return fmt.Errorf("invalid reaction type")
	}

	return nil
}

// isValidCategory checks if category is allowed
func (s *postService) isValidCategory(category string) bool {
	for _, allowed := range s.config.AllowedCategories {
		if category == allowed {
			return true
		}
	}
	return false
}

// moderateContent performs basic content moderation
func (s *postService) moderateContent(title, content string) error {
	// Basic content filtering - you can extend this
	bannedWords := []string{"spam", "scam", "illegal"} // This would be configurable

	fullText := strings.ToLower(title + " " + content)
	for _, word := range bannedWords {
		if strings.Contains(fullText, word) {
			return fmt.Errorf("content contains prohibited words")
		}
	}

	return nil
}

// checkPostRateLimit checks if user is posting too frequently
func (s *postService) checkPostRateLimit(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("post_rate_limit:%d", userID)
	count, _ := s.cache.Increment(ctx, key, 1)

	if count == 1 {
		// Set expiration for new key
		s.cache.SetTTL(ctx, key, 1*time.Hour)
	}

	if count > int64(s.config.MaxPostsPerHour) {
		return NewRateLimitError("posting rate limit exceeded", map[string]interface{}{
			"limit":      s.config.MaxPostsPerHour,
			"reset_time": "1 hour",
		})
	}

	return nil
}

// enrichPost adds additional data to a post
func (s *postService) enrichPost(ctx context.Context, post *models.Post, userID *int64) error {
	// Get author information
	author, err := s.userService.GetUserByID(ctx, post.UserID)
	if err == nil && author != nil {
		post.Username = author.Username
		post.AuthorProfileURL = author.ProfileURL
	}

	// Get engagement counts
	if stats, err := s.postRepo.GetPostStats(ctx, post.ID); err == nil {
		post.LikesCount = stats.LikesCount
		post.DislikesCount = stats.DislikesCount
		post.CommentsCount = stats.CommentsCount
		post.ViewsCount = stats.ViewsCount
	}

	// Add user-specific data if userID provided
	if userID != nil {
		s.enrichPostWithUserData(ctx, post, *userID)
	}

	return nil
}

// enrichPostWithUserData adds user-specific data to a post
func (s *postService) enrichPostWithUserData(ctx context.Context, post *models.Post, userID int64) {
	// Check if user has reacted
	if reaction, err := s.postRepo.GetUserReaction(ctx, post.ID, userID); err == nil {
		post.UserReaction = reaction
	}

	// Check if user has bookmarked
	if bookmarked, err := s.postRepo.IsBookmarked(ctx, userID, post.ID); err == nil {
		post.IsBookmarked = bookmarked
	}

	// Check ownership
	post.IsOwner = (post.UserID == userID)
}

// getChangedFields returns list of changed fields in update request
func (s *postService) getChangedFields(req *UpdatePostRequest) []string {
	var fields []string
	if req.Title != nil {
		fields = append(fields, "title")
	}
	if req.Content != nil {
		fields = append(fields, "content")
	}
	if req.Category != nil {
		fields = append(fields, "category")
	}
	if req.ImageURL != nil {
		fields = append(fields, "image")
	}
	return fields
}

// invalidatePostCaches invalidates relevant caches
func (s *postService) invalidatePostCaches(ctx context.Context, userID int64, category string) error {
	// Invalidate list caches
	if err := s.cache.DeletePattern(ctx, "posts:list:*"); err != nil {
		return fmt.Errorf("failed to invalidate list caches: %w", err)
	}

	if err := s.cache.DeletePattern(ctx, "posts:trending:*"); err != nil {
		return fmt.Errorf("failed to invalidate trending caches: %w", err)
	}

	if err := s.cache.DeletePattern(ctx, "posts:featured:*"); err != nil {
		return fmt.Errorf("failed to invalidate featured caches: %w", err)
	}

	// Invalidate user-specific caches
	if err := s.cache.DeletePattern(ctx, fmt.Sprintf("posts:user:%d:*", userID)); err != nil {
		return fmt.Errorf("failed to invalidate user caches: %w", err)
	}

	// Invalidate category caches
	if err := s.cache.DeletePattern(ctx, fmt.Sprintf("posts:category:%s:*", category)); err != nil {
		return fmt.Errorf("failed to invalidate category caches: %w", err)
	}

	return nil
}

// trackPostView tracks a post view
func (s *postService) trackPostView(ctx context.Context, postID int64, userID *int64) {
	if err := s.postRepo.IncrementViews(ctx, postID); err != nil {
		s.logger.Warn("Failed to increment view count", zap.Error(err), zap.Int64("post_id", postID))
	}

	// Publish view event
	ipAddress := "" // You might want to get the IP address from the context if available
	if err := s.events.Publish(ctx, events.NewPostViewedEvent(postID, userID, ipAddress)); err != nil {
		s.logger.Warn("Failed to publish view event", zap.Error(err))
	}
}

// cleanupPostResources cleans up resources associated with a deleted post
func (s *postService) cleanupPostResources(ctx context.Context, post *models.Post) {
	// Delete associated image if exists
	if post.ImagePublicID != nil && *post.ImagePublicID != "" {
		if err := s.fileService.DeleteFile(ctx, *post.ImagePublicID); err != nil {
			s.logger.Warn("Failed to delete post image",
				zap.Error(err),
				zap.String("public_id", *post.ImagePublicID),
			)
		}
	}
}