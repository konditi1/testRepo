// ===============================
// FILE: internal/handlers/api/v1/users/types.go
// MT-11: User API Request/Response DTOs (Additional types not in services)
// ===============================

package users

import (
	"io"
	"time"
)

// ===============================
// API-SPECIFIC REQUEST TYPES
// ===============================

// UserSearchFilters represents additional search filters for user API
type UserSearchFilters struct {
	Role              *string `json:"role,omitempty" form:"role"`
	Expertise         *string `json:"expertise,omitempty" form:"expertise"`
	MinExperience     *int    `json:"min_experience,omitempty" form:"min_experience"`
	MaxExperience     *int    `json:"max_experience,omitempty" form:"max_experience"`
	IsOnline          *bool   `json:"is_online,omitempty" form:"is_online"`
	MinReputation     *int    `json:"min_reputation,omitempty" form:"min_reputation"`
	HasProfilePicture *bool   `json:"has_profile_picture,omitempty" form:"has_profile_picture"`
	HasCV             *bool   `json:"has_cv,omitempty" form:"has_cv"`
	JoinedAfter       *time.Time `json:"joined_after,omitempty" form:"joined_after"`
	JoinedBefore      *time.Time `json:"joined_before,omitempty" form:"joined_before"`
}

// OnlineStatusRequest represents a request to update online status
type OnlineStatusRequest struct {
	Online bool `json:"online" validate:"required"`
}

// BulkActionRequest represents a request for bulk operations on users
type BulkActionRequest struct {
	UserIDs []int64 `json:"user_ids" validate:"required,min=1,max=100"`
	Action  string  `json:"action" validate:"required,oneof=activate deactivate"`
}

// UserPreferencesRequest represents user preference updates
type UserPreferencesRequest struct {
	EmailNotifications      *bool   `json:"email_notifications,omitempty"`
	PushNotifications       *bool   `json:"push_notifications,omitempty"`
	PrivacyLevel           *string `json:"privacy_level,omitempty" validate:"omitempty,oneof=public private friends"`
	ShowOnlineStatus       *bool   `json:"show_online_status,omitempty"`
	ShowEmail              *bool   `json:"show_email,omitempty"`
	AllowContactRequests   *bool   `json:"allow_contact_requests,omitempty"`
	TimeZone               *string `json:"timezone,omitempty"`
	Language               *string `json:"language,omitempty"`
	ThemePreference        *string `json:"theme_preference,omitempty" validate:"omitempty,oneof=light dark auto"`
}

// PasswordUpdateRequest represents a password change request
type PasswordUpdateRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=128"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

// EmailUpdateRequest represents an email change request
type EmailUpdateRequest struct {
	NewEmail    string `json:"new_email" validate:"required,email"`
	Password    string `json:"password" validate:"required"`
}

// UsernameUpdateRequest represents a username change request
type UsernameUpdateRequest struct {
	NewUsername string `json:"new_username" validate:"required,min=3,max=30,alphanum"`
	Password    string `json:"password" validate:"required"`
}

// ===============================
// API-SPECIFIC RESPONSE TYPES
// ===============================

// UserProfileResponse represents a comprehensive user profile response
type UserProfileResponse struct {
	UserID           int64                  `json:"user_id"`
	Username         string                 `json:"username"`
	Email            string                 `json:"email,omitempty"` // Only shown to self
	DisplayName      string                 `json:"display_name"`
	FirstName        *string                `json:"first_name,omitempty"`
	LastName         *string                `json:"last_name,omitempty"`
	JobTitle         *string                `json:"job_title,omitempty"`
	Affiliation      *string                `json:"affiliation,omitempty"`
	Bio              *string                `json:"bio,omitempty"`
	YearsExperience  int                    `json:"years_experience"`
	CoreCompetencies *string                `json:"core_competencies,omitempty"`
	Expertise        string                 `json:"expertise"`
	ProfileURL       *string                `json:"profile_url,omitempty"`
	CVURL           *string                `json:"cv_url,omitempty"`
	WebsiteURL       *string                `json:"website_url,omitempty"`
	LinkedinProfile  *string                `json:"linkedin_profile,omitempty"`
	TwitterHandle    *string                `json:"twitter_handle,omitempty"`
	Role             string                 `json:"role"`
	Level            string                 `json:"level"`
	LevelColor       string                 `json:"level_color"`
	ReputationPoints int                    `json:"reputation_points"`
	IsOnline         bool                   `json:"is_online"`
	IsVerified       bool                   `json:"is_verified"`
	JoinedAt         time.Time              `json:"joined_at"`
	LastSeen         *time.Time             `json:"last_seen,omitempty"`
	Stats            *UserStatsResponse     `json:"stats,omitempty"`
	Badges           []Badge                `json:"badges,omitempty"`
	SocialLinks      map[string]string      `json:"social_links,omitempty"`
	Preferences      *UserPreferences       `json:"preferences,omitempty"` // Only shown to self
}

