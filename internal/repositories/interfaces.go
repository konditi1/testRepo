// file: internal/repositories/interfaces.go
package repositories

import (
	"context"
	"evalhub/internal/models"
	"time"
)

// ===============================
// CORE REPOSITORY INTERFACES
// ===============================

// UserRepository defines the contract for user data operations
type UserRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int64) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByGitHubID(ctx context.Context, githubID int64) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error

	// Batch operations
	GetByIDs(ctx context.Context, ids []int64) ([]*models.User, error)
	UpdateLastSeen(ctx context.Context, userID int64) error
	SetOnlineStatus(ctx context.Context, userID int64, online bool) error
	BulkSetOffline(ctx context.Context, userIDs []int64) error

	// Search and listing
	List(ctx context.Context, params models.PaginationParams, excludeID int64) (*models.PaginatedResponse[*models.User], error)
	Search(ctx context.Context, query string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	GetOnlineUsers(ctx context.Context, limit int) ([]*models.User, error)
	GetByRole(ctx context.Context, role string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	GetByExpertise(ctx context.Context, expertise string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)

	// Analytics
	GetUserStats(ctx context.Context, userID int64) (*UserStats, error)
	GetLeaderboard(ctx context.Context, limit int) ([]*models.User, error)
	GetActiveUsers(ctx context.Context, since time.Time) ([]*models.User, error)
	CountByRole(ctx context.Context) (map[string]int, error)

	// Social features
	FollowUser(ctx context.Context, followerID, followeeID int64) error
	UnfollowUser(ctx context.Context, followerID, followeeID int64) error
	GetFollowers(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	GetFollowing(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error)
	IsFollowing(ctx context.Context, followerID, followeeID int64) (bool, error)
}

// PostRepository defines the contract for post data operations
type PostRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, post *models.Post) error
	GetByID(ctx context.Context, id int64, userID *int64) (*models.Post, error)
	Update(ctx context.Context, post *models.Post) error
	Delete(ctx context.Context, id int64) error

	// Listing and filtering
	List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error)
	GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error)
	GetByCategory(ctx context.Context, category string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error)
	GetByStatus(ctx context.Context, status string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error)
	GetTrending(ctx context.Context, limit int, userID *int64) ([]*models.Post, error)
	GetFeatured(ctx context.Context, limit int, userID *int64) ([]*models.Post, error)
	GetDrafts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error)

	// Search operations
	Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error)
	SearchByTags(ctx context.Context, tags []string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error)

	// Engagement operations
	AddReaction(ctx context.Context, postID, userID int64, reactionType string) error
	RemoveReaction(ctx context.Context, postID, userID int64) error
	GetUserReaction(ctx context.Context, postID, userID int64) (*string, error)
	GetReactionCounts(ctx context.Context, postID int64) (likes, dislikes int, err error)

	// Bookmark operations
	AddBookmark(ctx context.Context, postID, userID int64) error
	RemoveBookmark(ctx context.Context, postID, userID int64) error
	IsBookmarked(ctx context.Context, postID, userID int64) (bool, error)
	GetBookmarkedPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error)

	// Moderation operations
	AddReport(ctx context.Context, postID, reporterID int64, reason, description string) error
	ModeratePost(ctx context.Context, postID, moderatorID int64, action, reason string) error

	// Batch operations
	GetByIDs(ctx context.Context, ids []int64, userID *int64) ([]*models.Post, error)
	BulkUpdateStatus(ctx context.Context, ids []int64, status string) error
	IncrementViews(ctx context.Context, postID int64) error

	// Analytics
	GetPostStats(ctx context.Context, postID int64) (*PostStats, error)
	GetUserPostStats(ctx context.Context, userID int64) (*UserPostStats, error)
	GetCategoryStats(ctx context.Context) ([]*CategoryStats, error)
	GetPostAnalytics(ctx context.Context, userID int64, days int) (*PostAnalytics, error)

	// Share operations
	IncrementShareCount(ctx context.Context, postID int64) error
}

