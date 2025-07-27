package docs

import "time"

// Authentication request/response models for Swagger documentation

// RegisterRequest represents the registration request body
// swagger:parameters register
//
// # This is used for the Swagger documentation
//
// swagger:model RegisterRequest
type RegisterRequest struct {
	// Required: true
	Email string `json:"email" validate:"required,email"`
	// Required: true
	// Minimum length: 3
	// Maximum length: 50
	Username string `json:"username" validate:"required,min=3,max=50"`
	// Required: true
	// Minimum length: 8
	Password string `json:"password" validate:"required,min=8"`
	// Required: true
	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
	// Required: true
	// Minimum length: 1
	// Maximum length: 100
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	// Required: true
	// Minimum length: 1
	// Maximum length: 100
	LastName string `json:"last_name" validate:"required,min=1,max=100"`
	// Required: true
	// Enum: expert,evaluator,admin
	Role string `json:"role" validate:"required,oneof=expert evaluator admin"`
	// Required: true
	AcceptTerms bool `json:"accept_terms" validate:"required"`

	// Optional fields
	Affiliation      string `json:"affiliation,omitempty"`
	Bio              string `json:"bio,omitempty"`
	YearsExperience  int    `json:"years_experience,omitempty"`
	CoreCompetencies string `json:"core_competencies,omitempty"`
	Expertise        string `json:"expertise,omitempty"`
}

// LoginRequest represents the login request body
// swagger:parameters login
//
// swagger:model LoginRequest
type LoginRequest struct {
	// Required: true
	// Can be either email or username
	Login string `json:"login" validate:"required"`
	// Required: true
	Password string `json:"password" validate:"required"`
	// Whether to create a persistent session
	Remember bool `json:"remember,omitempty"`
}

// AuthResponse represents the authentication response
//
// swagger:response authResponse
type AuthResponse struct {
	// in: body
	Body struct {
		// The JWT access token
		AccessToken string `json:"access_token"`
		// The refresh token (for getting new access tokens)
		RefreshToken string `json:"refresh_token,omitempty"`
		// Token expiration in seconds
		ExpiresIn int `json:"expires_in"`
		// Type of token (usually "Bearer")
		TokenType string `json:"token_type"`
		// User information
		User struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			FirstName string `json:"first_name,omitempty"`
			LastName  string `json:"last_name,omitempty"`
			Role      string `json:"role"`
		} `json:"user"`
	} `json:"body"`
}

// ErrorResponse is defined in models.go
// swagger:response errorResponse
type errorResponseWrapper struct {
	// in: body
	Body ErrorResponse
}

// RefreshTokenRequest represents a refresh token request
// swagger:parameters refreshToken
type RefreshTokenRequest struct {
	// Required: true
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ForgotPasswordRequest represents a forgot password request
// swagger:parameters forgotPassword
type ForgotPasswordRequest struct {
	// Required: true
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest represents a password reset request
// swagger:parameters resetPassword
type ResetPasswordRequest struct {
	// Required: true
	Token string `json:"token" validate:"required"`
	// Required: true
	// Minimum length: 8
	NewPassword string `json:"new_password" validate:"required,min=8"`
	// Required: true
	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
}

// ChangePasswordRequest represents a password change request
// swagger:parameters changePassword
type ChangePasswordRequest struct {
	// Required: true
	CurrentPassword string `json:"current_password" validate:"required"`
	// Required: true
	// Minimum length: 8
	NewPassword string `json:"new_password" validate:"required,min=8"`
	// Required: true
	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
}

// VerifyEmailRequest represents an email verification request
// swagger:parameters verifyEmail
type VerifyEmailRequest struct {
	// Required: true
	Token string `json:"token" validate:"required"`
}

// OAuthLoginRequest represents an OAuth login request
// swagger:parameters oauthLogin
type OAuthLoginRequest struct {
	// Required: true
	// Enum: google,github
	Provider string `json:"provider" validate:"required,oneof=google github"`
	// Required: true
	AccessToken string `json:"access_token" validate:"required"`
	// Optional refresh token
	RefreshToken string `json:"refresh_token,omitempty"`
}

// SessionInfo represents an active session
//
// swagger:model SessionInfo
type SessionInfo struct {
	ID           string    `json:"id"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	LastActivity time.Time `json:"last_activity"`
	IsCurrent    bool      `json:"is_current"`
}

// SuccessResponse represents a generic success response
//
// swagger:response successResponse
type SuccessResponse struct {
	// in: body
	Body struct {
		// The success message
		Message string `json:"message"`
	} `json:"body"`
}
