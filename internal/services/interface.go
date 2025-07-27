// file: internal/services/interfaces.go
package services

import (
	"context"
	"evalhub/internal/events"
	"evalhub/internal/models"
	"fmt"
	"time"
)

// ===============================
// CORE SERVICE INTERFACES
// ===============================

// UserService defines comprehensive user business logic
type UserService interface {
	// Core CRUD operations
	CreateUser(ctx context.Context, req *CreateUserRequest) (*models.User, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByGitHubID(ctx context.Context, githubID int64) (*models.User, error)
	UpdateUser(ctx context.Context, req *UpdateUserRequest) (*models.User, error)
	DeactivateUser(ctx context.Context, userID int64, reason string) error

	// User management
	ListUsers(ctx context.Context, req *ListUsersRequest) (*models.PaginatedResponse[*models.User], error)
	SearchUsers(ctx context.Context, req *SearchUsersRequest) (*models.PaginatedResponse[*models.User], error)
	GetOnlineUsers(ctx context.Context, limit int) ([]*models.User, error)
	UpdateOnlineStatus(ctx context.Context, userID int64, online bool) error

	// Profile management
	UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*models.User, error)
	UploadProfileImage(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error)
	UploadCV(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error)

	// Analytics and stats
	GetUserStats(ctx context.Context, userID int64) (*UserStatsResponse, error)
	GetLeaderboard(ctx context.Context, limit int) ([]*models.User, error)
	GetUserActivity(ctx context.Context, userID int64, days int) (*UserActivityResponse, error)

	// Relationships and social
	FollowUser(ctx context.Context, followerID, followeeID int64) error
	UnfollowUser(ctx context.Context, followerID, followeeID int64) error
	GetFollowers(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	GetFollowing(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	IsFollowing(ctx context.Context, followerID, followeeID int64) (bool, error)
}

// PostService defines comprehensive post business logic
type PostService interface {
	// Core CRUD operations
	CreatePost(ctx context.Context, req *CreatePostRequest) (*models.Post, error)
	GetPostByID(ctx context.Context, id int64, userID *int64) (*models.Post, error)
	UpdatePost(ctx context.Context, req *UpdatePostRequest) (*models.Post, error)
	DeletePost(ctx context.Context, postID, userID int64) error

	// Listing and filtering
	ListPosts(ctx context.Context, req *ListPostsRequest) (*models.PaginatedResponse[*models.Post], error)
	GetPostsByUser(ctx context.Context, req *GetPostsByUserRequest) (*models.PaginatedResponse[*models.Post], error)
	GetPostsByCategory(ctx context.Context, req *GetPostsByCategoryRequest) (*models.PaginatedResponse[*models.Post], error)
	GetTrendingPosts(ctx context.Context, limit int, userID *int64) ([]*models.Post, error)
	GetFeaturedPosts(ctx context.Context, limit int, userID *int64) ([]*models.Post, error)
	GetDraftPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error)

	// Search operations
	SearchPosts(ctx context.Context, req *SearchPostsRequest) (*models.PaginatedResponse[*models.Post], error)

	// Engagement operations
	ReactToPost(ctx context.Context, req *ReactToPostRequest) error
	RemoveReaction(ctx context.Context, postID, userID int64) error
	BookmarkPost(ctx context.Context, userID, postID int64) error
	UnbookmarkPost(ctx context.Context, userID, postID int64) error
	SharePost(ctx context.Context, req *SharePostRequest) error
	GetBookmarkedPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error)

	// Content moderation
	ReportPost(ctx context.Context, req *ReportContentRequest) error
	ModeratePost(ctx context.Context, req *ModerateContentRequest) error

	// Analytics
	GetPostStats(ctx context.Context, postID int64) (*PostStatsResponse, error)
	GetPostAnalytics(ctx context.Context, userID int64, days int) (*PostAnalyticsResponse, error)
}

