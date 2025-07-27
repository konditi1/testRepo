// ===============================
// FILE: internal/handlers/api/v1/auth/auth_controller.go
// UPGRADED TO MATCH ENTERPRISE PATTERNS
// ===============================

package auth

import (
	"context"
	"encoding/json"
	"evalhub/internal/middleware"
	"evalhub/internal/response"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AuthController handles authentication API endpoints using existing services
type AuthController struct {
	serviceCollection *services.ServiceCollection
	logger            *zap.Logger
	responseBuilder   *response.Builder                
	paginationParser  *response.PaginationParser       
	paginationBuilder *response.PaginationBuilder      
}

// NewAuthController creates a new authentication controller with enterprise patterns
func NewAuthController(
	serviceCollection *services.ServiceCollection, 
	logger *zap.Logger,
	responseBuilder *response.Builder, 
) *AuthController {
	return &AuthController{
		serviceCollection: serviceCollection,
		logger:            logger,
		responseBuilder:   responseBuilder,
		paginationParser:  response.NewPaginationParser(response.DefaultPaginationConfig()),  // 🆕 ADDED
		paginationBuilder: response.NewPaginationBuilder(response.DefaultPaginationConfig()), // 🆕 ADDED
	}
}

// ===============================
// AUTHENTICATION ENDPOINTS
// ===============================

// Register handles user registration - POST /api/v1/auth/register
func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "register"))

	// Parse request using existing RegisterRequest from services
	var req services.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "register")
		return
	}

	// 🆕 ENHANCED: Structured validation
	if err := c.validateRegistrationRequest(&req); err != nil {
		logger.Warn("Registration validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "register")
		return
	}

	// Call existing auth service (no duplication!)
	authService := c.serviceCollection.GetAuthService()
	authResp, err := authService.Register(ctx, &req)
	if err != nil {
		logger.Error("Registration failed", zap.Error(err))
		c.handleServiceError(w, r, err, "register")
		return
	}

	logger.Info("User registered successfully",
		zap.Int64("user_id", authResp.User.ID),
		zap.String("username", authResp.User.Username),
	)

	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":  "User registered successfully",
		"user_id":  authResp.User.ID,
		"username": authResp.User.Username,
		"email":    authResp.User.Email,
	})
}

// Login handles user authentication - POST /api/v1/auth/login
func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "login"))

	// Parse request using existing LoginRequest from services
	var req services.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "login")
		return
	}

	// Structured validation
	if err := c.validateLoginRequest(&req); err != nil {
		logger.Warn("Login validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "login")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	authResp, err := authService.Login(ctx, &req)
	if err != nil {
		logger.Warn("Login failed", zap.Error(err), zap.String("login", req.Login))
		c.handleServiceError(w, r, err, "login")
		return
	}

	logger.Info("User logged in successfully",
		zap.Int64("user_id", authResp.User.ID),
		zap.String("username", authResp.User.Username),
		zap.Bool("remember", req.Remember),
	)

	// Set session cookie for backward compatibility with web handlers
	if authResp.AccessToken != "" {
		sessionTTL := 24 * time.Hour
		if req.Remember {
			sessionTTL = 30 * 24 * time.Hour
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    authResp.AccessToken,
			Expires:  time.Now().Add(sessionTTL),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   r.TLS != nil,
			Path:     "/",
		})
	}

	// Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":           "Login successful",
		"user":              authResp.User,
		"access_token":      authResp.AccessToken,
		"refresh_token":     authResp.RefreshToken,
		"expires_in":        authResp.ExpiresIn,
		"refresh_expires_in": authResp.RefreshExpiresIn,
		"token_type":        authResp.TokenType,
		"expires_at":        time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second).Unix(),
	})
}

// Logout handles user logout - POST /api/v1/auth/logout
func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "logout"))

	// Get session token from various sources
	sessionToken := c.getSessionToken(r)
	if sessionToken == "" {
		logger.Warn("No session token provided for logout")
		c.handleServiceError(w, r, services.NewValidationError("No session token provided", nil), "logout")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	logoutReq := &services.LogoutRequest{SessionToken: sessionToken}

	if err := authService.Logout(ctx, logoutReq); err != nil {
		logger.Error("Logout failed", zap.Error(err))
		c.handleServiceError(w, r, err, "logout")
		return
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		Path:     "/",
	})

	logger.Info("User logged out successfully")
	
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Logout successful"})
}