// UserListResponse represents a user in list responses
type UserListResponse struct {
	UserID           int64     `json:"user_id"`
	Username         string    `json:"username"`
	DisplayName      string    `json:"display_name"`
	ProfileURL       *string   `json:"profile_url,omitempty"`
	Affiliation      *string   `json:"affiliation,omitempty"`
	Expertise        string    `json:"expertise"`
	Role             string    `json:"role"`
	Level            string    `json:"level"`
	LevelColor       string    `json:"level_color"`
	ReputationPoints int       `json:"reputation_points"`
	IsOnline         bool      `json:"is_online"`
	IsVerified       bool      `json:"is_verified"`
	JoinedAt         time.Time `json:"joined_at"`
	LastSeen         *time.Time `json:"last_seen,omitempty"`
}

// UserStatsResponse represents comprehensive user statistics
type UserStatsResponse struct {
	UserID              int64                 `json:"user_id"`
	ReputationPoints    int                   `json:"reputation_points"`
	Level               string                `json:"level"`
	NextLevelPoints     int                   `json:"next_level_points"`
	PostsCount          int                   `json:"posts_count"`
	QuestionsCount      int                   `json:"questions_count"`
	CommentsCount       int                   `json:"comments_count"`
	TotalContributions  int                   `json:"total_contributions"`
	LikesReceived       int                   `json:"likes_received"`
	LikesGiven          int                   `json:"likes_given"`
	JoinedAt            time.Time             `json:"joined_at"`
	LastActivity        *time.Time            `json:"last_activity,omitempty"`
	Badges              []Badge               `json:"badges"`
	ActivityStats       *ActivityStats        `json:"activity_stats,omitempty"`
	ContributionHistory *ContributionHistory  `json:"contribution_history,omitempty"`
}

// UserActivityResponse represents user activity data
type UserActivityResponse struct {
	UserID       int64                `json:"user_id"`
	Days         int                  `json:"days"`
	TotalActions int                  `json:"total_actions"`
	DailyStats   []DailyActivityStats `json:"daily_stats"`
	Summary      ActivitySummary      `json:"summary"`
}

