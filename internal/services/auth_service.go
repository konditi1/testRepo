// file: internal/services/auth_service.go
package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"evalhub/internal/cache"
	"evalhub/internal/events"
	"evalhub/internal/models"
	"evalhub/internal/repositories"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// authService implements AuthService with enterprise features
type authService struct {
	userRepo     repositories.UserRepository
	sessionRepo  repositories.SessionRepository
	cache        cache.Cache
	events       events.EventBus
	userService  UserService
	fileService  FileService
	emailService EmailService
	logger       *zap.Logger
	validate     *validator.Validate
	authConfig   *AuthConfig // Modified: Consolidated configuration
	mu           sync.Mutex  // Added: Mutex for thread safety
}

// Auth service configuration types
type (
	// LockoutConfig holds account lockout configuration
	LockoutConfig struct {
		MaxAttempts   int           `json:"max_attempts"`
		LockoutTime   time.Duration `json:"lockout_time"`
		WindowTime    time.Duration `json:"window_time"`
		EnableLockout bool          `json:"enable_lockout"`
	}

	// AuthConfig holds authentication service configuration
	AuthConfig struct {
		SessionTTL    time.Duration  `json:"session_ttl"`
		BCryptCost    int            `json:"bcrypt_cost"`
		MaxSessions   int            `json:"max_sessions"`
		LockoutConfig *LockoutConfig `json:"lockout_config"`
		// Added: Token settings for refresh tokens
		AccessTokenTTL   time.Duration `json:"access_token_ttl"`
		RefreshTokenTTL  time.Duration `json:"refresh_token_ttl"`
		MaxRefreshTokens int           `json:"max_refresh_tokens"`
		TokenRotation    bool          `json:"token_rotation"`
		ReuseDetection   bool          `json:"reuse_detection"`
		SecureTransport  bool          `json:"secure_transport"`
	}

	// RefreshTokenData represents stored refresh token metadata
	RefreshTokenData struct {
		UserID      int64      `json:"user_id"`
		TokenHash   string     `json:"token_hash"`
		DeviceID    string     `json:"device_id,omitempty"`
		DeviceInfo  string     `json:"device_info,omitempty"`
		IPAddress   string     `json:"ip_address,omitempty"`
		UserAgent   string     `json:"user_agent,omitempty"`
		ExpiresAt   time.Time  `json:"expires_at"`
		CreatedAt   time.Time  `json:"created_at"`
		LastUsed    time.Time  `json:"last_used"`
		IsRevoked   bool       `json:"is_revoked"`
		RevokedAt   *time.Time `json:"revoked_at,omitempty"`
		ParentToken string     `json:"parent_token,omitempty"`
	}
)

// DefaultAuthConfig returns default authentication configuration
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		SessionTTL:  24 * time.Hour,
		BCryptCost:  12,
		MaxSessions: 5,
		LockoutConfig: &LockoutConfig{
			MaxAttempts:   5,
			LockoutTime:   15 * time.Minute,
			WindowTime:    1 * time.Hour,
			EnableLockout: true,
		},
		// Added: Default token settings
		AccessTokenTTL:   72 * time.Minute,
		RefreshTokenTTL:  30 * 24 * time.Hour,
		MaxRefreshTokens: 10,
		TokenRotation:    true,
		ReuseDetection:   true,
		SecureTransport:  true,
	}
}

// NewAuthService creates a new enterprise authentication service
func NewAuthService(
	userRepo repositories.UserRepository,
	sessionRepo repositories.SessionRepository,
	cache cache.Cache,
	events events.EventBus,
	userService UserService,
	fileService FileService,
	emailService EmailService,
	logger *zap.Logger,
	config *AuthConfig,
) AuthService {
	validate := validator.New()
	if config == nil {
		config = DefaultAuthConfig()
	}

	return &authService{
		userRepo:     userRepo,
		sessionRepo:  sessionRepo,
		cache:        cache,
		events:       events,
		userService:  userService,
		fileService:  fileService,
		emailService: emailService,
		logger:       logger,
		validate:     validate,
		authConfig:   config,
	}
}

// ===============================
// AUTHENTICATION
// ===============================

// Register creates a new user account
func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	// Step 1: Validate request structure
	if err := s.validateRegisterRequest(req); err != nil {
		return nil, err
	}

	// Step 2: Check for rate limiting
	if err := s.checkRegistrationRateLimit(ctx, req.Email); err != nil {
		return nil, err
	}

	// Step 3: Enhanced business rule validation
	if err := s.validateBusinessRules(ctx, req); err != nil {
		return nil, err
	}

	// Step 4: Process file uploads BEFORE creating user
	var profileURL, profilePublicID, cvURL, cvPublicID string
	if req.ProfileImage != nil && s.fileService != nil {
		fileReq, ok := req.ProfileImage.(*FileUploadRequest)
		if !ok {
			s.logger.Error("Invalid profile image format")
			return nil, NewValidationError("invalid profile image format", nil)
		}

		result, err := s.fileService.UploadImage(ctx, fileReq)
		if err != nil {
			s.logger.Error("Failed to upload profile image", zap.Error(err))
			return nil, NewInternalError("failed to upload profile image")
		}

		profileURL = result.URL
		profilePublicID = result.PublicID
		s.logger.Info("Profile image uploaded successfully",
			zap.String("url", profileURL),
			zap.String("public_id", profilePublicID))
	}

	if req.CVDocument != nil && s.fileService != nil {
		docReq, ok := req.CVDocument.(*FileUploadRequest)
		if !ok {
			s.logger.Error("Invalid CV document format")
			return nil, NewValidationError("invalid CV document format", nil)
		}

		result, err := s.fileService.UploadDocument(ctx, docReq)
		if err != nil {
			s.logger.Error("Failed to upload CV document", zap.Error(err))
			if profilePublicID != "" {
				s.cleanupUploadedFiles(ctx, profilePublicID, "")
			}
			return nil, NewInternalError("failed to upload CV document")
		}
		cvURL = result.URL
		cvPublicID = result.PublicID
		s.logger.Info("CV document uploaded successfully",
			zap.String("url", cvURL),
			zap.String("public_id", cvPublicID))
	}

	// Step 5: Create user through user service with enhanced fields
	createUserReq := &CreateUserRequest{
		Email:            req.Email,
		Username:         req.Username,
		Password:         req.Password,
		FirstName:        getStringPtr(req.FirstName),
		LastName:         getStringPtr(req.LastName),
		AcceptTerms:      req.AcceptTerms,
		MarketingEmails:  false,
		Role:             getStringPtr(req.Role),
		Affiliation:      getStringPtr(req.Affiliation),
		Bio:              getStringPtr(req.Bio),
		YearsExperience:  getInt16Ptr(req.YearsExperience),
		CoreCompetencies: getStringPtr(req.CoreCompetencies),
		Expertise:        getStringPtr(req.Expertise),
		ProfileURL:       getStringPtr(profileURL),
		ProfilePublicID:  getStringPtr(profilePublicID),
		CVURL:            getStringPtr(cvURL),
		CVPublicID:       getStringPtr(cvPublicID),
	}

	user, err := s.userService.CreateUser(ctx, createUserReq)
	if err != nil {
		s.cleanupUploadedFiles(ctx, profilePublicID, cvPublicID)
		s.logger.Error("Failed to create user during registration",
			zap.Error(err),
			zap.String("email", req.Email),
			zap.String("username", req.Username),
		)
		return nil, err
	}

	// Step 6: Create initial session
	sessionToken, err := s.generateSessionToken()
	if err != nil {
		return nil, NewInternalError("failed to generate session token")
	}

	session := &models.Session{
		UserID:       user.ID,
		SessionToken: sessionToken,
		ExpiresAt:    time.Now().Add(s.authConfig.SessionTTL),
		CreatedAt:    time.Now(),
		IsActive:     true,
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		s.logger.Error("Failed to create session during registration",
			zap.Error(err),
			zap.Int64("user_id", user.ID),
		)
		return nil, NewInternalError("failed to create session")
	}

	// Step 7: Set user online status
	if err := s.setUserOnlineStatus(ctx, user.ID, true); err != nil {
		s.logger.Warn("Failed to set user online status", zap.Error(err))
	}

	// Step 8: Send verification email (async)
	go func() {
		if err := s.SendVerificationEmail(context.Background(), user.ID); err != nil {
			s.logger.Error("Failed to send verification email",
				zap.Error(err),
				zap.Int64("user_id", user.ID))
		}
	}()

	// Step 9: Publish registration event
	if err := s.events.Publish(ctx, events.NewUserCreatedEvent(user.ID, user.Email, user.Username)); err != nil {
		s.logger.Warn("Failed to publish user created event", zap.Error(err))
	}

	s.logger.Info("User registered successfully",
		zap.Int64("user_id", user.ID),
		zap.String("email", req.Email),
		zap.String("username", req.Username),
	)

	return &AuthResponse{
		User:        user,
		AccessToken: sessionToken,
		ExpiresIn:   int64(s.authConfig.SessionTTL.Seconds()),
		TokenType:   "Bearer",
	}, nil
}

// Login authenticates a user and creates a session with refresh token
func (s *authService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Added: Enforce secure transport
	if err := s.validateSecureTransport(ctx); err != nil {
		return nil, err
	}

	// Step 1: Validate request
	if err := s.validateLoginRequest(req); err != nil {
		return nil, err
	}

	// Step 2: Check account lockout
	if err := s.checkAccountLockout(ctx, req.Login); err != nil {
		return nil, err
	}

	// Step 3: Find user by email or username
	var user *models.User
	var err error
	if strings.Contains(req.Login, "@") {
		user, err = s.userRepo.GetByEmail(ctx, req.Login)
	} else {
		user, err = s.userRepo.GetByUsername(ctx, req.Login)
	}
	if err != nil {
		s.logger.Error("Failed to get user during login", zap.Error(err), zap.String("login", req.Login))
		return nil, NewInternalError("authentication failed")
	}
	if user == nil {
		s.recordFailedAttempt(ctx, req.Login, "user_not_found")
		return nil, NewAuthenticationError("invalid credentials", "invalid_login", nil, req.Login)
	}

	// Step 4: Check user status
	if !user.IsActive {
		s.recordFailedAttempt(ctx, req.Login, "account_deactivated")
		return nil, NewAuthenticationError("account is deactivated", "account_deactivated", &user.ID, user.Username)
	}

	// Step 5: Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.recordFailedAttempt(ctx, req.Login, "invalid_password")
		s.logger.Warn("Invalid password attempt",
			zap.Int64("user_id", user.ID),
			zap.String("username", user.Username),
			zap.String("ip_address", req.IPAddress),
		)
		return nil, NewAuthenticationError("invalid credentials", "invalid_password", &user.ID, user.Username)
	}

	// Step 6: Clear failed attempts
	s.clearFailedAttempts(ctx, req.Login)

	// Step 7: Manage sessions
	if err := s.manageUserSessions(ctx, user.ID); err != nil {
		s.logger.Warn("Failed to manage user sessions", zap.Error(err), zap.Int64("user_id", user.ID))
	}

	// Step 8: Generate tokens
	accessToken, err := s.generateAccessToken(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to generate access token", zap.Error(err))
		return nil, NewInternalError("failed to generate access token")
	}

	// Added: Generate and store refresh token
	refreshToken, err := s.generateRefreshToken(ctx, user.ID, req)
	if err != nil {
		s.logger.Error("Failed to generate refresh token", zap.Error(err))
		return nil, NewInternalError("failed to generate refresh token")
	}
	if err := s.storeRefreshToken(ctx, refreshToken, user.ID, req); err != nil {
		s.logger.Error("Failed to store refresh token", zap.Error(err))
		return nil, NewInternalError("failed to store refresh token")
	}

	// Added: Cleanup expired tokens
	go s.cleanupExpiredTokens(context.Background(), user.ID)

	// Step 9: Update status and last login
	if err := s.setUserOnlineStatus(ctx, user.ID, true); err != nil {
		s.logger.Warn("Failed to set user online status", zap.Error(err))
	}
	if err := s.userService.UpdateOnlineStatus(ctx, user.ID, true); err != nil {
		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", user.ID))
	}
	if err := s.updateLastLogin(ctx, user.ID, req.IPAddress); err != nil {
		s.logger.Warn("Failed to update last login", zap.Error(err))
	}

	// Step 10: Publish login event
	if err := s.events.Publish(ctx, &events.UserLoggedInEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "user.logged_in",
			Timestamp: time.Now(),
			UserID:    &user.ID,
		},
		LoginAt:   time.Now(),
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
	}); err != nil {
		s.logger.Warn("Failed to publish login event", zap.Error(err))
	}

	s.logger.Info("User logged in successfully",
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
		zap.String("ip_address", req.IPAddress),
		zap.Bool("remember", req.Remember),
	)

	// Clear sensitive data
	user.PasswordHash = ""

	return &AuthResponse{
		User:             user,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int64(s.authConfig.AccessTokenTTL.Seconds()),
		RefreshExpiresIn: int64(s.authConfig.RefreshTokenTTL.Seconds()),
		TokenType:        "Bearer",
	}, nil
}

// LoginWithProvider handles OAuth provider login
func (s *authService) LoginWithProvider(ctx context.Context, req *OAuthLoginRequest) (*AuthResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return nil, NewValidationError("invalid OAuth login request", err)
	}

	// This would integrate with OAuth providers (Google, GitHub, etc.)
	// For now, return a not implemented error
	return nil, NewNotImplementedError("OAuth login not implemented")
}

