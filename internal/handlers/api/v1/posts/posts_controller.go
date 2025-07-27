// ===============================
// FILE: internal/handlers/api/v1/posts/posts_controller.go
// UPGRADED TO MATCH COMMENTS CONTROLLER PATTERNS
// ===============================

package posts

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"evalhub/internal/middleware"
	"evalhub/internal/models"
	"evalhub/internal/response"
	"evalhub/internal/services"
)

// PostController handles enterprise post management API endpoints with enhanced security
type PostController struct {
	serviceCollection *services.ServiceCollection
	responseBuilder   *response.Builder
	logger            *zap.Logger

	// 🆕 UPGRADED PAGINATION SYSTEM
	paginationParser  *response.PaginationParser
	paginationBuilder *response.PaginationBuilder
}

// NewPostController creates a new post API controller with enhanced features
func NewPostController(
	serviceCollection *services.ServiceCollection,
	logger *zap.Logger,
	responseBuilder *response.Builder,
) *PostController {
	return &PostController{
		serviceCollection: serviceCollection,
		logger:            logger,
		responseBuilder:   responseBuilder,
		paginationParser:  response.NewPaginationParser(response.DefaultPaginationConfig()),
		paginationBuilder: response.NewPaginationBuilder(response.DefaultPaginationConfig()),
	}
}

// ===============================
// 🛡️ ENHANCED SECURITY HELPER FUNCTIONS
// ===============================

// canUserModifyPost checks if user can modify a post using existing RBAC system
func (c *PostController) canUserModifyPost(r *http.Request, post *models.Post) bool {
	authCtx := middleware.GetAuthContext(r.Context())
	if authCtx == nil {
		return false
	}

	// ✅ Admin can modify any post (existing role system)
	if authCtx.Role == "admin" {
		return true
	}

	// ✅ Moderator can modify any post (existing role system)
	if authCtx.Role == "moderator" {
		return true
	}

	// ✅ Owner can modify own post (existing ownership check)
	return post.UserID == authCtx.UserID
}

// canUserModeratePost checks if user can moderate posts using existing RBAC
func (c *PostController) canUserModeratePost(r *http.Request) bool {
	authCtx := middleware.GetAuthContext(r.Context())
	if authCtx == nil {
		return false
	}

	// ✅ Only admin and moderator can moderate (existing role system)
	return authCtx.Role == "admin" || authCtx.Role == "moderator"
}

// canUserDeletePost checks if user can delete a post
func (c *PostController) canUserDeletePost(r *http.Request, post *models.Post) bool {
	authCtx := middleware.GetAuthContext(r.Context())
	if authCtx == nil {
		return false
	}

	// ✅ Admin can delete any post
	if authCtx.Role == "admin" {
		return true
	}

	// ✅ Moderator can delete any post
	if authCtx.Role == "moderator" {
		return true
	}

	// ✅ Owner can delete own post
	return post.UserID == authCtx.UserID
}

// validateContentSecurity performs enhanced content validation using existing systems
func (c *PostController) validateContentSecurity(title, content string) error {
	if err := models.ContentValidator("title", title, 5, 255); err != nil {
		return err
	}

	if err := models.ContentValidator("content", content, 10, 50000); err != nil {
		return err
	}

	// ✅ Enhanced spam detection (building on existing patterns)
	if c.containsSpamPatterns(title + " " + content) {
		return &services.ValidationError{
			ServiceError: &services.ServiceError{
				Message: "Content detected as potential spam",
				Code:    "SPAM_DETECTED",
			},
			Fields: []services.FieldError{
				{
					Field:   "content",
					Message: "Content detected as potential spam",
					Code:    "SPAM_DETECTED",
				},
			},
		}
	}

	// ✅ Enhanced profanity check (building on existing validation)
	if c.containsInappropriateContent(content) {
		return &services.ValidationError{
			ServiceError: &services.ServiceError{
				Message: "Content contains inappropriate material",
				Code:    "INAPPROPRIATE_CONTENT",
			},
			Fields: []services.FieldError{
				{
					Field:   "content",
					Message: "Content contains inappropriate material",
					Code:    "INAPPROPRIATE_CONTENT",
				},
			},
		}
	}

	// 🆕 ENHANCED XSS DETECTION
	if c.checkXSS(title + " " + content) {
		return fmt.Errorf("content contains potential XSS")
	}

	// 🆕 ENHANCED SQL INJECTION DETECTION
	if c.checkSQLInjection(title + " " + content) {
		return fmt.Errorf("content contains potential SQL injection")
	}

	return nil
}

