// file: internal/services/user_service.go
package services

import (
	"context"
	"evalhub/internal/cache"
	"evalhub/internal/events"
	"evalhub/internal/models"
	"evalhub/internal/validation"
	"evalhub/internal/repositories"
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// userService implements UserService with enterprise features
type userService struct {
	userRepo    repositories.UserRepository
	sessionRepo repositories.SessionRepository
	cache       cache.Cache
	events      events.EventBus	
	fileService FileService
	logger *zap.Logger
}

// NewUserService creates a new enterprise user service
func NewUserService(
	userRepo repositories.UserRepository,
	sessionRepo repositories.SessionRepository,
	cache cache.Cache,
	events events.EventBus,
	fileService FileService,
	logger *zap.Logger,
) UserService {
	return &userService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		cache:       cache,
		events:      events,
		fileService: fileService,
		logger:      logger,
	}
}

// ===============================
// CORE CRUD OPERATIONS
// ===============================

// CreateUser creates a new user with comprehensive validation and side effects
func (s *userService) CreateUser(ctx context.Context, req *CreateUserRequest) (*models.User, error) {
	// Validate request
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid create user request", err)
	}

	// Check if user already exists
	if existingUser, _ := s.userRepo.GetByEmail(ctx, req.Email); existingUser != nil {
		return nil, NewBusinessError("user already exists", "USER_ALREADY_EXISTS")
	}

	if existingUser, _ := s.userRepo.GetByUsername(ctx, req.Username); existingUser != nil {
		return nil, NewBusinessError("username already taken", "USERNAME_TAKEN")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		return nil, NewInternalError("failed to process password")
	}

	// Create user model
	user := &models.User{
		Email:              req.Email,
		Username:           req.Username,
		PasswordHash:       string(hashedPassword),
		FirstName:          req.FirstName,
		LastName:           req.LastName,
		Affiliation:        req.Affiliation,
		Role:               "user",
		Expertise:          "none",
		EmailNotifications: req.MarketingEmails,
		IsActive:           true,
		EmailVerified:      false,
	}

	// Create user in database
	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error("Failed to create user", zap.Error(err), zap.String("email", req.Email))
		return nil, NewInternalError("failed to create user")
	}

	// Publish user creation event
	if err := s.events.Publish(ctx, &events.UserCreatedEvent{
		UserID:    user.ID,
		Email:     user.Email,
		Username:  user.Username,
		CreatedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish user created event", zap.Error(err), zap.Int64("user_id", user.ID))
	}

	s.logger.Info("User created successfully",
		zap.Int64("user_id", user.ID),
		zap.String("email", user.Email),
		zap.String("username", user.Username),
	)

	// Return user without password hash
	user.PasswordHash = ""
	return user, nil
}

// GetUserByID retrieves a user by ID with caching
func (s *userService) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	if id <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("user:%d", id)
	if cachedUser, found := s.cache.Get(ctx, cacheKey); found {
		if user, ok := cachedUser.(*models.User); ok {
			s.logger.Debug("User retrieved from cache", zap.Int64("user_id", id))
			return user, nil
		}
	}

	// Get from database
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get user by ID", zap.Error(err), zap.Int64("user_id", id))
		return nil, NewInternalError("failed to retrieve user")
	}

	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, user, 15*time.Minute); err != nil {
		s.logger.Warn("Failed to cache user", zap.Error(err), zap.Int64("user_id", id))
	}

	// Clear password hash before returning
	user.PasswordHash = ""
	return user, nil
}

// GetUserByUsername retrieves a user by username with caching
func (s *userService) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	if username == "" {
		return nil, NewValidationError("username is required", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("user:username:%s", username)
	if cachedUser, found := s.cache.Get(ctx, cacheKey); found {
		if user, ok := cachedUser.(*models.User); ok {
			return user, nil
		}
	}

	// Get from database
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		s.logger.Error("Failed to get user by username", zap.Error(err), zap.String("username", username))
		return nil, NewInternalError("failed to retrieve user")
	}

	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, user, 15*time.Minute); err != nil {
		s.logger.Warn("Failed to cache user", zap.Error(err), zap.String("username", username))
	}

	// Clear password hash before returning
	user.PasswordHash = ""
	return user, nil
}

