// internal/handlers/web/notifications.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/services"
	"evalhub/internal/utils"
)

// NotificationService handles all notification operations
type NotificationService struct{}

// CreateNotification creates a new notification
func (ns *NotificationService) CreateNotification(userID int, notificationType, title, message string, entityID *int, entityType *string, actorID *int) error {
	// Check if user wants this type of notification
	if !ns.shouldSendNotification(userID, notificationType) {
		return nil
	}

	var actorUsername, actorProfileURL *string
	if actorID != nil {
		query := `SELECT username, COALESCE(profile_url, '') FROM users WHERE id = $1`
		var username, profileURL string
		err := database.DB.QueryRowContext(context.Background(), query, *actorID).Scan(&username, &profileURL)
		if err == nil {
			actorUsername = &username
			if profileURL != "" {
				actorProfileURL = &profileURL
			}
		}
	}

	query := `
        INSERT INTO notifications (user_id, type, title, message, entity_id, entity_type, actor_id, actor_username, actor_profile_url)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := database.DB.ExecContext(context.Background(), query, userID, notificationType, title, message, entityID, entityType, actorID, actorUsername, actorProfileURL)
	if err != nil {
		log.Printf("Failed to create notification: %v", err)
		return err
	}

	return nil
}

// shouldSendNotification checks user preferences
func (ns *NotificationService) shouldSendNotification(userID int, notificationType string) bool {
	prefs, err := ns.GetUserPreferences(userID)
	if err != nil {
		// Default to sending notification if preferences can't be retrieved
		return true
	}

	switch notificationType {
	case "new_post":
		return prefs.NewPosts
	case "new_question":
		return prefs.NewQuestions
	case "post_comment", "question_comment":
		if notificationType == "post_comment" {
			return prefs.CommentsOnMyPosts
		}
		return prefs.CommentsOnMyQuestions
	case "post_like", "question_like", "comment_like":
		return prefs.LikesOnMyContent
	case "chat_message":
		return prefs.ChatMessages
	case "job_posted":
		return prefs.JobPostings
	case "job_application":
		return prefs.JobApplications
	case "announcement":
		return prefs.Announcements
	default:
		return true
	}
}

// NotificationDisplay extends Notification with display-specific fields
type NotificationDisplay struct {
	models.Notification
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	ActionURL   string `json:"action_url"`
	CreatedAtHuman string `json:"created_at_human"`
}

// GetUserNotifications retrieves notifications for a user
func (ns *NotificationService) GetUserNotifications(userID int, limit int) ([]NotificationDisplay, error) {
	query := `
        SELECT id, user_id, type, title, message, read, entity_id, entity_type, 
               actor_id, actor_username, actor_profile_url, created_at
        FROM notifications 
        WHERE user_id = $1 
        ORDER BY created_at DESC 
        LIMIT $2`

	rows, err := database.DB.QueryContext(context.Background(), query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []NotificationDisplay
	for rows.Next() {
		var n models.Notification
		err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.IsRead,
			&n.RelatedPostID, &n.RelatedQuestionID, &n.RelatedCommentID, &n.ActorID, &n.ActorUsername, &n.ActorProfileURL, &n.CreatedAt)
		if err != nil {
			log.Printf("Error scanning notification: %v", err)
			continue
		}

		// Set display properties
		// Create display notification with additional fields
		display := NotificationDisplay{
			Notification: n,
			CreatedAtHuman: utils.TimeAgo(n.CreatedAt),
		}
		display.Icon, display.Color = ns.getNotificationStyle(n.Type)
		
		// Determine the appropriate entity ID based on notification type
		var entityID *int
		switch n.Type {
		case "post_comment", "post_like":
			if n.RelatedPostID != nil {
				val := int(*n.RelatedPostID)
				entityID = &val
			}
		case "question_comment", "question_like":
			if n.RelatedQuestionID != nil {
				val := int(*n.RelatedQuestionID)
				entityID = &val
			}
		case "comment_reply", "comment_like":
			if n.RelatedCommentID != nil {
				val := int(*n.RelatedCommentID)
				entityID = &val
			}
		}
		display.ActionURL = ns.getActionURL(n.Type, entityID, &n.Type)

		notifications = append(notifications, display)
	}

	return notifications, nil
}

// getNotificationStyle returns icon and color for notification type
func (ns *NotificationService) getNotificationStyle(notificationType string) (icon, color string) {
	switch notificationType {
	case "new_post":
		return "fa-solid fa-file-lines", "#3498db"
	case "new_question":
		return "fa-solid fa-circle-question", "#9b59b6"
	case "post_comment", "question_comment":
		return "fa-solid fa-comment", "#2ecc71"
	case "post_like", "question_like", "comment_like":
		return "fa-solid fa-heart", "#e74c3c"
	case "chat_message":
		return "fa-solid fa-envelope", "#f39c12"
	case "job_posted":
		return "fa-solid fa-briefcase", "#1abc9c"
	case "job_application":
		return "fa-solid fa-paper-plane", "#3498db"
	case "announcement":
		return "fa-solid fa-bullhorn", "#e67e22"
	default:
		return "fa-solid fa-bell", "#95a5a6"
	}
}

// getActionURL generates URL for notification action
func (ns *NotificationService) getActionURL(notificationType string, entityID *int, entityType *string) string {
	if entityID == nil {
		return "/dashboard"
	}

	switch notificationType {
	case "new_post", "post_comment", "post_like":
		return fmt.Sprintf("/view-post?id=%d", *entityID)
	case "new_question", "question_comment", "question_like":
		return fmt.Sprintf("/view-question?id=%d", *entityID)
	case "job_posted", "job_application":
		return fmt.Sprintf("/view-job?id=%d", *entityID)
	case "chat_message":
		return fmt.Sprintf("/chat?recipient_id=%d", *entityID)
	default:
		return "/dashboard"
	}
}

// GetUnreadCount returns count of unread notifications
func (ns *NotificationService) GetUnreadCount(userID int) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE`
	err := database.DB.QueryRowContext(context.Background(), query, userID).Scan(&count)
	return count, err
}

