package web

import (
	"context"
	"evalhub/internal/database"
	"net/http"
	"testing"
	"time"
)

func TestValidateSession(t *testing.T) {
	setupTestDB(t)
	defer database.DB.Close()

	// Insert test session
	validToken := "valid_session_token"
	expiredToken := "expired_session_token"
	futureTime := time.Now().Add(24 * time.Hour)
	pastTime := time.Now().Add(-24 * time.Hour)

	_, err := database.DB.ExecContext(context.Background(), `
		INSERT INTO sessions (user_id, session_token, expires_at) VALUES 
		(1, $1, $2),
		(2, $3, $4)
	`, validToken, futureTime, expiredToken, pastTime)
	if err != nil {
		t.Fatalf("Failed to insert test sessions: %v", err)
	}

	tests := []struct {
		name         string
		sessionToken string
		expectError  bool
		expectedUser int
	}{
		{
			name:         "Valid Session",
			sessionToken: validToken,
			expectError:  false,
			expectedUser: 1,
		},
		{
			name:         "Expired Session",
			sessionToken: expiredToken,
			expectError:  true,
		},
		{
			name:         "Non-existent Session",
			sessionToken: "nonexistent_token",
			expectError:  true,
		},
		{
			name:         "Empty Session Token",
			sessionToken: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{
					Name:  "session_token",
					Value: tt.sessionToken,
				})
			}

			userID, err := ValidateSession(req)
			if tt.expectError {
				if err == nil {
					t.Error("ValidateSession() expected error, got nil")
				}
				if userID != nil {
					t.Error("ValidateSession() expected nil user, got value")
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSession() unexpected error: %v", err)
				}
				if userID == nil {
					t.Error("ValidateSession() got nil user, want value")
				} else if userID.UserID != int64(tt.expectedUser) {
					t.Errorf("ValidateSession() user = %v, want %v", userID.UserID, tt.expectedUser)
				}
			}
		})
	}
}