// containsSpamPatterns checks for spam-like content patterns
func (c *PostController) containsSpamPatterns(content string) bool {
	lowerContent := strings.ToLower(content)

	// Check for excessive repeated characters (building on existing logic)
	if c.hasExcessiveRepetition(content) {
		return true
	}

	// Check for common spam indicators
	spamPatterns := []string{
		"buy now", "click here", "free money", "get rich quick",
		"guaranteed", "limited time", "act now", "call now",
		"100% free", "risk free", "no obligation",
	}

	for _, pattern := range spamPatterns {
		if strings.Contains(lowerContent, pattern) {
			return true
		}
	}

	// Check for excessive links
	linkCount := strings.Count(lowerContent, "http://") + strings.Count(lowerContent, "https://")
	if linkCount > 3 {
		return true
	}

	return false
}

// containsInappropriateContent checks for inappropriate content
func (c *PostController) containsInappropriateContent(content string) bool {
	// This would integrate with your content moderation policies
	// For now, basic check for excessive caps (shouting)
	upperCount := 0
	for _, char := range content {
		if char >= 'A' && char <= 'Z' {
			upperCount++
		}
	}

	// If more than 70% is uppercase, consider it inappropriate
	if len(content) > 10 && float64(upperCount)/float64(len(content)) > 0.7 {
		return true
	}

	return false
}

// hasExcessiveRepetition checks for spam-like repeated characters (from existing validation)
func (c *PostController) hasExcessiveRepetition(content string) bool {
	if len(content) < 10 {
		return false
	}

	// Check for more than 5 consecutive identical characters
	for i := 0; i < len(content)-5; i++ {
		char := content[i]
		count := 1

		for j := i + 1; j < len(content) && content[j] == char; j++ {
			count++
			if count > 5 {
				return true
			}
		}
	}

	return false
}