// LogoutAllDevices handles logout from all devices - POST /api/v1/auth/logout-all
func (c *AuthController) LogoutAllDevices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "logout_all"))

	// Get user from context
	user := middleware.GetUser(r.Context())
	if user == nil {
		logger.Warn("No user in context for logout all")
		c.handleServiceError(w, r, services.NewUnauthorizedError("Authentication required"), "logout_all")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.LogoutAllDevices(ctx, user.ID); err != nil {
		logger.Error("Logout all devices failed", zap.Error(err), zap.Int64("user_id", user.ID))
		c.handleServiceError(w, r, err, "logout_all")
		return
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		Path:     "/",
	})

	logger.Info("User logged out from all devices", zap.Int64("user_id", user.ID))
	
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Logged out from all devices"})
}

// RefreshToken handles token refresh - POST /api/v1/auth/refresh
func (c *AuthController) RefreshToken(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "refresh_token"))

	// Parse request using existing RefreshTokenRequest from services
	var req services.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "refresh_token")
		return
	}

	if err := c.validateRefreshTokenRequest(&req); err != nil {
		logger.Warn("Refresh token validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "refresh_token")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	authResp, err := authService.RefreshToken(ctx, &req)
	if err != nil {
		logger.Warn("Token refresh failed", zap.Error(err))
		c.handleServiceError(w, r, err, "refresh_token")
		return
	}

	logger.Info("Token refreshed successfully")
	
	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":      "Token refreshed successfully",
		"access_token": authResp.AccessToken,
		"expires_in":   int64(24 * time.Hour.Seconds()), // Default TTL
	})
}

// ===============================
// PASSWORD MANAGEMENT ENDPOINTS
// ===============================

// ForgotPassword handles password reset requests - POST /api/v1/auth/forgot-password
func (c *AuthController) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "forgot_password"))

	// Parse request using existing ForgotPasswordRequest from services
	var req services.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "forgot_password")
		return
	}

	if err := c.validateForgotPasswordRequest(&req); err != nil {
		logger.Warn("Forgot password validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "forgot_password")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.ForgotPassword(ctx, &req); err != nil {
		logger.Error("Forgot password failed", zap.Error(err))
		c.handleServiceError(w, r, err, "forgot_password")
		return
	}

	logger.Info("Password reset requested", zap.String("email", req.Email))

	// Always return success to prevent email enumeration (as per existing service)
	// 🆕 UPDATED: Using responseBuilder but maintaining security pattern
	response := c.responseBuilder.Success(r.Context(), "Password reset email sent. If the email exists, a password reset link has been sent")
	c.responseBuilder.WriteJSON(w, r, response, http.StatusOK)
}

// ResetPassword handles password reset with token - POST /api/v1/auth/reset-password
func (c *AuthController) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "reset_password"))

	// Parse request using ResetPasswordRequest from services
	var req services.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "reset_password")
		return
	}

	if err := c.validateResetPasswordRequest(&req); err != nil {
		logger.Warn("Reset password validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "reset_password")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.ResetPassword(ctx, &req); err != nil {
		logger.Error("Password reset failed", zap.Error(err))
		c.handleServiceError(w, r, err, "reset_password")
		return
	}

	logger.Info("Password reset successfully")
	
	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Password reset successful"})
}

// ChangePassword handles password change for authenticated users - POST /api/v1/auth/change-password
func (c *AuthController) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "change_password"))

	// Get user from context
	user := middleware.GetUser(r.Context())
	if user == nil {
		logger.Warn("No user in context for password change")
		c.handleServiceError(w, r, services.NewUnauthorizedError("Authentication required"), "change_password")
		return
	}

	// Parse request using ChangePasswordRequest from services
	var req services.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "change_password")
		return
	}

	// Set user ID from context
	req.UserID = user.ID

	if err := c.validateChangePasswordRequest(&req); err != nil {
		logger.Warn("Change password validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "change_password")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.ChangePassword(ctx, &req); err != nil {
		logger.Error("Password change failed", zap.Error(err), zap.Int64("user_id", user.ID))
		c.handleServiceError(w, r, err, "change_password")
		return
	}

	logger.Info("Password changed successfully", zap.Int64("user_id", user.ID))
	
	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Password changed successfully"})
}