// RefreshToken refreshes an access token
func (s *authService) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error) {
	// Added: Enforce secure transport
	if err := s.validateSecureTransport(ctx); err != nil {
		return nil, err
	}

	// Step 1: Validate request
	if err := s.validate.Struct(req); err != nil {
		return nil, NewValidationError("invalid refresh token request", err)
	}

	// Added: Validate token format
	if !s.isValidTokenFormat(req.RefreshToken) {
		s.logger.Warn("Invalid refresh token format", zap.String("token", req.RefreshToken[:12]))
		return nil, NewAuthenticationError("invalid refresh token format", "invalid_token", nil, "")
	}

	// Added: Retrieve token data
	tokenData, err := s.getRefreshTokenData(ctx, req.RefreshToken)
	if err != nil {
		s.logger.Warn("Invalid refresh token", zap.Error(err))
		return nil, NewAuthenticationError("invalid refresh token", "invalid_token", nil, "")
	}

	// Added: Check expiration
	if time.Now().After(tokenData.ExpiresAt) {
		s.logger.Warn("Expired refresh token used",
			zap.Int64("user_id", tokenData.UserID),
			zap.Time("expired_at", tokenData.ExpiresAt),
		)
		s.revokeRefreshToken(ctx, req.RefreshToken)
		return nil, NewAuthenticationError("refresh token expired", "token_expired", nil, "")
	}

	// Added: Check revocation
	if tokenData.IsRevoked {
		s.logger.Warn("Revoked refresh token used", zap.Int64("user_id", tokenData.UserID))
		return nil, NewAuthenticationError("refresh token revoked", "token_revoked", nil, "")
	}

	// Added: Detect token reuse
	if s.authConfig.ReuseDetection {
		if err := s.detectTokenReuse(ctx, tokenData, req); err != nil {
			return nil, err
		}
	}

	// Step 2: Get user
	user, err := s.userRepo.GetByID(ctx, tokenData.UserID)
	if err != nil || user == nil {
		s.logger.Error("User not found for refresh token",
			zap.Error(err),
			zap.Int64("user_id", tokenData.UserID),
		)
		return nil, NewAuthenticationError("user not found", "user_not_found", nil, "")
	}

	// Added: Validate user status
	if !user.IsActive {
		return nil, NewAuthenticationError("account is deactivated", "account_deactivated", &user.ID, user.Username)
	}

	// Step 3: Generate new access token
	accessToken, err := s.generateAccessToken(ctx, user.ID)
	if err != nil {
		s.logger.Error("Failed to generate new access token", zap.Error(err))
		return nil, NewInternalError("token generation failed")
	}

	var newRefreshToken string
	// Added: Token rotation
	if s.authConfig.TokenRotation {
		newRefreshToken, err = s.generateRefreshToken(ctx, user.ID, &LoginRequest{
			IPAddress: req.IPAddress,
			UserAgent: req.UserAgent,
		})
		if err != nil {
			s.logger.Error("Failed to generate new refresh token", zap.Error(err))
			return nil, NewInternalError("token generation failed")
		}

		if err := s.storeRefreshTokenWithParent(ctx, newRefreshToken, tokenData, req); err != nil {
			s.logger.Error("Failed to store new refresh token", zap.Error(err))
			return nil, NewInternalError("token storage failed")
		}

		if err := s.revokeRefreshToken(ctx, req.RefreshToken); err != nil {
			s.logger.Warn("Failed to revoke old refresh token", zap.Error(err))
		}
	} else {
		newRefreshToken = req.RefreshToken
		if err := s.updateRefreshTokenUsage(ctx, req.RefreshToken); err != nil {
			s.logger.Warn("Failed to update token usage", zap.Error(err))
		}
	}

	// Added: Publish refresh event
	event := events.NewTokenRefreshedEvent(
		user.ID,
		"", // Token ID is not available in this context
		time.Now().Add(s.authConfig.AccessTokenTTL),
		fmt.Sprintf("IP: %s, User-Agent: %s, Rotated: %v", req.IPAddress, req.UserAgent, s.authConfig.TokenRotation),
	)

	if err := s.events.Publish(ctx, event); err != nil {
		s.logger.Warn("Failed to publish token refreshed event", zap.Error(err))
	}

	s.logger.Info("Token refreshed successfully",
		zap.Int64("user_id", user.ID),
		zap.Bool("rotated", s.authConfig.TokenRotation),
	)

	user.PasswordHash = ""

	return &AuthResponse{
		User:             user,
		AccessToken:      accessToken,
		RefreshToken:     newRefreshToken,
		ExpiresIn:        int64(s.authConfig.AccessTokenTTL.Seconds()),
		RefreshExpiresIn: int64(s.authConfig.RefreshTokenTTL.Seconds()),
		TokenType:        "Bearer",
	}, nil
}

// Logout invalidates a session and refresh token
func (s *authService) Logout(ctx context.Context, req *LogoutRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid logout request", err)
	}

	session, err := s.sessionRepo.GetByToken(ctx, req.SessionToken)
	if err != nil {
		s.logger.Warn("Failed to get session during logout", zap.Error(err))
	}

	var userID int64
	if session != nil {
		userID = session.UserID
		if req.LogoutAll {
			if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
				s.logger.Error("Failed to delete all user sessions", zap.Error(err))
				return NewInternalError("failed to logout from all devices")
			}
			// Added: Revoke all refresh tokens
			if err := s.revokeAllRefreshTokens(ctx, userID); err != nil {
				s.logger.Warn("Failed to revoke all refresh tokens", zap.Error(err))
			}
			s.logger.Info("User logged out from all devices", zap.Int64("user_id", userID))
		} else {
			if err := s.sessionRepo.Delete(ctx, req.SessionToken); err != nil {
				s.logger.Error("Failed to delete session during logout", zap.Error(err))
				return NewInternalError("failed to logout")
			}
			// Added: Revoke associated refresh token
			if err := s.revokeRefreshToken(ctx, req.SessionToken); err != nil {
				s.logger.Warn("Failed to revoke refresh token", zap.Error(err))
			}
			s.logger.Info("User logged out", zap.Int64("user_id", userID))
		}

		if err := s.setUserOnlineStatus(ctx, userID, false); err != nil {
			s.logger.Warn("Failed to update online status during logout", zap.Error(err))
		}
		if err := s.userService.UpdateOnlineStatus(ctx, userID, false); err != nil {
			s.logger.Warn("Failed to update online status", zap.Error(err))
		}

		if err := s.events.Publish(ctx, &events.UserLoggedOutEvent{
			BaseEvent: events.BaseEvent{
				EventID:   events.GenerateEventID(),
				EventType: "user.logged_out",
				Timestamp: time.Now(),
				UserID:    &userID,
			},
			LogoutAt: time.Now(),
		}); err != nil {
			s.logger.Warn("Failed to publish logout event", zap.Error(err))
		}
	} else {
		if err := s.sessionRepo.Delete(ctx, req.SessionToken); err != nil {
			s.logger.Error("Failed to delete session during logout", zap.Error(err))
			return NewInternalError("failed to logout")
		}
		// Added: Attempt to revoke refresh token
		if err := s.revokeRefreshToken(ctx, req.SessionToken); err != nil {
			s.logger.Warn("Failed to revoke refresh token", zap.Error(err))
		}
	}

	return nil
}

// LogoutAllDevices invalidates all sessions and tokens
func (s *authService) LogoutAllDevices(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return NewValidationError("invalid user ID", nil)
	}

	// Delete all sessions for the user
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		s.logger.Error("Failed to delete all sessions", zap.Error(err), zap.Int64("user_id", userID))
		return NewInternalError("failed to logout from all devices")
	}

	// Added: Revoke all refresh tokens
	if err := s.revokeAllRefreshTokens(ctx, userID); err != nil {
		s.logger.Warn("Failed to revoke all refresh tokens", zap.Error(err))
	}

	// Update user online status
	if err := s.setUserOnlineStatus(ctx, userID, false); err != nil {
		s.logger.Warn("Failed to update online status", zap.Error(err))
	}

	// Update user online status in user service
	if err := s.userService.UpdateOnlineStatus(ctx, userID, false); err != nil {
		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", userID))
	}

	s.logger.Info("User logged out from all devices", zap.Int64("user_id", userID))
	return nil
}

// ===============================
// PASSWORD MANAGEMENT
// ===============================

// ForgotPassword initiates a password reset
func (s *authService) ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid forgot password request", err)
	}

	if err := s.checkPasswordResetRateLimit(ctx, req.Email); err != nil {
		return err
	}

	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		s.logger.Error("Failed to get user for password reset", zap.Error(err))
		return NewInternalError("failed to process password reset")
	}

	if user == nil {
		s.logger.Info("Password reset requested for non-existent email", zap.String("email", req.Email))
		return nil
	}

	resetToken, err := s.generateResetToken()
	if err != nil {
		return NewInternalError("failed to generate reset token")
	}

	resetKey := fmt.Sprintf("password_reset:%s", resetToken)
	if err := s.cache.Set(ctx, resetKey, user.ID, 1*time.Hour); err != nil {
		s.logger.Error("Failed to store reset token", zap.Error(err))
		return NewInternalError("failed to process password reset")
	}

	if s.emailService != nil {
		go func() {
			if err := s.emailService.SendPasswordResetEmail(context.Background(), user.Email, resetToken); err != nil {
				s.logger.Error("Failed to send password reset email",
					zap.Error(err),
					zap.String("email", user.Email))
			}
		}()
	}

	s.logger.Info("Password reset token generated",
		zap.Int64("user_id", user.ID),
		zap.String("email", user.Email),
	)
	return nil
}

// ResetPassword resets a user's password
func (s *authService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid reset password request", err)
	}
	if req.NewPassword != req.ConfirmPassword {
		return NewValidationError("passwords do not match", nil)
	}

	resetKey := fmt.Sprintf("password_reset:%s", req.Token)
	userIDInterface, found := s.cache.Get(ctx, resetKey)
	if !found {
		return NewValidationError("invalid or expired reset token", nil)
	}

	userID, ok := userIDInterface.(int64)
	if !ok {
		s.logger.Error("Invalid user ID type in reset token cache")
		return NewInternalError("invalid reset token")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user for password reset", zap.Error(err))
		return NewInternalError("failed to reset password")
	}
	if user == nil {
		return NewNotFoundError("user not found")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.authConfig.BCryptCost)
	if err != nil {
		s.logger.Error("Failed to hash new password", zap.Error(err))
		return NewInternalError("failed to reset password")
	}

	// Update user password
	user.PasswordHash = string(hashedPassword)
	user.PasswordChangedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", userID))
		return NewInternalError("failed to reset password")
	}

	// Delete reset token from cache
	s.cache.Delete(ctx, resetKey)

	// Invalidate sessions and tokens
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		s.logger.Warn("Failed to invalidate sessions after password reset", zap.Error(err))
	}
	// Revoke all refresh tokens
	if err := s.revokeAllRefreshTokens(ctx, userID); err != nil {
		s.logger.Warn("Failed to revoke all refresh tokens", zap.Error(err))
	}

	// Publish password changed event
	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "user.password_changed",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		ChangedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
	}

	s.logger.Info("Password reset successfully", zap.Int64("user_id", userID))
	return nil
}

// ChangePassword updates a user's password
func (s *authService) ChangePassword(ctx context.Context, req *ChangePasswordRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid change password request", err)
	}
	if req.NewPassword != req.ConfirmPassword {
		return NewValidationError("passwords do not match", nil)
	}

	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		s.logger.Error("Failed to get user for password change", zap.Error(err))
		return NewInternalError("failed to change password")
	}
	if user == nil {
		return NewNotFoundError("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return NewValidationError("current password is incorrect", nil)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.authConfig.BCryptCost)
	if err != nil {
		s.logger.Error("Failed to hash new password", zap.Error(err))
		return NewInternalError("failed to change password")
	}

	user.PasswordHash = string(hashedPassword)
	user.PasswordChangedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", req.UserID))
		return NewInternalError("failed to change password")
	}

	// Added: Revoke all refresh tokens
	if err := s.revokeAllRefreshTokens(ctx, req.UserID); err != nil {
		s.logger.Warn("Failed to revoke all refresh tokens", zap.Error(err))
	}

	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "user.password_changed",
			Timestamp: time.Now(),
			UserID:    &req.UserID,
		},
		ChangedAt: time.Now(),
	}); err != nil {
		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
	}

	s.logger.Info("Password changed successfully", zap.Int64("user_id", req.UserID))
	return nil
}


// ===============================
// EMAIL VERIFICATION
// ===============================

// SendVerificationEmail sends an email verification link
func (s *authService) SendVerificationEmail(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return NewValidationError("invalid user ID", nil)
	}

	// Check if user exists
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return NewInternalError("failed to get user")
	}
	if user == nil {
		return NewNotFoundError("user not found")
	}
	if user.EmailVerified {
		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
	}

	// Generate verification token
	verificationToken, err := s.generateVerificationToken()
	if err != nil {
		return NewInternalError("failed to generate verification token")
	}

	// Store verification token in cache
	verificationKey := fmt.Sprintf("email_verification:%s", verificationToken)
	if err := s.cache.Set(ctx, verificationKey, userID, 24*time.Hour); err != nil {
		s.logger.Error("Failed to store verification token", zap.Error(err))
		return NewInternalError("failed to send verification email")
	}

	// Send verification email using email service
	if s.emailService != nil {
		go func() {
			if err := s.emailService.SendVerificationEmail(context.Background(), user.Email, verificationToken); err != nil {
				s.logger.Error("Failed to send verification email",
					zap.Error(err),
					zap.String("email", user.Email))
			}
		}()
	}

	s.logger.Info("Email verification token generated",
		zap.Int64("user_id", userID),
		zap.String("email", user.Email),
	)
	return nil
}

