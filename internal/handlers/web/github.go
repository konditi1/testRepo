// file: internal/handlers/web/github.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"evalhub/internal/database"
	"evalhub/internal/utils"
	"fmt"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubUser represents a GitHub user
type GitHubUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	GitHubID string `json:"github_id,omitempty"`
}

var githubOauthConfig = &oauth2.Config{
	ClientID:     "Ov23lipGxLzIcNEwVlI9",
	ClientSecret: "ed44c55ccf80fee2c3306ea98bc2ccef7356377f",
	RedirectURL:  "http://localhost:8080/auth/github/callback",
	Scopes:       []string{"user:email", "read:user"},
	Endpoint:     github.Endpoint,
}

// GitHubLogin initiates the GitHub OAuth flow
func GitHubLogin(w http.ResponseWriter, r *http.Request) {
	// Add debug logging
	fmt.Printf("GitHub ClientID: %s\n", os.Getenv("GITHUB_CLIENT_ID"))
	if os.Getenv("GITHUB_CLIENT_ID") == "" || os.Getenv("GITHUB_CLIENT_SECRET") == "" {
		http.Error(w, "GitHub OAuth credentials not configured", http.StatusInternalServerError)
		return
	}

	url := githubOauthConfig.AuthCodeURL("state")
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the OAuth callback from GitHub
func GitHubCallback(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GitHub callback received")
	code := r.URL.Query().Get("code")
	fmt.Printf("Received code: %s\n", code)

	token, err := githubOauthConfig.Exchange(r.Context(), code)
	if err != nil {
		fmt.Printf("Token exchange error: %v\n", err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	client := githubOauthConfig.Client(r.Context(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		fmt.Printf("GitHub API error: %v\n", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var githubUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		fmt.Printf("JSON decode error: %v\n", err)
		http.Error(w, "Failed to decode user info", http.StatusInternalServerError)
		return
	}

	fmt.Printf("GitHub user info received: %+v\n", githubUser)

	// Check if user exists or create new user
	var userID int
	err = database.DB.QueryRowContext(context.Background(), `
		SELECT id FROM users WHERE github_id = $1`,
		githubUser.ID).Scan(&userID)

	if err == sql.ErrNoRows {
		fmt.Println("Creating new user from GitHub account")
		// Create new user - Updated with new field structure
		result, err := database.DB.ExecContext(r.Context(), `
			INSERT INTO users (
				username, 
				email, 
				github_id, 
				password, 
				first_name, 
				last_name, 
				profile_url, 
				profile_public_id, 
				role
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			githubUser.Login,
			githubUser.Email,
			githubUser.ID,
			"",     // empty password for OAuth users
			"",     // empty first name
			"",     // empty last name
			"",     // empty profile URL
			"",     // empty profile public ID
			"user", // default role
		)
		if err != nil {
			fmt.Printf("Database insert error: %v\n", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		lastID, err := result.LastInsertId()
		if err != nil {
			fmt.Printf("Failed to get user ID: %v\n", err)
			http.Error(w, "Failed to get user ID", http.StatusInternalServerError)
			return
		}
		userID = int(lastID)
	} else if err != nil {
		fmt.Printf("Database query error: %v\n", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Delete any existing sessions for this user
	_, err = database.DB.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		fmt.Printf("Failed to delete existing sessions: %v\n", err)
	}

	// Generate session token
	sessionToken, err := utils.GenerateSessionToken()
	if err != nil {
		fmt.Printf("Session token generation error: %v\n", err)
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}

	// Set session expiration
	expiresAt := time.Now().Add(24 * time.Hour)

	// Store session in database
	_, err = database.DB.ExecContext(r.Context(), `
		INSERT INTO sessions (user_id, session_token, expires_at) 
		VALUES ($1, $2, $3)`,
		userID, sessionToken, expiresAt,
	)
	if err != nil {
		fmt.Printf("Session creation error: %v\n", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Expires:  expiresAt,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil, // Set secure flag if request is HTTPS
	})

	fmt.Println("Authentication successful, redirecting to dashboard")
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}