// GetUserByGitHubID retrieves a user by their GitHub ID
func (s *userService) GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	if githubID <= 0 {
		return nil, NewValidationError("invalid GitHub ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("user:github:%d", githubID)
	if cachedUser, found := s.cache.Get(ctx, cacheKey); found {
		if user, ok := cachedUser.(*models.User); ok {
			return user, nil
		}
	}

	// Get from database
	user, err := s.userRepo.GetByGitHubID(ctx, githubID)
	if err != nil {
		s.logger.Error("Failed to get user by GitHub ID", zap.Error(err), zap.Int64("github_id", githubID))
		return nil, NewInternalError("failed to retrieve user")
	}

	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, user, 15*time.Minute); err != nil {
		s.logger.Warn("Failed to cache user", zap.Error(err), zap.Int64("github_id", githubID))
	}

	// Clear password hash before returning
	user.PasswordHash = ""
	return user, nil
}

// GetUserByEmail retrieves a user by email (for authentication)
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	if email == "" {
		return nil, NewValidationError("email is required", nil)
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		s.logger.Error("Failed to get user by email", zap.Error(err), zap.String("email", email))
		return nil, NewInternalError("failed to retrieve user")
	}

	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	return user, nil
}

// UpdateUser updates user information with validation and cache invalidation
func (s *userService) UpdateUser(ctx context.Context, req *UpdateUserRequest) (*models.User, error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid update user request", err)
	}

	// Get current user
	currentUser, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, NewInternalError("failed to retrieve current user")
	}
	if currentUser == nil {
		return nil, NewNotFoundError("user not found")
	}

	// Update fields if provided
	updated := false
	if req.FirstName != nil && *req.FirstName != *currentUser.FirstName {
		currentUser.FirstName = req.FirstName
		updated = true
	}
	if req.LastName != nil && *req.LastName != *currentUser.LastName {
		currentUser.LastName = req.LastName
		updated = true
	}
	if req.JobTitle != nil && *req.JobTitle != *currentUser.JobTitle {
		currentUser.JobTitle = req.JobTitle
		updated = true
	}
	if req.Affiliation != nil && *req.Affiliation != *currentUser.Affiliation {
		currentUser.Affiliation = req.Affiliation
		updated = true
	}
	if req.Bio != nil && *req.Bio != *currentUser.Bio {
		currentUser.Bio = req.Bio
		updated = true
	}
	if req.YearsExperience != nil && *req.YearsExperience != currentUser.YearsExperience {
		currentUser.YearsExperience = *req.YearsExperience
		updated = true
	}
	if req.CoreCompetencies != nil && *req.CoreCompetencies != *currentUser.CoreCompetencies {
		currentUser.CoreCompetencies = req.CoreCompetencies
		updated = true
	}
	if req.Expertise != nil && *req.Expertise != currentUser.Expertise {
		currentUser.Expertise = *req.Expertise
		updated = true
	}
	if req.WebsiteURL != nil && *req.WebsiteURL != *currentUser.WebsiteURL {
		currentUser.WebsiteURL = req.WebsiteURL
		updated = true
	}
	if req.LinkedinProfile != nil && *req.LinkedinProfile != *currentUser.LinkedinProfile {
		currentUser.LinkedinProfile = req.LinkedinProfile
		updated = true
	}
	if req.TwitterHandle != nil && *req.TwitterHandle != *currentUser.TwitterHandle {
		currentUser.TwitterHandle = req.TwitterHandle
		updated = true
	}
	if req.EmailNotifications != nil && *req.EmailNotifications != currentUser.EmailNotifications {
		currentUser.EmailNotifications = *req.EmailNotifications
		updated = true
	}

	if !updated {
		// No changes, return current user
		currentUser.PasswordHash = ""
		return currentUser, nil
	}

	// Update in database
	if err := s.userRepo.Update(ctx, currentUser); err != nil {
		s.logger.Error("Failed to update user", zap.Error(err), zap.Int64("user_id", req.UserID))
		return nil, NewInternalError("failed to update user")
	}

	// Invalidate cache
	s.invalidateUserCache(ctx, currentUser)

	// Publish user updated event
	if err := s.events.Publish(ctx, &events.UserUpdatedEvent{
		UserID:    currentUser.ID,
		UpdatedAt: time.Now(),
		Changes:   s.getChangedFields(req),
	}); err != nil {
		s.logger.Warn("Failed to publish user updated event", zap.Error(err), zap.Int64("user_id", currentUser.ID))
	}

	s.logger.Info("User updated successfully",
		zap.Int64("user_id", currentUser.ID),
		zap.String("username", currentUser.Username),
	)

	// Clear password hash before returning
	currentUser.PasswordHash = ""
	return currentUser, nil
}