// QuestionService defines comprehensive question business logic
type QuestionService interface {
	// Core CRUD operations
	CreateQuestion(ctx context.Context, req *CreateQuestionRequest) (*models.Question, error)
	GetQuestionByID(ctx context.Context, id int64, userID *int64) (*models.Question, error)
	UpdateQuestion(ctx context.Context, req *UpdateQuestionRequest) (*models.Question, error)
	DeleteQuestion(ctx context.Context, questionID, userID int64) error

	// Listing and filtering
	ListQuestions(ctx context.Context, req *ListQuestionsRequest) (*models.PaginatedResponse[*models.Question], error)
	GetQuestionsByUser(ctx context.Context, req *GetQuestionsByUserRequest) (*models.PaginatedResponse[*models.Question], error)
	GetQuestionsByCategory(ctx context.Context, req *GetQuestionsByCategoryRequest) (*models.PaginatedResponse[*models.Question], error)
	GetUnansweredQuestions(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)
	GetTrendingQuestions(ctx context.Context, limit int, userID *int64) ([]*models.Question, error)

	// Search operations
	SearchQuestions(ctx context.Context, req *SearchQuestionsRequest) (*models.PaginatedResponse[*models.Question], error)

	// Answer operations
	AcceptAnswer(ctx context.Context, req *AcceptAnswerRequest) error
	GetAcceptedAnswer(ctx context.Context, questionID int64) (*models.Comment, error)

	// Engagement operations
	ReactToQuestion(ctx context.Context, req *ReactToQuestionRequest) error
	RemoveQuestionReaction(ctx context.Context, questionID, userID int64) error

	// Analytics
	GetQuestionStats(ctx context.Context, questionID int64) (*QuestionStatsResponse, error)
	GetQuestionAnalytics(ctx context.Context, userID int64, days int) (*QuestionAnalyticsResponse, error)
}

// CommentService defines comprehensive comment business logic - FIXED VERSION
type CommentService interface {
	// Core CRUD operations - FIXED SIGNATURES
	CreateComment(ctx context.Context, req *CreateCommentRequest) (*models.Comment, error) // ✅ FIXED: Pointer request and response
	GetCommentByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error)   // ✅ FIXED: Pointer userID and response
	UpdateComment(ctx context.Context, req *UpdateCommentRequest) (*models.Comment, error) // ✅ FIXED: Pointer request and response
	DeleteComment(ctx context.Context, commentID, userID int64) error
	
	// Listing operations - FIXED SIGNATURES
	GetCommentsByPost(ctx context.Context, req *GetCommentsByPostRequest) (*models.PaginatedResponse[*models.Comment], error)     // ✅ FIXED: Pointer request
	GetCommentsByQuestion(ctx context.Context, req *GetCommentsByQuestionRequest) (*models.PaginatedResponse[*models.Comment], error) // ✅ FIXED: Pointer request
	GetCommentsByDocument(ctx context.Context, req *GetCommentsByDocumentRequest) (*models.PaginatedResponse[*models.Comment], error) // ✅ NEW METHOD
	GetCommentsByUser(ctx context.Context, req *GetCommentsByUserRequest) (*models.PaginatedResponse[*models.Comment], error)    // ✅ FIXED: Pointer request
	GetModerationQueue(ctx context.Context, req *GetModerationQueueRequest) (*models.PaginatedResponse[*models.Comment], error) // ✅ NEW METHOD
	
	// Search operations - NEW METHOD
	SearchComments(ctx context.Context, req *SearchCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) // ✅ NEW METHOD
	
	// Trending & Recent - FIXED SIGNATURES
	GetTrendingComments(ctx context.Context, req *GetTrendingCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) // ✅ FIXED: Pointer request
	GetRecentComments(ctx context.Context, req *GetRecentCommentsRequest) (*models.PaginatedResponse[*models.Comment], error)     // ✅ FIXED: Pointer request
	
	// Threading operations - NEW METHODS
	GetCommentReplies(ctx context.Context, req *GetCommentRepliesRequest) (*models.PaginatedResponse[*models.Comment], error)
	GetCommentThread(ctx context.Context, commentID int64, userID *int64) ([]*models.Comment, error)
	
	// Engagement operations
	ReactToComment(ctx context.Context, req *ReactToCommentRequest) error
	RemoveCommentReaction(ctx context.Context, commentID, userID int64) error
	
	// Moderation
	ReportComment(ctx context.Context, req *ReportContentRequest) error
	ModerateComment(ctx context.Context, req *ModerateContentRequest) error
	
	// Analytics - FIXED SIGNATURES
	GetCommentStats(ctx context.Context, commentID int64) (*CommentStatsResponse, error)                                 // ✅ FIXED: Pointer response
	GetCommentAnalytics(ctx context.Context, req *GetCommentAnalyticsRequest) (*CommentAnalyticsResponse, error)       // ✅ NEW METHOD
}

