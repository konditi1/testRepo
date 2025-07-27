package web

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

func ChatHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value(userIDKey).(int)
	if !ok {
		RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	recipientIDStr := r.URL.Query().Get("recipient_id")
	recipientID, err := strconv.Atoi(recipientIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid recipient ID"))
		return
	}

	messagesByDate, err := GetMessages(ctx, userID, recipientID)
	if err != nil {
		log.Printf("Error fetching messages: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch messages"))
		return
	}

	recipientUsername, recipientProfileURL, err := GetRecipientInfo(ctx, recipientID)
	if err != nil {
		log.Printf("Error fetching recipient info: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch recipient info"))
		return
	}

	// Update the last seen time for the current user
	_, err = database.DB.ExecContext(
		ctx,
		"UPDATE users SET last_seen = NOW() WHERE id = $1",
		userID,
	)
	if err != nil {
		// Log the error but don't fail the request
		log.Printf("Failed to update last seen time: %v", err)
	}

	// Get recipient's online status and last seen time
	isOnline, lastSeen, err := utils.GetUserOnlineStatus(recipientID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch user status: %v", err))
		return
	}

	// Format the status text
	var statusText string
	if isOnline {
		statusText = "online"
	} else {
		statusText = utils.FormatLastSeen(lastSeen)
	}

	// Generate initials avatar for recipient
	initialsAvatar := utils.GenerateInitialsAvatar(recipientUsername)

	data := map[string]interface{}{
		"Title":             "Chat - EvalHub",
		"IsLoggedIn":        true,
		"Username":          getUsername(userID),
		"MessagesByDate":    messagesByDate,
		"RecipientID":       recipientID,
		"RecipientUsername": recipientUsername,
		"RecipientProfile":  recipientProfileURL,
		"UserID":            userID,
		"RecipientOnline":   isOnline,
		"RecipientLastSeen": statusText,
		"RecipientInitials": initialsAvatar,
	}
	err = templates.ExecuteTemplate(w, "chat", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

type MessagesByDate struct {
	Date     string
	Messages []models.Message
}

func GetMessages(ctx context.Context, userID, recipientID int) ([]MessagesByDate, error) {
	query := `
		SELECT m.id, m.sender_id, m.recipient_id, m.content, m.created_at,
		       us.username AS sender_username, ur.username AS recipient_username
		FROM messages m
		JOIN users us ON m.sender_id = us.id
		JOIN users ur ON m.recipient_id = ur.id
		WHERE (m.sender_id = $1 AND m.recipient_id = $2) OR (m.sender_id = $3 AND m.recipient_id = $4)
		ORDER BY m.created_at ASC`
	
	rows, err := database.DB.QueryContext(ctx, query, userID, recipientID, recipientID, userID)
	if err != nil {
		return nil, fmt.Errorf("error querying messages: %v", err)
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		err := rows.Scan(
			&msg.ID,
			&msg.SenderID,
			&msg.RecipientID,
			&msg.Content,
			&msg.CreatedAt,
			&msg.SenderUsername,
			&msg.RecipientUsername,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning message: %v", err)
		}
		messages = append(messages, msg)
	}
	// Group messages by date
	var messagesByDate []MessagesByDate
	if len(messages) == 0 {
		return messagesByDate, nil
	}

	currentDate := ""

	// Use UTC for consistency in date comparisons
	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	for _, msg := range messages {
		// Ensure the timestamp is in UTC to match today/yesterday
		msgDate := msg.CreatedAt.UTC().Truncate(24 * time.Hour)
		var dateStr string

		if msgDate.Equal(today) {
			dateStr = "Today"
		} else if msgDate.Equal(yesterday) {
			dateStr = "Yesterday"
		} else {
			// Format with explicit time zone to ensure correct display
			dateStr = msg.CreatedAt.Format("Monday, January 2, 2006")
		}
		if dateStr != currentDate {
			currentDate = dateStr
			messagesByDate = append(messagesByDate, MessagesByDate{
				Date:     dateStr,
				Messages: []models.Message{},
			})
		}
		messagesByDate[len(messagesByDate)-1].Messages = append(messagesByDate[len(messagesByDate)-1].Messages, msg)
	}
	return messagesByDate, nil
}

func GetRecipientInfo(ctx context.Context, recipientID int) (username, profileURL string, err error) {
	err = database.DB.QueryRowContext(
		ctx,
		"SELECT username, profile_image_url FROM users WHERE id = $1",
		recipientID,
	).Scan(&username, &profileURL)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("recipient not found")
		}
		return "", "", fmt.Errorf("error querying recipient info: %v", err)
	}

	return username, profileURL, nil
}