// DeactivateUser soft deletes a user account
func (s *userService) DeactivateUser(ctx context.Context, userID int64, reason string) error {
	if userID <= 0 {
		return NewValidationError("invalid user ID", nil)
	}

	// Get user first to check if exists
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return NewInternalError("failed to retrieve user")
	}
	if user == nil {
		return NewNotFoundError("user not found")
	}

	// Deactivate user
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		s.logger.Error("Failed to deactivate user", zap.Error(err), zap.Int64("user_id", userID))
		return NewInternalError("failed to deactivate user")
	}

	// Invalidate all user sessions
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		s.logger.Warn("Failed to delete user sessions", zap.Error(err), zap.Int64("user_id", userID))
	}

	// Invalidate cache
	s.invalidateUserCache(ctx, user)

	// Publish user deactivated event
	if err := s.events.Publish(ctx, &events.UserDeactivatedEvent{
		UserID:        userID,
		Username:      user.Username,
		Email:         user.Email,
		Reason:        reason,
		DeactivatedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish user deactivated event", zap.Error(err), zap.Int64("user_id", userID))
	}

	s.logger.Info("User deactivated successfully",
		zap.Int64("user_id", userID),
		zap.String("username", user.Username),
		zap.String("reason", reason),
	)

	return nil
}

// ===============================
// USER MANAGEMENT
// ===============================

// ListUsers retrieves paginated list of users with filtering
func (s *userService) ListUsers(ctx context.Context, req *ListUsersRequest) (*models.PaginatedResponse[*models.User], error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid list users request", err)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	var excludeID int64
	if req.ExcludeID != nil {
		excludeID = *req.ExcludeID
	}

	// Get users based on filters
	var response *models.PaginatedResponse[*models.User]
	var err error

	if req.Role != nil {
		response, err = s.userRepo.GetByRole(ctx, *req.Role, req.Pagination)
	} else if req.Expertise != nil {
		response, err = s.userRepo.GetByExpertise(ctx, *req.Expertise, req.Pagination)
	} else {
		response, err = s.userRepo.List(ctx, req.Pagination, excludeID)
	}

	if err != nil {
		s.logger.Error("Failed to list users", zap.Error(err))
		return nil, NewInternalError("failed to retrieve users")
	}

	// Clear password hashes
	for _, user := range response.Data {
		user.PasswordHash = ""
	}

	return response, nil
}

// SearchUsers searches for users with the given query
func (s *userService) SearchUsers(ctx context.Context, req *SearchUsersRequest) (*models.PaginatedResponse[*models.User], error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid search users request", err)
	}

	// Set default pagination
	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = 20
	}
	if req.Pagination.Limit > 100 {
		req.Pagination.Limit = 100
	}

	response, err := s.userRepo.Search(ctx, req.Query, req.Pagination)
	if err != nil {
		s.logger.Error("Failed to search users", zap.Error(err), zap.String("query", req.Query))
		return nil, NewInternalError("failed to search users")
	}

	// Clear password hashes
	for _, user := range response.Data {
		user.PasswordHash = ""
	}

	return response, nil
}