// VerifyEmail verifies a user's email
func (s *authService) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid email verification request", err)
	}

	// Validate verification token
	verificationKey := fmt.Sprintf("email_verification:%s", req.Token)
	userIDInterface, found := s.cache.Get(ctx, verificationKey)
	if !found {
		return NewValidationError("invalid or expired verification token", nil)
	}

	// Validate user ID
	userID, ok := userIDInterface.(int64)
	if !ok {
		return NewInternalError("invalid verification token")
	}

	// Get user by ID
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return NewInternalError("failed to verify email")
	}
	if user == nil {
		return NewNotFoundError("user not found")
	}

	// Check if email is already verified
	if user.EmailVerified {
		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
	}

	// Update email verification status
	now := time.Now()
	user.EmailVerified = true
	user.EmailVerifiedAt = &now
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update email verification status", zap.Error(err), zap.Int64("user_id", userID))
		return NewInternalError("failed to verify email")
	}

	// Delete verification token from cache
	s.cache.Delete(ctx, verificationKey)

	s.logger.Info("Email verified successfully", zap.Int64("user_id", userID))
	return nil
}

// ===============================
// SESSION MANAGEMENT
// ===============================

// GetActiveSessions retrieves active sessions
func (s *authService) GetActiveSessions(ctx context.Context, userID int64) ([]*SessionInfo, error) {
	if userID <= 0 {
		return nil, NewValidationError("invalid user ID", nil)
	}

	// Get active sessions
	sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, false)
	if err != nil {
		s.logger.Error("Failed to get active sessions", zap.Error(err), zap.Int64("user_id", userID))
		return nil, NewInternalError("failed to retrieve sessions")
	}

	var sessionInfos []*SessionInfo
	for _, session := range sessions {
		// Added: Retrieve device info from refresh token
		var deviceInfo string
		cacheKey := s.getRefreshTokenCacheKey(session.SessionToken)
		if tokenData, exists := s.cache.Get(ctx, cacheKey); exists {
			if refreshData, ok := tokenData.(*RefreshTokenData); ok {
				deviceInfo = refreshData.DeviceInfo
			}
		}

		sessionInfos = append(sessionInfos, &SessionInfo{
			ID:           session.ID,
			Token:        session.SessionToken,
			IPAddress:    deviceInfo,
			ExpiresAt:    session.ExpiresAt,
			LastActivity: session.LastActivity,
			// Device, Browser, OS, and Location could be extracted from deviceInfo if needed
		})
	}

	return sessionInfos, nil
}

// RevokeSession revokes a specific session
func (s *authService) RevokeSession(ctx context.Context, sessionID int64, userID int64) error {
	if sessionID <= 0 || userID <= 0 {
		return NewValidationError("invalid session or user ID", nil)
	}
	// TODO: Implement session revocation by ID
	// This would require adding a method to get session by ID and verify ownership
	return NewNotImplementedError("session revocation by ID not implemented")
}

// ===============================
// TWO-FACTOR AUTHENTICATION (Placeholder)
// ===============================

// EnableTwoFactor enables two-factor authentication
func (s *authService) EnableTwoFactor(ctx context.Context, userID int64) (*TwoFactorSetupResponse, error) {
	return nil, NewNotImplementedError("two-factor authentication not implemented")
}

// DisableTwoFactor disables two-factor authentication
func (s *authService) DisableTwoFactor(ctx context.Context, req *DisableTwoFactorRequest) error {
	return NewNotImplementedError("two-factor authentication not implemented")
}

// VerifyTwoFactor verifies a two-factor authentication code
func (s *authService) VerifyTwoFactor(ctx context.Context, req *VerifyTwoFactorRequest) error {
	return NewNotImplementedError("two-factor authentication not implemented")
}

// ===============================
// ENHANCED VALIDATION METHODS
// ===============================

// validateRegisterRequest validates registration request structure
func (s *authService) validateRegisterRequest(req *RegisterRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid registration request", err)
	}

	// Additional validation
	if req.Password != req.ConfirmPassword {
		return NewValidationError("passwords do not match", nil)
	}

	if !req.AcceptTerms {
		return NewValidationError("must accept terms and conditions", nil)
	}

	return nil
}

// validateLoginRequest validates login request structure
func (s *authService) validateLoginRequest(req *LoginRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return NewValidationError("invalid login request", err)
	}

	return nil
}

// validateBusinessRules validates business-specific rules during registration
func (s *authService) validateBusinessRules(ctx context.Context, req *RegisterRequest) error {
	// Check if email exists
	if user, _ := s.userRepo.GetByEmail(ctx, req.Email); user != nil {
		return NewBusinessError("email already exists", "EMAIL_EXISTS")
	}

	// Check if username exists
	if user, _ := s.userRepo.GetByUsername(ctx, req.Username); user != nil {
		return NewBusinessError("username already exists", "USERNAME_EXISTS")
	}

	// Enhanced password validation would go here
	// This could integrate with a password strength service

	return nil
}

// ===============================
// ENHANCED HELPER METHODS
// ===============================

// Added: generateRefreshToken creates a secure refresh token
func (s *authService) generateRefreshToken(ctx context.Context, userID int64, req *LoginRequest) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	token := fmt.Sprintf("rt_%s_%s",
		hex.EncodeToString(tokenBytes[:4]),
		base64.URLEncoding.EncodeToString(tokenBytes),
	)
	return token, nil
}

// Added: generateAccessToken creates a session-based access token
func (s *authService) generateAccessToken(ctx context.Context, userID int64) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate access token: %w", err)
	}

	token := base64.URLEncoding.EncodeToString(tokenBytes)

	session := &models.Session{
		UserID:    userID,
		SessionToken:     token,
		ExpiresAt: time.Now().Add(s.authConfig.AccessTokenTTL),
		CreatedAt: time.Now(),
		IsActive:  true,
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}

	return token, nil
}

// Added: storeRefreshToken stores refresh token securely
func (s *authService) storeRefreshToken(ctx context.Context, token string, userID int64, req *LoginRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Modified: Use SHA-256 for token hashing
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Safely handle pointer fields
	var deviceID, deviceInfo string
	if req.DeviceID != nil {
		deviceID = *req.DeviceID
	}
	if req.DeviceInfo != nil {
		deviceInfo = *req.DeviceInfo
	}

	tokenData := &RefreshTokenData{
		UserID:     userID,
		TokenHash:  tokenHash,
		DeviceID:   deviceID,
		DeviceInfo: deviceInfo,
		IPAddress:  req.IPAddress,
		UserAgent:  req.UserAgent,
		ExpiresAt:  time.Now().Add(s.authConfig.RefreshTokenTTL),
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
		IsRevoked:  false,
	}

	cacheKey := s.getRefreshTokenCacheKey(token)
	if err := s.cache.Set(ctx, cacheKey, tokenData, s.authConfig.RefreshTokenTTL); err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}

	if err := s.enforceTokenLimit(ctx, userID); err != nil {
		s.logger.Warn("Failed to enforce token limit", zap.Error(err))
	}

	return nil
}

// Added: storeRefreshTokenWithParent stores rotated token
func (s *authService) storeRefreshTokenWithParent(ctx context.Context, token string, parent *RefreshTokenData, req *RefreshTokenRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	tokenData := &RefreshTokenData{
		UserID:      parent.UserID,
		TokenHash:   tokenHash,
		DeviceID:    parent.DeviceID,
		DeviceInfo:  parent.DeviceInfo,
		IPAddress:   req.IPAddress,
		UserAgent:   req.UserAgent,
		ExpiresAt:   time.Now().Add(s.authConfig.RefreshTokenTTL),
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
		IsRevoked:   false,
		ParentToken: parent.TokenHash,
	}

	cacheKey := s.getRefreshTokenCacheKey(token)
	if err := s.cache.Set(ctx, cacheKey, tokenData, s.authConfig.RefreshTokenTTL); err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}

	if err := s.enforceTokenLimit(ctx, parent.UserID); err != nil {
		s.logger.Warn("Failed to enforce token limit", zap.Error(err))
	}

	return nil
}

// Added: getRefreshTokenData retrieves and validates token
func (s *authService) getRefreshTokenData(ctx context.Context, token string) (*RefreshTokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := s.getRefreshTokenCacheKey(token)
	cachedData, found := s.cache.Get(ctx, cacheKey)
	if !found {
		return nil, fmt.Errorf("refresh token not found")
	}

	tokenData, ok := cachedData.(*RefreshTokenData)
	if !ok {
		return nil, fmt.Errorf("invalid token data format")
	}

	// Modified: Validate token with SHA-256
	hash := sha256.Sum256([]byte(token))
	if tokenData.TokenHash != hex.EncodeToString(hash[:]) {
		return nil, fmt.Errorf("token hash mismatch")
	}

	return tokenData, nil
}

// Added: revokeRefreshToken invalidates a token
func (s *authService) revokeRefreshToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokenData, err := s.getRefreshTokenData(ctx, token)
	if err != nil {
		return nil // Token doesn't exist
	}

	now := time.Now()
	tokenData.IsRevoked = true
	tokenData.RevokedAt = &now

	cacheKey := s.getRefreshTokenCacheKey(token)
	if err := s.cache.Set(ctx, cacheKey, tokenData, time.Until(tokenData.ExpiresAt)); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

// Added: revokeAllRefreshTokens revokes all user tokens
func (s *authService) revokeAllRefreshTokens(ctx context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pattern := fmt.Sprintf("refresh_token:%d_*", userID)
	return s.cache.DeletePattern(ctx, pattern)
}

// Added: updateRefreshTokenUsage updates token usage
func (s *authService) updateRefreshTokenUsage(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokenData, err := s.getRefreshTokenData(ctx, token)
	if err != nil {
		return err
	}

	tokenData.LastUsed = time.Now()
	cacheKey := s.getRefreshTokenCacheKey(token)
	if err := s.cache.Set(ctx, cacheKey, tokenData, time.Until(tokenData.ExpiresAt)); err != nil {
		return fmt.Errorf("failed to update token usage: %w", err)
	}

	return nil
}

// Added: isValidTokenFormat checks token prefix
func (s *authService) isValidTokenFormat(token string) bool {
	return strings.HasPrefix(token, "rt_") && len(token) > 20
}

// Added: getRefreshTokenCacheKey generates cache key
func (s *authService) getRefreshTokenCacheKey(token string) string {
	return fmt.Sprintf("refresh_token:%s", token[:12])
}

// Added: enforceTokenLimit enforces max tokens
func (s *authService) enforceTokenLimit(ctx context.Context, userID int64) error {
	// TODO: Implement token limit cleanup
	return nil
}

// Added: detectTokenReuse checks for reuse
func (s *authService) detectTokenReuse(ctx context.Context, tokenData *RefreshTokenData, req *RefreshTokenRequest) error {
	if tokenData.LastUsed.After(time.Now().Add(-1 * time.Minute)) {
		s.logger.Warn("Potential token reuse detected",
			zap.Int64("user_id", tokenData.UserID),
			zap.Time("last_used", tokenData.LastUsed),
		)
		return NewAuthenticationError("potential token reuse detected", "token_reuse", nil, "")
	}
	return nil
}

// Added: cleanupExpiredTokens removes expired tokens
// cleanupExpiredTokens removes expired or revoked refresh tokens for a user
func (s *authService) cleanupExpiredTokens(ctx context.Context, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all refresh tokens for the user
	pattern := fmt.Sprintf("refresh_token:%d_*", userID)

	// Use the cache's DeletePattern method if available
	if cacheWithPattern, ok := s.cache.(interface {
		DeletePattern(context.Context, string) error
	}); ok {
		if err := cacheWithPattern.DeletePattern(ctx, pattern); err != nil {
			s.logger.Error("Failed to delete expired tokens by pattern",
				zap.Error(err),
				zap.String("pattern", pattern))
		}
		return
	}

	// Fallback implementation for caches that don't support DeletePattern
	// This is less efficient as it requires fetching all matching keys first
	if redisCache, ok := s.cache.(interface {
		Keys(context.Context, string) ([]string, error)
	}); ok {
		keys, err := redisCache.Keys(ctx, pattern)
		if err != nil {
			s.logger.Error("Failed to get keys for pattern",
				zap.Error(err),
				zap.String("pattern", pattern))
			return
		}

		for _, key := range keys {
			if data, exists := s.cache.Get(ctx, key); exists {
				if tokenData, ok := data.(*RefreshTokenData); ok {
					if time.Now().After(tokenData.ExpiresAt) || tokenData.IsRevoked {
						if err := s.cache.Delete(ctx, key); err != nil {
							s.logger.Error("Failed to delete expired token",
								zap.Error(err),
								zap.String("key", key))
						}
					}
				}
			}
		}
	}
}

// Added: updateLastLogin updates last login info
func (s *authService) updateLastLogin(ctx context.Context, userID int64, ipAddress string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.LastSeen = time.Now()
	// Note: If you need to track the last login IP, you'll need to add a LastLoginIP field to the User model
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// Added: updateSessionActivity updates session expiry
func (s *authService) updateSessionActivity(ctx context.Context, token string) error {
	session, err := s.sessionRepo.GetByToken(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	session.ExpiresAt = time.Now().Add(s.authConfig.SessionTTL)
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// generateSessionToken generates a secure session token
func (s *authService) generateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// generateResetToken generates a secure reset token
func (s *authService) generateResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// generateVerificationToken generates a secure verification token
func (s *authService) generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// setUserOnlineStatus sets user online/offline status
func (s *authService) setUserOnlineStatus(ctx context.Context, userID int64, online bool) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.IsOnline = online
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	s.logger.Debug("Setting user online status",
		zap.Int64("user_id", userID),
		zap.Bool("online", online))
	return nil
}

// cleanupUploadedFiles cleans up uploaded files on error
func (s *authService) cleanupUploadedFiles(ctx context.Context, profilePublicID, cvPublicID string) {
	if s.fileService == nil {
		return
	}

	// Use a wait group to handle cleanup in parallel
	var wg sync.WaitGroup

	// Clean up profile image if exists
	if profilePublicID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.fileService.DeleteFile(ctx, profilePublicID); err != nil {
				s.logger.Error("Failed to cleanup profile image",
					zap.Error(err),
					zap.String("public_id", profilePublicID))
			}
		}()
	}

	// Clean up CV document if exists
	if cvPublicID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.fileService.DeleteFile(ctx, cvPublicID); err != nil {
				s.logger.Error("Failed to cleanup CV document",
					zap.Error(err),
					zap.String("public_id", cvPublicID))
			}
		}()
	}

	// Wait for all cleanup operations to complete
	wg.Wait()
}