// QuestionRepository defines the contract for question data operations
type QuestionRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, question *models.Question) error
	GetByID(ctx context.Context, id int64, userID *int64) (*models.Question, error)
	Update(ctx context.Context, question *models.Question) error
	Delete(ctx context.Context, id int64) error

	// Listing and filtering
	List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)
	GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Question], error)
	GetByCategory(ctx context.Context, category string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)
	GetByTargetGroup(ctx context.Context, targetGroup string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)
	GetUnanswered(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)
	GetTrending(ctx context.Context, limit int, userID *int64) ([]*models.Question, error)

	// Search operations
	Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Question], error)

	// Engagement operations
	AddReaction(ctx context.Context, questionID, userID int64, reactionType string) error
	RemoveReaction(ctx context.Context, questionID, userID int64) error
	GetUserReaction(ctx context.Context, questionID, userID int64) (*string, error)
	IncrementViews(ctx context.Context, questionID int64) error

	// Answer operations
	MarkAsAnswered(ctx context.Context, questionID int64, acceptedAnswerID int64) error
	GetAcceptedAnswer(ctx context.Context, questionID int64) (*models.Comment, error)

	// Analytics
	GetQuestionStats(ctx context.Context, questionID int64) (*QuestionStats, error)
	GetUserQuestionStats(ctx context.Context, userID int64) (*UserQuestionStats, error)
}

// CommentRepository defines the contract for comment data operations - FIXED VERSION
type CommentRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, comment *models.Comment) error
	GetByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error) // ✅ FIXED: Added userID parameter
	Update(ctx context.Context, comment *models.Comment) error
	Delete(ctx context.Context, id int64) error

	// Listing operations
	GetByPostID(ctx context.Context, postID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetByQuestionID(ctx context.Context, questionID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetByDocumentID(ctx context.Context, documentID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Comment], error)
	GetTrendingComments(ctx context.Context, startTime, endTime time.Time, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetRecentComments(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetCommentsForModeration(ctx context.Context, status *string, priority *string, params models.PaginationParams) (*models.PaginatedResponse[*models.Comment], error)

	// Search operations
	Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)

	// Engagement operations
	AddReaction(ctx context.Context, commentID, userID int64, reactionType string) error
	RemoveReaction(ctx context.Context, commentID, userID int64) error
	GetUserReaction(ctx context.Context, commentID, userID int64) (*string, error) // ✅ FIXED: Return pointer to string
	GetReactionCounts(ctx context.Context, commentID int64) (likes, dislikes int, err error)

	// Threading operations
	GetReplies(ctx context.Context, parentCommentID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error)
	GetCommentThread(ctx context.Context, commentID int64, userID *int64) ([]*models.Comment, error)

	// Analytics
	CountByPostID(ctx context.Context, postID int64) (int, error)
	CountByQuestionID(ctx context.Context, questionID int64) (int, error)
	CountByDocumentID(ctx context.Context, documentID int64) (int, error)
	CountByUserID(ctx context.Context, userID int64) (int, error)
	GetCommentStats(ctx context.Context, commentID int64) (*CommentStats, error)

	// Batch operations
	GetLatestByPostIDs(ctx context.Context, postIDs []int64, limit int) ([]*models.Comment, error)
	BulkDelete(ctx context.Context, ids []int64) error
	BulkUpdateStatus(ctx context.Context, ids []int64, status string) error
}

// SessionRepository defines the contract for session data operations
type SessionRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, session *models.Session) error
	GetByToken(ctx context.Context, token string) (*models.Session, error)
	GetByUserID(ctx context.Context, userID int64) ([]*models.Session, error)
	Update(ctx context.Context, session *models.Session) error
	Delete(ctx context.Context, token string) error

	// Session management
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	RefreshActivity(ctx context.Context, token string) error
	CountActiveSessions(ctx context.Context, userID int64) (int, error)
	GetActiveSessions(ctx context.Context, userID int64, includeInactive bool) ([]*models.Session, error)

	// Analytics
	GetSessionStatistics(ctx context.Context) (map[string]interface{}, error)

	// Cleanup operations
	CleanupExpiredSessions(ctx context.Context) (int, error)
	GetExpiredSessions(ctx context.Context, olderThan time.Time) ([]*models.Session, error)
}

