// file: internal/models/models.go
package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// ===============================
// CORE ENTITIES
// ===============================


// User represents a user in the system with comprehensive validation
type User struct {
	// Primary fields
	ID       int64   `json:"id" db:"id"`
	GitHubID *int64  `json:"github_id,omitempty" db:"github_id"`
	Email    string  `json:"email" db:"email" validate:"required,email,max=320"`
	Username string  `json:"username" db:"username" validate:"required,min=3,max=50,alphanum"`

	// Authentication
	PasswordHash  string `json:"-" db:"password_hash"`
	EmailVerified bool   `json:"email_verified" db:"is_verified"`
	IsActive      bool   `json:"is_active" db:"is_active"`

	// Profile information
	FirstName   *string `json:"first_name,omitempty" db:"first_name" validate:"omitempty,max=100"`
	LastName    *string `json:"last_name,omitempty" db:"last_name" validate:"omitempty,max=100"`
	DisplayName string  `json:"display_name" db:"display_name"`
	JobTitle    *string `json:"job_title,omitempty" db:"job_title" validate:"omitempty,max=150"`
	Affiliation *string `json:"affiliation,omitempty" db:"affiliation" validate:"omitempty,max=255"`
	Bio         *string `json:"bio,omitempty" db:"bio" validate:"omitempty,max=1000"`

	// Professional details
	YearsExperience  int16   `json:"years_experience" db:"years_experience" validate:"min=0,max=100"`
	CoreCompetencies *string `json:"core_competencies,omitempty" db:"core_competencies"`
	Expertise        string  `json:"expertise" db:"expertise" validate:"required,oneof=none beginner intermediate advanced expert"`

	// Files (Cloudinary)
	ProfileURL      *string `json:"profile_url,omitempty" db:"profile_url"`
	ProfilePublicID *string `json:"profile_public_id,omitempty" db:"profile_public_id"`
	CVURL           *string `json:"cv_url,omitempty" db:"cv_url"`
	CVPublicID      *string `json:"cv_public_id,omitempty" db:"cv_public_id"`

	// Social links
	WebsiteURL      *string `json:"website_url,omitempty" db:"website_url" validate:"omitempty,url"`
	LinkedinProfile *string `json:"linkedin_profile,omitempty" db:"linkedin_profile"`
	TwitterHandle   *string `json:"twitter_handle,omitempty" db:"twitter_handle" validate:"omitempty,max=50"`

	// System fields
	Role               string `json:"role" db:"role" validate:"required,oneof=user reviewer moderator admin"`
	IsOnline           bool   `json:"is_online" db:"is_online"`
	EmailNotifications bool   `json:"email_notifications" db:"email_notifications"`

	// Timestamps
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
	LastSeen          time.Time  `json:"last_seen" db:"last_seen"`
	EmailVerifiedAt   *time.Time `json:"email_verified_at,omitempty" db:"email_verified_at"`
	PasswordChangedAt time.Time  `json:"password_changed_at" db:"password_changed_at"`

	// Computed/joined fields (not in DB)
	ReputationPoints   int    `json:"reputation_points,omitempty" db:"-"`
	TotalContributions int    `json:"total_contributions,omitempty" db:"-"`
	BadgeCount         int    `json:"badge_count,omitempty" db:"-"`
	Level              string `json:"level,omitempty" db:"-"`
	LevelColor         string `json:"level_color,omitempty" db:"-"`
	PostsCount         int    `json:"posts_count,omitempty" db:"-"`
	QuestionsCount     int    `json:"questions_count,omitempty" db:"-"`
	CommentsCount      int    `json:"comments_count,omitempty" db:"-"`
}

// Category represents a content category
type Category struct {
	ID          int       `json:"id" db:"id"`
	Name        string    `json:"name" db:"name" validate:"required,max=100"`
	Description *string   `json:"description,omitempty" db:"description"`
	Icon        *string   `json:"icon,omitempty" db:"icon" validate:"omitempty,max=100"`
	Color       *string   `json:"color,omitempty" db:"color" validate:"omitempty,len=7"` // hex color
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`

	// Computed fields (not in DB)
	PostsCount     int `json:"posts_count,omitempty" db:"-"`
	QuestionsCount int `json:"questions_count,omitempty" db:"-"`
}

// Session represents a user session with enhanced security
type Session struct {
	ID           int64     `json:"id" db:"id"`
	UserID       int64     `json:"user_id" db:"user_id" validate:"required"`
	SessionToken string    `json:"session_token" db:"session_token" validate:"required"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at" validate:"required"`
	LastActivity time.Time `json:"last_activity" db:"last_activity"`
	
	// Enhanced security fields
	IPAddress *string `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent *string `json:"user_agent,omitempty" db:"user_agent"`
	IsActive  bool    `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	
	// Joined fields
	UserRole      string `json:"user_role" db:"-"`      // Joined from user
	IsExpiredFlag bool   `json:"is_expired" db:"-"`     // Computed
}