// manageUserSessions manages the number of active sessions per user
func (s *authService) manageUserSessions(ctx context.Context, userID int64) error {
	sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, true) // true for sorted by last activity
	if err != nil {
		s.logger.Error("Failed to get active sessions",
			zap.Int64("user_id", userID),
			zap.Error(err))
		return fmt.Errorf("failed to get active sessions: %w", err)
	}

	if len(sessions) > s.authConfig.MaxSessions {
		sessionsToRemove := len(sessions) - s.authConfig.MaxSessions
		s.logger.Info("Removing oldest sessions",
			zap.Int64("user_id", userID),
			zap.Int("sessions_to_remove", sessionsToRemove),
			zap.Int("max_sessions", s.authConfig.MaxSessions),
		)

		// Remove oldest sessions (they're already sorted by last activity)
		for _, session := range sessions[:sessionsToRemove] {
			if err := s.sessionRepo.Delete(ctx, session.SessionToken); err != nil {
				s.logger.Warn("Failed to delete old session",
					zap.Int64("user_id", userID),
					zap.String("session_token", session.SessionToken),
					zap.Error(err))
				// Continue with other sessions even if one fails
				continue
			}
		}
	}

	return nil
}

// ===============================
// HELPER UTILITY FUNCTIONS
// ===============================

// getStringPtr returns a pointer to string if not empty, otherwise nil
func getStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// getInt16Ptr returns a pointer to int16 if > 0, otherwise nil
func getInt16Ptr(i int) *int16 {
	if i <= 0 {
		return nil
	}
	val := int16(i)
	return &val
}

// ===============================
// RATE LIMITING AND LOCKOUT METHODS
// ===============================

func (s *authService) checkRegistrationRateLimit(ctx context.Context, email string) error {
	// Check if authConfig or LockoutConfig is nil, or if lockout is disabled
	if s.authConfig == nil || s.authConfig.LockoutConfig == nil || !s.authConfig.LockoutConfig.EnableLockout {
		return nil
	}

	key := fmt.Sprintf("reg_rate_limit:%s", email)
	attempts, _ := s.cache.Get(ctx, key)

	if attempts != nil {
		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 registration attempts per hour
			return NewRateLimitError("too many registration attempts", map[string]interface{}{
				"retry_after": "1 hour",
			})
		}
	}

	// Increment counter
	s.cache.Increment(ctx, key, 1)
	s.cache.SetTTL(ctx, key, 1*time.Hour)

	return nil
}

// checkPasswordResetRateLimit checks if the user has exceeded the password reset rate limit
func (s *authService) checkPasswordResetRateLimit(ctx context.Context, email string) error {
	key := fmt.Sprintf("reset_rate_limit:%s", email)
	attempts, _ := s.cache.Get(ctx, key)

	if attempts != nil {
		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 reset attempts per hour
			return NewRateLimitError("too many password reset attempts", map[string]interface{}{
				"retry_after": "1 hour",
			})
		}
	}

	s.cache.Increment(ctx, key, 1)
	s.cache.SetTTL(ctx, key, 1*time.Hour)

	return nil
}

func (s *authService) checkAccountLockout(ctx context.Context, login string) error {
	if !s.authConfig.LockoutConfig.EnableLockout {
		return nil
	}
	key := fmt.Sprintf("lockout:%s", login)
	count, _ := s.cache.Get(ctx, key)
	if countInt, ok := count.(int64); ok && countInt >= int64(s.authConfig.LockoutConfig.MaxAttempts) {
		return NewBusinessError("account locked", "ACCOUNT_LOCKED")
	}
	return nil
}

func (s *authService) recordFailedAttempt(ctx context.Context, login string, reason string) {
	if s.authConfig.LockoutConfig.EnableLockout {
		key := fmt.Sprintf("lockout:%s", login)
		// Convert time.Duration to seconds (int64)
		windowSeconds := int64(s.authConfig.LockoutConfig.WindowTime / time.Second)
		s.cache.Increment(ctx, key, windowSeconds)
	}
	s.logger.Info("Failed login attempt",
		zap.String("login", login),
		zap.String("reason", reason),
	)
}
func (s *authService) clearFailedAttempts(ctx context.Context, login string) {
	if s.authConfig != nil && s.authConfig.LockoutConfig != nil && s.authConfig.LockoutConfig.EnableLockout {
		key := fmt.Sprintf("lockout:%s", login)
		s.cache.Delete(ctx, key)
	}
}

// Added: validateSecureTransport ensures secure connection
func (s *authService) validateSecureTransport(ctx context.Context) error {
	if s.authConfig.SecureTransport {
		// TODO: Implement HTTPS check via middleware
	}
	return nil
}

// // file: internal/services/auth_service.go
// package services

// import (
// 	"context"
// 	"crypto/rand"
// 	"encoding/base64"
// 	"evalhub/internal/cache"
// 	"evalhub/internal/events"
// 	"evalhub/internal/models"
// 	"evalhub/internal/repositories"
// 	"fmt"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/go-playground/validator/v10"
// 	"go.uber.org/zap"
// 	"golang.org/x/crypto/bcrypt"
// )

// // authService implements AuthService with enterprise features
// type authService struct {
// 	userRepo     repositories.UserRepository
// 	sessionRepo  repositories.SessionRepository
// 	cache        cache.Cache
// 	events       events.EventBus
// 	userService  UserService
// 	fileService  FileService  // Added for file upload support
// 	emailService EmailService // Added for enhanced email handling
// 	logger       *zap.Logger
// 	validate     *validator.Validate
// 	sessionTTL   time.Duration
// 	bcryptCost   int
// 	maxSessions  int
// 	lockoutConfig *LockoutConfig
// }

// // Auth service configuration types
// type (
// 	// LockoutConfig holds account lockout configuration
// 	LockoutConfig struct {
// 		MaxAttempts   int           `json:"max_attempts"`
// 		LockoutTime   time.Duration `json:"lockout_time"`
// 		WindowTime    time.Duration `json:"window_time"`
// 		EnableLockout bool          `json:"enable_lockout"`
// 	}

// 	// AuthConfig holds authentication service configuration
// 	AuthConfig struct {
// 		SessionTTL    time.Duration  `json:"session_ttl"`
// 		BCryptCost    int            `json:"bcrypt_cost"`
// 		MaxSessions   int            `json:"max_sessions"`
// 		LockoutConfig *LockoutConfig `json:"lockout_config"`
// 	}
// )

// // DefaultAuthConfig returns default authentication configuration
// func DefaultAuthConfig() *AuthConfig {
// 	return &AuthConfig{
// 		SessionTTL:  24 * time.Hour,
// 		BCryptCost:  12,
// 		MaxSessions: 5,
// 		LockoutConfig: &LockoutConfig{
// 			MaxAttempts:   5,
// 			LockoutTime:   15 * time.Minute,
// 			WindowTime:    1 * time.Hour,
// 			EnableLockout: true,
// 		},
// 	}
// }

// // NewAuthService creates a new enterprise authentication service with enhanced capabilities
// func NewAuthService(
// 	userRepo repositories.UserRepository,
// 	sessionRepo repositories.SessionRepository,
// 	cache cache.Cache,
// 	events events.EventBus,
// 	userService UserService,
// 	fileService FileService,
// 	emailService EmailService,
// 	logger *zap.Logger,
// 	config *AuthConfig,
// ) AuthService {
// 	// Initialize validator
// 	validate := validator.New()
// 	if config == nil {
// 		config = DefaultAuthConfig()
// 	}

// 	return &authService{
// 		userRepo:     userRepo,
// 		sessionRepo:  sessionRepo,
// 		cache:        cache,
// 		events:       events,
// 		userService:  userService,
// 		fileService:  fileService,
// 		emailService: emailService,
// 		logger:       logger,
// 		validate:     validate,
// 		sessionTTL:   config.SessionTTL,
// 		bcryptCost:   config.BCryptCost,
// 		maxSessions:  config.MaxSessions,
// 		lockoutConfig: config.LockoutConfig,
// 	}
// }

// // ===============================
// // AUTHENTICATION
// // ===============================

// // Register creates a new user account with enhanced validation, file uploads, and security checks
// func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
// 	// Step 1: Validate request structure
// 	if err := s.validateRegisterRequest(req); err != nil {
// 		return nil, err
// 	}

// 	// Step 2: Check for rate limiting
// 	if err := s.checkRegistrationRateLimit(ctx, req.Email); err != nil {
// 		return nil, err
// 	}

// 	// Step 3: Enhanced business rule validation
// 	if err := s.validateBusinessRules(ctx, req); err != nil {
// 		return nil, err
// 	}

// 	// Step 4: Process file uploads BEFORE creating user
// 	var profileURL, profilePublicID, cvURL, cvPublicID string

// 	if req.ProfileImage != nil && s.fileService != nil {
// 		// Create a FileUploadRequest from the profile image
// 		fileReq, ok := req.ProfileImage.(*FileUploadRequest)
// 		if !ok {
// 			s.logger.Error("Invalid profile image format")
// 			return nil, NewValidationError("invalid profile image format", nil)
// 		}

// 		// Upload the image
// 		result, err := s.fileService.UploadImage(ctx, fileReq)
// 		if err != nil {
// 			s.logger.Error("Failed to upload profile image", zap.Error(err))
// 			return nil, NewInternalError("failed to upload profile image")
// 		}
// 		// Store the public ID and URL for user creation and cleanup
// 		profileURL = result.URL
// 		profilePublicID = result.PublicID

// 		s.logger.Info("Profile image uploaded successfully",
// 			zap.String("url", profileURL),
// 			zap.String("public_id", profilePublicID))
// 	}

// 	if req.CVDocument != nil && s.fileService != nil {
// 		// Create a FileUploadRequest from the CV document
// 		docReq, ok := req.CVDocument.(*FileUploadRequest)
// 		if !ok {
// 			s.logger.Error("Invalid CV document format")
// 			return nil, NewValidationError("invalid CV document format", nil)
// 		}

// 		// Upload the document
// 		result, err := s.fileService.UploadDocument(ctx, docReq)
// 		if err != nil {
// 			s.logger.Error("Failed to upload CV document", zap.Error(err))
// 			// Clean up any uploaded files if CV upload fails
// 			if profilePublicID != "" {
// 				s.cleanupUploadedFiles(ctx, profilePublicID, "")
// 			}
// 			return nil, NewInternalError("failed to upload CV document")
// 		}
// 		cvURL = result.URL
// 		cvPublicID = result.PublicID

// 		s.logger.Info("CV document uploaded successfully",
// 			zap.String("url", cvURL),
// 			zap.String("public_id", cvPublicID))
// 	}

// 	// Step 5: Create user through user service with enhanced fields
// 	createUserReq := &CreateUserRequest{
// 		Email:           req.Email,
// 		Username:        req.Username,
// 		Password:        req.Password,
// 		FirstName:       &req.FirstName,
// 		LastName:        &req.LastName,
// 		AcceptTerms:     req.AcceptTerms,
// 		MarketingEmails: false, // Default to false for privacy

// 		// Enhanced fields - using pointers to match the type definition
// 		Role:             &req.Role,
// 		Affiliation:      getStringPtr(req.Affiliation),
// 		Bio:              getStringPtr(req.Bio),
// 		YearsExperience:  getInt16Ptr(req.YearsExperience),
// 		CoreCompetencies: getStringPtr(req.CoreCompetencies),
// 		Expertise:        getStringPtr(req.Expertise),

// 		// File URLs
// 		ProfileURL:      getStringPtr(profileURL),
// 		ProfilePublicID: getStringPtr(profilePublicID),
// 		CVURL:          getStringPtr(cvURL),
// 		CVPublicID:     getStringPtr(cvPublicID),
// 	}

// 	user, err := s.userService.CreateUser(ctx, createUserReq)
// 	if err != nil {
// 		// Clean up uploaded files if user creation fails
// 		s.cleanupUploadedFiles(ctx, profilePublicID, cvPublicID)

// 		s.logger.Error("Failed to create user during registration",
// 			zap.Error(err),
// 			zap.String("email", req.Email),
// 			zap.String("username", req.Username),
// 		)
// 		return nil, err
// 	}

// 	// Step 6: Create initial session
// 	sessionToken, err := s.generateSessionToken()
// 	if err != nil {
// 		return nil, NewInternalError("failed to generate session token")
// 	}

// 	session := &models.Session{
// 		UserID:       user.ID,
// 		SessionToken: sessionToken,
// 		ExpiresAt:    time.Now().Add(s.sessionTTL),
// 	}

// 	if err := s.sessionRepo.Create(ctx, session); err != nil {
// 		s.logger.Error("Failed to create session during registration",
// 			zap.Error(err),
// 			zap.Int64("user_id", user.ID),
// 		)
// 		return nil, NewInternalError("failed to create session")
// 	}

// 	// Step 7: Set user online status
// 	if err := s.setUserOnlineStatus(ctx, user.ID, true); err != nil {
// 		s.logger.Warn("Failed to set user online status", zap.Error(err))
// 	}

