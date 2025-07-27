// file: internal/services/types.go
package services

import (
	"database/sql"
	"evalhub/internal/models"
	"sync"
	"time"
)

// ===============================
// TRANSACTION SERVICE TYPES
// ===============================

type TransactionSummary struct {
	ID       string        `json:"id"`
	Duration time.Duration `json:"duration"`
	UserID   *int64        `json:"user_id,omitempty"`
}

type TransactionInfo struct {
	ID             string                 `json:"id"`
	Status         string                 `json:"status"`
	StartTime      time.Time              `json:"start_time"`
	Duration       time.Duration          `json:"duration"`
	UserID         *int64                 `json:"user_id,omitempty"`
	OperationCount int                    `json:"operation_count"`
	Operations     []*OperationInfo       `json:"operations,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type OperationInfo struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Duration    *time.Duration         `json:"duration,omitempty"`
	Error       *string                `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ===============================
// FILE SERVICE TYPES
// ===============================

// UploadStatistics contains file upload statistics
type UploadStatistics struct {
	TotalFiles    int     `json:"total_files"`
	TotalSize     int64   `json:"total_size"`
	ImageCount    int     `json:"image_count"`
	DocumentCount int     `json:"document_count"`
	AverageSize   float64 `json:"average_size"`
}

// URLOptions defines options for generating signed URLs
type URLOptions struct {
	ExpiresIn time.Duration `json:"expires_in"` // Duration until the URL expires
	Width     int          `json:"width,omitempty"`  // Optional width for image resizing
	Height    int          `json:"height,omitempty"` // Optional height for image resizing
}

type FileAnalysis struct {
	Filename    string   `json:"filename"`
	Size        int64    `json:"size"`
	ContentType string   `json:"content_type"`
	IsValid     bool     `json:"is_valid"`
	Issues      []string `json:"issues,omitempty"`
	MimeType    string   `json:"mime_type,omitempty"`
	Extension   string   `json:"extension,omitempty"`
}

type BatchDeleteResult struct {
	Total   int      `json:"total"`
	Deleted int      `json:"deleted"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// CleanupResult contains information about the cleanup operation results
type CleanupResult struct {
	FilesProcessed int      `json:"files_processed"` // Total number of files processed
	FilesDeleted   int      `json:"files_deleted"`   // Number of files successfully deleted
	SpaceFreed     int64    `json:"space_freed"`     // Total space freed in bytes
	Errors         []string `json:"errors,omitempty"` // Any errors that occurred during cleanup
}

// ===============================
// USER SERVICE TYPES
// ===============================

// User Service Requests
// Enhanced CreateUserRequest with all profile fields and file uploads
type CreateUserRequest struct {
	// Core authentication fields
	Email           string  `json:"email" validate:"required,email"`
	Username        string  `json:"username" validate:"required,min=3,max=50"`
	Password        string  `json:"password" validate:"required,min=8"`
	FirstName       *string `json:"first_name,omitempty"`
	LastName        *string `json:"last_name,omitempty"`
	AcceptTerms     bool    `json:"accept_terms" validate:"required"`
	MarketingEmails bool    `json:"marketing_emails"`

	// Enhanced profile fields
	Role             *string `json:"role,omitempty"`
	Affiliation      *string `json:"affiliation,omitempty"`
	Bio              *string `json:"bio,omitempty"`
	YearsExperience  *int16  `json:"years_experience,omitempty"`
	CoreCompetencies *string `json:"core_competencies,omitempty"`
	Expertise        *string `json:"expertise,omitempty"`

	// File upload fields
	ProfileURL      *string `json:"profile_url,omitempty"`
	ProfilePublicID *string `json:"profile_public_id,omitempty"`
	CVURL           *string `json:"cv_url,omitempty"`
	CVPublicID      *string `json:"cv_public_id,omitempty"`
}

type UpdateUserRequest struct {
	UserID             int64   `json:"-" validate:"required"`
	FirstName          *string `json:"first_name,omitempty"`
	LastName           *string `json:"last_name,omitempty"`
	JobTitle           *string `json:"job_title,omitempty"`
	Affiliation        *string `json:"affiliation,omitempty"`
	Bio                *string `json:"bio,omitempty"`
	YearsExperience    *int16  `json:"years_experience,omitempty"`
	CoreCompetencies   *string `json:"core_competencies,omitempty"`
	Expertise          *string `json:"expertise,omitempty"`
	WebsiteURL         *string `json:"website_url,omitempty"`
	LinkedinProfile    *string `json:"linkedin_profile,omitempty"`
	TwitterHandle      *string `json:"twitter_handle,omitempty"`
	EmailNotifications *bool   `json:"email_notifications,omitempty"`
}

type UpdateProfileRequest struct {
	UserID          int64   `json:"-" validate:"required"`
	FirstName       *string `json:"first_name,omitempty"`
	LastName        *string `json:"last_name,omitempty"`
	Bio             *string `json:"bio,omitempty"`
	YearsExperience *int16  `json:"years_experience,omitempty"`
	Expertise       *string `json:"expertise,omitempty"`
	JobTitle        *string `json:"job_title,omitempty"`
	Affiliation     *string `json:"affiliation,omitempty"`
	WebsiteURL      *string `json:"website_url,omitempty"`
	LinkedinProfile *string `json:"linkedin_profile,omitempty"`
	TwitterHandle   *string `json:"twitter_handle,omitempty"`
}

type ListUsersRequest struct {
	Pagination models.PaginationParams `json:"pagination"`
	Role       *string                 `json:"role,omitempty"`
	Expertise  *string                 `json:"expertise,omitempty"`
	ExcludeID  *int64                  `json:"exclude_id,omitempty"`
}

type SearchUsersRequest struct {
	Query      string                  `json:"query" validate:"required,min=2"`
	Pagination models.PaginationParams `json:"pagination"`
}

// User Service Responses
type UserStatsResponse struct {
	UserID             int64     `json:"user_id"`
	ReputationPoints   int       `json:"reputation_points"`
	Level              string    `json:"level"`
	NextLevelPoints    int       `json:"next_level_points"`
	PostsCount         int       `json:"posts_count"`
	QuestionsCount     int       `json:"questions_count"`
	CommentsCount      int       `json:"comments_count"`
	TotalContributions int       `json:"total_contributions"`
	JoinedAt           time.Time `json:"joined_at"`
	LastActivity       time.Time `json:"last_activity"`
	Badges             []Badge   `json:"badges"`
	FollowersCount     int       `json:"followers_count"`
	FollowingCount     int       `json:"following_count"`
}

type UserActivityResponse struct {
	UserID       int64                `json:"user_id"`
	Days         int                  `json:"days"`
	TotalActions int                  `json:"total_actions"`
	DailyStats   []DailyActivityStats `json:"daily_stats"`
	Summary      ActivitySummary      `json:"summary"`
}

// ===============================
// POST SERVICE TYPES
// ===============================

// Post Service Requests
type CreatePostRequest struct {
	UserID        int64    `json:"-" validate:"required"`
	Title         string   `json:"title" validate:"required,min=5,max=255"`
	Content       string   `json:"content" validate:"required,min=10"`
	Category      string   `json:"category" validate:"required"`
	Status        *string  `json:"status,omitempty"`
	ImageURL      *string  `json:"image_url,omitempty"`
	ImagePublicID *string  `json:"image_public_id,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type UpdatePostRequest struct {
	PostID        int64    `json:"-" validate:"required"`
	UserID        int64    `json:"-" validate:"required"`
	Title         *string  `json:"title,omitempty"`
	Content       *string  `json:"content,omitempty"`
	Category      *string  `json:"category,omitempty"`
	Status        *string  `json:"status,omitempty"`
	ImageURL      *string  `json:"image_url,omitempty"`
	ImagePublicID *string  `json:"image_public_id,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type ListPostsRequest struct {
	Pagination models.PaginationParams `json:"pagination"`
	UserID     *int64                  `json:"-"`
	Category   *string                 `json:"category,omitempty"`
	Status     *string                 `json:"status,omitempty"`
	SortBy     *string                 `json:"sort_by,omitempty"`
	SortOrder  *string                 `json:"sort_order,omitempty"`
}

type GetPostsByUserRequest struct {
	TargetUserID int64                   `json:"target_user_id" validate:"required"`
	ViewerID     *int64                  `json:"-"`
	Pagination   models.PaginationParams `json:"pagination"`
	Status       *string                 `json:"status,omitempty"`
}

type GetPostsByCategoryRequest struct {
	Category   string                  `json:"category" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
}

type SearchPostsRequest struct {
	Query      string                  `json:"query" validate:"required,min=2"`
	UserID     *int64                  `json:"-"`
	Category   *string                 `json:"category,omitempty"`
	Tags       []string                `json:"tags,omitempty"`
	Pagination models.PaginationParams `json:"pagination"`
}

type ReactToPostRequest struct {
	PostID       int64  `json:"post_id" validate:"required"`
	UserID       int64  `json:"-" validate:"required"`
	ReactionType string `json:"reaction_type" validate:"required,oneof=like dislike"`
}

type SharePostRequest struct {
	PostID   int64  `json:"post_id" validate:"required"`
	UserID   int64  `json:"-" validate:"required"`
	Platform string `json:"platform" validate:"required"`
	Message  string `json:"message,omitempty"`
}

// Post Service Responses
type PostStatsResponse struct {
	PostID         int64 `json:"post_id"`
	ViewsCount     int   `json:"views_count"`
	LikesCount     int   `json:"likes_count"`
	DislikesCount  int   `json:"dislikes_count"`
	CommentsCount  int   `json:"comments_count"`
	SharesCount    int   `json:"shares_count"`
	BookmarksCount int   `json:"bookmarks_count"`
}

type PostAnalyticsResponse struct {
	UserID     int64            `json:"user_id"`
	Days       int              `json:"days"`
	TotalPosts int              `json:"total_posts"`
	TotalViews int              `json:"total_views"`
	TotalLikes int              `json:"total_likes"`
	TopPosts   []*models.Post   `json:"top_posts"`
	DailyStats []DailyPostStats `json:"daily_stats"`
}

// ===============================
// QUESTION SERVICE TYPES
// ===============================

// Question Service Requests
type CreateQuestionRequest struct {
	UserID      int64    `json:"-" validate:"required"`
	Title       string   `json:"title" validate:"required,min=5,max=255"`
	Content     string   `json:"content" validate:"required,min=10"`
	Category    string   `json:"category" validate:"required"`
	TargetGroup *string  `json:"target_group,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Urgency     *string  `json:"urgency,omitempty"`
}

type UpdateQuestionRequest struct {
	QuestionID  int64    `json:"-" validate:"required"`
	UserID      int64    `json:"-" validate:"required"`
	Title       *string  `json:"title,omitempty"`
	Content     *string  `json:"content,omitempty"`
	Category    *string  `json:"category,omitempty"`
	TargetGroup *string  `json:"target_group,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Urgency     *string  `json:"urgency,omitempty"`
}

type ListQuestionsRequest struct {
	Pagination  models.PaginationParams `json:"pagination"`
	UserID      *int64                  `json:"-"`
	Category    *string                 `json:"category,omitempty"`
	TargetGroup *string                 `json:"target_group,omitempty"`
	Status      *string                 `json:"status,omitempty"`
	SortBy      *string                 `json:"sort_by,omitempty"`
	SortOrder   *string                 `json:"sort_order,omitempty"`
}

type GetQuestionsByUserRequest struct {
	TargetUserID int64                   `json:"target_user_id" validate:"required"`
	ViewerID     *int64                  `json:"-"`
	Pagination   models.PaginationParams `json:"pagination"`
	Status       *string                 `json:"status,omitempty"`
}

type GetQuestionsByCategoryRequest struct {
	Category   string                  `json:"category" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
}

type SearchQuestionsRequest struct {
	Query       string                  `json:"query" validate:"required,min=2"`
	UserID      *int64                  `json:"-"`
	Category    *string                 `json:"category,omitempty"`
	TargetGroup *string                 `json:"target_group,omitempty"`
	Tags        []string                `json:"tags,omitempty"`
	Pagination  models.PaginationParams `json:"pagination"`
}

type ReactToQuestionRequest struct {
	QuestionID   int64  `json:"question_id" validate:"required"`
	UserID       int64  `json:"-" validate:"required"`
	ReactionType string `json:"reaction_type" validate:"required,oneof=like dislike"`
}

type AcceptAnswerRequest struct {
	QuestionID int64 `json:"question_id" validate:"required"`
	CommentID  int64 `json:"comment_id" validate:"required"`
	UserID     int64 `json:"-" validate:"required"`
}

// Question Service Responses
type QuestionStatsResponse struct {
	QuestionID       int64  `json:"question_id"`
	ViewsCount       int    `json:"views_count"`
	LikesCount       int    `json:"likes_count"`
	DislikesCount    int    `json:"dislikes_count"`
	CommentsCount    int    `json:"comments_count"`
	IsAnswered       bool   `json:"is_answered"`
	AcceptedAnswerID *int64 `json:"accepted_answer_id,omitempty"`
}

type QuestionAnalyticsResponse struct {
	UserID         int64                `json:"user_id"`
	Days           int                  `json:"days"`
	TotalQuestions int                  `json:"total_questions"`
	TotalViews     int                  `json:"total_views"`
	TotalLikes     int                  `json:"total_likes"`
	AnsweredRate   float64              `json:"answered_rate"`
	TopQuestions   []*models.Question   `json:"top_questions"`
	DailyStats     []DailyQuestionStats `json:"daily_stats"`
}

// ===============================
// COMMENT SERVICE TYPES
// ===============================

// Comment Service Requests
type CreateCommentRequest struct {
	UserID     int64  `json:"-" validate:"required"`
	PostID     *int64 `json:"post_id,omitempty"`
	QuestionID *int64 `json:"question_id,omitempty"`
	DocumentID *int64 `json:"document_id,omitempty"`
	ParentID   *int64 `json:"parent_id,omitempty"`
	Content    string `json:"content" validate:"required,min=1,max=10000"`
}

type UpdateCommentRequest struct {
	CommentID int64  `json:"-" validate:"required"`
	UserID    int64  `json:"-" validate:"required"`
	Content   string `json:"content" validate:"required,min=1,max=10000"`
}

type GetCommentsByPostRequest struct {
	PostID     int64                   `json:"post_id" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
	SortBy     *string                 `json:"sort_by,omitempty"`
	SortOrder  *string                 `json:"sort_order,omitempty"`
}

type GetCommentsByQuestionRequest struct {
	QuestionID int64                   `json:"question_id" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
	SortBy     *string                 `json:"sort_by,omitempty"`
	SortOrder  *string                 `json:"sort_order,omitempty"`
}

type GetCommentsByDocumentRequest struct {
	DocumentID int64                   `json:"document_id" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
	SortBy     *string                 `json:"sort_by,omitempty"`
	SortOrder  *string                 `json:"sort_order,omitempty"`
}

type GetCommentsByUserRequest struct {
	TargetUserID int64                   `json:"target_user_id" validate:"required"`
	Pagination   models.PaginationParams `json:"pagination"`
	ContentType  *string                 `json:"content_type,omitempty"`
}

type GetCommentRepliesRequest struct {
	ParentCommentID int64                   `json:"parent_comment_id" validate:"required"`
	UserID          *int64                  `json:"-"`
	Pagination      models.PaginationParams `json:"pagination"`
	SortBy          *string                 `json:"sort_by,omitempty"`
	SortOrder       *string                 `json:"sort_order,omitempty"`
}

type GetModerationQueueRequest struct {
	ModeratorID int64                   `json:"-"`
	Status      *string                 `json:"status,omitempty"`
	Priority    *string                 `json:"priority,omitempty"`
	Pagination  models.PaginationParams `json:"pagination"`
}

type GetTrendingCommentsRequest struct {
	TimeRange  *TimeRange              `json:"time_range,omitempty"`
	Pagination models.PaginationParams `json:"pagination"`
	UserID     *int64                  `json:"-"`
}

type GetRecentCommentsRequest struct {
	Pagination models.PaginationParams `json:"pagination"`
	UserID     *int64                  `json:"-"`
	Limit      *int                    `json:"limit,omitempty"`
}

type SearchCommentsRequest struct {
	Query       string                  `json:"query" validate:"required,min=2"`
	PostID      *int64                  `json:"post_id,omitempty"`
	QuestionID  *int64                  `json:"question_id,omitempty"`
	DocumentID  *int64                  `json:"document_id,omitempty"`
	UserID      *int64                  `json:"-"`
	Pagination  models.PaginationParams `json:"pagination"`
	ContentType *string                 `json:"content_type,omitempty"`
}

type ReactToCommentRequest struct {
	CommentID    int64  `json:"comment_id" validate:"required"`
	UserID       int64  `json:"-" validate:"required"`
	ReactionType string `json:"reaction_type" validate:"required,oneof=like dislike"`
}

type GetCommentAnalyticsRequest struct {
	UserID    int64      `json:"user_id"`
	TimeRange *TimeRange `json:"time_range,omitempty"`
}

// Comment Service Responses
type CommentStatsResponse struct {
	CommentID     int64 `json:"comment_id"`
	LikesCount    int   `json:"likes_count"`
	DislikesCount int   `json:"dislikes_count"`
	RepliesCount  int   `json:"replies_count"`
	IsAccepted    bool  `json:"is_accepted"`
}

type CommentAnalyticsResponse struct {
	TotalComments   int                 `json:"total_comments"`
	CommentsByDay   map[string]int      `json:"comments_by_day"`
	CommentsByType  map[string]int      `json:"comments_by_type"`
	AvgResponseTime float64             `json:"avg_response_time_hours"`
	EngagementRate  float64             `json:"engagement_rate"`
	TopComments     []*models.Comment   `json:"top_comments"`
	DailyStats      []DailyCommentStats `json:"daily_stats"`
}

// ===============================
// AUTH SERVICE TYPES
// ===============================

// Auth Service Requests
type RegisterRequest struct {
	Email           string `json:"email" validate:"required,email"`
	Username        string `json:"username" validate:"required,min=3,max=50"`
	Password        string `json:"password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
	FirstName       string `json:"first_name" validate:"required,min=1,max=100"`
	LastName        string `json:"last_name" validate:"required,min=1,max=100"`
	Role            string `json:"role" validate:"required,oneof=expert evaluator admin"`
	AcceptTerms     bool   `json:"accept_terms" validate:"required"`

	// Enhanced fields
	Affiliation      string `json:"affiliation,omitempty"`
	Bio              string `json:"bio,omitempty"`
	YearsExperience  int    `json:"years_experience,omitempty"`
	CoreCompetencies string `json:"core_competencies,omitempty"`
	Expertise        string `json:"expertise,omitempty"`

	// File upload fields (these would be handled separately in HTTP handler)
	ProfileImage interface{} `json:"-"` // File upload handled by multipart
	CVDocument   interface{} `json:"-"` // File upload handled by multipart
}

type LoginRequest struct {
	Login      string  `json:"login" validate:"required"`
	Password   string  `json:"password" validate:"required"`
	Remember   bool    `json:"remember,omitempty"`
	DeviceID   *string `json:"device_id,omitempty"`
	DeviceInfo *string `json:"device_info,omitempty"`
	IPAddress  string  `json:"-"` // Set by middleware
	UserAgent  string  `json:"-"` // Set by middleware
}

type OAuthLoginRequest struct {
	Provider     string         `json:"provider" validate:"required,oneof=google github"`
	AccessToken  string         `json:"access_token" validate:"required"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	UserInfo     *OAuthUserInfo `json:"user_info,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
	IPAddress    string `json:"-"` // Set by middleware
	UserAgent    string `json:"-"` // Set by middleware
}

type LogoutRequest struct {
	SessionToken string `json:"session_token" validate:"required"`
	LogoutAll    bool   `json:"logout_all,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token           string `json:"token" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
}

type ChangePasswordRequest struct {
	UserID          int64  `json:"-" validate:"required"`
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

type DisableTwoFactorRequest struct {
	UserID   int64  `json:"-" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type VerifyTwoFactorRequest struct {
	UserID int64  `json:"-" validate:"required"`
	Code   string `json:"code" validate:"required"`
}

// Auth Service Responses
type AuthResponse struct {
	User         *models.User `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	ExpiresIn    int64        `json:"expires_in"`
	RefreshExpiresIn int64    `json:"refresh_expires_in"`
	TokenType    string       `json:"token_type"`
}

type TwoFactorSetupResponse struct {
	QRCodeURL   string   `json:"qr_code_url"`
	SecretKey   string   `json:"secret_key"`
	BackupCodes []string `json:"backup_codes"`
}

type OAuthUserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Avatar    string `json:"avatar,omitempty"`
}

// ===============================
// JOB SERVICE TYPES
// ===============================

// Job Service Requests
type CreateJobRequest struct {
	EmployerID          int64      `json:"-" validate:"required"`
	Title               string     `json:"title" validate:"required,min=5,max=255"`
	Description         string     `json:"description" validate:"required,min=50"`
	Requirements        string     `json:"requirements" validate:"required"`
	Location            string     `json:"location" validate:"required"`
	EmploymentType      string     `json:"employment_type" validate:"required,oneof=full-time part-time contract internship"`
	SalaryMin           *int       `json:"salary_min,omitempty"`
	SalaryMax           *int       `json:"salary_max,omitempty"`
	Currency            *string    `json:"currency,omitempty"`
	Skills              []string   `json:"skills,omitempty"`
	ExperienceLevel     *string    `json:"experience_level,omitempty"`
	Remote              bool       `json:"remote"`
	Benefits            *string    `json:"benefits,omitempty"`
	ApplicationDeadline *time.Time `json:"application_deadline,omitempty"`
}

type UpdateJobRequest struct {
	JobID               int64      `json:"-" validate:"required"`
	EmployerID          int64      `json:"-" validate:"required"`
	Title               *string    `json:"title,omitempty"`
	Description         *string    `json:"description,omitempty"`
	Requirements        *string    `json:"requirements,omitempty"`
	Location            *string    `json:"location,omitempty"`
	EmploymentType      *string    `json:"employment_type,omitempty"`
	SalaryMin           *int       `json:"salary_min,omitempty"`
	SalaryMax           *int       `json:"salary_max,omitempty"`
	Currency            *string    `json:"currency,omitempty"`
	Skills              []string   `json:"skills,omitempty"`
	ExperienceLevel     *string    `json:"experience_level,omitempty"`
	Remote              *bool      `json:"remote,omitempty"`
	Benefits            *string    `json:"benefits,omitempty"`
	Status              *string    `json:"status,omitempty"`
	ApplicationDeadline *time.Time `json:"application_deadline,omitempty"`
}

type ListJobsRequest struct {
	Pagination      models.PaginationParams `json:"pagination"`
	UserID          *int64                  `json:"-"`
	Location        *string                 `json:"location,omitempty"`
	EmploymentType  *string                 `json:"employment_type,omitempty"`
	Remote          *bool                   `json:"remote,omitempty"`
	SalaryMin       *int                    `json:"salary_min,omitempty"`
	SalaryMax       *int                    `json:"salary_max,omitempty"`
	ExperienceLevel *string                 `json:"experience_level,omitempty"`
	Skills          []string                `json:"skills,omitempty"`
	SortBy          *string                 `json:"sort_by,omitempty"`
	SortOrder       *string                 `json:"sort_order,omitempty"`
}

type SearchJobsRequest struct {
	Query           string                  `json:"query" validate:"required,min=2"`
	UserID          *int64                  `json:"-"`
	Location        *string                 `json:"location,omitempty"`
	EmploymentType  *string                 `json:"employment_type,omitempty"`
	Remote          *bool                   `json:"remote,omitempty"`
	SalaryMin       *int                    `json:"salary_min,omitempty"`
	SalaryMax       *int                    `json:"salary_max,omitempty"`
	ExperienceLevel *string                 `json:"experience_level,omitempty"`
	Skills          []string                `json:"skills,omitempty"`
	Pagination      models.PaginationParams `json:"pagination"`
}

type GetJobsByEmployerRequest struct {
	EmployerID int64                   `json:"employer_id" validate:"required"`
	Pagination models.PaginationParams `json:"pagination"`
	Status     *string                 `json:"status,omitempty"`
}

type ApplyForJobRequest struct {
	JobID        int64                  `json:"job_id" validate:"required"`
	UserID       int64                  `json:"-" validate:"required"`
	CoverLetter  *string                `json:"cover_letter,omitempty"`
	ResumeURL    *string                `json:"resume_url,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

type GetUserApplicationsRequest struct {
	UserID     int64                   `json:"-" validate:"required"`
	Pagination models.PaginationParams `json:"pagination"`
	Status     *string                 `json:"status,omitempty"`
}

type GetJobApplicationsRequest struct {
	JobID      int64                   `json:"job_id" validate:"required"`
	EmployerID int64                   `json:"-" validate:"required"`
	Pagination models.PaginationParams `json:"pagination"`
	Status     *string                 `json:"status,omitempty"`
}

type ReviewApplicationRequest struct {
	ApplicationID int64   `json:"application_id" validate:"required"`
	ReviewerID    int64   `json:"-" validate:"required"`
	Status        string  `json:"status" validate:"required,oneof=pending reviewed shortlisted accepted rejected"`
	Notes         *string `json:"notes,omitempty"`
	Rating        *int    `json:"rating,omitempty"`
}

type RejectApplicationRequest struct {
	ApplicationID int64   `json:"application_id" validate:"required"`
	ReviewerID    int64   `json:"-" validate:"required"`
	Reason        *string `json:"reason,omitempty"`
	Notes         *string `json:"notes,omitempty"`
}

type AcceptApplicationRequest struct {
	ApplicationID int64      `json:"application_id" validate:"required"`
	ReviewerID    int64      `json:"-" validate:"required"`
	Notes         *string    `json:"notes,omitempty"`
	StartDate     *time.Time `json:"start_date,omitempty"`
	Salary        *int       `json:"salary,omitempty"`
}

// Job Service Responses
type JobStatsResponse struct {
	EmployerID        int64 `json:"employer_id"`
	TotalJobs         int   `json:"total_jobs"`
	ActiveJobs        int   `json:"active_jobs"`
	ClosedJobs        int   `json:"closed_jobs"`
	TotalApplications int   `json:"total_applications"`
	TotalViews        int   `json:"total_views"`
	FilledJobs        int   `json:"filled_jobs"`
	AverageTimeToFill int   `json:"average_time_to_fill_days"`
}

type ApplicationStatsResponse struct {
	JobID                   int64   `json:"job_id"`
	TotalApplications       int     `json:"total_applications"`
	PendingApplications     int     `json:"pending_applications"`
	ReviewedApplications    int     `json:"reviewed_applications"`
	ShortlistedApplications int     `json:"shortlisted_applications"`
	AcceptedApplications    int     `json:"accepted_applications"`
	RejectedApplications    int     `json:"rejected_applications"`
	ConversionRate          float64 `json:"conversion_rate"`
}

// ===============================
// DOCUMENT SERVICE TYPES
// ===============================

// Document Service Requests
type CreateDocumentRequest struct {
	UserID      int64    `json:"-" validate:"required"`
	Title       string   `json:"title" validate:"required,min=5,max=255"`
	Content     *string  `json:"content,omitempty"`
	Category    string   `json:"category" validate:"required"`
	FileURL     *string  `json:"file_url,omitempty"`
	FileSize    *int64   `json:"file_size,omitempty"`
	FileType    *string  `json:"file_type,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IsPublic    bool     `json:"is_public"`
}

type UpdateDocumentRequest struct {
	DocumentID  int64    `json:"-" validate:"required"`
	UserID      int64    `json:"-" validate:"required"`
	Title       *string  `json:"title,omitempty"`
	Content     *string  `json:"content,omitempty"`
	Category    *string  `json:"category,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IsPublic    *bool    `json:"is_public,omitempty"`
}

type ListDocumentsRequest struct {
	Pagination models.PaginationParams `json:"pagination"`
	UserID     *int64                  `json:"-"`
	Category   *string                 `json:"category,omitempty"`
	IsPublic   *bool                   `json:"is_public,omitempty"`
	SortBy     *string                 `json:"sort_by,omitempty"`
	SortOrder  *string                 `json:"sort_order,omitempty"`
}

type GetDocumentsByUserRequest struct {
	TargetUserID int64                   `json:"target_user_id" validate:"required"`
	ViewerID     *int64                  `json:"-"`
	Pagination   models.PaginationParams `json:"pagination"`
	IsPublic     *bool                   `json:"is_public,omitempty"`
}

type GetDocumentsByCategoryRequest struct {
	Category   string                  `json:"category" validate:"required"`
	UserID     *int64                  `json:"-"`
	Pagination models.PaginationParams `json:"pagination"`
}

type SearchDocumentsRequest struct {
	Query      string                  `json:"query" validate:"required,min=2"`
	UserID     *int64                  `json:"-"`
	Category   *string                 `json:"category,omitempty"`
	FileType   *string                 `json:"file_type,omitempty"`
	Tags       []string                `json:"tags,omitempty"`
	Pagination models.PaginationParams `json:"pagination"`
}

// Document Service Responses
type DocumentStatsResponse struct {
	DocumentID     int64 `json:"document_id"`
	ViewsCount     int   `json:"views_count"`
	DownloadsCount int   `json:"downloads_count"`
	CommentsCount  int   `json:"comments_count"`
	SharesCount    int   `json:"shares_count"`
}

// ===============================
// NOTIFICATION SERVICE TYPES
// ===============================

// Notification Service Requests
type CreateNotificationRequest struct {
	UserID    int64                  `json:"user_id" validate:"required"`
	Type      string                 `json:"type" validate:"required"`
	Title     string                 `json:"title" validate:"required"`
	Content   string                 `json:"content" validate:"required"`
	ActionURL *string                `json:"action_url,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Priority  *string                `json:"priority,omitempty"`
	SendEmail bool                   `json:"send_email"`
	SendPush  bool                   `json:"send_push"`
}

type GetNotificationsRequest struct {
	UserID     int64                   `json:"-" validate:"required"`
	Pagination models.PaginationParams `json:"pagination"`
	Type       *string                 `json:"type,omitempty"`
	IsRead     *bool                   `json:"is_read,omitempty"`
}

type UpdateNotificationPreferencesRequest struct {
	UserID             int64 `json:"-" validate:"required"`
	EmailNotifications bool  `json:"email_notifications"`
	PushNotifications  bool  `json:"push_notifications"`
	PostLikes          bool  `json:"post_likes"`
	PostComments       bool  `json:"post_comments"`
	QuestionAnswers    bool  `json:"question_answers"`
	JobAlerts          bool  `json:"job_alerts"`
	WeeklyDigest       bool  `json:"weekly_digest"`
}

type BulkNotificationRequest struct {
	UserIDs   []int64                `json:"user_ids" validate:"required,min=1"`
	Type      string                 `json:"type" validate:"required"`
	Title     string                 `json:"title" validate:"required"`
	Content   string                 `json:"content" validate:"required"`
	ActionURL *string                `json:"action_url,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Priority  *string                `json:"priority,omitempty"`
	SendEmail bool                   `json:"send_email"`
	SendPush  bool                   `json:"send_push"`
}

// Notification Service Responses
type NotificationSummaryResponse struct {
	UnreadCount        int `json:"unread_count"`
	UnreadLikes        int `json:"unread_likes"`
	UnreadComments     int `json:"unread_comments"`
	UnreadAnswers      int `json:"unread_answers"`
	UnreadJobAlerts    int `json:"unread_job_alerts"`
	UnreadSystemAlerts int `json:"unread_system_alerts"`
}

// ===============================
// INFRASTRUCTURE SERVICE TYPES
// ===============================

// File Service Types
type FileUploadRequest struct {
	UserID      int64       `json:"user_id"`
	File        interface{} `json:"file"`
	Filename    string      `json:"filename"`
	ContentType string      `json:"content_type"`
	Size        int64       `json:"size"`
	Folder      string      `json:"folder,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
}

type FileUploadResult struct {
	URL      string `json:"url"`
	PublicID string `json:"public_id"`
	Size     int64  `json:"size"`
	Format   string `json:"format"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Secure   bool   `json:"secure"`
	Type     string `json:"type,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type FileDownloadResult struct {
	URL         string            `json:"url"`
	Filename    string            `json:"filename"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type FileInfo struct {
	PublicID  string    `json:"public_id"`
	URL       string    `json:"url"`
	SecureURL string    `json:"secure_url"`
	Format    string    `json:"format"`
	Size      int64     `json:"size"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags,omitempty"`
}

type GenerateUploadURLRequest struct {
	UserID      int64    `json:"user_id"`
	Filename    string   `json:"filename"`
	ContentType string   `json:"content_type"`
	Size        int64    `json:"size"`
	Folder      string   `json:"folder,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ResourceType string  `json:"resource_type,omitempty"`
	PublicID    string   `json:"public_id,omitempty"`
}

type UploadURLResult struct {
	UploadURL string            `json:"upload_url"`
	PublicID  string            `json:"public_id"`
	Fields    map[string]string `json:"fields,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type ProcessImageVariantsRequest struct {
	PublicID string               `json:"public_id"`
	Variants []ImageVariantConfig `json:"variants"`
	UserID   int64                `json:"user_id"`
}

type ImageVariantConfig struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Crop   string `json:"crop,omitempty"`
}