// Post represents a community post with enhanced metadata
type Post struct {
	// Core fields
	ID       int64  `json:"id" db:"id"`
	UserID   int64  `json:"user_id" db:"user_id" validate:"required"`
	Title    string `json:"title" db:"title" validate:"required,min=5,max=255"`
	Content  string `json:"content" db:"content" validate:"required,min=10,max=50000"`
	Category string `json:"category" db:"category" validate:"required,max=100"`
	Status   string `json:"status" db:"status" validate:"oneof=draft published archived deleted flagged approved rejected"`

	// Media
	ImageURL      *string `json:"image_url,omitempty" db:"image_url"`
	ImagePublicID *string `json:"image_public_id,omitempty" db:"image_public_id"`

	// Engagement tracking
	ViewsCount    int `json:"views_count" db:"views_count"`
	LikesCount    int `json:"likes_count" db:"likes_count"`
	DislikesCount int `json:"dislikes_count" db:"dislikes_count"`
	CommentsCount int `json:"comments_count" db:"comments_count"`

	// SEO and metadata
	Slug            *string     `json:"slug,omitempty" db:"slug"`
	MetaDescription *string     `json:"meta_description,omitempty" db:"meta_description"`
	Tags            StringArray `json:"tags" db:"tags"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`

	// Author information (joined)
	Username         string  `json:"username" db:"username"`
	DisplayName      string  `json:"display_name" db:"display_name"`
	AuthorProfileURL *string `json:"author_profile_url,omitempty" db:"author_profile_url"`

	// User-specific fields (requires user context)
	IsOwner      bool    `json:"is_owner" db:"-"`
	IsBookmarked bool    `json:"is_bookmarked" db:"-"`
	UserReaction *string `json:"user_reaction,omitempty" db:"-"` // "like" or "dislike"

	// Display helpers
	Preview        string   `json:"preview" db:"-"`
	CategoryArray  []string `json:"category_array" db:"-"`
	CreatedAtHuman string   `json:"created_at_human" db:"-"`
	UpdatedAtHuman string   `json:"updated_at_human" db:"-"`
}

// Question represents a community question with Q&A functionality
type Question struct {
	// Core fields
	ID          int64   `json:"id" db:"id"`
	UserID      int64   `json:"user_id" db:"user_id" validate:"required"`
	Title       string  `json:"title" db:"title" validate:"required,min=10,max=255"`
	Content     *string `json:"content,omitempty" db:"content" validate:"omitempty,max=50000"`
	Category    string  `json:"category" db:"category" validate:"required,max=100"`
	TargetGroup string  `json:"target_group" db:"target_group" validate:"max=100"`
	Status      string  `json:"status" db:"status" validate:"oneof=draft published archived deleted flagged approved rejected"`

	// Attachments
	FileURL      *string `json:"file_url,omitempty" db:"file_url"`
	FilePublicID *string `json:"file_public_id,omitempty" db:"file_public_id"`

	// Engagement tracking
	ViewsCount    int `json:"views_count" db:"views_count"`
	LikesCount    int `json:"likes_count" db:"likes_count"`
	DislikesCount int `json:"dislikes_count" db:"dislikes_count"`
	CommentsCount int `json:"comments_count" db:"comments_count"`

	// Question-specific fields
	IsAnswered        bool   `json:"is_answered" db:"is_answered"`
	AcceptedAnswerID  *int64 `json:"accepted_answer_id,omitempty" db:"accepted_answer_id"`

	// SEO and metadata
	Slug *string     `json:"slug,omitempty" db:"slug"`
	Tags StringArray `json:"tags" db:"tags"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`

	// Author information (joined)
	Username         string  `json:"username" db:"username"`
	DisplayName      string  `json:"display_name" db:"display_name"`
	AuthorProfileURL *string `json:"author_profile_url,omitempty" db:"author_profile_url"`

	// User-specific fields
	IsOwner      bool    `json:"is_owner" db:"-"`
	UserReaction *string `json:"user_reaction,omitempty" db:"-"`

	// Display helpers
	CategoryArray  []string `json:"category_array" db:"-"`
	CreatedAtHuman string   `json:"created_at_human" db:"-"`
	UpdatedAtHuman string   `json:"updated_at_human" db:"-"`
}