// 🆕 ENHANCED XSS DETECTION
func (c *PostController) checkXSS(content string) bool {
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

// 🆕 ENHANCED SQL INJECTION DETECTION
func (c *PostController) checkSQLInjection(content string) bool {
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

// ===============================
// CORE CRUD OPERATIONS (SECURITY ENHANCED)
// ===============================

// CreatePost creates a new post with enhanced security validation
// POST /api/v1/posts
func (c *PostController) CreatePost(w http.ResponseWriter, r *http.Request) {
	// 🆕 UPGRADED AUTHENTICATION PATTERN
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse Content-Type to handle both JSON and multipart form data
	contentType := r.Header.Get("Content-Type")

	var req services.CreatePostRequest
	req.UserID = authCtx.UserID

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle multipart form for file uploads
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
			validationErr := &services.ValidationError{
				ServiceError: &services.ServiceError{
					Type:       "VALIDATION_ERROR",
					Message:    "Failed to parse form data (max 32MB)",
					StatusCode: response.StatusBadRequest,
				},
			}
			c.responseBuilder.WriteError(w, r, validationErr)
			return
		}

		// Extract form fields
		req.Title = strings.TrimSpace(r.FormValue("title"))
		req.Content = strings.TrimSpace(r.FormValue("content"))
		req.Category = strings.TrimSpace(r.FormValue("category"))

		// 🛡️ Enhanced content security validation
		if err := c.validateContentSecurity(req.Title, req.Content); err != nil {
			c.responseBuilder.WriteError(w, r, err)
			return
		}

		// Handle status (optional, defaults to published)
		if status := r.FormValue("status"); status != "" {
			req.Status = &status
		}

		// Handle image upload with enhanced security
		file, handler, err := r.FormFile("image")
		if err == nil {
			defer file.Close()

			// ✅ Enhanced file validation (building on existing patterns)
			if err := c.validateImageUpload(handler); err != nil {
				validationErr := &services.ValidationError{
					ServiceError: &services.ServiceError{
						Type:       "VALIDATION_ERROR",
						Message:    "Failed to validate image",
						StatusCode: response.StatusBadRequest,
					},
				}
				c.responseBuilder.WriteError(w, r, validationErr)
				return
			}

			// Upload image via file service
			fileService := c.serviceCollection.GetFileService()
			if fileService != nil {
				uploadResult, err := fileService.UploadImage(r.Context(), &services.FileUploadRequest{
					UserID:      authCtx.UserID,
					File:        file,
					Filename:    handler.Filename,
					ContentType: handler.Header.Get("Content-Type"),
					Size:        handler.Size,
					Folder:      "posts",
				})
				if err != nil {
					validationErr := &services.ValidationError{
						ServiceError: &services.ServiceError{
							Type:       "VALIDATION_ERROR",
							Message:    "Failed to upload image",
							StatusCode: response.StatusBadRequest,
						},
					}
					c.responseBuilder.WriteError(w, r, validationErr)
					return
				}

				req.ImageURL = &uploadResult.URL
				req.ImagePublicID = &uploadResult.PublicID
			}
		} else if err != http.ErrMissingFile {
			validationErr := &services.ValidationError{
				ServiceError: &services.ServiceError{
					Type:       "VALIDATION_ERROR",
					Message:    "Error processing image",
					StatusCode: response.StatusBadRequest,
				},
			}
			c.responseBuilder.WriteError(w, r, validationErr)
			return
		}

	} else {
		// Handle JSON request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			validationErr := &services.ValidationError{
				ServiceError: &services.ServiceError{
					Type:       "VALIDATION_ERROR",
					Message:    "Invalid request body",
					StatusCode: response.StatusBadRequest,
				},
			}
			c.responseBuilder.WriteError(w, r, validationErr)
			return
		}
		req.UserID = authCtx.UserID

		// 🛡️ Enhanced content security validation for JSON requests too
		if err := c.validateContentSecurity(req.Title, req.Content); err != nil {
			c.responseBuilder.WriteError(w, r, err)
			return
		}
	}

	// 🆕 STRUCTURED VALIDATION
	if err := c.validateCreatePostRequest(&req); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid post data",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Create post via service
	postService := c.serviceCollection.GetPostService()
	post, err := postService.CreatePost(r.Context(), &req)
	if err != nil {
		c.handleServiceError(w, r, err, "create post") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("Post created successfully via API",
		zap.Int64("post_id", post.ID),
		zap.Int64("user_id", authCtx.UserID),
		zap.String("category", post.Category),
		zap.String("status", post.Status),
	)

	c.responseBuilder.WriteCreated(w, r, post)
}

// UpdatePost updates an existing post with enhanced authorization
// PUT /api/v1/posts/{post_id}
func (c *PostController) UpdatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3) // 🆕 STANDARDIZED URL PARSING
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

	// 🛡️ Get current post for enhanced authorization check
	postService := c.serviceCollection.GetPostService()
	currentPost, err := postService.GetPostByID(r.Context(), postID, &authCtx.UserID)
	if err != nil {
		c.handleServiceError(w, r, err, "get post for update")
		return
	}

	// 🛡️ Enhanced authorization check (Admin, Moderator, or Owner)
	if !c.canUserModifyPost(r, currentPost) {
		c.logger.Warn("Unauthorized post modification attempt",
			zap.Int64("user_id", authCtx.UserID),
			zap.String("user_role", authCtx.Role),
			zap.Int64("post_id", postID),
			zap.Int64("post_owner", currentPost.UserID),
		)
		authErr := &services.AuthorizationError{
			ServiceError: &services.ServiceError{
				Type:       "AUTHORIZATION_ERROR",
				Message:    "Insufficient permissions to update post",
				StatusCode: response.StatusForbidden,
			},
		}
		c.responseBuilder.WriteError(w, r, authErr)
		return
	}

	// Parse request body
	var req services.UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// 🛡️ Enhanced content security validation for updates
	if req.Title != nil && req.Content != nil {
		if err := c.validateContentSecurity(*req.Title, *req.Content); err != nil {
			c.responseBuilder.WriteError(w, r, err)
			return
		}
	} else if req.Title != nil {
		if err := c.validateContentSecurity(*req.Title, currentPost.Content); err != nil {
			c.responseBuilder.WriteError(w, r, err)
			return
		}
	} else if req.Content != nil {
		if err := c.validateContentSecurity(currentPost.Title, *req.Content); err != nil {
			c.responseBuilder.WriteError(w, r, err)
			return
		}
	}

	// Set IDs from context/path
	req.PostID = postID
	req.UserID = authCtx.UserID

	post, err := postService.UpdatePost(r.Context(), &req)
	if err != nil {
		c.handleServiceError(w, r, err, "update post")
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("Post updated successfully via API",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", authCtx.UserID),
	)

	c.responseBuilder.WriteSuccess(w, r, post)
}

