package web

import (
	"context"
	"encoding/json"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

type ShareRequest struct {
	ContentType string `json:"content_type"`
	ContentID   int    `json:"content_id"`
	Platform    string `json:"platform"`
}

// ShareContentHandler handles sharing for authenticated users only
func ShareContentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	// Check if user is authenticated (this will be ensured by AuthMiddleware in router)
	userID := r.Context().Value(userIDKey)
	if userID == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Authentication required to share content"})
		return
	}

	// Safe conversion since we know userID is not nil due to AuthMiddleware
	uid, ok := userID.(int)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid user session"})
		return
	}

	var shareReq ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&shareReq); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request format"})
		return
	}

	// Validate required fields
	if shareReq.ContentType == "" || shareReq.ContentID == 0 || shareReq.Platform == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing required fields"})
		return
	}

	// Get content details and generate share URL
	shareURL, title, description, err := GetContentShareDetails(shareReq.ContentType, shareReq.ContentID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Content not found"})
		return
	}

	// Generate platform-specific share URL
	platformURL, err := GeneratePlatformShareURL(shareReq.Platform, shareURL, title, description)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Track sharing activity for authenticated user
	go TrackContentSharing(uid, shareReq.ContentType, shareReq.ContentID, shareReq.Platform)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"share_url": platformURL,
		"message":   "Share URL generated successfully",
		"platform":  shareReq.Platform,
	})
}

func GetContentShareDetails(contentType string, contentID int) (shareURL, title, description string, err error) {
	baseURL := "https://melconnect.global"

	switch contentType {
	case "post":
		var post models.Post
		query := `SELECT id, title, content FROM posts WHERE id = $1`
		err = database.DB.QueryRowContext(context.Background(), query, contentID).Scan(&post.ID, &post.Title, &post.Content)
		if err != nil {
			return "", "", "", fmt.Errorf("post not found: %v", err)
		}
		shareURL = fmt.Sprintf("%s/view-post?id=%d", baseURL, contentID)
		title = post.Title
		description = truncateText(post.Content, 150)

	case "question":
		var question models.Question
		query := `SELECT id, title, COALESCE(content, '') FROM questions WHERE id = $1`
		err = database.DB.QueryRowContext(context.Background(), query, contentID).Scan(&question.ID, &question.Title, &question.Content)
		if err != nil {
			return "", "", "", fmt.Errorf("question not found: %v", err)
		}
		shareURL = fmt.Sprintf("%s/view-question?id=%d", baseURL, contentID)
		title = question.Title
		description = truncateText(*question.Content, 150)

	case "document":
		var document models.Document
		query := `SELECT id, title, COALESCE(description, '') FROM documents WHERE id = $1`
		err = database.DB.QueryRowContext(context.Background(), query, contentID).Scan(&document.ID, &document.Title, &document.Description)
		if err != nil {
			return "", "", "", fmt.Errorf("document not found: %v", err)
		}
		shareURL = fmt.Sprintf("%s/view-document?id=%d", baseURL, contentID)
		title = document.Title
		description = truncateText(document.Description, 150)

	default:
		return "", "", "", fmt.Errorf("invalid content type: %s", contentType)
	}

	return shareURL, title, description, nil
}

func GeneratePlatformShareURL(platform, shareURL, title, description string) (string, error) {
	switch platform {
	case "twitter":
		text := fmt.Sprintf("%s %s", title, shareURL)
		return fmt.Sprintf("https://twitter.com/intent/tweet?text=%s", url.QueryEscape(text)), nil

	case "linkedin":
		return fmt.Sprintf("https://www.linkedin.com/sharing/share-offsite/?url=%s", url.QueryEscape(shareURL)), nil

	case "facebook":
		return fmt.Sprintf("https://www.facebook.com/sharer/sharer.php?u=%s", url.QueryEscape(shareURL)), nil

	case "whatsapp":
		text := fmt.Sprintf("%s\n%s\n%s", title, description, shareURL)
		return fmt.Sprintf("https://wa.me/?text=%s", url.QueryEscape(text)), nil

	case "email":
		subject := fmt.Sprintf("Check out: %s", title)
		body := fmt.Sprintf("%s\n\n%s\n\n%s", title, description, shareURL)
		return fmt.Sprintf("mailto:?subject=%s&body=%s", url.QueryEscape(subject), url.QueryEscape(body)), nil

	default:
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
}

func TrackContentSharing(userID int, contentType string, contentID int, platform string) {
	// Log sharing activity
	log.Printf("User %d shared %s %d to %s", userID, contentType, contentID, platform)

	// Try to insert into database if table exists
	query := `INSERT INTO content_sharing (user_id, content_type, content_id, platform) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, contentType, contentID, platform)
	if err != nil {
		// Just log the error - don't break the sharing functionality
		log.Printf("Could not track sharing (table might not exist): %v", err)
	}
}

func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	if maxLength <= 3 {
		return "..."
	}
	return text[:maxLength-3] + "..."
}