// Comment represents a comment on posts/questions/documents with threading support
type Comment struct {
	// Core fields
	ID      int64  `json:"id" db:"id"`
	UserID  int64  `json:"user_id" db:"user_id" validate:"required"`
	Content string `json:"content" db:"content" validate:"required,min=1,max=10000"`

	// Parent references (exactly one must be set)
	PostID     *int64 `json:"post_id,omitempty" db:"post_id"`
	QuestionID *int64 `json:"question_id,omitempty" db:"question_id"`
	DocumentID *int64 `json:"document_id,omitempty" db:"document_id"`

	// Thread support
	ParentCommentID *int64 `json:"parent_comment_id,omitempty" db:"parent_comment_id"`
	ThreadLevel     int    `json:"thread_level" db:"thread_level"`

	// Engagement tracking
	LikesCount    int `json:"likes_count" db:"likes_count"`
	DislikesCount int `json:"dislikes_count" db:"dislikes_count"`

	// Moderation
	IsFlagged  bool `json:"is_flagged" db:"is_flagged"`
	IsApproved bool `json:"is_approved" db:"is_approved"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`

	// Author information (joined)
	Username         string  `json:"username" db:"username"`
	DisplayName      string  `json:"display_name" db:"display_name"`
	AuthorProfileURL *string `json:"author_profile_url,omitempty" db:"author_profile_url"`

	// User-specific fields
	IsOwner      bool    `json:"is_owner" db:"-"`
	UserReaction *string `json:"user_reaction,omitempty" db:"-"`

	// Display helpers
	CreatedAtHuman string `json:"created_at_human" db:"-"`
	UpdatedAtHuman string `json:"updated_at_human" db:"-"`

	// Context information (not in DB)
	ContextType  string `json:"context_type,omitempty" db:"-"`  // "post", "question", or "document"
	ContextTitle string `json:"context_title,omitempty" db:"-"` // Title of the parent entity

	// Thread display helpers
	Replies     []*Comment `json:"replies,omitempty" db:"-"`     // Child comments
	ReplyCount  int        `json:"reply_count,omitempty" db:"-"` // Number of replies
}

// ===============================
// JOB SYSTEM
// ===============================

// Job represents a job posting with enhanced features
type Job struct {
	// Core fields
	ID               int64   `json:"id" db:"id"`
	EmployerID       int64   `json:"employer_id" db:"employer_id" validate:"required"`
	Title            string  `json:"title" db:"title" validate:"required,min=5,max=255"`
	Description      string  `json:"description" db:"description" validate:"required,min=50,max=10000"`
	Requirements     *string `json:"requirements,omitempty" db:"requirements" validate:"omitempty,max=5000"`
	Responsibilities *string `json:"responsibilities,omitempty" db:"responsibilities" validate:"omitempty,max=5000"`

	// Employment details
	EmploymentType      string     `json:"employment_type" db:"employment_type" validate:"oneof=full_time part_time contract temporary internship volunteer freelance"`
	Location            *string    `json:"location,omitempty" db:"location" validate:"omitempty,max=255"`
	SalaryRange         *string    `json:"salary_range,omitempty" db:"salary_range" validate:"omitempty,max=100"`
	IsRemote            bool       `json:"is_remote" db:"is_remote"`
	ApplicationDeadline *time.Time `json:"application_deadline,omitempty" db:"application_deadline"`
	StartDate           *time.Time `json:"start_date,omitempty" db:"start_date"`

	// Status and tracking
	Status            string `json:"status" db:"status" validate:"oneof=draft active paused closed filled"`
	ViewsCount        int    `json:"views_count" db:"views_count"`
	ApplicationsCount int    `json:"applications_count" db:"applications_count"`

	// SEO and metadata
	Slug *string     `json:"slug,omitempty" db:"slug"`
	Tags StringArray `json:"tags" db:"tags"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`

	// Employer information (joined)
	EmployerUsername string  `json:"employer_username" db:"employer_username"`
	EmployerEmail    string  `json:"employer_email" db:"employer_email"`
	EmployerCompany  *string `json:"employer_company,omitempty" db:"employer_company"`

	// User-specific fields
	IsOwner    bool `json:"is_owner" db:"-"`
	HasApplied bool `json:"has_applied" db:"-"`

	// Display helpers
	CreatedAtHuman string `json:"created_at_human" db:"-"`
	DeadlineHuman  string `json:"deadline_human" db:"-"`
	StartDateHuman string `json:"start_date_human" db:"-"`
}