// 	// Step 8: Send verification email (async)
// 	go func() {
// 		if err := s.SendVerificationEmail(context.Background(), user.ID); err != nil {
// 			s.logger.Error("Failed to send verification email",
// 				zap.Error(err),
// 				zap.Int64("user_id", user.ID))
// 		}
// 	}()

// 	// Step 9: Publish registration event
// 	if err := s.events.Publish(ctx, events.NewUserCreatedEvent(user.ID, user.Email, user.Username)); err != nil {
// 		s.logger.Warn("Failed to publish user created event", zap.Error(err))
// 	}

// 	s.logger.Info("User registered successfully",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("email", user.Email),
// 		zap.String("username", user.Username),
// 	)

// 	return &AuthResponse{
// 		User:         user,
// 		AccessToken:  sessionToken,
// 		ExpiresIn:    int64(s.sessionTTL.Seconds()),
// 		TokenType:    "Bearer",
// 		// SessionID is not part of the standard OAuth2 response
// 	}, nil
// }

// // Login authenticates a user and creates a session with enhanced tracking
// func (s *authService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
// 	// Step 1: Validate request
// 	if err := s.validateLoginRequest(req); err != nil {
// 		return nil, err
// 	}

// 	// Step 2: Check for account lockout
// 	if err := s.checkAccountLockout(ctx, req.Login); err != nil {
// 		return nil, err
// 	}

// 	// Step 3: Find user by email or username
// 	var user *models.User
// 	var err error

// 	if strings.Contains(req.Login, "@") {
// 		user, err = s.userRepo.GetByEmail(ctx, req.Login)
// 	} else {
// 		user, err = s.userRepo.GetByUsername(ctx, req.Login)
// 	}

// 	if err != nil {
// 		s.logger.Error("Failed to get user during login", zap.Error(err), zap.String("login", req.Login))
// 		return nil, NewInternalError("authentication failed")
// 	}

// 	if user == nil {
// 		// Record failed attempt
// 		s.recordFailedAttempt(ctx, req.Login, "user_not_found")
// 		return nil, NewAuthenticationError("invalid credentials", "invalid_login", nil, req.Login)
// 	}

// 	// Step 4: Check if user is active
// 	if !user.IsActive {
// 		s.recordFailedAttempt(ctx, req.Login, "account_deactivated")
// 		return nil, NewAuthenticationError("account is deactivated", "account_deactivated", &user.ID, user.Username)
// 	}

// 	// Step 5: Verify password
// 	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
// 		s.recordFailedAttempt(ctx, req.Login, "invalid_password")
// 		s.logger.Warn("Invalid password attempt",
// 			zap.Int64("user_id", user.ID),
// 			zap.String("username", user.Username),
// 			zap.String("ip_address", req.IPAddress),
// 		)
// 		return nil, NewAuthenticationError("invalid credentials", "invalid_password", &user.ID, user.Username)
// 	}

// 	// Step 6: Clear failed attempts on successful login
// 	s.clearFailedAttempts(ctx, req.Login)

// 	// Step 7: Manage existing sessions
// 	if err := s.manageUserSessions(ctx, user.ID); err != nil {
// 		s.logger.Warn("Failed to manage user sessions", zap.Error(err), zap.Int64("user_id", user.ID))
// 	}

// 	// Step 8: Create new session
// 	sessionToken, err := s.generateSessionToken()
// 	if err != nil {
// 		return nil, NewInternalError("failed to generate session token")
// 	}

// 	sessionTTL := s.sessionTTL
// 	if req.Remember {
// 		sessionTTL = 30 * 24 * time.Hour // 30 days for "remember me"
// 	}

// 	session := &models.Session{
// 		UserID:       user.ID,
// 		SessionToken: sessionToken,
// 		ExpiresAt:    time.Now().Add(sessionTTL),
// 	}

// 	if err := s.sessionRepo.Create(ctx, session); err != nil {
// 		s.logger.Error("Failed to create session during login", zap.Error(err), zap.Int64("user_id", user.ID))
// 		return nil, NewInternalError("failed to create session")
// 	}

// 	// Step 9: Set user online status and update last seen
// 	if err := s.setUserOnlineStatus(ctx, user.ID, true); err != nil {
// 		s.logger.Warn("Failed to set user online status", zap.Error(err))
// 	}

// 	if err := s.userService.UpdateOnlineStatus(ctx, user.ID, true); err != nil {
// 		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", user.ID))
// 	}

// 	// Step 10: Publish login event with enhanced tracking
// 	if err := s.events.Publish(ctx, &events.UserLoggedInEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.logged_in",
// 			Timestamp: time.Now(),
// 			UserID:    &user.ID,
// 		},
// 		LoginAt:   time.Now(),
// 		IPAddress: req.IPAddress,
// 		UserAgent: req.UserAgent,
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish login event", zap.Error(err))
// 	}

// 	s.logger.Info("User logged in successfully",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("username", user.Username),
// 		zap.String("ip_address", req.IPAddress),
// 		zap.Bool("remember", req.Remember),
// 	)

// 	// Clear password hash before returning
// 	user.PasswordHash = ""

// 	return &AuthResponse{
// 		User:         user,
// 		AccessToken:  sessionToken,
// 		ExpiresIn:    int64(sessionTTL.Seconds()),
// 		TokenType:    "Bearer",
// 	}, nil
// }

// // LoginWithProvider handles OAuth provider login
// func (s *authService) LoginWithProvider(ctx context.Context, req *OAuthLoginRequest) (*AuthResponse, error) {
// 	if err := s.validate.Struct(req); err != nil {
// 		return nil, NewValidationError("invalid OAuth login request", err)
// 	}

// 	// This would integrate with OAuth providers (Google, GitHub, etc.)
// 	// For now, return a not implemented error
// 	return nil, NewNotImplementedError("OAuth login not implemented")
// }

// // RefreshToken refreshes an access token with enhanced session handling
// func (s *authService) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error) {
// 	if err := s.validate.Struct(req); err != nil {
// 		return nil, NewValidationError("invalid refresh token request", err)
// 	}

// 	// Get session by token
// 	session, err := s.sessionRepo.GetByToken(ctx, req.RefreshToken)
// 	if err != nil || session == nil {
// 		return nil, NewAuthenticationError("invalid or expired token", "token_expired", nil, "")
// 	}

// 	// Check if session is expired
// 	if time.Now().After(session.ExpiresAt) {
// 		// Clean up expired session
// 		s.sessionRepo.Delete(ctx, req.RefreshToken)
// 		return nil, NewAuthenticationError("session expired", "session_expired", nil, "")
// 	}

// 	// Get fresh user data
// 	user, err := s.userRepo.GetByID(ctx, session.UserID)
// 	if err != nil || user == nil {
// 		return nil, NewAuthenticationError("user not found", "user_not_found", nil, "")
// 	}

// 	// Update session activity
// 	if err := s.updateSessionActivity(ctx, req.RefreshToken); err != nil {
// 		s.logger.Warn("Failed to update session activity", zap.Error(err))
// 	}

// 	// Clear password hash before returning
// 	user.PasswordHash = ""

// 	return &AuthResponse{
// 		User:         user,
// 		AccessToken:  req.RefreshToken,
// 		ExpiresIn:    int64(time.Until(session.ExpiresAt).Seconds()),
// 		TokenType:    "Bearer",
// 	}, nil
// }

// // Logout invalidates a session with enhanced options
// func (s *authService) Logout(ctx context.Context, req *LogoutRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid logout request", err)
// 	}

// 	// Get session to get user ID for events
// 	session, err := s.sessionRepo.GetByToken(ctx, req.SessionToken)
// 	if err != nil {
// 		s.logger.Warn("Failed to get session during logout", zap.Error(err))
// 		// Still try to delete the session
// 	}

// 	var userID int64
// 	if session != nil {
// 		userID = session.UserID

// 		// Handle logout from all devices
// 		if req.LogoutAll {
// 			if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
// 				s.logger.Error("Failed to delete all user sessions", zap.Error(err))
// 				return NewInternalError("failed to logout from all devices")
// 			}
// 			s.logger.Info("User logged out from all devices", zap.Int64("user_id", userID))
// 		} else {
// 			// Delete single session
// 			if err := s.sessionRepo.Delete(ctx, req.SessionToken); err != nil {
// 				s.logger.Error("Failed to delete session during logout", zap.Error(err))
// 				return NewInternalError("failed to logout")
// 			}
// 			s.logger.Info("User logged out", zap.Int64("user_id", userID))
// 		}

// 		// Update online status
// 		if err := s.setUserOnlineStatus(ctx, userID, false); err != nil {
// 			s.logger.Warn("Failed to update online status during logout", zap.Error(err))
// 		}

// 		if err := s.userService.UpdateOnlineStatus(ctx, userID, false); err != nil {
// 			s.logger.Warn("Failed to update online status", zap.Error(err))
// 		}

// 		// Publish logout event
// 		if err := s.events.Publish(ctx, &events.UserLoggedOutEvent{
// 			BaseEvent: events.BaseEvent{
// 				EventID:   events.GenerateEventID(),
// 				EventType: "user.logged_out",
// 				Timestamp: time.Now(),
// 				UserID:    &userID,
// 			},
// 			LogoutAt: time.Now(),
// 		}); err != nil {
// 			s.logger.Warn("Failed to publish logout event", zap.Error(err))
// 		}
// 	} else {
// 		// Try to delete session even if not found in case it exists
// 		if err := s.sessionRepo.Delete(ctx, req.SessionToken); err != nil {
// 			s.logger.Error("Failed to delete session during logout", zap.Error(err))
// 			return NewInternalError("failed to logout")
// 		}
// 	}

// 	return nil
// }

// // LogoutAllDevices invalidates all sessions for a user
// func (s *authService) LogoutAllDevices(ctx context.Context, userID int64) error {
// 	if userID <= 0 {
// 		return NewValidationError("invalid user ID", nil)
// 	}

// 	// Delete all sessions for the user
// 	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
// 		s.logger.Error("Failed to delete all sessions", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to logout from all devices")
// 	}

// 	// Update online status
// 	if err := s.setUserOnlineStatus(ctx, userID, false); err != nil {
// 		s.logger.Warn("Failed to update online status", zap.Error(err))
// 	}

// 	if err := s.userService.UpdateOnlineStatus(ctx, userID, false); err != nil {
// 		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", userID))
// 	}

// 	s.logger.Info("User logged out from all devices", zap.Int64("user_id", userID))

// 	return nil
// }

// // ===============================
// // PASSWORD MANAGEMENT
// // ===============================

// // ForgotPassword initiates password reset process
// func (s *authService) ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid forgot password request", err)
// 	}

// 	// Check rate limiting
// 	if err := s.checkPasswordResetRateLimit(ctx, req.Email); err != nil {
// 		return err
// 	}

// 	// Get user by email
// 	user, err := s.userRepo.GetByEmail(ctx, req.Email)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password reset", zap.Error(err))
// 		return NewInternalError("failed to process password reset")
// 	}

// 	// Always return success to prevent email enumeration
// 	if user == nil {
// 		s.logger.Info("Password reset requested for non-existent email", zap.String("email", req.Email))
// 		return nil
// 	}

// 	// Generate reset token
// 	resetToken, err := s.generateResetToken()
// 	if err != nil {
// 		return NewInternalError("failed to generate reset token")
// 	}

// 	// Store reset token in cache with expiration
// 	resetKey := fmt.Sprintf("password_reset:%s", resetToken)
// 	if err := s.cache.Set(ctx, resetKey, user.ID, 1*time.Hour); err != nil {
// 		s.logger.Error("Failed to store reset token", zap.Error(err))
// 		return NewInternalError("failed to process password reset")
// 	}

// 	// Enhanced: Send password reset email using EmailService
// 	if s.emailService != nil {
// 		go func() {
// 			if err := s.emailService.SendPasswordResetEmail(context.Background(), user.Email, resetToken); err != nil {
// 				s.logger.Error("Failed to send password reset email",
// 					zap.Error(err),
// 					zap.String("email", user.Email))
// 			}
// 		}()
// 	}

// 	s.logger.Info("Password reset token generated",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("email", user.Email),
// 	)

// 	return nil
// }

// // ResetPassword resets a user's password using a reset token
// func (s *authService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid reset password request", err)
// 	}

// 	if req.NewPassword != req.ConfirmPassword {
// 		return NewValidationError("passwords do not match", nil)
// 	}

// 	// Validate reset token
// 	resetKey := fmt.Sprintf("password_reset:%s", req.Token)
// 	userIDInterface, found := s.cache.Get(ctx, resetKey)
// 	if !found {
// 		return NewValidationError("invalid or expired reset token", nil)
// 	}

// 	userID, ok := userIDInterface.(int64)
// 	if !ok {
// 		s.logger.Error("Invalid user ID type in reset token cache")
// 		return NewInternalError("invalid reset token")
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password reset", zap.Error(err))
// 		return NewInternalError("failed to reset password")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	// Hash new password
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
// 	if err != nil {
// 		s.logger.Error("Failed to hash new password", zap.Error(err))
// 		return NewInternalError("failed to reset password")
// 	}

// 	// Update password
// 	user.PasswordHash = string(hashedPassword)
// 	user.PasswordChangedAt = time.Now()

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to reset password")
// 	}

// 	// Delete reset token
// 	s.cache.Delete(ctx, resetKey)

// 	// Invalidate all existing sessions for security
// 	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
// 		s.logger.Warn("Failed to invalidate sessions after password reset", zap.Error(err))
// 	}

// 	// Publish password changed event
// 	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.password_changed",
// 			Timestamp: time.Now(),
// 			UserID:    &userID,
// 		},
// 		ChangedAt: time.Now(),
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
// 	}

// 	s.logger.Info("Password reset successfully", zap.Int64("user_id", userID))

// 	return nil
// }

// // ChangePassword changes a user's password
// func (s *authService) ChangePassword(ctx context.Context, req *ChangePasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid change password request", err)
// 	}