// GetNotificationSummary returns detailed unread counts
func (ns *NotificationService) GetNotificationSummary(userID int) (*services.NotificationSummaryResponse, error) {
	summary := &services.NotificationSummaryResponse{}

	// Total unread
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE`
	err := database.DB.QueryRowContext(context.Background(), query, userID).Scan(&summary.UnreadCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get unread count: %w", err)
	}

	// Unread by type
	typeQueries := map[string]*int{
		"post_like":        &summary.UnreadLikes,
		"question_like":    &summary.UnreadLikes,
		"comment_like":     &summary.UnreadLikes,
		"post_comment":     &summary.UnreadComments,
		"question_comment": &summary.UnreadComments,
		"comment_reply":    &summary.UnreadComments,
		"job_posted":       &summary.UnreadJobAlerts,
		"announcement":     &summary.UnreadSystemAlerts,
	}

	for notifType, countPtr := range typeQueries {
		query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE AND type = $2`
		err = database.DB.QueryRowContext(context.Background(), query, userID, notifType).Scan(countPtr)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Failed to get count for notification type %s: %v", notifType, err)
		}
	}

	return summary, nil
}

// MarkAsRead marks notifications as read
func (ns *NotificationService) MarkAsRead(userID int, notificationIDs []int) error {
	if len(notificationIDs) == 0 {
		return nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(notificationIDs))
	args := make([]interface{}, len(notificationIDs)+1)
	args[0] = userID

	for i, id := range notificationIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(`
        UPDATE notifications 
        SET read = TRUE 
        WHERE user_id = $1 AND id IN (%s)`,
		fmt.Sprintf("%s", placeholders))

	_, err := database.DB.ExecContext(context.Background(), query, args...)
	return err
}

// MarkAllAsRead marks all notifications as read for a user
func (ns *NotificationService) MarkAllAsRead(userID int) error {
	query := `UPDATE notifications SET read = TRUE WHERE user_id = $1 AND read = FALSE`
	_, err := database.DB.ExecContext(context.Background(), query, userID)
	return err
}