// JobRepository defines the contract for job data operations
type JobRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, job *models.Job) error
	GetByID(ctx context.Context, jobID int64, userID *int64) (*models.Job, error)
	Update(ctx context.Context, job *models.Job) error
	Delete(ctx context.Context, id int64) error

	// Listing and filtering
	List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)
	GetByEmployerID(ctx context.Context, employerID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Job], error)
	GetByStatus(ctx context.Context, status string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)
	GetByEmploymentType(ctx context.Context, empType string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)
	GetByLocation(ctx context.Context, location string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)
	GetFeatured(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)
	GetRecent(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)
	UpdateApplicationStatus(ctx context.Context, applicationID int64, status string, notes *string) error

	// Search operations
	Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)
	SearchBySkills(ctx context.Context, skills []string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error)

	// Application management
	HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error)
	CreateApplication(ctx context.Context, application *models.JobApplication) error
	GetApplication(ctx context.Context, jobID, userID int64) (*models.JobApplication, error)
	GetApplicationByID(ctx context.Context, applicationID int64) (*models.JobApplication, error)
	GetApplicationsByJob(ctx context.Context, jobID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error)
	GetApplicationsByUser(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error)
	UpdateApplication(ctx context.Context, application *models.JobApplication) error
	DeleteApplication(ctx context.Context, applicationID int64) error

	// Analytics
	GetJobStats(ctx context.Context, employerID int64) (*JobStats, error)
	GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStats, error)
	IncrementViews(ctx context.Context, jobID int64) error
	GetPopularJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error)
}

// DocumentRepository defines the contract for document data operations
type DocumentRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, document *models.Document) error
	GetByID(ctx context.Context, id int64, userID *int64) (*models.Document, error)
	Update(ctx context.Context, document *models.Document) error
	Delete(ctx context.Context, id int64) error

	// Listing operations
	List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Document], error)
	GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Document], error)
	GetByCategory(ctx context.Context, category string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Document], error)
	GetPublished(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Document], error)

	// Search operations
	Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Document], error)

	// Analytics
	IncrementViews(ctx context.Context, documentID int64) error
	GetDocumentStats(ctx context.Context, documentID int64) (*DocumentStats, error)
}

// NotificationRepository defines the contract for notification data operations
type NotificationRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, notification *models.Notification) error
	GetByID(ctx context.Context, id int64) (*models.Notification, error)
	Update(ctx context.Context, notification *models.Notification) error
	Delete(ctx context.Context, id int64) error

	// User notifications
	GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Notification], error)
	GetUnreadByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Notification], error)
	MarkAsRead(ctx context.Context, notificationID int64) error
	MarkAllAsRead(ctx context.Context, userID int64) error
	GetUnreadCount(ctx context.Context, userID int64) (int, error)

	// Batch operations
	CreateBulk(ctx context.Context, notifications []*models.Notification) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteOldNotifications(ctx context.Context, olderThan time.Time) error
}

// AuthRepository defines authentication-specific operations
type AuthRepository interface {
	// User operations (auth-focused)
	CreateUser(ctx context.Context, user *models.User) error
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
	VerifyUserEmail(ctx context.Context, userID int64) error
	UpdatePassword(ctx context.Context, userID int64, newPasswordHash string) error
	SetUserOnlineStatus(ctx context.Context, userID int64, online bool) error

	// Session operations
	CreateSession(ctx context.Context, session *models.Session) error
	GetSessionByToken(ctx context.Context, token string) (*models.Session, error)
	UpdateSessionActivity(ctx context.Context, token string) error
	DeleteSessionByToken(ctx context.Context, token string) error
	DeleteAllUserSessions(ctx context.Context, userID int64) error
	CleanupExpiredSessions(ctx context.Context) (int, error)

	// Security operations
	RecordLoginAttempt(ctx context.Context, email string, success bool, ipAddress string) error
	GetRecentLoginAttempts(ctx context.Context, email string, since time.Time) (int, error)
	LockAccount(ctx context.Context, userID int64, reason string) error
	UnlockAccount(ctx context.Context, userID int64) error
}

