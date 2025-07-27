// file: internal/handlers/web/google.go
package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
)

// GoogleLoginHandler handles the Google OAuth2 login process.
// It constructs the Google authorization URL with the necessary parameters
// and redirects the user to Google's OAuth2 authentication page.
//
// Parameters:
//   - w: http.ResponseWriter to write the HTTP response.
//   - r: *http.Request containing the HTTP request.
//
// The function uses the client ID and redirect URL from the utils package
// to build the authorization URL. It requests access to the user's email
// and profile information and specifies that the access type should be offline,
// which allows the application to refresh the access token when the user is not present.
func GoogleLoginHandler(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")

	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=email profile&access_type=offline",
		clientID,
		url.QueryEscape(redirectURL),
	)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GoogleCallbackHandler handles the callback from Google's OAuth2 authentication.
// It exchanges the authorization code for an access token, fetches the user's information,
// and either logs the user in or creates a new user account if it doesn't exist.
// It then creates a session for the user and sets a session cookie.
//
// Parameters:
//   - w: http.ResponseWriter to write the response
//   - r: *http.Request containing the request data
//
// The handler performs the following steps:
//  1. Retrieves the authorization code from the request URL.
//  2. Exchanges the authorization code for an access token.
//  3. Fetches the user's information using the access token.
//  4. Checks if the user exists in the database by email.
//  5. If the user does not exist, creates a new user account.
//  6. Generates a session token for the user.
//  7. Creates a session record in the database.
//  8. Sets a session cookie in the user's browser.
//  9. Redirects the user to the dashboard.
//
// If any step fails, it renders an error page with the appropriate status code and error message.
func GoogleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("authorization code not found"))
		return
	}
	token, err := exchangeCodeForToken(code)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to exchange token: %v", err))
		return
	}

	userInfo, err := fetchUserInfo(token)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to fetch uer info: %v", err))
		return
	}

	var userID int
	query := `SELECT id FROM users WHERE email = $1`
	err = database.DB.QueryRowContext(r.Context(), query, userInfo.Email).Scan(&userID)

	if err != nil {
		if err == sql.ErrNoRows {
			// User does not exist, create a new user with updated fields
			username := strings.Split(userInfo.Email, "@")[0]
			
			// Extract name parts if available
			firstName := ""
			lastName := ""
			if userInfo.Name != "" {
				nameParts := strings.Split(userInfo.Name, " ")
				if len(nameParts) > 0 {
					firstName = nameParts[0]
				}
				if len(nameParts) > 1 {
					lastName = strings.Join(nameParts[1:], " ")
				}
			}
			
			// Use profile picture from Google if available
			profileURL := ""
			profilePublicID := ""
			if userInfo.Picture != "" {
				profileURL = userInfo.Picture
			}
			
			query = `
				INSERT INTO users (
					email, 
					username, 
					first_name, 
					last_name, 
					profile_url, 
					profile_public_id, 
					role, 
					password
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`
			
			_, err := database.DB.ExecContext(r.Context(),
				query, 
				userInfo.Email, 
				username, 
				firstName, 
				lastName, 
				profileURL, 
				profilePublicID, 
				"user", 
				"_", // placeholder password for OAuth users
			)
			
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create user: %v", err))
				return
			}

			// Get the new user's ID
			err = database.DB.QueryRowContext(r.Context(), "SELECT id FROM users WHERE email = $1", userInfo.Email).Scan(&userID)
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to get user ID: %v", err))
				return
			}
		} else {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to query user: %v", err))
			return
		}
	}

	// Delete any existing sessions for this user
	_, err = database.DB.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		fmt.Printf("Failed to delete existing sessions: %v\n", err)
	}

	sessionToken, err := utils.GenerateSessionToken()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to generate session token: %v", err))
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	query = `INSERT INTO sessions (user_id, session_token, expires_at) VALUES ($1, $2, $3)`
	_, err = database.DB.ExecContext(r.Context(), query, userID, sessionToken, expiresAt)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create session: %v", err))
		return
	}

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

// exchangeCodeForToken exchanges an authorization code for an access token.
//
// Parameters:
//   - code: The authorization code received from the OAuth2 provider.
//
// Returns:
//   - string: The access token if the exchange is successful.
//   - error: An error if the token exchange fails, or if there is an issue with the HTTP request or response decoding.
func exchangeCodeForToken(code string) (string, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", os.Getenv("GOOGLE_CLIENT_ID"))
	data.Set("client_secret", os.Getenv("GOOGLE_CLIENT_SECRET"))
	data.Set("redirect_uri", os.Getenv("GOOGLE_REDIRECT_URL"))
	data.Set("grant_type", "authorization_code")


	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("failed to send token request: %v", err)
	}
	defer resp.Body.Close()

	var tokenResponse models.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode token response: %v", err)
	}

	return tokenResponse.AccessToken, nil
}

// fetchUserInfo retrieves user information from Google using the provided OAuth2 token.
// It sends a GET request to the Google OAuth2 userinfo endpoint and decodes the response
// into a UserInfo struct.
//
// Parameters:
//   - token: A string containing the OAuth2 token.
//
// Returns:
//   - A pointer to a UserInfo struct containing the user's information.
//   - An error if the request fails or the response cannot be decoded.
func fetchUserInfo(token string) (*models.UserInfo, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send user info request: %v", err)
	}
	defer resp.Body.Close()

	var userInfo models.UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info reponse: %v", err)
	}

	return &userInfo, nil
}