// GetUserPreferences retrieves notification preferences for a user
func (ns *NotificationService) GetUserPreferences(userID int) (*models.NotificationPreferences, error) {
	prefs := &models.NotificationPreferences{}
	query := `
        SELECT id, user_id, new_posts, new_questions, comments_on_my_posts, 
               comments_on_my_questions, likes_on_my_content, chat_messages, 
               job_postings, job_applications, announcements, email_notifications, 
               push_notifications, created_at, updated_at
        FROM notification_preferences 
        WHERE user_id = $1`

	err := database.DB.QueryRowContext(context.Background(), query, userID).Scan(
		&prefs.ID, &prefs.UserID, &prefs.NewPosts, &prefs.NewQuestions,
		&prefs.CommentsOnMyPosts, &prefs.CommentsOnMyQuestions, &prefs.LikesOnMyContent,
		&prefs.ChatMessages, &prefs.JobPostings, &prefs.JobApplications,
		&prefs.Announcements, &prefs.EmailNotifications, &prefs.PushNotifications,
		&prefs.CreatedAt, &prefs.UpdatedAt)

	if err == sql.ErrNoRows {
		// Create default preferences
		return ns.CreateDefaultPreferences(userID)
	}

	return prefs, err
}

// CreateDefaultPreferences creates default notification preferences for a user
func (ns *NotificationService) CreateDefaultPreferences(userID int) (*models.NotificationPreferences, error) {
	query := `
        INSERT INTO notification_preferences (user_id) 
        VALUES ($1) 
        RETURNING id, user_id, new_posts, new_questions, comments_on_my_posts, 
                  comments_on_my_questions, likes_on_my_content, chat_messages, 
                  job_postings, job_applications, announcements, email_notifications, 
                  push_notifications, created_at, updated_at`

	prefs := &models.NotificationPreferences{}
	err := database.DB.QueryRowContext(context.Background(), query, userID).Scan(
		&prefs.ID, &prefs.UserID, &prefs.NewPosts, &prefs.NewQuestions,
		&prefs.CommentsOnMyPosts, &prefs.CommentsOnMyQuestions, &prefs.LikesOnMyContent,
		&prefs.ChatMessages, &prefs.JobPostings, &prefs.JobApplications,
		&prefs.Announcements, &prefs.EmailNotifications, &prefs.PushNotifications,
		&prefs.CreatedAt, &prefs.UpdatedAt)

	return prefs, err
}

// UpdateUserPreferences updates notification preferences
func (ns *NotificationService) UpdateUserPreferences(userID int, prefs *models.NotificationPreferences) error {
	query := `
        UPDATE notification_preferences 
        SET new_posts = $2, new_questions = $3, comments_on_my_posts = $4,
            comments_on_my_questions = $5, likes_on_my_content = $6, chat_messages = $7,
            job_postings = $8, job_applications = $9, announcements = $10,
            email_notifications = $11, push_notifications = $12, updated_at = $13
        WHERE user_id = $1`

	_, err := database.DB.ExecContext(context.Background(), query, userID, prefs.NewPosts, prefs.NewQuestions,
		prefs.CommentsOnMyPosts, prefs.CommentsOnMyQuestions, prefs.LikesOnMyContent,
		prefs.ChatMessages, prefs.JobPostings, prefs.JobApplications,
		prefs.Announcements, prefs.EmailNotifications, prefs.PushNotifications,
		time.Now())

	return err
}

// Global notification service instance
var NotificationSvc = &NotificationService{}

// HTTP Handlers