type ImageVariantsResult struct {
	PublicID string                      `json:"public_id"`
	Variants map[string]FileUploadResult `json:"variants"`
}

// Email Service Types
type SendEmailRequest struct {
	To          []string          `json:"to" validate:"required,min=1"`
	From        string            `json:"from,omitempty"`
	Subject     string            `json:"subject" validate:"required"`
	Body        string            `json:"body" validate:"required"`
	IsHTML      bool              `json:"is_html"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
}

type SendBulkEmailRequest struct {
	Recipients  []EmailRecipient  `json:"recipients" validate:"required,min=1"`
	From        string            `json:"from,omitempty"`
	Subject     string            `json:"subject" validate:"required"`
	Body        string            `json:"body" validate:"required"`
	IsHTML      bool              `json:"is_html"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
}

type SendTemplateEmailRequest struct {
	To           []string               `json:"to" validate:"required,min=1"`
	From         string                 `json:"from,omitempty"`
	TemplateID   string                 `json:"template_id" validate:"required"`
	TemplateData map[string]interface{} `json:"template_data,omitempty"`
}

type EmailRecipient struct {
	Email string                 `json:"email" validate:"required,email"`
	Name  string                 `json:"name,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

type EmailAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
	URL         string `json:"url,omitempty"`
}

type EmailStats struct {
	CampaignID   string    `json:"campaign_id"`
	Sent         int       `json:"sent"`
	Delivered    int       `json:"delivered"`
	Opened       int       `json:"opened"`
	Clicked      int       `json:"clicked"`
	Bounced      int       `json:"bounced"`
	Complained   int       `json:"complained"`
	Unsubscribed int       `json:"unsubscribed"`
	CreatedAt    time.Time `json:"created_at"`
}

type EmailValidationResult struct {
	Email       string   `json:"email"`
	IsValid     bool     `json:"is_valid"`
	Reason      string   `json:"reason,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Search Service Types
type SearchRequest struct {
	Query      string                 `json:"query" validate:"required,min=1"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
	SortBy     string                 `json:"sort_by,omitempty"`
	SortOrder  string                 `json:"sort_order,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Offset     int                    `json:"offset,omitempty"`
	Highlights []string               `json:"highlights,omitempty"`
}

type SearchResult struct {
	Total        int64                  `json:"total"`
	Results      []SearchDocument       `json:"results"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
	Suggestions  []string               `json:"suggestions,omitempty"`
	QueryTime    time.Duration          `json:"query_time"`
}

type SearchDocument struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	URL        string                 `json:"url,omitempty"`
	Score      float64                `json:"score"`
	Highlights map[string][]string    `json:"highlights,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type IndexDocumentRequest struct {
	ID       string                 `json:"id" validate:"required"`
	Type     string                 `json:"type" validate:"required"`
	Title    string                 `json:"title" validate:"required"`
	Content  string                 `json:"content" validate:"required"`
	URL      string                 `json:"url,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type SearchStats struct {
	TotalDocuments int64            `json:"total_documents"`
	TotalQueries   int64            `json:"total_queries"`
	AvgQueryTime   time.Duration    `json:"avg_query_time"`
	PopularQueries []PopularQuery   `json:"popular_queries"`
	IndexSizes     map[string]int64 `json:"index_sizes"`
}

type PopularQuery struct {
	Query string `json:"query"`
	Count int64  `json:"count"`
}

// Transaction Service Types
type BeginTransactionRequest struct {
	UserID         *int64                `json:"user_id,omitempty"`
	IsolationLevel string        `json:"isolation_level,omitempty"`
	ReadOnly       bool          `json:"read_only"`
	Timeout        time.Duration `json:"timeout,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type ExecuteInTransactionRequest struct {
	UserID         *int64                `json:"user_id,omitempty"`
	IsolationLevel string                `json:"isolation_level,omitempty"`
	ReadOnly       bool                  `json:"read_only"`
	Timeout        time.Duration         `json:"timeout,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ExecuteWithRetryRequest contains configuration for executing a transaction with retry logic
type ExecuteWithRetryRequest struct {
	UserID         *int64                `json:"user_id,omitempty"`
	IsolationLevel string                `json:"isolation_level,omitempty"`
	ReadOnly       bool                  `json:"read_only"`
	Timeout        time.Duration         `json:"timeout,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	MaxRetries     int                   `json:"max_retries,omitempty"`
	RetryDelay     time.Duration         `json:"retry_delay,omitempty"`
}

// TransactionContext represents an active transaction with metadata
type TransactionContext struct {
	ID         string                 `json:"id"`
	Tx         *sql.Tx                `json:"-"`
	StartTime  time.Time              `json:"start_time"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Timeout    time.Time              `json:"timeout"`
	UserID     *int64                 `json:"user_id,omitempty"`
	Operations []TransactionOp        `json:"operations"`
	Status     TransactionStatus      `json:"status"`
	Metadata   map[string]interface{} `json:"metadata"`
	mu         sync.RWMutex
}

// AddOperationRequest represents a request to add an operation to a transaction
type AddOperationRequest struct {
	Type      string                 `json:"type" validate:"required"`
	Service   string                 `json:"service" validate:"required"`
	Method    string                 `json:"method" validate:"required"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TransactionOp represents a single operation within a transaction
type TransactionOp struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Service   string                 `json:"service"`
	Method    string                 `json:"method"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Status    OperationStatus        `json:"status"`
	Error     *string                `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TransactionStatusActive     TransactionStatus = "active"
	TransactionStatusCommitted  TransactionStatus = "committed"
	TransactionStatusRolledBack TransactionStatus = "rolled_back"
	TransactionStatusFailed     TransactionStatus = "failed"
)

// OperationStatus represents the status of an operation
type OperationStatus string

const (
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusRunning   OperationStatus = "running"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

type TransactionMetrics struct {
	ActiveTransactions int64         `json:"active_transactions"`
	TotalTransactions  int64         `json:"total_transactions"`
	CommittedCount     int64         `json:"committed_count"`
	RolledBackCount    int64         `json:"rolled_back_count"`
	AvgDuration        time.Duration `json:"avg_duration"`
	MaxDuration        time.Duration `json:"max_duration"`
	MaxConcurrent      int           `json:"max_concurrent"`
	ConfiguredTimeout  time.Duration `json:"configured_timeout"`
	OldestTransaction  *TransactionSummary `json:"oldest_transaction,omitempty"`
}

// Cache Service Types
type CacheStats struct {
	HitCount     int64   `json:"hit_count"`
	MissCount    int64   `json:"miss_count"`
	HitRate      float64 `json:"hit_rate"`
	Size         int64   `json:"size"`
	MaxSize      int64   `json:"max_size"`
	EvictedCount int64   `json:"evicted_count"`
}

// Event Service Types
// EventServiceMetrics provides comprehensive metrics about the event service
type EventServiceMetrics struct {
	// Basic event counters
	EventsPublished int64 `json:"events_published"`
	EventsProcessed int64 `json:"events_processed"`
	EventsFailed    int64 `json:"events_failed"`
	EventsRetried   int64 `json:"events_retried"`

	// Performance metrics
	AverageProcessTime time.Duration `json:"average_process_time"`
	PublishRate       float64       `json:"publish_rate"`
	ProcessRate       float64       `json:"process_rate"`

	// Queue metrics
	QueueDepth      int `json:"queue_depth"`
	DeadLetterDepth int `json:"dead_letter_depth"`

	// Additional metrics
	HandlerMetrics map[string]int64 `json:"handler_metrics"`
	Uptime         time.Duration    `json:"uptime"`
}

// ===============================
// SHARED HELPER TYPES
// ===============================

// TimeRange represents a time range
type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	
}

// Badge represents a user badge
type Badge struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	EarnedAt    time.Time `json:"earned_at"`
	Category    string    `json:"category"`
	Rarity      string    `json:"rarity"`
}

