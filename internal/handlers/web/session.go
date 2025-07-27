package web

import (
	"context"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"log"
	"net/http"
	"time"
)

func ValidateSession(r *http.Request) (*models.Session, error) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return nil, err
	}
	var session models.Session
	query := `
		SELECT id, user_id, session_token, expires_at, last_activity, user_role 
		FROM sessions 
		WHERE session_token = $1 AND expires_at > $2`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = database.DB.QueryRowContext(ctx, query, cookie.Value, time.Now()).Scan(&session.ID, &session.UserID, &session.SessionToken,
		&session.ExpiresAt, &session.LastActivity, &session.UserRole)
	if err != nil {
		return nil, err
	}
	
	// Update last activity and online status
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	query = `UPDATE sessions SET last_activity = $1 WHERE id = $2`
	_, err = database.DB.ExecContext(ctx, query, time.Now(), session.ID)
	if err != nil {
		log.Printf("Failed to update session last_activity: %v", err)
		return nil, err
	}
	
	// Update user's online status and last_seen
	_, err = database.DB.ExecContext(ctx, "UPDATE users SET is_online = TRUE, last_seen = CURRENT_TIMESTAMP WHERE id = $1", session.UserID)
	if err != nil {
		log.Printf("Failed to set user online: %v", err)
		return nil, err
	}
	
	return &session, nil
}