// NotificationsHandler serves the notifications page
func NotificationsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)

	notifications, err := NotificationSvc.GetUserNotifications(userID, 50)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch notifications: %v", err))
		return
	}

	user, err := getUserWithProfile(context.Background(), userID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(userID)}
	}

	data := map[string]interface{}{
		"Title":         "Notifications - Melconnect",
		"IsLoggedIn":    true,
		"Username":      user.Username,
		"User":          user,
		"Notifications": notifications,
	}

	err = templates.ExecuteTemplate(w, "notifications", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// GetNotificationsAPIHandler returns notifications as JSON
func GetNotificationsAPIHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)

	limitStr := r.URL.Query().Get("limit")
	limit := 20 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	notifications, err := NotificationSvc.GetUserNotifications(userID, limit)
	if err != nil {
		http.Error(w, "Failed to fetch notifications", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

// GetNotificationSummaryAPIHandler returns notification summary as JSON
func GetNotificationSummaryAPIHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)

	summary, err := NotificationSvc.GetNotificationSummary(userID)
	if err != nil {
		http.Error(w, "Failed to fetch notification summary", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// MarkNotificationsReadHandler marks notifications as read
func MarkNotificationsReadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Context().Value(userIDKey).(int)

	var request struct {
		NotificationIDs []int `json:"notification_ids"`
		MarkAll         bool  `json:"mark_all"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if request.MarkAll {
		err = NotificationSvc.MarkAllAsRead(userID)
	} else {
		err = NotificationSvc.MarkAsRead(userID, request.NotificationIDs)
	}

	if err != nil {
		http.Error(w, "Failed to mark notifications as read", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// NotificationPreferencesHandler handles notification preferences
func NotificationPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)

	if r.Method == http.MethodGet {
		prefs, err := NotificationSvc.GetUserPreferences(userID)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch preferences: %v", err))
			return
		}

		user, err := getUserWithProfile(context.Background(), userID)
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
			user = &models.User{Username: getUsername(userID)}
		}

		data := map[string]interface{}{
			"Title":       "Notification Preferences - Melconnect",
			"IsLoggedIn":  true,
			"Username":    user.Username,
			"User":        user,
			"Preferences": prefs,
		}

		err = templates.ExecuteTemplate(w, "notification-preferences", data)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}

	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("failed to parse form: %v", err))
			return
		}

		prefs := &models.NotificationPreferences{
			UserID:                int64(userID),
			NewPosts:              r.FormValue("new_posts") == "on",
			NewQuestions:          r.FormValue("new_questions") == "on",
			CommentsOnMyPosts:     r.FormValue("comments_on_my_posts") == "on",
			CommentsOnMyQuestions: r.FormValue("comments_on_my_questions") == "on",
			LikesOnMyContent:      r.FormValue("likes_on_my_content") == "on",
			ChatMessages:          r.FormValue("chat_messages") == "on",
			JobPostings:           r.FormValue("job_postings") == "on",
			JobApplications:       r.FormValue("job_applications") == "on",
			Announcements:         r.FormValue("announcements") == "on",
			EmailNotifications:    r.FormValue("email_notifications") == "on",
			PushNotifications:     r.FormValue("push_notifications") == "on",
		}

		err = NotificationSvc.UpdateUserPreferences(userID, prefs)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to update preferences: %v", err))
			return
		}

		http.Redirect(w, r, "/notification-preferences?success=1", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// NotifyPostCreated notifies when a new post is created
func NotifyPostCreated(postID, authorID int, title string) {
	// Get all users who want to be notified about new posts
	query := `
        SELECT u.id 
        FROM users u
        LEFT JOIN notification_preferences np ON u.id = np.user_id
        WHERE u.id != $1 AND (np.new_posts IS NULL OR np.new_posts = TRUE)`

	rows, err := database.DB.QueryContext(context.Background(), query, authorID)
	if err != nil {
		log.Printf("Failed to get users for post notification: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		go NotificationSvc.CreateNotification(
			userID,
			"new_post",
			"New Post",
			fmt.Sprintf("New post: %s", title),
			&postID,
			stringPtr("post"),
			&authorID,
		)
	}
}

// NotifyQuestionCreated notifies when a new question is created
func NotifyQuestionCreated(questionID, authorID int, title string) {
	query := `
        SELECT u.id 
        FROM users u
        LEFT JOIN notification_preferences np ON u.id = np.user_id
        WHERE u.id != $1 AND (np.new_questions IS NULL OR np.new_questions = TRUE)`

	rows, err := database.DB.QueryContext(context.Background(), query, authorID)
	if err != nil {
		log.Printf("Failed to get users for question notification: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		go NotificationSvc.CreateNotification(
			userID,
			"new_question",
			"New Question",
			fmt.Sprintf("New question: %s", title),
			&questionID,
			stringPtr("question"),
			&authorID,
		)
	}
}

// NotifyCommentCreated notifies when a comment is created
func NotifyCommentCreated(commentID, commenterID int, postID *int, questionID *int, content string) {
	var entityID int
	var entityType string
	var ownerID int
	var notificationType string

	if postID != nil {
		// Comment on post
		entityID = *postID
		entityType = "post"
		notificationType = "post_comment"

		// Get post owner
		query := `SELECT user_id FROM posts WHERE id = $1`
		if err := database.DB.QueryRowContext(context.Background(), query, *postID).Scan(&ownerID); err != nil {
			log.Printf("Failed to get post owner: %v", err)
			return
		}
	} else if questionID != nil {
		// Comment on question
		entityID = *questionID
		entityType = "question"
		notificationType = "question_comment"

		// Get question owner
		query := `SELECT user_id FROM questions WHERE id = $1`
		if err := database.DB.QueryRowContext(context.Background(), query, *questionID).Scan(&ownerID); err != nil {
			log.Printf("Failed to get question owner: %v", err)
			return
		}
	} else {
		return
	}

	// Don't notify if commenter is the owner
	if ownerID == commenterID {
		return
	}

	// Create notification for content owner
	message := fmt.Sprintf("New comment on your %s: %s", entityType, truncateString(content, 50))
	go NotificationSvc.CreateNotification(
		ownerID,
		notificationType,
		"New Comment",
		message,
		&entityID,
		&entityType,
		&commenterID,
	)
}

// NotifyLikeCreated notifies when a like is created
func NotifyLikeCreated(likerID int, postID *int, questionID *int, commentID *int) {
	var entityID int
	var entityType string
	var ownerID int
	var notificationType string

	if postID != nil {
		entityID = *postID
		entityType = "post"
		notificationType = "post_like"

		query := `SELECT user_id FROM posts WHERE id = $1`
		if err := database.DB.QueryRowContext(context.Background(), query, *postID).Scan(&ownerID); err != nil {
			return
		}
	} else if questionID != nil {
		entityID = *questionID
		entityType = "question"
		notificationType = "question_like"

		query := `SELECT user_id FROM questions WHERE id = $1`
		if err := database.DB.QueryRowContext(context.Background(), query, *questionID).Scan(&ownerID); err != nil {
			return
		}
	} else if commentID != nil {
		// For comment likes, link to the post/question containing the comment
		query := `SELECT user_id, post_id, question_id FROM comments WHERE id = $1`
		var postIDPtr, questionIDPtr sql.NullInt64
		if err := database.DB.QueryRowContext(context.Background(), query, *commentID).Scan(&ownerID, &postIDPtr, &questionIDPtr); err != nil {
			return
		}

		notificationType = "comment_like"
		if postIDPtr.Valid {
			entityID = int(postIDPtr.Int64)
			entityType = "post"
		} else if questionIDPtr.Valid {
			entityID = int(questionIDPtr.Int64)
			entityType = "question"
		} else {
			return
		}
	} else {
		return
	}

	// Don't notify if liker is the owner
	if ownerID == likerID {
		return
	}

	go NotificationSvc.CreateNotification(
		ownerID,
		notificationType,
		"New Like",
		fmt.Sprintf("Someone liked your %s", entityType),
		&entityID,
		&entityType,
		&likerID,
	)
}

// NotifyChatMessage notifies when a chat message is received
func NotifyChatMessage(senderID, recipientID int, content string) {
	message := fmt.Sprintf("New message: %s", truncateString(content, 50))
	go NotificationSvc.CreateNotification(
		recipientID,
		"chat_message",
		"New Message",
		message,
		&senderID,
		stringPtr("chat"),
		&senderID,
	)
}

// NotifyJobPosted notifies when a job is posted
func NotifyJobPosted(jobID, employerID int, title string) {
	query := `
        SELECT u.id 
        FROM users u
        LEFT JOIN notification_preferences np ON u.id = np.user_id
        WHERE u.id != $1 AND (np.job_postings IS NULL OR np.job_postings = TRUE)`

	rows, err := database.DB.QueryContext(context.Background(), query, employerID)
	if err != nil {
		log.Printf("Failed to get users for job notification: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		go NotificationSvc.CreateNotification(
			userID,
			"job_posted",
			"New Job Posted",
			fmt.Sprintf("New job: %s", title),
			&jobID,
			stringPtr("job"),
			&employerID,
		)
	}
}

// NotifyJobApplication notifies when someone applies for a job
func NotifyJobApplication(jobID, applicantID, employerID int, jobTitle string) {
	message := fmt.Sprintf("New application for: %s", jobTitle)
	go NotificationSvc.CreateNotification(
		employerID,
		"job_application",
		"New Job Application",
		message,
		&jobID,
		stringPtr("job"),
		&applicantID,
	)
}

// NotifyAnnouncement creates an announcement notification for all users
func NotifyAnnouncement(title, message string) {
	query := `
        SELECT u.id 
        FROM users u
        LEFT JOIN notification_preferences np ON u.id = np.user_id
        WHERE (np.announcements IS NULL OR np.announcements = TRUE)`

	rows, err := database.DB.QueryContext(context.Background(), query)
	if err != nil {
		log.Printf("Failed to get users for announcement: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		go NotificationSvc.CreateNotification(
			userID,
			"announcement",
			title,
			message,
			nil,
			nil,
			nil,
		)
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}