// ===============================
// EMAIL VERIFICATION ENDPOINTS
// ===============================

// SendVerificationEmail sends email verification - POST /api/v1/auth/send-verification
func (c *AuthController) SendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "send_verification"))

	// Get user from context
	user := middleware.GetUser(r.Context())
	if user == nil {
		logger.Warn("No user in context for email verification")
		c.handleServiceError(w, r, services.NewUnauthorizedError("Authentication required"), "send_verification")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.SendVerificationEmail(ctx, user.ID); err != nil {
		logger.Error("Send verification email failed", zap.Error(err), zap.Int64("user_id", user.ID))
		c.handleServiceError(w, r, err, "send_verification")
		return
	}

	logger.Info("Verification email sent", zap.Int64("user_id", user.ID))
	
	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Verification email sent successfully"})
}

// VerifyEmail handles email verification with token - POST /api/v1/auth/verify-email
func (c *AuthController) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "verify_email"))

	// Parse request using existing VerifyEmailRequest from services
	var req services.VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "verify_email")
		return
	}

	if err := c.validateVerifyEmailRequest(&req); err != nil {
		logger.Warn("Verify email validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "verify_email")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.VerifyEmail(ctx, &req); err != nil {
		logger.Error("Email verification failed", zap.Error(err))
		c.handleServiceError(w, r, err, "verify_email")
		return
	}

	logger.Info("Email verified successfully")
	
	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Email verified successfully"})
}

// ===============================
// OAUTH ENDPOINTS
// ===============================

// OAuthLogin handles OAuth provider login - POST /api/v1/auth/oauth/login
func (c *AuthController) OAuthLogin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "oauth_login"))

	// Parse request using existing OAuthLoginRequest from services
	var req services.OAuthLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid request body", zap.Error(err))
		c.handleServiceError(w, r, services.NewValidationError("Invalid request body", err), "oauth_login")
		return
	}

	if err := c.validateOAuthLoginRequest(&req); err != nil {
		logger.Warn("OAuth login validation failed", zap.Error(err))
		c.handleServiceError(w, r, err, "oauth_login")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	authResp, err := authService.LoginWithProvider(ctx, &req)
	if err != nil {
		logger.Error("OAuth login failed", zap.Error(err), zap.String("provider", req.Provider))
		c.handleServiceError(w, r, err, "oauth_login")
		return
	}

	logger.Info("OAuth login successful",
		zap.Int64("user_id", authResp.User.ID),
		zap.String("provider", req.Provider),
	)

	// Set session cookie for backward compatibility
	if authResp.AccessToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    authResp.AccessToken,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   r.TLS != nil,
			Path:     "/",
		})
	}

	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":      "OAuth login successful",
		"user":         authResp.User,
		"access_token": authResp.AccessToken,
		"expires_in":   int64(24 * time.Hour.Seconds()), // Default TTL
	})
}

// ===============================
// SESSION MANAGEMENT ENDPOINTS
// ===============================