// GetOnlineUsers retrieves currently online users
func (s *userService) GetOnlineUsers(ctx context.Context, limit int) ([]*models.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Try cache first
	cacheKey := fmt.Sprintf("online_users:%d", limit)
	if cachedUsers, found := s.cache.Get(ctx, cacheKey); found {
		if users, ok := cachedUsers.([]*models.User); ok {
			return users, nil
		}
	}

	users, err := s.userRepo.GetOnlineUsers(ctx, limit)
	if err != nil {
		s.logger.Error("Failed to get online users", zap.Error(err))
		return nil, NewInternalError("failed to retrieve online users")
	}

	// Clear password hashes
	for _, user := range users {
		user.PasswordHash = ""
	}

	// Cache for 30 seconds (online status changes frequently)
	if err := s.cache.Set(ctx, cacheKey, users, 30*time.Second); err != nil {
		s.logger.Warn("Failed to cache online users", zap.Error(err))
	}

	return users, nil
}

// UpdateOnlineStatus updates user's online status
func (s *userService) UpdateOnlineStatus(ctx context.Context, userID int64, online bool) error {
	if userID <= 0 {
		return NewValidationError("invalid user ID", nil)
	}

	if err := s.userRepo.SetOnlineStatus(ctx, userID, online); err != nil {
		s.logger.Error("Failed to update online status",
			zap.Error(err),
			zap.Int64("user_id", userID),
			zap.Bool("online", online),
		)
		return NewInternalError("failed to update online status")
	}

	// Invalidate online users cache
	s.cache.DeletePattern(ctx, "online_users:*")

	// Publish online status changed event
	if err := s.events.Publish(ctx, &events.UserOnlineStatusChangedEvent{
		UserID:    userID,
		Online:    online,
		ChangedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish online status changed event", zap.Error(err), zap.Int64("user_id", userID))
	}

	return nil
}

// ===============================
// PROFILE MANAGEMENT
// ===============================

// UpdateProfile updates user profile with business logic validation
func (s *userService) UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*models.User, error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid update profile request", err)
	}

	// Convert to UpdateUserRequest for reuse
	updateReq := &UpdateUserRequest{
		UserID:          req.UserID,
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		Bio:             req.Bio,
		YearsExperience: req.YearsExperience,
		Expertise:       req.Expertise,
	}

	return s.UpdateUser(ctx, updateReq)
}

// UploadProfileImage handles profile image upload
func (s *userService) UploadProfileImage(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid upload request", err)
	}

	// Validate image type and size
	if !isValidImageType(req.ContentType) {
		return nil, NewValidationError("invalid image type", nil)
	}

	if req.Size > 5*1024*1024 { // 5MB limit
		return nil, NewValidationError("image too large (max 5MB)", nil)
	}

	// Upload to file service
	result, err := s.fileService.UploadImage(ctx, &FileUploadRequest{
		UserID:      req.UserID,
		File:        req.File,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
		Folder:      "profiles",
	})
	if err != nil {
		s.logger.Error("Failed to upload profile image", zap.Error(err), zap.Int64("user_id", req.UserID))
		return nil, NewInternalError("failed to upload image")
	}

	// Update user profile URL
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, NewInternalError("failed to retrieve user")
	}
	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	user.ProfileURL = &result.URL
	user.ProfilePublicID = &result.PublicID

	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update user profile URL", zap.Error(err), zap.Int64("user_id", req.UserID))
		return nil, NewInternalError("failed to update profile")
	}

	// Invalidate cache
	s.invalidateUserCache(ctx, user)

	return &FileUploadResult{
		URL:      result.URL,
		PublicID: result.PublicID,
		Size:     result.Size,
		Format:   result.Format,
	}, nil
}