// DailyActivityStats represents daily activity statistics
type DailyActivityStats struct {
	Date     time.Time `json:"date"`
	Posts    int       `json:"posts"`
	Comments int       `json:"comments"`
	Likes    int       `json:"likes"`
	Views    int       `json:"views"`
	Score    float64   `json:"score"`
}

// ActivitySummary represents activity summary
type ActivitySummary struct {
	MostActiveDay          time.Time `json:"most_active_day"`
	AvgDailyActions        float64   `json:"avg_daily_actions"`
	ConsecutiveDays        int       `json:"consecutive_days"`
	TotalEngagement        int       `json:"total_engagement"`
	TrendDirection         string    `json:"trend_direction"`
	ImprovementSuggestions []string  `json:"improvement_suggestions"`
}

// DailyPostStats represents daily post statistics
type DailyPostStats struct {
	Date          time.Time `json:"date"`
	PostsCount    int       `json:"posts_count"`
	TotalViews    int       `json:"total_views"`
	TotalLikes    int       `json:"total_likes"`
	TotalShares   int       `json:"total_shares"`
	AvgEngagement float64   `json:"avg_engagement"`
}

// DailyQuestionStats represents daily question statistics
type DailyQuestionStats struct {
	Date            time.Time `json:"date"`
	QuestionsCount  int       `json:"questions_count"`
	TotalViews      int       `json:"total_views"`
	TotalLikes      int       `json:"total_likes"`
	AnsweredCount   int       `json:"answered_count"`
	AvgResponseTime float64   `json:"avg_response_time_hours"`
}