// AuthService defines authentication and authorization business logic
type AuthService interface {
	// Authentication
	Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error)
	LoginWithProvider(ctx context.Context, req *OAuthLoginRequest) (*AuthResponse, error)
	RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error)
	Logout(ctx context.Context, req *LogoutRequest) error
	LogoutAllDevices(ctx context.Context, userID int64) error

	// Password management
	ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error
	ResetPassword(ctx context.Context, req *ResetPasswordRequest) error
	ChangePassword(ctx context.Context, req *ChangePasswordRequest) error

	// Email verification
	SendVerificationEmail(ctx context.Context, userID int64) error
	VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error

	// Session management
	GetActiveSessions(ctx context.Context, userID int64) ([]*SessionInfo, error)
	RevokeSession(ctx context.Context, sessionID int64, userID int64) error

	// Two-factor authentication
	EnableTwoFactor(ctx context.Context, userID int64) (*TwoFactorSetupResponse, error)
	DisableTwoFactor(ctx context.Context, req *DisableTwoFactorRequest) error
	VerifyTwoFactor(ctx context.Context, req *VerifyTwoFactorRequest) error
}

// JobService defines job and recruitment business logic
type JobService interface {
	// Job management
	CreateJob(ctx context.Context, req *CreateJobRequest) (*models.Job, error)
	GetJobByID(ctx context.Context, jobID int64, currentUserID *int64) (*models.Job, error)
	UpdateJob(ctx context.Context, req *UpdateJobRequest) (*models.Job, error)
	DeleteJob(ctx context.Context, jobID, userID int64) error

	// Job listing and search
	ListJobs(ctx context.Context, req *ListJobsRequest) (*models.PaginatedResponse[*models.Job], error)
	SearchJobs(ctx context.Context, req *SearchJobsRequest) (*models.PaginatedResponse[*models.Job], error)
	GetJobsByEmployer(ctx context.Context, req *GetJobsByEmployerRequest) (*models.PaginatedResponse[*models.Job], error)
	GetFeaturedJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)
	GetRecentJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)
	GetPopularJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)

	// Application management
	ApplyForJob(ctx context.Context, req *ApplyForJobRequest) (*models.JobApplication, error)
	WithdrawApplication(ctx context.Context, applicationID, userID int64) error
	GetUserApplications(ctx context.Context, req *GetUserApplicationsRequest) (*models.PaginatedResponse[*models.JobApplication], error)
	GetJobApplications(ctx context.Context, req *GetJobApplicationsRequest) (*models.PaginatedResponse[*models.JobApplication], error)
	HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error)
	GetAllJobsWithDetails(ctx context.Context, currentUserID int64) ([]models.Job, error)

	// Application processing
	ReviewApplication(ctx context.Context, req *ReviewApplicationRequest) error
	ShortlistApplicant(ctx context.Context, applicationID, reviewerID int64) error
	RejectApplication(ctx context.Context, req *RejectApplicationRequest) error
	AcceptApplication(ctx context.Context, req *AcceptApplicationRequest) error

	// Job analytics
	GetJobStats(ctx context.Context, employerID int64) (*JobStatsResponse, error)
	GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStatsResponse, error)
}

// DocumentService defines document business logic
type DocumentService interface {
	// Core CRUD operations
	CreateDocument(ctx context.Context, req *CreateDocumentRequest) (*models.Document, error)
	GetDocumentByID(ctx context.Context, id int64, userID *int64) (*models.Document, error)
	UpdateDocument(ctx context.Context, req *UpdateDocumentRequest) (*models.Document, error)
	DeleteDocument(ctx context.Context, documentID, userID int64) error

	// Listing operations
	ListDocuments(ctx context.Context, req *ListDocumentsRequest) (*models.PaginatedResponse[*models.Document], error)
	GetDocumentsByUser(ctx context.Context, req *GetDocumentsByUserRequest) (*models.PaginatedResponse[*models.Document], error)
	GetDocumentsByCategory(ctx context.Context, req *GetDocumentsByCategoryRequest) (*models.PaginatedResponse[*models.Document], error)
	GetPublishedDocuments(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Document], error)

	// Search operations
	SearchDocuments(ctx context.Context, req *SearchDocumentsRequest) (*models.PaginatedResponse[*models.Document], error)

	// File operations
	UploadDocument(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error)
	DownloadDocument(ctx context.Context, documentID int64, userID *int64) (*FileDownloadResult, error)

	// Analytics
	GetDocumentStats(ctx context.Context, documentID int64) (*DocumentStatsResponse, error)
}