// UploadCV handles CV upload
func (s *userService) UploadCV(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error) {
	if err := validation.ValidateStruct(req); err != nil {
		return nil, NewValidationError("invalid upload request", err)
	}

	// Validate file type and size
	if !isValidDocumentType(req.ContentType) {
		return nil, NewValidationError("invalid document type", nil)
	}

	if req.Size > 10*1024*1024 { // 10MB limit
		return nil, NewValidationError("document too large (max 10MB)", nil)
	}

	// Upload to file service
	result, err := s.fileService.UploadDocument(ctx, &FileUploadRequest{
		UserID:      req.UserID,
		File:        req.File,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
		Folder:      "cvs",
	})
	if err != nil {
		s.logger.Error("Failed to upload CV", zap.Error(err), zap.Int64("user_id", req.UserID))
		return nil, NewInternalError("failed to upload document")
	}

	// Update user CV URL
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, NewInternalError("failed to retrieve user")
	}
	if user == nil {
		return nil, NewNotFoundError("user not found")
	}

	user.CVURL = &result.URL
	user.CVPublicID = &result.PublicID

	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update user CV URL", zap.Error(err), zap.Int64("user_id", req.UserID))
		return nil, NewInternalError("failed to update profile")
	}

	// Invalidate cache
	s.invalidateUserCache(ctx, user)

	return &FileUploadResult{
		URL:      result.URL,
		PublicID: result.PublicID,
		Size:     result.Size,
		Format:   result.Format,
	}, nil
}

// ===============================
// ANALYTICS AND STATS
// ===============================

// GetUserStats retrieves comprehensive user statistics
func (s *userService) GetUserStats(ctx context.Context, userID int64) (*UserStatsResponse, error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("user_stats:%d", userID)
	if cachedStats, found := s.cache.Get(ctx, cacheKey); found {
		if stats, ok := cachedStats.(*UserStatsResponse); ok {
			return stats, nil
		}
	}

	// Get stats from repository
	stats, err := s.userRepo.GetUserStats(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user stats", zap.Error(err), zap.Int64("user_id", userID))
		return nil, NewInternalError("failed to retrieve user statistics")
	}

	if stats == nil {
		return nil, NewNotFoundError("user stats not found")
	}

	// TODO: Get badges for user (this would require a badges repository)
	badges := []Badge{} // Placeholder

	response := &UserStatsResponse{
		UserID:             stats.UserID,
		ReputationPoints:   stats.ReputationPoints,
		Level:              stats.Level,
		NextLevelPoints:    stats.NextLevelPoints,
		PostsCount:         stats.PostsCount,
		QuestionsCount:     stats.QuestionsCount,
		CommentsCount:      stats.CommentsCount,
		TotalContributions: stats.TotalContributions,
		JoinedAt:           stats.JoinedAt,
		LastActivity:       stats.LastActivity,
		Badges:             badges,
	}

	// Cache for 5 minutes
	if err := s.cache.Set(ctx, cacheKey, response, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to cache user stats", zap.Error(err), zap.Int64("user_id", userID))
	}

	return response, nil
}

// GetLeaderboard retrieves top users by reputation
func (s *userService) GetLeaderboard(ctx context.Context, limit int) ([]*models.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Try cache first
	cacheKey := fmt.Sprintf("leaderboard:%d", limit)
	if cachedUsers, found := s.cache.Get(ctx, cacheKey); found {
		if users, ok := cachedUsers.([]*models.User); ok {
			return users, nil
		}
	}

	users, err := s.userRepo.GetLeaderboard(ctx, limit)
	if err != nil {
		s.logger.Error("Failed to get leaderboard", zap.Error(err))
		return nil, NewInternalError("failed to retrieve leaderboard")
	}

	// Clear password hashes
	for _, user := range users {
		user.PasswordHash = ""
	}

	// Cache for 10 minutes
	if err := s.cache.Set(ctx, cacheKey, users, 10*time.Minute); err != nil {
		s.logger.Warn("Failed to cache leaderboard", zap.Error(err))
	}

	return users, nil
}

// GetUserActivity retrieves user activity for the specified period
func (s *userService) GetUserActivity(ctx context.Context, userID int64, days int) (*UserActivityResponse, error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}
	if days <= 0 || days > 365 {
		days = 30 // Default to 30 days
	}

	// This would require additional queries to get activity data
	// For now, return a placeholder response
	response := &UserActivityResponse{
		UserID:       userID,
		Days:         days,
		TotalActions: 0,
		DailyStats:   []DailyActivityStats{},
		Summary: ActivitySummary{
			MostActiveDay:   time.Now(),
			AvgDailyActions: 0,
			ConsecutiveDays: 0,
			TotalEngagement: 0,
		},
	}

	return response, nil
}