// JobApplication represents a job application with enhanced tracking
type JobApplication struct {
	// Core fields
	ID          int64  `json:"id" db:"id"`
	JobID       int64  `json:"job_id" db:"job_id" validate:"required"`
	ApplicantID int64  `json:"applicant_id" db:"applicant_id" validate:"required"`
	CoverLetter string `json:"cover_letter" db:"cover_letter" validate:"required,min=50,max=5000"`

	// Documents
	ApplicationLetterURL      *string `json:"application_letter_url,omitempty" db:"application_letter_url"`
	ApplicationLetterPublicID *string `json:"application_letter_public_id,omitempty" db:"application_letter_public_id"`

	// Status tracking
	Status     string     `json:"status" db:"status" validate:"oneof=pending reviewing shortlisted interviewed accepted rejected withdrawn"`
	Notes      *string    `json:"notes,omitempty" db:"notes" validate:"omitempty,max=2000"`
	AppliedAt  time.Time  `json:"applied_at" db:"applied_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`

	// Related information (joined)
	JobTitle          string  `json:"job_title" db:"job_title"`
	EmployerUsername  string  `json:"employer_username" db:"employer_username"`
	EmployerCompany   *string `json:"employer_company,omitempty" db:"employer_company"`
	ApplicantUsername string  `json:"applicant_username" db:"applicant_username"`
	ApplicantEmail    string  `json:"applicant_email" db:"applicant_email"`
	ApplicantName     string  `json:"applicant_name" db:"applicant_name"`
	ApplicantCVURL    *string `json:"applicant_cv_url,omitempty" db:"applicant_cv_url"`

	// Display helpers
	AppliedAtHuman  string `json:"applied_at_human" db:"-"`
	ReviewedAtHuman string `json:"reviewed_at_human" db:"-"`
}

// ===============================
// MESSAGING & NOTIFICATIONS
// ===============================

// Message represents a direct message between users
type Message struct {
	ID          int64     `json:"id" db:"id"`
	SenderID    int64     `json:"sender_id" db:"sender_id" validate:"required"`
	RecipientID int64     `json:"recipient_id" db:"recipient_id" validate:"required"`
	Content     string    `json:"content" db:"content" validate:"required,min=1,max=10000"`
	IsRead      bool      `json:"is_read" db:"is_read"`
	MessageType string    `json:"message_type" db:"message_type" validate:"oneof=chat_message system_update announcement"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty" db:"read_at"`

	// Sender information (joined)
	SenderUsername    string  `json:"sender_username" db:"sender_username"`
	SenderDisplayName string  `json:"sender_display_name" db:"sender_display_name"`
	SenderProfileURL  *string `json:"sender_profile_url,omitempty" db:"sender_profile_url"`
	
	// Recipient information (joined)
	RecipientUsername    string  `json:"recipient_username" db:"recipient_username"`

	// Display helpers
	CreatedAtHuman string `json:"created_at_human" db:"-"`
	ReadAtHuman    string `json:"read_at_human" db:"-"`
}

// Notification represents a system notification
type Notification struct {
	ID      int64  `json:"id" db:"id"`
	UserID  int64  `json:"user_id" db:"user_id" validate:"required"`
	Type    string `json:"type" db:"type" validate:"required"`
	Title   string `json:"title" db:"title" validate:"required,max=255"`
	Content *string `json:"content,omitempty" db:"content"`

	// Related entity references
	RelatedPostID     *int64 `json:"related_post_id,omitempty" db:"related_post_id"`
	RelatedQuestionID *int64 `json:"related_question_id,omitempty" db:"related_question_id"`
	RelatedCommentID  *int64 `json:"related_comment_id,omitempty" db:"related_comment_id"`
	RelatedJobID      *int64 `json:"related_job_id,omitempty" db:"related_job_id"`
	RelatedUserID     *int64 `json:"related_user_id,omitempty" db:"related_user_id"`

	// Actor information (who triggered the notification)
	ActorID        *int64  `json:"actor_id,omitempty" db:"actor_id"`
	ActorUsername  *string `json:"actor_username,omitempty" db:"actor_username"`
	ActorProfileURL *string `json:"actor_profile_url,omitempty" db:"actor_profile_url"`

	// Status
	IsRead bool `json:"is_read" db:"is_read"`
	IsSent bool `json:"is_sent" db:"is_sent"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty" db:"read_at"`
	SentAt    *time.Time `json:"sent_at,omitempty" db:"sent_at"`

	// Related entity information (joined, optional)
	RelatedEntityTitle *string `json:"related_entity_title,omitempty" db:"-"`
	RelatedUserName    *string `json:"related_user_name,omitempty" db:"-"`

	// Display helpers
	CreatedAtHuman string `json:"created_at_human" db:"-"`
	ReadAtHuman    string `json:"read_at_human" db:"-"`
}