// ===============================
// ANALYTICS TYPES
// ===============================

// UserStats represents comprehensive user statistics
type UserStats struct {
	UserID             int64     `json:"user_id" db:"user_id"`
	ReputationPoints   int       `json:"reputation_points" db:"reputation_points"`
	PostsCount         int       `json:"posts_count" db:"posts_count"`
	QuestionsCount     int       `json:"questions_count" db:"questions_count"`
	CommentsCount      int       `json:"comments_count" db:"comments_count"`
	LikesGiven         int       `json:"likes_given" db:"likes_given"`
	LikesReceived      int       `json:"likes_received" db:"likes_received"`
	TotalContributions int       `json:"total_contributions" db:"total_contributions"`
	LastActivity       time.Time `json:"last_activity" db:"last_activity"`
	JoinedAt           time.Time `json:"joined_at" db:"created_at"`
	Level              string    `json:"level" db:"-"`
	NextLevelPoints    int       `json:"next_level_points" db:"-"`
	FollowersCount     int       `json:"followers_count" db:"followers_count"`
	FollowingCount     int       `json:"following_count" db:"following_count"`
}

// PostStats represents post analytics
type PostStats struct {
	PostID         int64 `json:"post_id" db:"post_id"`
	ViewsCount     int   `json:"views_count" db:"views_count"`
	LikesCount     int   `json:"likes_count" db:"likes_count"`
	DislikesCount  int   `json:"dislikes_count" db:"dislikes_count"`
	CommentsCount  int   `json:"comments_count" db:"comments_count"`
	SharesCount    int   `json:"shares_count" db:"shares_count"`
	BookmarksCount int   `json:"bookmarks_count" db:"bookmarks_count"`
}

// UserPostStats represents user's post statistics
type UserPostStats struct {
	UserID         int64 `json:"user_id" db:"user_id"`
	TotalPosts     int   `json:"total_posts" db:"total_posts"`
	PublishedPosts int   `json:"published_posts" db:"published_posts"`
	DraftPosts     int   `json:"draft_posts" db:"draft_posts"`
	TotalViews     int   `json:"total_views" db:"total_views"`
	TotalLikes     int   `json:"total_likes" db:"total_likes"`
	TotalComments  int   `json:"total_comments" db:"total_comments"`
	TotalShares    int   `json:"total_shares" db:"total_shares"`
}

// PostAnalytics represents detailed post analytics over time
type PostAnalytics struct {
	UserID     int64             `json:"user_id"`
	Days       int               `json:"days"`
	TotalPosts int               `json:"total_posts"`
	TotalViews int               `json:"total_views"`
	TotalLikes int               `json:"total_likes"`
	DailyStats []DailyPostStats  `json:"daily_stats"`
	TopPosts   []PostPerformance `json:"top_posts"`
}

// DailyPostStats represents daily post statistics
type DailyPostStats struct {
	Date       time.Time `json:"date"`
	PostsCount int       `json:"posts_count"`
	TotalViews int       `json:"total_views"`
	TotalLikes int       `json:"total_likes"`
}

// PostPerformance represents individual post performance
type PostPerformance struct {
	PostID        int64  `json:"post_id"`
	Title         string `json:"title"`
	ViewsCount    int    `json:"views_count"`
	LikesCount    int    `json:"likes_count"`
	CommentsCount int    `json:"comments_count"`
}

// CategoryStats represents category analytics
type CategoryStats struct {
	Category       string `json:"category" db:"category"`
	PostsCount     int    `json:"posts_count" db:"posts_count"`
	QuestionsCount int    `json:"questions_count" db:"questions_count"`
	TotalViews     int    `json:"total_views" db:"total_views"`
	ActiveAuthors  int    `json:"active_authors" db:"active_authors"`
}

