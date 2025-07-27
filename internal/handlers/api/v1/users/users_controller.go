// ===============================
// FILE: internal/handlers/api/v1/users/users_controller.go
// UPGRADED TO MATCH COMMENTS CONTROLLER PATTERNS
// ===============================

package users

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"evalhub/internal/middleware"
	"evalhub/internal/models"
	"evalhub/internal/response"
	"evalhub/internal/services"

	"go.uber.org/zap"
)

// UserController handles enterprise user management API endpoints with enhanced security
type UserController struct {
	serviceCollection *services.ServiceCollection
	responseBuilder   *response.Builder
	logger            *zap.Logger
	
	// 🆕 UPGRADED PAGINATION SYSTEM
	paginationParser  *response.PaginationParser
	paginationBuilder *response.PaginationBuilder
}

// NewUserController creates a new user API controller with enhanced features
func NewUserController(
	serviceCollection *services.ServiceCollection, 
	logger *zap.Logger,
	responseBuilder *response.Builder, // 🆕 ACCEPT AS PARAMETER
) *UserController {
	return &UserController{
		serviceCollection: serviceCollection,
		logger:            logger,
		responseBuilder:   responseBuilder, // 🆕 DON'T CREATE INLINE
		paginationParser:  response.NewPaginationParser(response.DefaultPaginationConfig()),
		paginationBuilder: response.NewPaginationBuilder(response.DefaultPaginationConfig()),
	}
}

// ===============================
// 🛡️ ENHANCED SECURITY HELPER FUNCTIONS
// ===============================

// validateContentSecurity performs enhanced content validation
func (c *UserController) validateContentSecurity(content string) error {
	if content == "" {
		return nil // Allow empty content for optional fields
	}

	// 🆕 ENHANCED XSS DETECTION
	if c.checkXSS(content) {
		return fmt.Errorf("content contains potential XSS")
	}

	// 🆕 ENHANCED SQL INJECTION DETECTION  
	if c.checkSQLInjection(content) {
		return fmt.Errorf("content contains potential SQL injection")
	}

	return nil
}

