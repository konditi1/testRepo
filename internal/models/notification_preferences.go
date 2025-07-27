package models

import "time"

// NotificationPreferences represents a user's notification preferences
type NotificationPreferences struct {
	ID                     int64     `json:"id" db:"id"`
	UserID                 int64     `json:"user_id" db:"user_id"`
	NewPosts               bool      `json:"new_posts" db:"new_posts"`
	NewQuestions           bool      `json:"new_questions" db:"new_questions"`
	CommentsOnMyPosts      bool      `json:"comments_on_my_posts" db:"comments_on_my_posts"`
	CommentsOnMyQuestions  bool      `json:"comments_on_my_questions" db:"comments_on_my_questions"`
	LikesOnMyContent       bool      `json:"likes_on_my_content" db:"likes_on_my_content"`
	ChatMessages           bool      `json:"chat_messages" db:"chat_messages"`
	JobPostings            bool      `json:"job_postings" db:"job_postings"`
	JobApplications        bool      `json:"job_applications" db:"job_applications"`
	Announcements          bool      `json:"announcements" db:"announcements"`
	EmailNotifications     bool      `json:"email_notifications" db:"email_notifications"`
	PushNotifications      bool      `json:"push_notifications" db:"push_notifications"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// DefaultNotificationPreferences returns default notification preferences
func DefaultNotificationPreferences(userID int64) *NotificationPreferences {
	return &NotificationPreferences{
		UserID:                userID,
		NewPosts:              true,
		NewQuestions:          true,
		CommentsOnMyPosts:     true,
		CommentsOnMyQuestions: true,
		LikesOnMyContent:      true,
		ChatMessages:          true,
		JobPostings:           true,
		JobApplications:       true,
		Announcements:         true,
		EmailNotifications:    true,
		PushNotifications:     true,
	}
}