// DailyCommentStats represents daily comment statistics
type DailyCommentStats struct {
	Date          time.Time `json:"date"`
	CommentsCount int       `json:"comments_count"`
	TotalLikes    int       `json:"total_likes"`
	RepliesCount  int       `json:"replies_count"`
	AvgLength     float64   `json:"avg_length"`
}

// SessionInfo represents session information
type SessionInfo struct {
	ID               int64     `json:"id"`
	Token            string    `json:"token,omitempty"`
	LastActivity     time.Time `json:"last_activity"`
	ExpiresAt        time.Time `json:"expires_at"`
	IsCurrentSession bool      `json:"is_current_session"`
	Device           string    `json:"device,omitempty"`
	Browser          string    `json:"browser,omitempty"`
	OS               string    `json:"os,omitempty"`
	Location         string    `json:"location,omitempty"`
	IPAddress        string    `json:"ip_address,omitempty"`
}

// Content moderation types
type ReportContentRequest struct {
	ContentType string `json:"content_type" validate:"required,oneof=post comment question document"`
	ContentID   int64  `json:"content_id" validate:"required"`
	ReporterID  int64  `json:"-" validate:"required"`
	Reason      string `json:"reason" validate:"required"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

type ModerateContentRequest struct {
	ContentType string        `json:"content_type" validate:"required,oneof=post comment question document"`
	ContentID   int64         `json:"content_id" validate:"required"`
	ModeratorID int64         `json:"-" validate:"required"`
	Action      string        `json:"action" validate:"required,oneof=approve reject hide warn"`
	Reason      string        `json:"reason,omitempty"`
	Notes       string        `json:"notes,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
}