// 	if req.NewPassword != req.ConfirmPassword {
// 		return NewValidationError("passwords do not match", nil)
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, req.UserID)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password change", zap.Error(err))
// 		return NewInternalError("failed to change password")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	// Verify current password
// 	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
// 		return NewValidationError("current password is incorrect", nil)
// 	}

// 	// Hash new password
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
// 	if err != nil {
// 		s.logger.Error("Failed to hash new password", zap.Error(err))
// 		return NewInternalError("failed to change password")
// 	}

// 	// Update password
// 	user.PasswordHash = string(hashedPassword)
// 	user.PasswordChangedAt = time.Now()

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", req.UserID))
// 		return NewInternalError("failed to change password")
// 	}

// 	// Publish password changed event
// 	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.password_changed",
// 			Timestamp: time.Now(),
// 			UserID:    &req.UserID,
// 		},
// 		ChangedAt: time.Now(),
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
// 	}

// 	s.logger.Info("Password changed successfully", zap.Int64("user_id", req.UserID))

// 	return nil
// }

// // ===============================
// // EMAIL VERIFICATION
// // ===============================

// // SendVerificationEmail sends an email verification link using EmailService
// func (s *authService) SendVerificationEmail(ctx context.Context, userID int64) error {
// 	if userID <= 0 {
// 		return NewValidationError("invalid user ID", nil)
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		return NewInternalError("failed to get user")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	if user.EmailVerified {
// 		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
// 	}

// 	// Generate verification token
// 	verificationToken, err := s.generateVerificationToken()
// 	if err != nil {
// 		return NewInternalError("failed to generate verification token")
// 	}

// 	// Store verification token
// 	verificationKey := fmt.Sprintf("email_verification:%s", verificationToken)
// 	if err := s.cache.Set(ctx, verificationKey, userID, 24*time.Hour); err != nil {
// 		s.logger.Error("Failed to store verification token", zap.Error(err))
// 		return NewInternalError("failed to send verification email")
// 	}

// 	// Enhanced: Send verification email using EmailService
// 	if s.emailService != nil {
// 		go func() {
// 			if err := s.emailService.SendVerificationEmail(context.Background(), user.Email, verificationToken); err != nil {
// 				s.logger.Error("Failed to send verification email",
// 					zap.Error(err),
// 					zap.String("email", user.Email))
// 			}
// 		}()
// 	}

// 	s.logger.Info("Email verification token generated",
// 		zap.Int64("user_id", userID),
// 		zap.String("email", user.Email),
// 	)

// 	return nil
// }

// // VerifyEmail verifies a user's email address
// func (s *authService) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid email verification request", err)
// 	}

// 	// Validate verification token
// 	verificationKey := fmt.Sprintf("email_verification:%s", req.Token)
// 	userIDInterface, found := s.cache.Get(ctx, verificationKey)
// 	if !found {
// 		return NewValidationError("invalid or expired verification token", nil)
// 	}

// 	userID, ok := userIDInterface.(int64)
// 	if !ok {
// 		return NewInternalError("invalid verification token")
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		return NewInternalError("failed to verify email")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	if user.EmailVerified {
// 		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
// 	}

// 	// Mark email as verified
// 	now := time.Now()
// 	user.EmailVerified = true
// 	user.EmailVerifiedAt = &now

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update email verification status", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to verify email")
// 	}

// 	// Delete verification token
// 	s.cache.Delete(ctx, verificationKey)

// 	s.logger.Info("Email verified successfully", zap.Int64("user_id", userID))

// 	return nil
// }

// // ===============================
// // SESSION MANAGEMENT
// // ===============================

// // GetActiveSessions retrieves active sessions for a user
// func (s *authService) GetActiveSessions(ctx context.Context, userID int64) ([]*SessionInfo, error) {
// 	if userID <= 0 {
// 		return nil, NewValidationError("invalid user ID", nil)
// 	}

// 	sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, false)
// 	if err != nil {
// 		s.logger.Error("Failed to get active sessions", zap.Error(err), zap.Int64("user_id", userID))
// 		return nil, NewInternalError("failed to retrieve sessions")
// 	}

// 	var sessionInfos []*SessionInfo
// 	for _, session := range sessions {
// 		sessionInfos = append(sessionInfos, &SessionInfo{
// 			ID:           session.ID,
// 			LastActivity: session.LastActivity,
// 			ExpiresAt:    session.ExpiresAt,
// 			// TODO: Add device and location info from session metadata
// 		})
// 	}

// 	return sessionInfos, nil
// }

// // RevokeSession revokes a specific session
// func (s *authService) RevokeSession(ctx context.Context, sessionID int64, userID int64) error {
// 	if sessionID <= 0 || userID <= 0 {
// 		return NewValidationError("invalid session or user ID", nil)
// 	}

// 	// TODO: Implement session revocation by ID
// 	// This would require adding a method to get session by ID and verify ownership

// 	return NewNotImplementedError("session revocation by ID not implemented")
// }

// // ===============================
// // TWO-FACTOR AUTHENTICATION (Placeholder)
// // ===============================

// // EnableTwoFactor enables two-factor authentication
// func (s *authService) EnableTwoFactor(ctx context.Context, userID int64) (*TwoFactorSetupResponse, error) {
// 	return nil, NewNotImplementedError("two-factor authentication not implemented")
// }

// // DisableTwoFactor disables two-factor authentication
// func (s *authService) DisableTwoFactor(ctx context.Context, req *DisableTwoFactorRequest) error {
// 	return NewNotImplementedError("two-factor authentication not implemented")
// }

// // VerifyTwoFactor verifies a two-factor authentication code
// func (s *authService) VerifyTwoFactor(ctx context.Context, req *VerifyTwoFactorRequest) error {
// 	return NewNotImplementedError("two-factor authentication not implemented")
// }

// // ===============================
// // ENHANCED VALIDATION METHODS
// // ===============================

// // validateRegisterRequest validates registration request structure
// func (s *authService) validateRegisterRequest(req *RegisterRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid registration request", err)
// 	}

// 	// Additional validation
// 	if req.Password != req.ConfirmPassword {
// 		return NewValidationError("passwords do not match", nil)
// 	}

// 	if !req.AcceptTerms {
// 		return NewValidationError("must accept terms and conditions", nil)
// 	}

// 	return nil
// }

// // validateLoginRequest validates login request structure
// func (s *authService) validateLoginRequest(req *LoginRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid login request", err)
// 	}

// 	return nil
// }

// // validateBusinessRules validates business-specific rules during registration
// func (s *authService) validateBusinessRules(ctx context.Context, req *RegisterRequest) error {
// 	// Check if email exists
// 	if user, _ := s.userRepo.GetByEmail(ctx, req.Email); user != nil {
// 		return NewBusinessError("email already exists", "EMAIL_EXISTS")
// 	}

// 	// Check if username exists
// 	if user, _ := s.userRepo.GetByUsername(ctx, req.Username); user != nil {
// 		return NewBusinessError("username already exists", "USERNAME_EXISTS")
// 	}

// 	// Enhanced password validation would go here
// 	// This could integrate with a password strength service

// 	return nil
// }

// // ===============================
// // ENHANCED HELPER METHODS
// // ===============================

// // generateSessionToken generates a secure session token
// func (s *authService) generateSessionToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // generateResetToken generates a secure reset token
// func (s *authService) generateResetToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // generateVerificationToken generates a secure verification token
// func (s *authService) generateVerificationToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // setUserOnlineStatus sets user online/offline status
// func (s *authService) setUserOnlineStatus(ctx context.Context, userID int64, online bool) error {
// 	// This would integrate with a user status service or update the user table directly
// 	// For now, just log the action
// 	s.logger.Debug("Setting user online status",
// 		zap.Int64("user_id", userID),
// 		zap.Bool("online", online))
// 	return nil
// }

// // updateSessionActivity updates session last activity timestamp
// func (s *authService) updateSessionActivity(ctx context.Context, sessionToken string) error {
// 	// This would update the session's last_activity timestamp
// 	// Implementation depends on your session repository structure
// 	s.logger.Debug("Updating session activity", zap.String("token", sessionToken[:10]+"..."))
// 	return nil
// }

// // cleanupUploadedFiles cleans up uploaded files on error
// func (s *authService) cleanupUploadedFiles(ctx context.Context, profilePublicID, cvPublicID string) {
// 	if s.fileService == nil {
// 		return
// 	}

// 	// Use a wait group to handle cleanup in parallel
// 	var wg sync.WaitGroup

// 	// Clean up profile image if exists
// 	if profilePublicID != "" {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			if err := s.fileService.DeleteFile(ctx, profilePublicID); err != nil {
// 				s.logger.Error("Failed to cleanup profile image",
// 					zap.Error(err),
// 					zap.String("public_id", profilePublicID))
// 			}
// 		}()
// 	}

// 	// Clean up CV document if exists
// 	if cvPublicID != "" {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			if err := s.fileService.DeleteFile(ctx, cvPublicID); err != nil {
// 				s.logger.Error("Failed to cleanup CV document",
// 					zap.Error(err),
// 					zap.String("public_id", cvPublicID))
// 			}
// 		}()
// 	}

// 	// Wait for all cleanup operations to complete
// 	wg.Wait()
// }

// // manageUserSessions manages the number of active sessions per user
// func (s *authService) manageUserSessions(ctx context.Context, userID int64) error {
// 	activeCount, err := s.sessionRepo.CountActiveSessions(ctx, userID)
// 	if err != nil {
// 		return err
// 	}

// 	if activeCount >= s.maxSessions {
// 		// Remove oldest sessions to make room
// 		sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, false)
// 		if err != nil {
// 			return err
// 		}

// 		// Sort by last activity and remove oldest
// 		sessionsToRemove := len(sessions) - s.maxSessions + 1
// 		if sessionsToRemove > 0 {
// 			// This would require implementing session removal by criteria
// 			s.logger.Info("Removing oldest sessions",
// 				zap.Int64("user_id", userID),
// 				zap.Int("sessions_to_remove", sessionsToRemove),
// 			)
// 		}
// 	}

// 	return nil
// }

// // ===============================
// // HELPER UTILITY FUNCTIONS
// // ===============================

// // getStringPtr returns a pointer to string if not empty, otherwise nil
// func getStringPtr(s string) *string {
// 	if s == "" {
// 		return nil
// 	}
// 	return &s
// }

// // getInt16Ptr returns a pointer to int16 if > 0, otherwise nil
// func getInt16Ptr(i int) *int16 {
// 	if i <= 0 {
// 		return nil
// 	}
// 	val := int16(i)
// 	return &val
// }

// // ===============================
// // RATE LIMITING AND LOCKOUT METHODS
// // ===============================

// func (s *authService) checkRegistrationRateLimit(ctx context.Context, email string) error {
// 	if !s.lockoutConfig.EnableLockout {
// 		return nil
// 	}

// 	key := fmt.Sprintf("reg_rate_limit:%s", email)
// 	attempts, _ := s.cache.Get(ctx, key)

// 	if attempts != nil {
// 		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 registration attempts per hour
// 			return NewRateLimitError("too many registration attempts", map[string]interface{}{
// 				"retry_after": "1 hour",
// 			})
// 		}
// 	}

// 	// Increment counter
// 	s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, 1*time.Hour)

// 	return nil
// }

// func (s *authService) checkPasswordResetRateLimit(ctx context.Context, email string) error {
// 	key := fmt.Sprintf("reset_rate_limit:%s", email)
// 	attempts, _ := s.cache.Get(ctx, key)

// 	if attempts != nil {
// 		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 reset attempts per hour
// 			return NewRateLimitError("too many password reset attempts", map[string]interface{}{
// 				"retry_after": "1 hour",
// 			})
// 		}
// 	}

// 	s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, 1*time.Hour)

// 	return nil
// }

// func (s *authService) checkAccountLockout(ctx context.Context, login string) error {
// 	if !s.lockoutConfig.EnableLockout {
// 		return nil
// 	}

// 	lockoutKey := fmt.Sprintf("lockout:%s", login)
// 	if locked, found := s.cache.Get(ctx, lockoutKey); found && locked.(bool) {
// 		return NewAuthenticationError("account temporarily locked", "account_locked", nil, login)
// 	}

// 	return nil
// }

// func (s *authService) recordFailedAttempt(ctx context.Context, login, reason string) {
// 	if !s.lockoutConfig.EnableLockout {
// 		return
// 	}

// 	key := fmt.Sprintf("failed_attempts:%s", login)
// 	attempts, _ := s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, s.lockoutConfig.WindowTime)

// 	if attempts >= int64(s.lockoutConfig.MaxAttempts) {
// 		lockoutKey := fmt.Sprintf("lockout:%s", login)
// 		s.cache.Set(ctx, lockoutKey, true, s.lockoutConfig.LockoutTime)

// 		s.logger.Warn("Account locked due to failed attempts",
// 			zap.String("login", login),
// 			zap.String("reason", reason),
// 			zap.Int64("attempts", attempts),
// 		)
// 	}
// }

// func (s *authService) clearFailedAttempts(ctx context.Context, login string) {
// 	key := fmt.Sprintf("failed_attempts:%s", login)
// 	s.cache.Delete(ctx, key)

// 	lockoutKey := fmt.Sprintf("lockout:%s", login)
// 	s.cache.Delete(ctx, lockoutKey)
// }

// // file: internal/services/auth_service.go
// package services

// import (
// 	"context"
// 	"crypto/rand"
// 	"encoding/base64"
// 	"evalhub/internal/cache"
// 	"evalhub/internal/events"
// 	"evalhub/internal/models"
// 	"evalhub/internal/repositories"
// 	"fmt"
// 	"strings"
// 	"time"

// 	"github.com/go-playground/validator/v10"
// 	"go.uber.org/zap"
// 	"golang.org/x/crypto/bcrypt"
// )

// var validateS = validator.New()

// // authService implements AuthService with enterprise features
// type authService struct {
// 	userRepo    repositories.UserRepository
// 	sessionRepo repositories.SessionRepository
// 	cache       cache.Cache
// 	events      events.EventBus
// 	userService UserService
// 	logger      *zap.Logger
// 	validate    *validator.Validate
// 	sessionTTL    time.Duration
// 	bcryptCost    int
// 	maxSessions   int
// 	lockoutConfig *LockoutConfig
// }

// // Request/Response types for authentication service

// type RefreshTokenRequest struct {
// 	RefreshToken string `json:"refresh_token" validate:"required"`
// }

// type LogoutRequest struct {
// 	SessionToken string `json:"session_token" validate:"required"`
// }

// type ForgotPasswordRequest struct {
// 	Email string `json:"email" validate:"required,email"`
// }

// type ResetPasswordRequest struct {
// 	Token           string `json:"token" validate:"required"`
// 	NewPassword     string `json:"new_password" validate:"required,min=8"`
// 	ConfirmPassword string `json:"confirm_password" validate:"required,min=8"`
// }

// type TwoFactorSetupResponse struct {
// 	Secret string `json:"secret"`
// 	QRCode string `json:"qr_code"`
// }

// type DisableTwoFactorRequest struct {
// 	UserID int64  `json:"user_id" validate:"required"`
// 	Token  string `json:"token" validate:"required"`
// }

// type VerifyTwoFactorRequest struct {
// 	UserID int64  `json:"user_id" validate:"required"`
// 	Token  string `json:"token" validate:"required"`
// }

// // VerifyEmailRequest represents a request to verify an email address
// type VerifyEmailRequest struct {
// 	Token string `json:"token" validate:"required"`
// }

// // LockoutConfig holds account lockout configuration
// type LockoutConfig struct {
// 	MaxAttempts    int           `json:"max_attempts"`
// 	LockoutTime    time.Duration `json:"lockout_time"`
// 	WindowTime     time.Duration `json:"window_time"`
// 	EnableLockout  bool          `json:"enable_lockout"`
// }

// // AuthConfig holds authentication service configuration
// type AuthConfig struct {
// 	SessionTTL    time.Duration  `json:"session_ttl"`
// 	BCryptCost    int            `json:"bcrypt_cost"`
// 	MaxSessions   int            `json:"max_sessions"`
// 	LockoutConfig *LockoutConfig `json:"lockout_config"`
// }

// // DefaultAuthConfig returns default authentication configuration
// func DefaultAuthConfig() *AuthConfig {
// 	return &AuthConfig{
// 		SessionTTL:  24 * time.Hour,
// 		BCryptCost:  12,
// 		MaxSessions: 5,
// 		LockoutConfig: &LockoutConfig{
// 			MaxAttempts:   5,
// 			LockoutTime:   15 * time.Minute,
// 			WindowTime:    1 * time.Hour,
// 			EnableLockout: true,
// 		},
// 	}
// }

// // NewAuthService creates a new enterprise authentication service
// func NewAuthService(
// 	userRepo repositories.UserRepository,
// 	sessionRepo repositories.SessionRepository,
// 	cache cache.Cache,
// 	events events.EventBus,
// 	userService UserService,
// 	logger *zap.Logger,
// 	config *AuthConfig,
// ) AuthService {
// 	// Initialize validator
// 	validate := validator.New()
// 	if config == nil {
// 		config = DefaultAuthConfig()
// 	}

// 	return &authService{
// 		userRepo:     userRepo,
// 		sessionRepo:   sessionRepo,
// 		cache:         cache,
// 		events:        events,
// 		userService:   userService,
// 		logger:        logger,
// 		validate:      validate,
// 		sessionTTL:    config.SessionTTL,
// 		bcryptCost:    config.BCryptCost,
// 		maxSessions:   config.MaxSessions,
// 		lockoutConfig: config.LockoutConfig,
// 	}
// }

// // ===============================
// // AUTHENTICATION
// // ===============================

// // Register creates a new user account with validation and security checks
// func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
// 	// Validate request
// 	if err := s.validate.Struct(req); err != nil {
// 		return nil, NewValidationError("invalid registration request", err)
// 	}

// 	// Additional validation
// 	if req.Password != req.ConfirmPassword {
// 		return nil, NewValidationError("passwords do not match", nil)
// 	}

// 	if !req.AcceptTerms {
// 		return nil, NewValidationError("must accept terms and conditions", nil)
// 	}

// 	// Check for rate limiting
// 	if err := s.checkRegistrationRateLimit(ctx, req.Email); err != nil {
// 		return nil, err
// 	}

// 	// Create user through user service
// 	createUserReq := &CreateUserRequest{
// 		Email:           req.Email,
// 		Username:        req.Username,
// 		Password:        req.Password,
// 		FirstName:       &req.FirstName,
// 		LastName:        &req.LastName,
// 		AcceptTerms:     req.AcceptTerms,
// 		MarketingEmails: false, // Default to false for privacy
// 	}

// 	user, err := s.userService.CreateUser(ctx, createUserReq)
// 	if err != nil {
// 		s.logger.Error("Failed to create user during registration",
// 			zap.Error(err),
// 			zap.String("email", req.Email),
// 			zap.String("username", req.Username),
// 		)
// 		return nil, err
// 	}

// 	// Create initial session
// 	sessionToken, err := s.generateSessionToken()
// 	if err != nil {
// 		return nil, NewInternalError("failed to generate session token")
// 	}

// 	session := &models.Session{
// 		UserID:       user.ID,
// 		SessionToken: sessionToken,
// 		ExpiresAt:    time.Now().Add(s.sessionTTL),
// 	}

// 	if err := s.sessionRepo.Create(ctx, session); err != nil {
// 		s.logger.Error("Failed to create session during registration",
// 			zap.Error(err),
// 			zap.Int64("user_id", user.ID),
// 		)
// 		return nil, NewInternalError("failed to create session")
// 	}

// 	go func() {
// 		if err := s.SendVerificationEmail(context.Background(), user.ID); err != nil {
// 			s.logger.Error("Failed to send verification email",
// 				zap.Error(err),
// 				zap.Int64("user_id", user.ID))
// 		}
// 	}()

// 	// Publish registration event
// 	if err := s.events.Publish(ctx, events.NewUserCreatedEvent(user.ID, user.Email, user.Username)); err != nil {
// 		s.logger.Warn("Failed to publish user created event", zap.Error(err))
// 	}

// 	s.logger.Info("User registered successfully",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("email", user.Email),
// 		zap.String("username", user.Username),
// 	)

// 	return &AuthResponse{
// 		User:         user,
// 		AccessToken:  sessionToken,
// 		ExpiresIn:    int64(s.sessionTTL.Seconds()),
// 		TokenType:    "Bearer",
// 	}, nil
// }

// // Login authenticates a user and creates a session
// func (s *authService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
// 	// Validate request
// 	if err := s.validate.Struct(req); err != nil {
// 		return nil, NewValidationError("invalid login request", err)
// 	}

// 	// Check for account lockout
// 	if err := s.checkAccountLockout(ctx, req.Login); err != nil {
// 		return nil, err
// 	}

// 	// Find user by email or username
// 	var user *models.User
// 	var err error

// 	if strings.Contains(req.Login, "@") {
// 		user, err = s.userRepo.GetByEmail(ctx, req.Login)
// 	} else {
// 		user, err = s.userRepo.GetByUsername(ctx, req.Login)
// 	}

// 	if err != nil {
// 		s.logger.Error("Failed to get user during login", zap.Error(err), zap.String("login", req.Login))
// 		return nil, NewInternalError("authentication failed")
// 	}

// 	if user == nil {
// 		// Record failed attempt
// 		s.recordFailedAttempt(ctx, req.Login, "user_not_found")
// 		return nil, NewAuthenticationError("invalid credentials", "invalid_login", nil, req.Login)
// 	}

// 	// Check if user is active
// 	if !user.IsActive {
// 		s.recordFailedAttempt(ctx, req.Login, "account_deactivated")
// 		return nil, NewAuthenticationError("account is deactivated", "account_deactivated", &user.ID, user.Username)
// 	}

// 	// Verify password
// 	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
// 		s.recordFailedAttempt(ctx, req.Login, "invalid_password")
// 		s.logger.Warn("Invalid password attempt",
// 			zap.Int64("user_id", user.ID),
// 			zap.String("username", user.Username),
// 		)
// 		return nil, NewAuthenticationError("invalid credentials", "invalid_password", &user.ID, user.Username)
// 	}

// 	// Clear failed attempts on successful login
// 	s.clearFailedAttempts(ctx, req.Login)

// 	// Manage existing sessions
// 	if err := s.manageUserSessions(ctx, user.ID); err != nil {
// 		s.logger.Warn("Failed to manage user sessions", zap.Error(err), zap.Int64("user_id", user.ID))
// 	}

// 	// Create new session
// 	sessionToken, err := s.generateSessionToken()
// 	if err != nil {
// 		return nil, NewInternalError("failed to generate session token")
// 	}

// 	sessionTTL := s.sessionTTL
// 	if req.Remember {
// 		sessionTTL = 30 * 24 * time.Hour // 30 days for "remember me"
// 	}

// 	session := &models.Session{
// 		UserID:       user.ID,
// 		SessionToken: sessionToken,
// 		ExpiresAt:    time.Now().Add(sessionTTL),
// 	}

// 	if err := s.sessionRepo.Create(ctx, session); err != nil {
// 		s.logger.Error("Failed to create session during login", zap.Error(err), zap.Int64("user_id", user.ID))
// 		return nil, NewInternalError("failed to create session")
// 	}

// 	// Update user's online status and last seen
// 	if err := s.userService.UpdateOnlineStatus(ctx, user.ID, true); err != nil {
// 		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", user.ID))
// 	}

// 	// Publish login event
// 	if err := s.events.Publish(ctx, &events.UserLoggedInEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.logged_in",
// 			Timestamp: time.Now(),
// 			UserID:    &user.ID,
// 		},
// 		LoginAt: time.Now(),
// 		// IPAddress and UserAgent would come from HTTP context
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish login event", zap.Error(err))
// 	}

// 	s.logger.Info("User logged in successfully",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("username", user.Username),
// 		zap.Bool("remember", req.Remember),
// 	)

// 	// Clear password hash before returning
// 	user.PasswordHash = ""

// 	return &AuthResponse{
// 		User:         user,
// 		AccessToken:  sessionToken,
// 		ExpiresIn:    int64(sessionTTL.Seconds()),
// 		TokenType:    "Bearer",
// 	}, nil
// }

// // LoginWithProvider handles OAuth provider login
// func (s *authService) LoginWithProvider(ctx context.Context, req *OAuthLoginRequest) (*AuthResponse, error) {
// 	if err := s.validate.Struct(req); err != nil {
// 		return nil, NewValidationError("invalid OAuth login request", err)
// 	}

// 	// This would integrate with OAuth providers (Google, GitHub, etc.)
// 	// For now, return a not implemented error
// 	return nil, NewNotImplementedError("OAuth login not implemented")
// }

// // RefreshToken refreshes an access token
// func (s *authService) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error) {
// 	// This would implement token refresh logic
// 	// For now, return a not implemented error
// 	return nil, NewNotImplementedError("token refresh not implemented")
// }

// // Logout invalidates a session
// func (s *authService) Logout(ctx context.Context, req *LogoutRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid logout request", err)
// 	}

// 	// Get session to get user ID for events
// 	session, err := s.sessionRepo.GetByToken(ctx, req.SessionToken)
// 	if err != nil {
// 		s.logger.Warn("Failed to get session during logout", zap.Error(err))
// 		// Still try to delete the session
// 	}

// 	// Delete session
// 	if err := s.sessionRepo.Delete(ctx, req.SessionToken); err != nil {
// 		s.logger.Error("Failed to delete session during logout", zap.Error(err))
// 		return NewInternalError("failed to logout")
// 	}

// 	// Update online status if we have session info
// 	if session != nil {
// 		if err := s.userService.UpdateOnlineStatus(ctx, session.UserID, false); err != nil {
// 			s.logger.Warn("Failed to update online status during logout", zap.Error(err))
// 		}

// 		// Publish logout event
// 		if err := s.events.Publish(ctx, &events.UserLoggedOutEvent{
// 			BaseEvent: events.BaseEvent{
// 				EventID:   events.GenerateEventID(),
// 				EventType: "user.logged_out",
// 				Timestamp: time.Now(),
// 				UserID:    &session.UserID,
// 			},
// 			LogoutAt: time.Now(),
// 		}); err != nil {
// 			s.logger.Warn("Failed to publish logout event", zap.Error(err))
// 		}

// 		s.logger.Info("User logged out successfully", zap.Int64("user_id", session.UserID))
// 	}

// 	return nil
// }

// // LogoutAllDevices invalidates all sessions for a user
// func (s *authService) LogoutAllDevices(ctx context.Context, userID int64) error {
// 	if userID <= 0 {
// 		return NewValidationError("invalid user ID", nil)
// 	}

// 	// Delete all sessions for the user
// 	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
// 		s.logger.Error("Failed to delete all sessions", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to logout from all devices")
// 	}

// 	// Update online status
// 	if err := s.userService.UpdateOnlineStatus(ctx, userID, false); err != nil {
// 		s.logger.Warn("Failed to update online status", zap.Error(err), zap.Int64("user_id", userID))
// 	}

// 	s.logger.Info("User logged out from all devices", zap.Int64("user_id", userID))

// 	return nil
// }

// // ===============================
// // PASSWORD MANAGEMENT
// // ===============================

// // ForgotPassword initiates password reset process
// func (s *authService) ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid forgot password request", err)
// 	}

// 	// Check rate limiting
// 	if err := s.checkPasswordResetRateLimit(ctx, req.Email); err != nil {
// 		return err
// 	}

// 	// Get user by email
// 	user, err := s.userRepo.GetByEmail(ctx, req.Email)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password reset", zap.Error(err))
// 		return NewInternalError("failed to process password reset")
// 	}

// 	// Always return success to prevent email enumeration
// 	if user == nil {
// 		s.logger.Info("Password reset requested for non-existent email", zap.String("email", req.Email))
// 		return nil
// 	}

// 	// Generate reset token
// 	resetToken, err := s.generateResetToken()
// 	if err != nil {
// 		return NewInternalError("failed to generate reset token")
// 	}

// 	// Store reset token in cache with expiration
// 	resetKey := fmt.Sprintf("password_reset:%s", resetToken)
// 	if err := s.cache.Set(ctx, resetKey, user.ID, 1*time.Hour); err != nil {
// 		s.logger.Error("Failed to store reset token", zap.Error(err))
// 		return NewInternalError("failed to process password reset")
// 	}

// 	// TODO: Send password reset email
// 	s.logger.Info("Password reset token generated",
// 		zap.Int64("user_id", user.ID),
// 		zap.String("email", user.Email),
// 	)

// 	return nil
// }

// // ResetPassword resets a user's password using a reset token
// func (s *authService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid reset password request", err)
// 	}

// 	if req.NewPassword != req.ConfirmPassword {
// 		return NewValidationError("passwords do not match", nil)
// 	}

// 	// Validate reset token
// 	resetKey := fmt.Sprintf("password_reset:%s", req.Token)
// 	userIDInterface, found := s.cache.Get(ctx, resetKey)
// 	if !found {
// 		return NewValidationError("invalid or expired reset token", nil)
// 	}

// 	userID, ok := userIDInterface.(int64)
// 	if !ok {
// 		s.logger.Error("Invalid user ID type in reset token cache")
// 		return NewInternalError("invalid reset token")
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password reset", zap.Error(err))
// 		return NewInternalError("failed to reset password")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	// Hash new password
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
// 	if err != nil {
// 		s.logger.Error("Failed to hash new password", zap.Error(err))
// 		return NewInternalError("failed to reset password")
// 	}

// 	// Update password
// 	user.PasswordHash = string(hashedPassword)
// 	user.PasswordChangedAt = time.Now()

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to reset password")
// 	}

// 	// Delete reset token
// 	s.cache.Delete(ctx, resetKey)

// 	// Invalidate all existing sessions for security
// 	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
// 		s.logger.Warn("Failed to invalidate sessions after password reset", zap.Error(err))
// 	}

// 	// Publish password changed event
// 	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.password_changed",
// 			Timestamp: time.Now(),
// 			UserID:    &userID,
// 		},
// 		ChangedAt: time.Now(),
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
// 	}

// 	s.logger.Info("Password reset successfully", zap.Int64("user_id", userID))

// 	return nil
// }

// // ChangePassword changes a user's password
// func (s *authService) ChangePassword(ctx context.Context, req *ChangePasswordRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid change password request", err)
// 	}

// 	if req.NewPassword != req.ConfirmPassword {
// 		return NewValidationError("passwords do not match", nil)
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, req.UserID)
// 	if err != nil {
// 		s.logger.Error("Failed to get user for password change", zap.Error(err))
// 		return NewInternalError("failed to change password")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	// Verify current password
// 	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
// 		return NewValidationError("current password is incorrect", nil)
// 	}