// QuestionStats represents question analytics
type QuestionStats struct {
	QuestionID    int64 `json:"question_id" db:"question_id"`
	ViewsCount    int   `json:"views_count" db:"views_count"`
	LikesCount    int   `json:"likes_count" db:"likes_count"`
	DislikesCount int   `json:"dislikes_count" db:"dislikes_count"`
	CommentsCount int   `json:"comments_count" db:"comments_count"`
	IsAnswered    bool  `json:"is_answered" db:"is_answered"`
}

// UserQuestionStats represents user's question statistics
type UserQuestionStats struct {
	UserID              int64 `json:"user_id" db:"user_id"`
	TotalQuestions      int   `json:"total_questions" db:"total_questions"`
	AnsweredQuestions   int   `json:"answered_questions" db:"answered_questions"`
	UnansweredQuestions int   `json:"unanswered_questions" db:"unanswered_questions"`
	TotalViews          int   `json:"total_views" db:"total_views"`
	TotalLikes          int   `json:"total_likes" db:"total_likes"`
	AcceptedAnswers     int   `json:"accepted_answers" db:"accepted_answers"`
}

// CommentStats represents comment analytics
type CommentStats struct {
	CommentID     int64 `json:"comment_id" db:"comment_id"`
	LikesCount    int   `json:"likes_count" db:"likes_count"`
	DislikesCount int   `json:"dislikes_count" db:"dislikes_count"`
	RepliesCount  int   `json:"replies_count" db:"replies_count"`
	IsAccepted    bool  `json:"is_accepted" db:"is_accepted"`
}

// JobStats represents job posting analytics
type JobStats struct {
	EmployerID        int64 `json:"employer_id" db:"employer_id"`
	TotalJobs         int   `json:"total_jobs" db:"total_jobs"`
	ActiveJobs        int   `json:"active_jobs" db:"active_jobs"`
	ClosedJobs        int   `json:"closed_jobs" db:"closed_jobs"`
	TotalApplications int   `json:"total_applications" db:"total_applications"`
	TotalViews        int   `json:"total_views" db:"total_views"`
	FilledJobs        int   `json:"filled_jobs" db:"filled_jobs"`
}

// ApplicationStats represents job application analytics
type ApplicationStats struct {
	JobID                   int64 `json:"job_id" db:"job_id"`
	TotalApplications       int   `json:"total_applications" db:"total_applications"`
	PendingApplications     int   `json:"pending_applications" db:"pending_applications"`
	ReviewedApplications    int   `json:"reviewed_applications" db:"reviewed_applications"`
	ShortlistedApplications int   `json:"shortlisted_applications" db:"shortlisted_applications"`
	AcceptedApplications    int   `json:"accepted_applications" db:"accepted_applications"`
	RejectedApplications    int   `json:"rejected_applications" db:"rejected_applications"`
}

// DocumentStats represents document analytics
type DocumentStats struct {
	DocumentID     int64 `json:"document_id" db:"document_id"`
	ViewsCount     int   `json:"views_count" db:"views_count"`
	DownloadsCount int   `json:"downloads_count" db:"downloads_count"`
	CommentsCount  int   `json:"comments_count" db:"comments_count"`
	SharesCount    int   `json:"shares_count" db:"shares_count"`
}

// ===============================
// BATCH OPERATION TYPES
// ===============================

// BatchUpdateResult represents the result of a batch operation
type BatchUpdateResult struct {
	Updated int     `json:"updated"`
	Failed  int     `json:"failed"`
	Errors  []error `json:"errors,omitempty"`
}

// BulkInsertResult represents the result of a bulk insert operation
type BulkInsertResult struct {
	Inserted int     `json:"inserted"`
	Failed   int     `json:"failed"`
	IDs      []int64 `json:"ids,omitempty"`
	Errors   []error `json:"-"`
}