// NotificationService defines notification business logic
type NotificationService interface {
	// Notification management
	CreateNotification(ctx context.Context, req *CreateNotificationRequest) error
	GetUserNotifications(ctx context.Context, req *GetNotificationsRequest) (*models.PaginatedResponse[*models.Notification], error)
	MarkAsRead(ctx context.Context, notificationID, userID int64) error
	MarkAllAsRead(ctx context.Context, userID int64) error
	DeleteNotification(ctx context.Context, notificationID, userID int64) error

	// Notification preferences
	GetNotificationPreferences(ctx context.Context, userID int64) (*models.NotificationPreferences, error)
	UpdateNotificationPreferences(ctx context.Context, req *UpdateNotificationPreferencesRequest) error

	// Bulk operations
	SendBulkNotification(ctx context.Context, req *BulkNotificationRequest) error
	GetUnreadCount(ctx context.Context, userID int64) (*NotificationSummaryResponse, error)

	// Real-time notifications
	SubscribeToNotifications(ctx context.Context, userID int64) (<-chan *models.Notification, error)
	UnsubscribeFromNotifications(ctx context.Context, userID int64) error
}

// ===============================
// INFRASTRUCTURE SERVICES
// ===============================

// TransactionService handles database transactions
type TransactionService interface {
	BeginTransaction(ctx context.Context, req *BeginTransactionRequest) (*TransactionContext, error)
	CommitTransaction(ctx context.Context, transactionID string) error
	RollbackTransaction(ctx context.Context, transactionID string) error
	ExecuteInTransaction(ctx context.Context, req *ExecuteInTransactionRequest, fn TransactionFunc) error
	GetTransactionMetrics(ctx context.Context) (*TransactionMetrics, error)
	GetActiveTransactions(ctx context.Context) ([]*TransactionInfo, error)
	AddOperation(ctx context.Context, transactionID string, req *AddOperationRequest) error
}

// CacheService handles application caching
type CacheService interface {
	Get(ctx context.Context, key string) (interface{}, bool)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error)
	SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error
	DeleteMultiple(ctx context.Context, keys []string) error
	Clear(ctx context.Context) error
	GetStats(ctx context.Context) *CacheStats
}

// EventService handles application events
type EventService interface {
	PublishEvent(ctx context.Context, event events.Event) error
	RegisterHandler(eventType string, handler EventHandler) error
	UnregisterHandler(eventType string, handlerID string) error
	GetEventHistory(ctx context.Context, eventType string, limit int) ([]events.Event, error)
	GetMetrics() *EventServiceMetrics
	Shutdown(ctx context.Context) error
}

// FileService handles file operations
type FileService interface {
	UploadImage(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error)
	UploadDocument(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error)
	DeleteFile(ctx context.Context, publicID string) error
	GetFileInfo(ctx context.Context, publicID string) (*FileInfo, error)
	GenerateUploadURL(ctx context.Context, req *GenerateUploadURLRequest) (*UploadURLResult, error)
	ProcessImageVariants(ctx context.Context, req *ProcessImageVariantsRequest) (*ImageVariantsResult, error)
}

// EmailService handles email operations
type EmailService interface {
	SendEmail(ctx context.Context, req *SendEmailRequest) error
	SendBulkEmail(ctx context.Context, req *SendBulkEmailRequest) error
	SendTemplateEmail(ctx context.Context, req *SendTemplateEmailRequest) error
	GetEmailStats(ctx context.Context, campaignID string) (*EmailStats, error)
	ValidateEmail(ctx context.Context, email string) (*EmailValidationResult, error)
	SendPasswordResetEmail(ctx context.Context, email, token string) error
	// SendVerificationEmail sends an email verification link to the user
	SendVerificationEmail(ctx context.Context, email, token string) error
}

// SearchService handles search operations
type SearchService interface {
	IndexDocument(ctx context.Context, req *IndexDocumentRequest) error
	SearchPosts(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	SearchUsers(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	SearchJobs(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	SearchAll(ctx context.Context, req *SearchRequest) (*SearchResult, error)
	DeleteDocument(ctx context.Context, documentID string) error
	GetSearchSuggestions(ctx context.Context, query string, limit int) ([]string, error)
	GetSearchStats(ctx context.Context) (*SearchStats, error)
}

// ===============================
// FUNCTION TYPES
// ===============================

// TransactionFunc represents a function that executes within a transaction
type TransactionFunc func(ctx context.Context, txCtx *TransactionContext) error

// EventHandler defines the interface for handling events
type EventHandler interface {
	Handle(ctx context.Context, event events.Event) error
	GetHandlerID() string
}

// EventHandlerFunc is a function type that implements EventHandler
type EventHandlerFunc func(ctx context.Context, event events.Event) error

// Handle implements the EventHandler interface for EventHandlerFunc
func (f EventHandlerFunc) Handle(ctx context.Context, event events.Event) error {
	return f(ctx, event)
}

// GetHandlerID returns a unique identifier for the handler
func (f EventHandlerFunc) GetHandlerID() string {
	// Use the function's memory address as a unique ID
	return fmt.Sprintf("%p", f)
}