// 	// Hash new password
// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
// 	if err != nil {
// 		s.logger.Error("Failed to hash new password", zap.Error(err))
// 		return NewInternalError("failed to change password")
// 	}

// 	// Update password
// 	user.PasswordHash = string(hashedPassword)
// 	user.PasswordChangedAt = time.Now()

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update password", zap.Error(err), zap.Int64("user_id", req.UserID))
// 		return NewInternalError("failed to change password")
// 	}

// 	// Publish password changed event
// 	if err := s.events.Publish(ctx, &events.PasswordChangedEvent{
// 		BaseEvent: events.BaseEvent{
// 			EventID:   events.GenerateEventID(),
// 			EventType: "user.password_changed",
// 			Timestamp: time.Now(),
// 			UserID:    &req.UserID,
// 		},
// 		ChangedAt: time.Now(),
// 	}); err != nil {
// 		s.logger.Warn("Failed to publish password changed event", zap.Error(err))
// 	}

// 	s.logger.Info("Password changed successfully", zap.Int64("user_id", req.UserID))

// 	return nil
// }

// // ===============================
// // EMAIL VERIFICATION
// // ===============================

// // SendVerificationEmail sends an email verification link
// func (s *authService) SendVerificationEmail(ctx context.Context, userID int64) error {
// 	if userID <= 0 {
// 		return NewValidationError("invalid user ID", nil)
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		return NewInternalError("failed to get user")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	if user.EmailVerified {
// 		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
// 	}

// 	// Generate verification token
// 	verificationToken, err := s.generateVerificationToken()
// 	if err != nil {
// 		return NewInternalError("failed to generate verification token")
// 	}

// 	// Store verification token
// 	verificationKey := fmt.Sprintf("email_verification:%s", verificationToken)
// 	if err := s.cache.Set(ctx, verificationKey, userID, 24*time.Hour); err != nil {
// 		s.logger.Error("Failed to store verification token", zap.Error(err))
// 		return NewInternalError("failed to send verification email")
// 	}

// 	// TODO: Send verification email
// 	s.logger.Info("Email verification token generated",
// 		zap.Int64("user_id", userID),
// 		zap.String("email", user.Email),
// 	)

// 	return nil
// }

// // VerifyEmail verifies a user's email address
// func (s *authService) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error {
// 	if err := s.validate.Struct(req); err != nil {
// 		return NewValidationError("invalid email verification request", err)
// 	}

// 	// Validate verification token
// 	verificationKey := fmt.Sprintf("email_verification:%s", req.Token)
// 	userIDInterface, found := s.cache.Get(ctx, verificationKey)
// 	if !found {
// 		return NewValidationError("invalid or expired verification token", nil)
// 	}

// 	userID, ok := userIDInterface.(int64)
// 	if !ok {
// 		return NewInternalError("invalid verification token")
// 	}

// 	// Get user
// 	user, err := s.userRepo.GetByID(ctx, userID)
// 	if err != nil {
// 		return NewInternalError("failed to verify email")
// 	}

// 	if user == nil {
// 		return NewNotFoundError("user not found")
// 	}

// 	if user.EmailVerified {
// 		return NewBusinessError("email already verified", "EMAIL_ALREADY_VERIFIED")
// 	}

// 	// Mark email as verified
// 	now := time.Now()
// 	user.EmailVerified = true
// 	user.EmailVerifiedAt = &now

// 	if err := s.userRepo.Update(ctx, user); err != nil {
// 		s.logger.Error("Failed to update email verification status", zap.Error(err), zap.Int64("user_id", userID))
// 		return NewInternalError("failed to verify email")
// 	}

// 	// Delete verification token
// 	s.cache.Delete(ctx, verificationKey)

// 	s.logger.Info("Email verified successfully", zap.Int64("user_id", userID))

// 	return nil
// }

// // ===============================
// // SESSION MANAGEMENT
// // ===============================

// // GetActiveSessions retrieves active sessions for a user
// func (s *authService) GetActiveSessions(ctx context.Context, userID int64) ([]*SessionInfo, error) {
// 	if userID <= 0 {
// 		return nil, NewValidationError("invalid user ID", nil)
// 	}

// 	sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, false)
// 	if err != nil {
// 		s.logger.Error("Failed to get active sessions", zap.Error(err), zap.Int64("user_id", userID))
// 		return nil, NewInternalError("failed to retrieve sessions")
// 	}

// 	var sessionInfos []*SessionInfo
// 	for _, session := range sessions {
// 		sessionInfos = append(sessionInfos, &SessionInfo{
// 			ID:           session.ID,
// 			LastActivity: session.LastActivity,
// 			ExpiresAt:    session.ExpiresAt,
// 			// TODO: Add device and location info from session metadata
// 		})
// 	}

// 	return sessionInfos, nil
// }

// // RevokeSession revokes a specific session
// func (s *authService) RevokeSession(ctx context.Context, sessionID int64, userID int64) error {
// 	if sessionID <= 0 || userID <= 0 {
// 		return NewValidationError("invalid session or user ID", nil)
// 	}

// 	// TODO: Implement session revocation by ID
// 	// This would require adding a method to get session by ID and verify ownership

// 	return NewNotImplementedError("session revocation by ID not implemented")
// }

// // ===============================
// // TWO-FACTOR AUTHENTICATION (Placeholder)
// // ===============================

// // EnableTwoFactor enables two-factor authentication
// func (s *authService) EnableTwoFactor(ctx context.Context, userID int64) (*TwoFactorSetupResponse, error) {
// 	return nil, NewNotImplementedError("two-factor authentication not implemented")
// }

// // DisableTwoFactor disables two-factor authentication
// func (s *authService) DisableTwoFactor(ctx context.Context, req *DisableTwoFactorRequest) error {
// 	return NewNotImplementedError("two-factor authentication not implemented")
// }

// // VerifyTwoFactor verifies a two-factor authentication code
// func (s *authService) VerifyTwoFactor(ctx context.Context, req *VerifyTwoFactorRequest) error {
// 	return NewNotImplementedError("two-factor authentication not implemented")
// }

// // ===============================
// // HELPER METHODS
// // ===============================

// // generateSessionToken generates a secure session token
// func (s *authService) generateSessionToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // generateResetToken generates a secure reset token
// func (s *authService) generateResetToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // generateVerificationToken generates a secure verification token
// func (s *authService) generateVerificationToken() (string, error) {
// 	bytes := make([]byte, 32)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return base64.URLEncoding.EncodeToString(bytes), nil
// }

// // manageUserSessions manages the number of active sessions per user
// func (s *authService) manageUserSessions(ctx context.Context, userID int64) error {
// 	activeCount, err := s.sessionRepo.CountActiveSessions(ctx, userID)
// 	if err != nil {
// 		return err
// 	}

// 	if activeCount >= s.maxSessions {
// 		// Remove oldest sessions to make room
// 		sessions, err := s.sessionRepo.GetActiveSessions(ctx, userID, false)
// 		if err != nil {
// 			return err
// 		}

// 		// Sort by last activity and remove oldest
// 		sessionsToRemove := len(sessions) - s.maxSessions + 1
// 		if sessionsToRemove > 0 {
// 			// This would require implementing session removal by criteria
// 			s.logger.Info("Removing oldest sessions",
// 				zap.Int64("user_id", userID),
// 				zap.Int("sessions_to_remove", sessionsToRemove),
// 			)
// 		}
// 	}

// 	return nil
// }

// // Rate limiting and lockout methods
// func (s *authService) checkRegistrationRateLimit(ctx context.Context, email string) error {
// 	if !s.lockoutConfig.EnableLockout {
// 		return nil
// 	}

// 	key := fmt.Sprintf("reg_rate_limit:%s", email)
// 	attempts, _ := s.cache.Get(ctx, key)

// 	if attempts != nil {
// 		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 registration attempts per hour
// 			return NewRateLimitError("too many registration attempts", map[string]interface{}{
// 				"retry_after": "1 hour",
// 			})
// 		}
// 	}

// 	// Increment counter
// 	s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, 1*time.Hour)

// 	return nil
// }

// func (s *authService) checkPasswordResetRateLimit(ctx context.Context, email string) error {
// 	key := fmt.Sprintf("reset_rate_limit:%s", email)
// 	attempts, _ := s.cache.Get(ctx, key)

// 	if attempts != nil {
// 		if count, ok := attempts.(int); ok && count >= 3 { // Max 3 reset attempts per hour
// 			return NewRateLimitError("too many password reset attempts", map[string]interface{}{
// 				"retry_after": "1 hour",
// 			})
// 		}
// 	}

// 	s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, 1*time.Hour)

// 	return nil
// }

// func (s *authService) checkAccountLockout(ctx context.Context, login string) error {
// 	if !s.lockoutConfig.EnableLockout {
// 		return nil
// 	}

// 	lockoutKey := fmt.Sprintf("lockout:%s", login)
// 	if locked, found := s.cache.Get(ctx, lockoutKey); found && locked.(bool) {
// 		return NewAuthenticationError("account temporarily locked", "account_locked", nil, login)
// 	}

// 	return nil
// }

// func (s *authService) recordFailedAttempt(ctx context.Context, login, reason string) {
// 	if !s.lockoutConfig.EnableLockout {
// 		return
// 	}

// 	key := fmt.Sprintf("failed_attempts:%s", login)
// 	attempts, _ := s.cache.Increment(ctx, key, 1)
// 	s.cache.SetTTL(ctx, key, s.lockoutConfig.WindowTime)

// 	if attempts >= int64(s.lockoutConfig.MaxAttempts) {
// 		lockoutKey := fmt.Sprintf("lockout:%s", login)
// 		s.cache.Set(ctx, lockoutKey, true, s.lockoutConfig.LockoutTime)

// 		s.logger.Warn("Account locked due to failed attempts",
// 			zap.String("login", login),
// 			zap.String("reason", reason),
// 			zap.Int64("attempts", attempts),
// 		)
// 	}
// }

// func (s *authService) clearFailedAttempts(ctx context.Context, login string) {
// 	key := fmt.Sprintf("failed_attempts:%s", login)
// 	s.cache.Delete(ctx, key)

// 	lockoutKey := fmt.Sprintf("lockout:%s", login)
// 	s.cache.Delete(ctx, lockoutKey)
// }