// GetSessions returns active sessions for the authenticated user - GET /api/v1/auth/sessions
func (c *AuthController) GetSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "get_sessions"))

	// Get user from context
	user := middleware.GetUser(r.Context())
	if user == nil {
		logger.Warn("No user in context for get sessions")
		c.handleServiceError(w, r, services.NewUnauthorizedError("Authentication required"), "get_sessions")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	sessions, err := authService.GetActiveSessions(ctx, user.ID)
	if err != nil {
		logger.Error("Get sessions failed", zap.Error(err), zap.Int64("user_id", user.ID))
		c.handleServiceError(w, r, err, "get_sessions")
		return
	}

	logger.Info("Sessions retrieved", zap.Int64("user_id", user.ID), zap.Int("session_count", len(sessions)))

	// 🆕 UPDATED: Consistent response building
	c.responseBuilder.WriteSuccess(w, r, map[string]interface{}{
		"message":  "Sessions retrieved",
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// RevokeSession revokes a specific session - DELETE /api/v1/auth/sessions/{session_id}
func (c *AuthController) RevokeSession(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	requestID := middleware.GetRequestID(r.Context())
	logger := c.logger.With(zap.String("request_id", requestID), zap.String("endpoint", "revoke_session"))

	// Get user from context
	user := middleware.GetUser(r.Context())
	if user == nil {
		logger.Warn("No user in context for revoke session")
		c.handleServiceError(w, r, services.NewUnauthorizedError("Authentication required"), "revoke_session")
		return
	}

	// 🆕 UPDATED: Extract session ID using standardized helper
	sessionID, err := c.extractIDFromPath(r.URL.Path, 5) // /api/v1/auth/sessions/{id}
	if err != nil {
		logger.Warn("Invalid session ID", zap.Error(err))
		validationErr := &services.ServiceError{
			Type:       "VALIDATION_ERROR",
			Message:    "Invalid session ID",
			StatusCode: http.StatusBadRequest,
		}
		c.handleServiceError(w, r, validationErr, "revoke_session")
		return
	}

	// Call existing auth service
	authService := c.serviceCollection.GetAuthService()
	if err := authService.RevokeSession(ctx, sessionID, user.ID); err != nil {
		logger.Error("Revoke session failed", zap.Error(err), zap.Int64("user_id", user.ID), zap.Int64("session_id", sessionID))
		c.handleServiceError(w, r, err, "revoke_session")
		return
	}

	logger.Info("Session revoked", zap.Int64("user_id", user.ID), zap.Int64("session_id", sessionID))
	
	c.responseBuilder.WriteSuccess(w, r, map[string]string{"message": "Session revoked successfully"})
}

// ===============================
// HELPER METHODS
// ===============================

// getSessionToken extracts session token from request (supports multiple sources)
func (c *AuthController) getSessionToken(r *http.Request) string {
	// Try Authorization header first
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Try cookie (for backward compatibility with web handlers)
	if cookie, err := r.Cookie("session_token"); err == nil {
		return cookie.Value
	}

	// Try form value
	return r.FormValue("session_token")
}

func (c *AuthController) extractIDFromPath(path string, position int) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) <= position {
		return 0, fmt.Errorf("missing ID in path")
	}
	
	id, err := strconv.ParseInt(parts[position], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ID format")
	}
	
	return id, nil
}

// handleServiceError converts service errors to proper HTTP responses using existing error system
func (c *AuthController) handleServiceError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	// Enhanced logging with operation context
	c.logger.Error("Auth service error",
		zap.Error(err),
		zap.String("operation", operation),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method),
	)

	// Use existing ServiceError system!
	serviceErr := services.GetServiceError(err)

	// Handle specific error types with appropriate headers
	switch serviceErr.Type {
	case "RATE_LIMIT":
		w.Header().Set("Retry-After", "3600") // 1 hour
		// 🆕 UPDATED: Use responseBuilder for consistency
		response := c.responseBuilder.Error(r.Context(), serviceErr)
		c.responseBuilder.WriteJSON(w, r, response, serviceErr.GetStatusCode())
	case "AUTHENTICATION_ERROR":
		if authErr, ok := err.(*services.AuthenticationError); ok {
			// 🆕 UPDATED: Use responseBuilder for consistency
			response := c.responseBuilder.Error(r.Context(), authErr)
			c.responseBuilder.WriteJSON(w, r, response, serviceErr.GetStatusCode())
		} else {
			// 🆕 UPDATED: Use responseBuilder for consistency
			response := c.responseBuilder.Error(r.Context(), serviceErr)
			c.responseBuilder.WriteJSON(w, r, response, serviceErr.GetStatusCode())
		}
	default:
		// For all other errors, use responseBuilder with the error object
		// 🆕 UPDATED: Use responseBuilder for consistency
		c.responseBuilder.WriteError(w, r, serviceErr)
	}
}