// DeletePost deletes a post with enhanced authorization
// DELETE /api/v1/posts/{post_id}/delete
func (c *PostController) DeletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// 🛡️ Get current post for enhanced authorization check
	postService := c.serviceCollection.GetPostService()
	currentPost, err := postService.GetPostByID(r.Context(), postID, &authCtx.UserID)
	if err != nil {
		c.handleServiceError(w, r, err, "get post for delete") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🛡️ Enhanced authorization check (Admin, Moderator, or Owner)
	if !c.canUserDeletePost(r, currentPost) {
		c.logger.Warn("Unauthorized post deletion attempt",
			zap.Int64("user_id", authCtx.UserID),
			zap.String("user_role", authCtx.Role),
			zap.Int64("post_id", postID),
			zap.Int64("post_owner", currentPost.UserID),
		)
		authErr := &services.AuthorizationError{
			ServiceError: &services.ServiceError{
				Type:       "AUTHORIZATION_ERROR",
				Message:    "Insufficient permissions to delete post",
				StatusCode: response.StatusForbidden,
			},
		}
		c.responseBuilder.WriteError(w, r, authErr)
		return
	}

	if err := postService.DeletePost(r.Context(), postID, authCtx.UserID); err != nil {
		c.handleServiceError(w, r, err, "delete post") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("Post deleted successfully via API",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", authCtx.UserID),
	)

	c.responseBuilder.WriteNoContent(w, r)
}

// ===============================
// 🛡️ ENHANCED MODERATION OPERATIONS (Role-based)
// ===============================

// ModeratePost handles moderation actions with role-based access control
// POST /api/v1/posts/{post_id}/moderate
func (c *PostController) ModeratePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// 🛡️ Enhanced role-based authorization (Admin or Moderator only)
	if !c.canUserModeratePost(r) {
		c.logger.Warn("Unauthorized moderation attempt",
			zap.Int64("user_id", authCtx.UserID),
			zap.String("user_role", authCtx.Role),
		)
		authErr := &services.AuthorizationError{
			ServiceError: &services.ServiceError{
				Type:       "AUTHORIZATION_ERROR",
				Message:    "Insufficient permissions to moderate content",
				StatusCode: response.StatusForbidden,
			},
		}
		c.responseBuilder.WriteError(w, r, authErr)
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3) // 🆕 STANDARDIZED URL PARSING
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

	// Parse request body
	var requestBody struct {
		Action string `json:"action" validate:"required,oneof=approve reject hide flag"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build moderation request
	req := &services.ModerateContentRequest{
		ContentType: "post",
		ContentID:   postID,
		ModeratorID: authCtx.UserID,
		Action:      requestBody.Action,
		Reason:      requestBody.Reason,
	}

	postService := c.serviceCollection.GetPostService()
	if err := postService.ModeratePost(r.Context(), req); err != nil {
		c.handleServiceError(w, r, err, "moderate post") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🛡️ Enhanced logging for moderation actions
	c.logger.Info("Post moderation action completed",
		zap.Int64("post_id", postID),
		zap.Int64("moderator_id", authCtx.UserID),
		zap.String("moderator_role", authCtx.Role),
		zap.String("action", requestBody.Action),
		zap.String("reason", requestBody.Reason),
	)

	response := map[string]interface{}{
		"message": "Post moderated successfully",
		"post_id": postID,
		"action":  requestBody.Action,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// 🛡️ ENHANCED FILE UPLOAD VALIDATION
// ===============================

// validateImageUpload performs enhanced image upload validation
func (c *PostController) validateImageUpload(fileHeader *multipart.FileHeader) error {
	// ✅ File size validation
	if fileHeader.Size > 10*1024*1024 { // 10MB limit
		return fmt.Errorf("image too large (max 10MB)")
	}

	// ✅ Content type validation
	contentType := fileHeader.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("file must be an image")
	}

	// ✅ Allowed image types
	allowedTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp",
	}

	isAllowed := false
	for _, allowedType := range allowedTypes {
		if contentType == allowedType {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return fmt.Errorf("unsupported image type")
	}

	// ✅ Filename security check
	filename := strings.ToLower(fileHeader.Filename)

	// Check for dangerous file extensions
	dangerousExtensions := []string{
		".exe", ".bat", ".cmd", ".com", ".scr", ".sh", ".ps1", ".vbs", ".js",
	}

	for _, ext := range dangerousExtensions {
		if strings.HasSuffix(filename, ext) {
			return fmt.Errorf("dangerous file extension detected")
		}
	}

	return nil
}

// GetPost retrieves a post by ID with engagement data
// GET /api/v1/posts/{post_id}
func (c *PostController) GetPost(w http.ResponseWriter, r *http.Request) {
	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// Get current user ID (optional for user-specific data)
	authCtx := middleware.GetAuthContext(r.Context())
	var userIDPtr *int64
	if authCtx != nil {
		userIDPtr = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	post, err := postService.GetPostByID(r.Context(), postID, userIDPtr)
	if err != nil {
		c.handleServiceError(w, r, err, "get post")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, post)
}

// ===============================
// LISTING AND FILTERING (UPGRADED PAGINATION)
// ===============================

// ListPosts retrieves paginated list of posts with filtering
// GET /api/v1/posts
func (c *PostController) ListPosts(w http.ResponseWriter, r *http.Request) {
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
	req := &services.ListPostsRequest{
		Pagination: modelsPagination,
	}

	// Optional filters
	query := r.URL.Query()
	if category := query.Get("category"); category != "" {
		req.Category = &category
	}

	if status := query.Get("status"); status != "" {
		req.Status = &status
	}

	// Get current user ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context())
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	result, err := postService.ListPosts(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "list posts")
		return
	}

	c.writePaginatedResponse(w, r, result, paginationParams)
}

// GetPostsByUser retrieves posts by a specific user
// GET /api/v1/posts/user/{user_id}
func (c *PostController) GetPostsByUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path using new helper
	targetUserID, err := c.extractIDFromPath(r.URL.Path, 4)
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

	// 🆕 UPGRADED PAGINATION PARSING
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
	req := &services.GetPostsByUserRequest{
		TargetUserID: targetUserID,
		Pagination:   modelsPagination,
	}

	// Set viewer ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context())
	if authCtx != nil {
		req.ViewerID = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	result, err := postService.GetPostsByUser(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "get posts by user")
		return
	}

	// 🆕 UPGRADED PAGINATION RESPONSE
	c.writePaginatedResponse(w, r, result, paginationParams)
}

// GetPostsByCategory retrieves posts by category
// GET /api/v1/posts/category/{category}
func (c *PostController) GetPostsByCategory(w http.ResponseWriter, r *http.Request) {
	// Extract category from URL path using new helper
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Category required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	category := pathParts[4]
	if category == "" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Category cannot be empty",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// 🆕 UPGRADED PAGINATION PARSING
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
	req := &services.GetPostsByCategoryRequest{
		Category:   category,
		Pagination: modelsPagination,
	}

	// Set user ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context()) // 🆕 UPGRADED AUTHENTICATION PATTERN
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	result, err := postService.GetPostsByCategory(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "get posts by category") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 UPGRADED PAGINATION RESPONSE
	c.writePaginatedResponse(w, r, result, paginationParams)
}

// GetTrendingPosts retrieves trending posts based on engagement
// GET /api/v1/posts/category/{category}
func (c *PostController) GetTrendingPosts(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// Get current user ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context())
	var userIDPtr *int64
	if authCtx != nil {
		userIDPtr = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	posts, err := postService.GetTrendingPosts(r.Context(), limit, userIDPtr)
	if err != nil {
		c.handleServiceError(w, r, err, "get trending posts")
		return
	}

	response := map[string]interface{}{
		"posts": posts,
		"count": len(posts),
		"limit": limit,
		"type":  "trending",
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// GetFeaturedPosts retrieves featured posts
// GET /api/v1/posts/featured
func (c *PostController) GetFeaturedPosts(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 5
	}

	// Get current user ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context())
	var userIDPtr *int64
	if authCtx != nil {
		userIDPtr = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	posts, err := postService.GetFeaturedPosts(r.Context(), limit, userIDPtr)
	if err != nil {
		c.handleServiceError(w, r, err, "get featured posts")
		return
	}

	response := map[string]interface{}{
		"posts": posts,
		"count": len(posts),
		"limit": limit,
		"type":  "featured",
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// SEARCH AND DISCOVERY (UPGRADED)
// ===============================

// SearchPosts performs full-text search on posts
// GET /api/v1/posts/search?q={query}
func (c *PostController) SearchPosts(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	searchQuery := query.Get("q")
	if searchQuery == "" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Search query required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// 🆕 UPGRADED PAGINATION PARSING
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
	req := &services.SearchPostsRequest{
		Query:      searchQuery,
		Pagination: modelsPagination,
	}

	// Optional category filter
	if category := query.Get("category"); category != "" {
		req.Category = &category
	}

	// Set user ID for user-specific data
	authCtx := middleware.GetAuthContext(r.Context()) // 🆕 UPGRADED AUTHENTICATION PATTERN
	if authCtx != nil {
		req.UserID = &authCtx.UserID
	}

	postService := c.serviceCollection.GetPostService()
	result, err := postService.SearchPosts(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "search posts") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 UPGRADED PAGINATION RESPONSE
	c.writePaginatedResponse(w, r, result, paginationParams)
}

// ===============================
// ENGAGEMENT OPERATIONS (UPGRADED)
// ===============================

// ReactToPost handles post reactions (like/dislike)
// POST /api/v1/posts/{post_id}/react
func (c *PostController) ReactToPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// Parse request body
	var requestBody struct {
		ReactionType string `json:"reaction_type" validate:"required,oneof=like dislike"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build reaction request
	req := &services.ReactToPostRequest{
		PostID:       postID,
		UserID:       authCtx.UserID,
		ReactionType: requestBody.ReactionType,
	}

	postService := c.serviceCollection.GetPostService()
	if err := postService.ReactToPost(r.Context(), req); err != nil {
		c.handleServiceError(w, r, err, "react to post")
		return
	}

	response := map[string]interface{}{
		"message":       "Reaction added successfully",
		"post_id":       postID,
		"reaction_type": requestBody.ReactionType,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// RemoveReaction removes a user's reaction from a post
// DELETE /api/v1/posts/{post_id}/react
func (c *PostController) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	postService := c.serviceCollection.GetPostService()
	if err := postService.RemoveReaction(r.Context(), postID, authCtx.UserID); err != nil {
		c.handleServiceError(w, r, err, "remove reaction")
		return
	}

	response := map[string]interface{}{
		"message": "Reaction removed successfully",
		"post_id": postID,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// BookmarkPost bookmarks a post for the user
// POST /api/v1/posts/{post_id}/bookmark
func (c *PostController) BookmarkPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	postService := c.serviceCollection.GetPostService()
	if err := postService.BookmarkPost(r.Context(), authCtx.UserID, postID); err != nil {
		c.handleServiceError(w, r, err, "bookmark post") 
		return
	}

	response := map[string]interface{}{
		"message": "Post bookmarked successfully",
		"post_id": postID,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// UnbookmarkPost removes a bookmark from a post
// DELETE /api/v1/posts/{post_id}/bookmark
func (c *PostController) UnbookmarkPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	postService := c.serviceCollection.GetPostService()
	if err := postService.UnbookmarkPost(r.Context(), authCtx.UserID, postID); err != nil {
		c.handleServiceError(w, r, err, "unbookmark post")
		return
	}

	response := map[string]interface{}{
		"message": "Bookmark removed successfully",
		"post_id": postID,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// SharePost handles post sharing
// POST /api/v1/posts/{post_id}/share
func (c *PostController) SharePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// Parse request body
	var requestBody struct {
		Platform string `json:"platform" validate:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build share request
	req := &services.SharePostRequest{
		PostID:   postID,
		UserID:   authCtx.UserID,
		Platform: requestBody.Platform,
	}

	postService := c.serviceCollection.GetPostService()
	if err := postService.SharePost(r.Context(), req); err != nil {
		c.handleServiceError(w, r, err, "share post")
		return
	}

	response := map[string]interface{}{
		"message":  "Post shared successfully",
		"post_id":  postID,
		"platform": requestBody.Platform,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// CONTENT MODERATION (UPGRADED)
// ===============================

// ReportPost reports a post for moderation
// POST /api/v1/posts/{post_id}/report
func (c *PostController) ReportPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// Parse request body
	var requestBody struct {
		Reason      string `json:"reason" validate:"required"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid request body",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Build report request
	req := &services.ReportContentRequest{
		ContentType: "post",
		ContentID:   postID,
		ReporterID:  authCtx.UserID,
		Reason:      requestBody.Reason,
		Description: requestBody.Description,
	}

	postService := c.serviceCollection.GetPostService()
	if err := postService.ReportPost(r.Context(), req); err != nil {
		c.handleServiceError(w, r, err, "report post") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	response := map[string]interface{}{
		"message": "Post reported successfully",
		"post_id": postID,
		"reason":  requestBody.Reason,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// ANALYTICS AND STATISTICS (UPGRADED)
// ===============================

// GetPostStats retrieves comprehensive post statistics
// GET /api/v1/posts/{post_id}/stats
func (c *PostController) GetPostStats(w http.ResponseWriter, r *http.Request) {
	// Extract post ID from URL path using new helper
	postID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	postService := c.serviceCollection.GetPostService()
	stats, err := postService.GetPostStats(r.Context(), postID)
	if err != nil {
		c.handleServiceError(w, r, err, "get post stats")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, stats)
}

// GetPostAnalytics retrieves post analytics for the current user
// GET /api/v1/posts/{post_id}/analytics
func (c *PostController) GetPostAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse days parameter
	query := r.URL.Query()
	days, _ := strconv.Atoi(query.Get("days"))
	if days < 1 || days > 365 {
		days = 30 // Default to 30 days
	}

	postService := c.serviceCollection.GetPostService()
	analytics, err := postService.GetPostAnalytics(r.Context(), authCtx.UserID, days)
	if err != nil {
		c.handleServiceError(w, r, err, "get post analytics")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, analytics)
}

// ===============================
// 🆕 UPGRADED HELPER METHODS
// ===============================

// extractIDFromPath extracts an ID from URL path at specified position (standardized)
func (c *PostController) extractIDFromPath(path string, position int) (int64, error) {
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

// handleServiceError handles service errors with proper logging and response (centralized)
func (c *PostController) handleServiceError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	// Log the error with context
	c.logger.Error("Post service error",
		zap.Error(err),
		zap.String("operation", operation),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method),
	)

	// Handle service error using the response builder
	c.responseBuilder.WriteError(w, r, err)
}

// convertToModelsPagination converts response.PaginationParams to models.PaginationParams (upgraded)
func (c *PostController) convertToModelsPagination(params *response.PaginationParams) models.PaginationParams {
	return models.PaginationParams{
		Limit:  params.PageSize,
		Offset: params.Offset,
		Sort:   params.Sort,
		Order:  params.Order,
	}
}

// writePaginatedResponse writes a paginated response using the integrated systems (upgraded)
func (c *PostController) writePaginatedResponse(
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

// 🆕 STRUCTURED VALIDATION HELPERS
func (c *PostController) validateCreatePostRequest(req *services.CreatePostRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

// truncateContent safely truncates content for logging (utility)
// func (c *PostController) truncateContent(content string, maxLen int) string {
// 	if len(content) <= maxLen {
// 		return content
// 	}
// 	return content[:maxLen] + "..."
// }