// 🆕 ENHANCED XSS DETECTION
func (c *UserController) checkXSS(content string) bool {
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
func (c *UserController) checkSQLInjection(content string) bool {
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

// 🆕 ENHANCED FILE UPLOAD VALIDATION
func (c *UserController) validateFileUpload(fileHeader *multipart.FileHeader, allowedTypes []string, maxSize int64) error {
	// File size validation
	if fileHeader.Size > maxSize {
		return fmt.Errorf("file too large (max %d MB)", maxSize/(1024*1024))
	}

	// Content type validation
	contentType := fileHeader.Header.Get("Content-Type")
	isAllowed := false
	for _, allowedType := range allowedTypes {
		if contentType == allowedType {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return fmt.Errorf("unsupported file type: %s", contentType)
	}

	// Filename security check
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

// ===============================
// USER PROFILE ENDPOINTS (UPGRADED)
// ===============================

// GetProfile retrieves the current user's profile
// GET /api/v1/users/profile
func (c *UserController) GetProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	userService := c.serviceCollection.GetUserService()
	user, err := userService.GetUserByID(r.Context(), authCtx.UserID)
	if err != nil {
		c.handleServiceError(w, r, err, "get user profile")
		return
	}

	// Get user stats for enhanced profile with fallback
	var stats interface{}
	if statsService, ok := userService.(interface {
		GetUserStats(context.Context, int64) (*services.UserStatsResponse, error)
	}); ok {
		if userStats, err := statsService.GetUserStats(r.Context(), authCtx.UserID); err == nil {
			stats = userStats
		}
	}

	profileData := map[string]interface{}{
		"user":  user,
		"stats": stats,
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("User profile retrieved successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.String("operation", "get_profile"),
	)

	c.responseBuilder.WriteSuccess(w, r, profileData)
}

// GetUserByID retrieves a user by ID
// GET /api/v1/users/{id}
func (c *UserController) GetUserByID(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path using standardized helper
	userID, err := c.extractIDFromPath(r.URL.Path, 3) // 🆕 STANDARDIZED URL PARSING
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

	userService := c.serviceCollection.GetUserService()
	user, err := userService.GetUserByID(r.Context(), userID)
	if err != nil {
		c.handleServiceError(w, r, err, "get user by ID")
		return
	}

	c.responseBuilder.WriteSuccess(w, r, user)
}

// GetUserByUsername retrieves a user by username
// GET /api/v1/users/username/{username}
func (c *UserController) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Username required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	username := pathParts[4]
	if username == "" {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Username cannot be empty",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	userService := c.serviceCollection.GetUserService()
	user, err := userService.GetUserByUsername(r.Context(), username)
	if err != nil {
		c.handleServiceError(w, r, err, "get user by username") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	c.responseBuilder.WriteSuccess(w, r, user)
}

// UpdateProfile updates the current user's profile
// PUT /api/v1/users/profile
func (c *UserController) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse request body
	var req services.UpdateUserRequest
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

	// Set user ID from context
	req.UserID = authCtx.UserID

	// 🆕 STRUCTURED VALIDATION
	if err := c.validateUpdateUserRequest(&req); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Invalid user data",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	userService := c.serviceCollection.GetUserService()
	user, err := userService.UpdateUser(r.Context(), &req)
	if err != nil {
		c.handleServiceError(w, r, err, "update user profile")
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("User profile updated successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.String("operation", "update_profile"),
	)

	c.responseBuilder.WriteSuccess(w, r, user)
}

// ===============================
// FILE UPLOAD ENDPOINTS (ENHANCED SECURITY)
// ===============================

// UploadProfileImage handles profile image upload with enhanced security
// POST /api/v1/users/profile/image
func (c *UserController) UploadProfileImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Failed to parse form data (max 10MB)",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Image file required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}
	defer file.Close()

	allowedTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp",
	}
	if err := c.validateFileUpload(handler, allowedTypes, 10*1024*1024); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("File validation failed: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Create upload request
	uploadReq := &services.FileUploadRequest{
		UserID:      authCtx.UserID,
		File:        file,
		Filename:    handler.Filename,
		ContentType: handler.Header.Get("Content-Type"),
		Size:        handler.Size,
		Folder:      "profile_images",
	}

	userService := c.serviceCollection.GetUserService()
	result, err := userService.UploadProfileImage(r.Context(), uploadReq)
	if err != nil {
		c.handleServiceError(w, r, err, "upload profile image")
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("Profile image uploaded successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.String("public_id", result.PublicID),
		zap.String("operation", "upload_profile_image"),
	)

	response := map[string]interface{}{
		"upload_result": result,
		"message":       "Profile image uploaded successfully",
	}

	c.responseBuilder.WriteCreated(w, r, response)
}

// UploadCV handles CV/resume upload with enhanced security
// POST /api/v1/users/cv
func (c *UserController) UploadCV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse multipart form (max 15MB)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "Failed to parse form data (max 15MB)",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	file, handler, err := r.FormFile("cv")
	if err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    "CV file required",
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}
	defer file.Close()

	// 🆕 ENHANCED FILE VALIDATION
	allowedTypes := []string{
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
	if err := c.validateFileUpload(handler, allowedTypes, 15*1024*1024); err != nil {
		validationErr := &services.ValidationError{
			ServiceError: &services.ServiceError{
				Type:       "VALIDATION_ERROR",
				Message:    fmt.Sprintf("File validation failed: %s", err.Error()),
				StatusCode: response.StatusBadRequest,
			},
		}
		c.responseBuilder.WriteError(w, r, validationErr)
		return
	}

	// Create upload request
	uploadReq := &services.FileUploadRequest{
		UserID:      authCtx.UserID,
		File:        file,
		Filename:    handler.Filename,
		ContentType: handler.Header.Get("Content-Type"),
		Size:        handler.Size,
		Folder:      "user_cvs",
	}

	userService := c.serviceCollection.GetUserService()
	result, err := userService.UploadCV(r.Context(), uploadReq)
	if err != nil {
		c.handleServiceError(w, r, err, "upload CV") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("CV uploaded successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.String("public_id", result.PublicID),
		zap.String("operation", "upload_cv"),
	)

	response := map[string]interface{}{
		"upload_result": result,
		"message":       "CV uploaded successfully",
	}

	c.responseBuilder.WriteCreated(w, r, response)
}

// ===============================
// USER LISTING AND SEARCH (UPGRADED PAGINATION)
// ===============================

// ListUsers retrieves a paginated list of users with optional filtering
// GET /api/v1/user
func (c *UserController) ListUsers(w http.ResponseWriter, r *http.Request) {
	authCtx := middleware.GetAuthContext(r.Context())

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
	req := &services.ListUsersRequest{
		Pagination: modelsPagination,
	}

	// Add exclude current user
	if authCtx != nil {
		req.ExcludeID = &authCtx.UserID
	}

	// Optional filters
	query := r.URL.Query()
	if role := query.Get("role"); role != "" {
		req.Role = &role
	}
	if expertise := query.Get("expertise"); expertise != "" {
		req.Expertise = &expertise
	}

	userService := c.serviceCollection.GetUserService()
	result, err := userService.ListUsers(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "list users") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 UPGRADED PAGINATION RESPONSE
	c.writePaginatedResponse(w, r, result, paginationParams)
}

// SearchUsers searches for users based on query criteria
// GET /api/v1/users/search
func (c *UserController) SearchUsers(w http.ResponseWriter, r *http.Request) {
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
	req := &services.SearchUsersRequest{
		Query:      searchQuery,
		Pagination: modelsPagination,
	}

	userService := c.serviceCollection.GetUserService()
	result, err := userService.SearchUsers(r.Context(), req)
	if err != nil {
		c.handleServiceError(w, r, err, "search users") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 UPGRADED PAGINATION RESPONSE
	c.writePaginatedResponse(w, r, result, paginationParams)
}

// ===============================
// USER ANALYTICS AND STATS (ENHANCED)
// ===============================

// GetUserStats retrieves comprehensive statistics for a user
func (c *UserController) GetUserStats(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path using standardized helper
	userID, err := c.extractIDFromPath(r.URL.Path, 3) // 🆕 STANDARDIZED URL PARSING
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

	userService := c.serviceCollection.GetUserService()
	
	// 🆕 TYPE-SAFE SERVICE METHOD CHECKING
	if statsService, ok := userService.(interface {
		GetUserStats(context.Context, int64) (*services.UserStatsResponse, error)
	}); ok {
		stats, err := statsService.GetUserStats(r.Context(), userID)
		if err != nil {
			c.handleServiceError(w, r, err, "get user stats") // 🆕 CENTRALIZED ERROR HANDLING
			return
		}
		c.responseBuilder.WriteSuccess(w, r, stats)
	} else {
		// 🆕 FALLBACK IMPLEMENTATION
		c.getUserStatsFallback(w, r, userID)
	}
}

// GetOnlineUsers retrieves currently online users
// GET /api/v1/users/online
func (c *UserController) GetOnlineUsers(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	userService := c.serviceCollection.GetUserService()
	users, err := userService.GetOnlineUsers(r.Context(), limit)
	if err != nil {
		c.handleServiceError(w, r, err, "get online users") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	response := map[string]interface{}{
		"users": users,
		"count": len(users),
		"limit": limit,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// GetLeaderboard retrieves user leaderboard by reputation
// GET /api/v1/users/leaderboard
func (c *UserController) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	// Parse limit parameter
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	userService := c.serviceCollection.GetUserService()
	users, err := userService.GetLeaderboard(r.Context(), limit)
	if err != nil {
		c.handleServiceError(w, r, err, "get leaderboard") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	response := map[string]interface{}{
		"leaderboard": users,
		"count":       len(users),
		"limit":       limit,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// USER ACTIVITY AND PRESENCE (UPGRADED)
// ===============================

// UpdateOnlineStatus updates the current user's online status
// POST /api/v1/users/online-status
func (c *UserController) UpdateOnlineStatus(w http.ResponseWriter, r *http.Request) {
	// 🆕 UPGRADED AUTHENTICATION PATTERN
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse request body
	var req struct {
		Online bool `json:"online"`
	}
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

	userService := c.serviceCollection.GetUserService()
	if err := userService.UpdateOnlineStatus(r.Context(), authCtx.UserID, req.Online); err != nil {
		c.handleServiceError(w, r, err, "update online status") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("User online status updated successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.Bool("online", req.Online),
		zap.String("operation", "update_online_status"),
	)

	response := map[string]interface{}{
		"status":  "success",
		"online":  req.Online,
		"user_id": authCtx.UserID,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// GetUserActivity retrieves user activity for a specific period
// GET /api/v1/users/{id}/activity
func (c *UserController) GetUserActivity(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from URL path using standardized helper
	userID, err := c.extractIDFromPath(r.URL.Path, 3)
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

	// Parse days parameter
	query := r.URL.Query()
	days, _ := strconv.Atoi(query.Get("days"))
	if days < 1 || days > 365 {
		days = 30 // Default to 30 days
	}

	userService := c.serviceCollection.GetUserService()
	
	// 🆕 TYPE-SAFE SERVICE METHOD CHECKING
	if activityService, ok := userService.(interface {
		GetUserActivity(context.Context, int64, int) (*services.UserActivityResponse, error)
	}); ok {
		activity, err := activityService.GetUserActivity(r.Context(), userID, days)
		if err != nil {
			c.handleServiceError(w, r, err, "get user activity") // 🆕 CENTRALIZED ERROR HANDLING
			return
		}
		c.responseBuilder.WriteSuccess(w, r, activity)
	} else {
		// 🆕 FALLBACK IMPLEMENTATION
		c.getUserActivityFallback(w, r, userID, days)
	}
}

// ===============================
// ACCOUNT MANAGEMENT (UPGRADED)
// ===============================

// DeactivateAccount deactivates the current user's account
// POST /api/v1/users/deactivate
func (c *UserController) DeactivateAccount(w http.ResponseWriter, r *http.Request) {
	// 🆕 UPGRADED AUTHENTICATION PATTERN
	ctx := r.Context()
	authCtx := middleware.GetAuthContext(ctx)
	if authCtx == nil {
		c.responseBuilder.WriteUnauthorized(w, r, "Authentication required")
		return
	}

	// Parse optional reason
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Reason == "" {
		req.Reason = "User requested account deactivation"
	}

	userService := c.serviceCollection.GetUserService()
	if err := userService.DeactivateUser(r.Context(), authCtx.UserID, req.Reason); err != nil {
		c.handleServiceError(w, r, err, "deactivate account") // 🆕 CENTRALIZED ERROR HANDLING
		return
	}

	// 🆕 ENHANCED LOGGING
	c.logger.Info("User account deactivated successfully via API",
		zap.Int64("user_id", authCtx.UserID),
		zap.String("reason", req.Reason),
		zap.String("operation", "deactivate_account"),
	)

	response := map[string]interface{}{
		"status":  "success",
		"message": "Account deactivated successfully",
		"reason":  req.Reason,
	}

	c.responseBuilder.WriteSuccess(w, r, response)
}

// ===============================
// 🆕 UPGRADED HELPER METHODS
// ===============================

// extractIDFromPath extracts an ID from URL path at specified position (standardized)
func (c *UserController) extractIDFromPath(path string, position int) (int64, error) {
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
func (c *UserController) handleServiceError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	// Log the error with context
	c.logger.Error("User service error",
		zap.Error(err),
		zap.String("operation", operation),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method),
	)

	// Handle service error using the response builder
	c.responseBuilder.WriteError(w, r, err)
}

// convertToModelsPagination converts response.PaginationParams to models.PaginationParams (upgraded)
func (c *UserController) convertToModelsPagination(params *response.PaginationParams) models.PaginationParams {
	return models.PaginationParams{
		Limit:  params.PageSize,
		Offset: params.Offset,
		Sort:   params.Sort,
		Order:  params.Order,
	}
}

// writePaginatedResponse writes a paginated response using the integrated systems (upgraded)
func (c *UserController) writePaginatedResponse(
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
func (c *UserController) validateUpdateUserRequest(req *services.UpdateUserRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user ID is required")
	}

	// Validate optional fields with security checks
	if req.FirstName != nil {
		if err := c.validateContentSecurity(*req.FirstName); err != nil {
			return fmt.Errorf("first name validation failed: %s", err.Error())
		}
	}

	if req.LastName != nil {
		if err := c.validateContentSecurity(*req.LastName); err != nil {
			return fmt.Errorf("last name validation failed: %s", err.Error())
		}
	}

	if req.Bio != nil {
		if err := c.validateContentSecurity(*req.Bio); err != nil {
			return fmt.Errorf("bio validation failed: %s", err.Error())
		}
	}

	return nil
}

// truncateContent safely truncates content for logging (utility)
func (c *UserController) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// ===============================
// 🆕 FALLBACK IMPLEMENTATIONS
// ===============================

// getUserStatsFallback provides fallback when service method doesn't exist
// GET /api/v1/users/{id}/stats
func (c *UserController) getUserStatsFallback(w http.ResponseWriter, r *http.Request, userID int64) {
	fallbackData := map[string]interface{}{
		"message": "Advanced user statistics feature is coming soon",
		"user_id": userID,
		"basic_stats": map[string]interface{}{
			"posts_count":    0,
			"comments_count": 0,
			"reputation":     0,
		},
	}
	c.responseBuilder.WriteSuccess(w, r, fallbackData)
}

// getUserActivityFallback provides fallback when service method doesn't exist
// GET /api/v1/users/{id}/activity
func (c *UserController) getUserActivityFallback(w http.ResponseWriter, r *http.Request, userID int64, days int) {
	fallbackData := map[string]interface{}{
		"message": "Advanced user activity tracking feature is coming soon",
		"user_id": userID,
		"days":    days,
		"activity": map[string]interface{}{
			"total_actions": 0,
			"daily_stats":   []interface{}{},
		},
	}
	c.responseBuilder.WriteSuccess(w, r, fallbackData)
}