// ===============================
// 🆕 VALIDATION HELPER METHODS (Enterprise Enhancement)
// ===============================

// validateRegistrationRequest validates registration request structure
func (c *AuthController) validateRegistrationRequest(req *services.RegisterRequest) error {
	if strings.TrimSpace(req.Email) == "" {
		return services.NewValidationError("email is required", nil)
	}
	if strings.TrimSpace(req.Username) == "" {
		return services.NewValidationError("username is required", nil)
	}
	if strings.TrimSpace(req.Password) == "" {
		return services.NewValidationError("password is required", nil)
	}
	if req.Password != req.ConfirmPassword {
		return services.NewValidationError("passwords do not match", nil)
	}
	if strings.TrimSpace(req.FirstName) == "" {
		return services.NewValidationError("first name is required", nil)
	}
	if strings.TrimSpace(req.LastName) == "" {
		return services.NewValidationError("last name is required", nil)
	}
	if !req.AcceptTerms {
		return services.NewValidationError("must accept terms and conditions", nil)
	}
	return nil
}

// validateLoginRequest validates login request structure
func (c *AuthController) validateLoginRequest(req *services.LoginRequest) error {
	if strings.TrimSpace(req.Login) == "" {
		return services.NewValidationError("login is required", nil)
	}
	if strings.TrimSpace(req.Password) == "" {
		return services.NewValidationError("password is required", nil)
	}
	return nil
}

// validateRefreshTokenRequest validates refresh token request structure
func (c *AuthController) validateRefreshTokenRequest(req *services.RefreshTokenRequest) error {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return services.NewValidationError("refresh token is required", nil)
	}
	return nil
}

// validateForgotPasswordRequest validates forgot password request structure
func (c *AuthController) validateForgotPasswordRequest(req *services.ForgotPasswordRequest) error {
	if strings.TrimSpace(req.Email) == "" {
		return services.NewValidationError("email is required", nil)
	}
	return nil
}

// validateResetPasswordRequest validates reset password request structure
func (c *AuthController) validateResetPasswordRequest(req *services.ResetPasswordRequest) error {
	if strings.TrimSpace(req.Token) == "" {
		return services.NewValidationError("reset token is required", nil)
	}
	if strings.TrimSpace(req.NewPassword) == "" {
		return services.NewValidationError("new password is required", nil)
	}
	if req.NewPassword != req.ConfirmPassword {
		return services.NewValidationError("passwords do not match", nil)
	}
	return nil
}

// validateChangePasswordRequest validates change password request structure
func (c *AuthController) validateChangePasswordRequest(req *services.ChangePasswordRequest) error {
	if req.UserID <= 0 {
		return services.NewValidationError("user ID is required", nil)
	}
	if strings.TrimSpace(req.CurrentPassword) == "" {
		return services.NewValidationError("current password is required", nil)
	}
	if strings.TrimSpace(req.NewPassword) == "" {
		return services.NewValidationError("new password is required", nil)
	}
	if req.NewPassword != req.ConfirmPassword {
		return services.NewValidationError("passwords do not match", nil)
	}
	return nil
}

// validateVerifyEmailRequest validates verify email request structure
func (c *AuthController) validateVerifyEmailRequest(req *services.VerifyEmailRequest) error {
	if strings.TrimSpace(req.Token) == "" {
		return services.NewValidationError("verification token is required", nil)
	}
	return nil
}

// validateOAuthLoginRequest validates OAuth login request structure
func (c *AuthController) validateOAuthLoginRequest(req *services.OAuthLoginRequest) error {
	if strings.TrimSpace(req.Provider) == "" {
		return services.NewValidationError("OAuth provider is required", nil)
	}
	if strings.TrimSpace(req.AccessToken) == "" {
		return services.NewValidationError("access token is required", nil)
	}
	
	// Validate provider is supported
	supportedProviders := []string{"google", "github"}
	isSupported := false
	for _, provider := range supportedProviders {
		if req.Provider == provider {
			isSupported = true
			break
		}
	}
	if !isSupported {
		return services.NewValidationError("unsupported OAuth provider", nil)
	}
	
	return nil
}