// ===============================
// REACTION TABLES
// ===============================

// PostReaction represents a user's reaction to a post
type PostReaction struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id" validate:"required"`
	PostID    int64     `json:"post_id" db:"post_id" validate:"required"`
	Reaction  string    `json:"reaction" db:"reaction" validate:"oneof=like dislike"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// QuestionReaction represents a user's reaction to a question
type QuestionReaction struct {
	ID         int64     `json:"id" db:"id"`
	UserID     int64     `json:"user_id" db:"user_id" validate:"required"`
	QuestionID int64     `json:"question_id" db:"question_id" validate:"required"`
	Reaction   string    `json:"reaction" db:"reaction" validate:"oneof=like dislike"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// CommentReaction represents a user's reaction to a comment
type CommentReaction struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id" validate:"required"`
	CommentID int64     `json:"comment_id" db:"comment_id" validate:"required"`
	Reaction  string    `json:"reaction" db:"reaction" validate:"oneof=like dislike"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ===============================
// PAGINATION & QUERY HELPERS
// ===============================

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Limit  int    `json:"limit" validate:"min=1,max=100"`
	Offset int    `json:"offset" validate:"min=0"`
	Cursor string `json:"cursor,omitempty"` // For cursor-based pagination
	Sort   string `json:"sort,omitempty" validate:"omitempty,oneof=created_at updated_at title likes_count"`
	Order  string `json:"order,omitempty" validate:"omitempty,oneof=asc desc"`
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
	Filters    map[string]any `json:"filters,omitempty"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	CurrentPage  int    `json:"current_page"`
	TotalPages   int    `json:"total_pages"`
	TotalItems   int64  `json:"total_items"`
	ItemsPerPage int    `json:"items_per_page"`
	HasNext      bool   `json:"has_next"`
	HasPrev      bool   `json:"has_prev"`
	NextCursor   string `json:"next_cursor,omitempty"`
	PrevCursor   string `json:"prev_cursor,omitempty"`
}

// ===============================
// CUSTOM TYPES
// ===============================

// StringArray handles PostgreSQL array types
type StringArray []string

// Scan implements sql.Scanner
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = StringArray{}
		return nil
	}

	switch v := value.(type) {
	case string:
		// Handle PostgreSQL array format: {item1,item2,item3}
		v = strings.Trim(v, "{}")
		if v == "" {
			*s = StringArray{}
			return nil
		}
		*s = StringArray(strings.Split(v, ","))
	case []byte:
		return s.Scan(string(v))
	default:
		return fmt.Errorf("cannot scan %T into StringArray", value)
	}
	return nil
}

// Value implements driver.Valuer
func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}
	return "{" + strings.Join(s, ",") + "}", nil
}

// ===============================
// LEGACY COMPATIBILITY TYPES
// ===============================

// TokenResponse for OAuth integration
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

// UserInfo for OAuth user information
type UserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}



// ===============================
// HELPER METHODS
// ===============================

// IsExpired checks if a session is expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// GetFullName returns the user's full name or username
func (u *User) GetFullName() string {
	if u.FirstName != nil && u.LastName != nil {
		return *u.FirstName + " " + *u.LastName
	}
	if u.FirstName != nil {
		return *u.FirstName
	}
	return u.Username
}

// HasValidProfile checks if user has a complete profile
func (u *User) HasValidProfile() bool {
	return u.FirstName != nil && u.LastName != nil && u.Bio != nil
}

// IsOwner checks if the user owns the content
func (p *Post) IsOwnedBy(userID int64) bool {
	return p.UserID == userID
}