// OnlineUsersResponse represents the response for online users
type OnlineUsersResponse struct {
	Users     []UserListResponse `json:"users"`
	Count     int                `json:"count"`
	Limit     int                `json:"limit"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// LeaderboardResponse represents the user leaderboard
type LeaderboardResponse struct {
	Leaderboard []UserListResponse `json:"leaderboard"`
	Count       int                `json:"count"`
	Limit       int                `json:"limit"`
	Period      string             `json:"period"` // all_time, monthly, weekly
	UpdatedAt   time.Time          `json:"updated_at"`
}

// FileUploadResponse represents a file upload result
type FileUploadResponse struct {
	URL         string    `json:"url"`
	PublicID    string    `json:"public_id"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// BulkActionResponse represents the result of a bulk operation
type BulkActionResponse struct {
	Action      string              `json:"action"`
	TotalCount  int                 `json:"total_count"`
	SuccessCount int                `json:"success_count"`
	FailureCount int                `json:"failure_count"`
	Results     []BulkActionResult  `json:"results"`
}

// BulkActionResult represents individual result in bulk operation
type BulkActionResult struct {
	UserID  int64  `json:"user_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ===============================
// SUPPORTING TYPES
// ===============================

// Badge represents a user achievement badge
type Badge struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IconURL     string    `json:"icon_url"`
	Color       string    `json:"color"`
	Rarity      string    `json:"rarity"` // common, rare, epic, legendary
	EarnedAt    time.Time `json:"earned_at"`
}

// UserPreferences represents user preference settings
type UserPreferences struct {
	EmailNotifications      bool   `json:"email_notifications"`
	PushNotifications       bool   `json:"push_notifications"`
	PrivacyLevel           string `json:"privacy_level"`
	ShowOnlineStatus       bool   `json:"show_online_status"`
	ShowEmail              bool   `json:"show_email"`
	AllowContactRequests   bool   `json:"allow_contact_requests"`
	TimeZone               string `json:"timezone"`
	Language               string `json:"language"`
	ThemePreference        string `json:"theme_preference"`
}

// ActivityStats represents user activity statistics
type ActivityStats struct {
	DailyAverage    float64 `json:"daily_average"`
	WeeklyAverage   float64 `json:"weekly_average"`
	MonthlyAverage  float64 `json:"monthly_average"`
	StreakDays      int     `json:"streak_days"`
	LongestStreak   int     `json:"longest_streak"`
	MostActiveDay   string  `json:"most_active_day"`
	MostActiveHour  int     `json:"most_active_hour"`
}

// ContributionHistory represents user contribution over time
type ContributionHistory struct {
	Daily   []DailyContribution   `json:"daily,omitempty"`
	Weekly  []WeeklyContribution  `json:"weekly,omitempty"`
	Monthly []MonthlyContribution `json:"monthly,omitempty"`
}

// DailyActivityStats represents activity data for a specific day
type DailyActivityStats struct {
	Date         time.Time `json:"date"`
	PostsCreated int       `json:"posts_created"`
	Comments     int       `json:"comments"`
	Likes        int       `json:"likes"`
	Questions    int       `json:"questions"`
	TotalActions int       `json:"total_actions"`
}

// DailyContribution represents contributions for a specific day
type DailyContribution struct {
	Date         time.Time `json:"date"`
	Posts        int       `json:"posts"`
	Comments     int       `json:"comments"`
	Questions    int       `json:"questions"`
	Likes        int       `json:"likes"`
	Total        int       `json:"total"`
}

// WeeklyContribution represents contributions for a specific week
type WeeklyContribution struct {
	Week         time.Time `json:"week"`
	Posts        int       `json:"posts"`
	Comments     int       `json:"comments"`
	Questions    int       `json:"questions"`
	Likes        int       `json:"likes"`
	Total        int       `json:"total"`
}

// MonthlyContribution represents contributions for a specific month
type MonthlyContribution struct {
	Month        time.Time `json:"month"`
	Posts        int       `json:"posts"`
	Comments     int       `json:"comments"`
	Questions    int       `json:"questions"`
	Likes        int       `json:"likes"`
	Total        int       `json:"total"`
}

// ActivitySummary represents a summary of user activity
type ActivitySummary struct {
	MostActiveDay   time.Time `json:"most_active_day"`
	AvgDailyActions float64   `json:"avg_daily_actions"`
	ConsecutiveDays int       `json:"consecutive_days"`
	TotalEngagement int       `json:"total_engagement"`
}

// ===============================
// FILE UPLOAD TYPES
// ===============================

// FileUploadRequest represents a file upload request
type FileUploadRequest struct {
	UserID      int64  `json:"user_id"`
	File        io.Reader `json:"-"`
	FileName    string `json:"file_name" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	FileSize    int64  `json:"file_size" validate:"required,min=1"`
	Folder      string `json:"folder" validate:"required"`
}

// ImageUploadRequest represents an image upload request
type ImageUploadRequest struct {
	UserID      int64     `json:"user_id"`
	File        io.Reader `json:"-"`
	FileName    string    `json:"file_name" validate:"required"`
	ContentType string    `json:"content_type" validate:"required,oneof=image/jpeg image/png image/gif image/webp"`
	FileSize    int64     `json:"file_size" validate:"required,min=1,max=5242880"` // 5MB max
}

// DocumentUploadRequest represents a document upload request
type DocumentUploadRequest struct {
	UserID      int64     `json:"user_id"`
	File        io.Reader `json:"-"`
	FileName    string    `json:"file_name" validate:"required"`
	ContentType string    `json:"content_type" validate:"required"`
	FileSize    int64     `json:"file_size" validate:"required,min=1,max=10485760"` // 10MB max
	DocumentType string   `json:"document_type" validate:"required,oneof=cv resume portfolio"`
}

// ===============================
// ERROR TYPES
// ===============================

// UserAPIError represents API-specific error types
type UserAPIError struct {
	Type       string                 `json:"type"`
	Message    string                 `json:"message"`
	Code       string                 `json:"code,omitempty"`
	Field      string                 `json:"field,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []UserAPIError `json:"errors"`
}

// ===============================
// HELPER TYPES FOR API RESPONSES
// ===============================

// UserListMetadata represents metadata for user list responses
type UserListMetadata struct {
	TotalCount       int64                  `json:"total_count"`
	FilteredCount    int64                  `json:"filtered_count"`
	Page             int                    `json:"page"`
	PageSize         int                    `json:"page_size"`
	TotalPages       int                    `json:"total_pages"`
	HasNext          bool                   `json:"has_next"`
	HasPrevious      bool                   `json:"has_previous"`
	Filters          *UserSearchFilters     `json:"filters,omitempty"`
	SortBy           string                 `json:"sort_by,omitempty"`
	SortOrder        string                 `json:"sort_order,omitempty"`
	SearchQuery      string                 `json:"search_query,omitempty"`
	ProcessingTime   float64                `json:"processing_time_ms"`
}

// UserStatsMetadata represents metadata for user statistics
type UserStatsMetadata struct {
	GeneratedAt     time.Time `json:"generated_at"`
	IncludesBadges  bool      `json:"includes_badges"`
	IncludesHistory bool      `json:"includes_history"`
	TimePeriod      string    `json:"time_period,omitempty"`
	ProcessingTime  float64   `json:"processing_time_ms"`
}