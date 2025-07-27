package events

import "time"

// TokenRefreshedEvent is emitted when a user's access token is refreshed
// using a refresh token
//
// This event is typically used for auditing and security monitoring purposes
// to track token refresh operations in the system.
type TokenRefreshedEvent struct {
	BaseEvent
	UserID     int64     `json:"user_id"`
	TokenID    string    `json:"token_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	ClientInfo string    `json:"client_info,omitempty"`
}

// NewTokenRefreshedEvent creates a new TokenRefreshedEvent
//
// Parameters:
// - userID: ID of the user whose token was refreshed
// - tokenID: The ID of the refresh token used
// - expiresAt: When the new access token expires
// - clientInfo: Optional information about the client (e.g., user agent, IP)
func NewTokenRefreshedEvent(userID int64, tokenID string, expiresAt time.Time, clientInfo string) *TokenRefreshedEvent {
	return &TokenRefreshedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "token.refreshed",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		UserID:     userID,
		TokenID:    tokenID,
		ExpiresAt:  expiresAt,
		ClientInfo: clientInfo,
	}
}