// IsOwner checks if the user owns the question
func (q *Question) IsOwnedBy(userID int64) bool {
	return q.UserID == userID
}

// IsOwner checks if the user owns the comment
func (c *Comment) IsOwnedBy(userID int64) bool {
	return c.UserID == userID
}

// IsOwner checks if the user owns the job
func (j *Job) IsOwnedBy(userID int64) bool {
	return j.EmployerID == userID
}

// CalculateOffset calculates offset from page and limit
func (p *PaginationParams) CalculateOffset() int {
	if p.Offset > 0 {
		return p.Offset
	}
	// If using page-based pagination
	page := p.Offset/p.Limit + 1
	if page < 1 {
		page = 1
	}
	return (page - 1) * p.Limit
}

// IsThreaded checks if comment supports threading
func (c *Comment) IsThreaded() bool {
	return c.ParentCommentID != nil
}

// GetParentType returns the type of parent entity
func (c *Comment) GetParentType() string {
	if c.PostID != nil {
		return "post"
	}
	if c.QuestionID != nil {
		return "question"
	}
	if c.DocumentID != nil {
		return "document"
	}
	return "unknown"
}

// IsPublished checks if content is published
func (p *Post) IsPublished() bool {
	return p.Status == "published" && p.PublishedAt != nil
}

// IsPublished checks if question is published
func (q *Question) IsPublished() bool {
	return q.Status == "published" && q.PublishedAt != nil
}

// IsActive checks if job is active
func (j *Job) IsActive() bool {
	return j.Status == "active" && (j.ApplicationDeadline == nil || time.Now().Before(*j.ApplicationDeadline))
}

// IsExpired checks if job application deadline has passed
func (j *Job) IsExpired() bool {
	return j.ApplicationDeadline != nil && time.Now().After(*j.ApplicationDeadline)
}

// CanApply checks if applications are still being accepted
func (j *Job) CanApply() bool {
	return j.Status == "active" && !j.IsExpired()
}

// IsUnread checks if notification is unread
func (n *Notification) IsUnread() bool {
	return !n.IsRead
}

// IsUnread checks if message is unread
func (m *Message) IsUnread() bool {
	return !m.IsRead
}

// ===============================
// VALIDATION HELPERS
// ===============================

// ValidatePostStatus validates post status enum
func ValidatePostStatus(status string) bool {
	validStatuses := []string{"draft", "published", "archived", "deleted", "flagged", "approved", "rejected"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

// ValidateJobStatus validates job status enum  
func ValidateJobStatus(status string) bool {
	validStatuses := []string{"draft", "active", "paused", "closed", "filled"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

// ValidateApplicationStatus validates application status enum
func ValidateApplicationStatus(status string) bool {
	validStatuses := []string{"pending", "reviewing", "shortlisted", "interviewed", "accepted", "rejected", "withdrawn"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

// ValidateUserRole validates user role enum
func ValidateUserRole(role string) bool {
	validRoles := []string{"user", "reviewer", "moderator", "admin"}
	for _, valid := range validRoles {
		if role == valid {
			return true
		}
	}
	return false
}

// ValidateEmploymentType validates employment type enum
func ValidateEmploymentType(empType string) bool {
	validTypes := []string{"full_time", "part_time", "contract", "temporary", "internship", "volunteer", "freelance"}
	for _, valid := range validTypes {
		if empType == valid {
			return true
		}
	}
	return false
}

// ValidateExpertiseLevel validates expertise level enum
func ValidateExpertiseLevel(level string) bool {
	validLevels := []string{"none", "beginner", "intermediate", "advanced", "expert"}
	for _, valid := range validLevels {
		if level == valid {
			return true
		}
	}
	return false
}

// ValidateReactionType validates reaction type enum
func ValidateReactionType(reaction string) bool {
	validReactions := []string{"like", "dislike"}
	for _, valid := range validReactions {
		if reaction == valid {
			return true
		}
	}
	return false
}

// ValidateNotificationType validates notification type enum
func ValidateNotificationType(notifType string) bool {
	validTypes := []string{
		"new_post", "new_question", "post_comment", "question_comment", "comment_reply",
		"post_like", "question_like", "comment_like", "chat_message", "job_posted",
		"job_application", "job_status_update", "announcement", "system_update", "security_alert",
	}
	for _, valid := range validTypes {
		if notifType == valid {
			return true
		}
	}
	return false
}