// ===============================
// RELATIONSHIPS AND SOCIAL (Placeholder)
// ===============================

// FollowUser adds a follow relationship between users
func (s *userService) FollowUser(ctx context.Context, followerID, followeeID int64) error {
	// This would require a follows/relationships repository
	// Placeholder implementation
	return NewNotImplementedError("follow functionality not implemented")
}

// UnfollowUser removes a follow relationship between users
func (s *userService) UnfollowUser(ctx context.Context, followerID, followeeID int64) error {
	// This would require a follows/relationships repository
	// Placeholder implementation
	return NewNotImplementedError("unfollow functionality not implemented")
}

// GetFollowers retrieves users following the specified user
func (s *userService) GetFollowers(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	// This would require a follows/relationships repository
	// Placeholder implementation
	return nil, NewNotImplementedError("followers functionality not implemented")
}

// GetFollowing retrieves users that the specified user is following
func (s *userService) GetFollowing(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	// This would require a follows/relationships repository
	// Placeholder implementation
	return nil, NewNotImplementedError("following functionality not implemented")
}

// IsFollowing checks if one user is following another
func (s *userService) IsFollowing(ctx context.Context, followerID, followeeID int64) (bool, error) {
	if followerID <= 0 || followeeID <= 0 {
		return false, NewValidationError("invalid user IDs", nil)
	}

	// This would typically check a follows/relationships table in the database
	// For now, return a placeholder implementation
	return false, NewNotImplementedError("isFollowing functionality not implemented")
}

// ===============================
// HELPER METHODS
// ===============================

// invalidateUserCache removes all cached entries for a user
func (s *userService) invalidateUserCache(ctx context.Context, user *models.User) {
	cacheKeys := []string{
		fmt.Sprintf("user:%d", user.ID),
		fmt.Sprintf("user:username:%s", user.Username),
		fmt.Sprintf("user_stats:%d", user.ID),
	}

	for _, key := range cacheKeys {
		if err := s.cache.Delete(ctx, key); err != nil {
			s.logger.Warn("Failed to invalidate cache", zap.Error(err), zap.String("key", key))
		}
	}

	// Invalidate pattern-based caches
	s.cache.DeletePattern(ctx, "online_users:*")
	s.cache.DeletePattern(ctx, "leaderboard:*")
}

// getChangedFields returns a list of fields that were changed in the update request
func (s *userService) getChangedFields(req *UpdateUserRequest) []string {
	var fields []string

	if req.FirstName != nil {
		fields = append(fields, "first_name")
	}
	if req.LastName != nil {
		fields = append(fields, "last_name")
	}
	if req.JobTitle != nil {
		fields = append(fields, "job_title")
	}
	if req.Affiliation != nil {
		fields = append(fields, "affiliation")
	}
	if req.Bio != nil {
		fields = append(fields, "bio")
	}
	if req.YearsExperience != nil {
		fields = append(fields, "years_experience")
	}
	if req.CoreCompetencies != nil {
		fields = append(fields, "core_competencies")
	}
	if req.Expertise != nil {
		fields = append(fields, "expertise")
	}
	if req.WebsiteURL != nil {
		fields = append(fields, "website_url")
	}
	if req.LinkedinProfile != nil {
		fields = append(fields, "linkedin_profile")
	}
	if req.TwitterHandle != nil {
		fields = append(fields, "twitter_handle")
	}
	if req.EmailNotifications != nil {
		fields = append(fields, "email_notifications")
	}

	return fields
}

// isValidImageType checks if the content type is a valid image
func isValidImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
	}

	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}
	return false
}

// isValidDocumentType checks if the content type is a valid document
func isValidDocumentType(contentType string) bool {
	validTypes := []string{
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"text/plain",
	}

	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}
	return false
}
