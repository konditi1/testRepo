// file: internal/middleware/auth.go
package middleware

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"evalhub/internal/cache"
	"evalhub/internal/contextutils"
	"evalhub/internal/models"
	"evalhub/internal/repositories"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// AuthConfig holds authentication middleware configuration
type AuthConfig struct {
	// JWT Configuration
	JWTSecret           string        `json:"jwt_secret"`
	JWTPublicKey        string        `json:"jwt_public_key"`  // RSA public key for JWT verification
	JWTPrivateKey       string        `json:"jwt_private_key"` // RSA private key for JWT signing
	JWTExpiration       time.Duration `json:"jwt_expiration"`
	JWTRefreshThreshold time.Duration `json:"jwt_refresh_threshold"` // Refresh token if expires within this time

	// Session Configuration
	SessionName       string        `json:"session_name"`
	SessionExpiration time.Duration `json:"session_expiration"`
	CookieSecure      bool          `json:"cookie_secure"`
	CookieHTTPOnly    bool          `json:"cookie_http_only"`
	CookieSameSite    http.SameSite `json:"cookie_same_site"`

	// Authentication Methods
	EnableJWT      bool `json:"enable_jwt"`
	EnableSessions bool `json:"enable_sessions"`
	EnableOAuth    bool `json:"enable_oauth"`
	EnableAPIKeys  bool `json:"enable_api_keys"`

	// Security
	EnableCSRF           bool          `json:"enable_csrf"`
	CSRFExpiration       time.Duration `json:"csrf_expiration"`
	RequireEmailVerified bool          `json:"require_email_verified"`

	// Performance
	CacheUserData bool          `json:"cache_user_data"`
	UserCacheTTL  time.Duration `json:"user_cache_ttl"`

	// Audit
	LogSuccessfulAuth   bool `json:"log_successful_auth"`
	LogFailedAuth       bool `json:"log_failed_auth"`
	LogPermissionChecks bool `json:"log_permission_checks"`
}

// DefaultAuthConfig returns production-ready authentication configuration
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		JWTExpiration:        24 * time.Hour,
		JWTRefreshThreshold:  4 * time.Hour,
		SessionName:          "evalhub_session",
		SessionExpiration:    24 * time.Hour,
		CookieSecure:         true,
		CookieHTTPOnly:       true,
		CookieSameSite:       http.SameSiteStrictMode,
		EnableJWT:            true,
		EnableSessions:       true,
		EnableOAuth:          true,
		EnableAPIKeys:        false,
		EnableCSRF:           true,
		CSRFExpiration:       1 * time.Hour,
		RequireEmailVerified: false, // Set to true in production if needed
		CacheUserData:        true,
		UserCacheTTL:         15 * time.Minute,
		LogSuccessfulAuth:    true,
		LogFailedAuth:        true,
		LogPermissionChecks:  false, // Set to true for detailed audit logs
	}
}

// AuthResult represents the result of authentication
type AuthResult struct {
	Authenticated bool         `json:"authenticated"`
	User          *models.User `json:"user,omitempty"`
	SessionID     string       `json:"session_id,omitempty"`
	TokenType     string       `json:"token_type,omitempty"` // "jwt", "session", "oauth", "api_key"
	ExpiresAt     time.Time    `json:"expires_at,omitempty"`
	Permissions   []string     `json:"permissions,omitempty"`
	Error         string       `json:"error,omitempty"`
}

// AuthContext holds authentication context for requests
type AuthContext struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	SessionID   string    `json:"session_id"`
	AuthMethod  string    `json:"auth_method"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsActive    bool      `json:"is_active"`
	IsVerified  bool      `json:"is_verified"`
}

// AuthMiddleware provides enterprise authentication
type AuthMiddleware struct {
	config        *AuthConfig
	cache         cache.Cache
	sessionRepo   repositories.SessionRepository
	userRepo      repositories.UserRepository
	authService   services.AuthService
	logger        *zap.Logger
	jwtPrivateKey *rsa.PrivateKey
	jwtPublicKey  *rsa.PublicKey
}

// NewAuthMiddleware creates enterprise authentication middleware
func NewAuthMiddleware(
	config *AuthConfig,
	cache cache.Cache,
	sessionRepo repositories.SessionRepository,
	userRepo repositories.UserRepository,
	authService services.AuthService,
	logger *zap.Logger,
) (*AuthMiddleware, error) {
	if config == nil {
		config = DefaultAuthConfig()
	}

	auth := &AuthMiddleware{
		config:      config,
		cache:       cache,
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
		authService: authService,
		logger:      logger,
	}

	// Initialize JWT keys if JWT is enabled
	if config.EnableJWT {
		if err := auth.initializeJWTKeys(); err != nil {
			return nil, fmt.Errorf("failed to initialize JWT keys: %w", err)
		}
	}

	return auth, nil
}

// ===============================
// MAIN AUTHENTICATION MIDDLEWARE
// ===============================

// Authenticate provides comprehensive authentication middleware
func (am *AuthMiddleware) Authenticate(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestLogger := GetRequestLogger(ctx)
			requestID := GetRequestID(ctx)

			// Attempt authentication using multiple methods
			authResult := am.authenticateRequest(r)

			// Handle authentication result
			if authResult.Authenticated {
				// Authentication successful
				if am.config.LogSuccessfulAuth {
					requestLogger.Info("Authentication successful",
						zap.Int64("user_id", authResult.User.ID),
						zap.String("username", authResult.User.Username),
						zap.String("auth_method", authResult.TokenType),
						zap.String("request_id", requestID),
					)
				}

				// Create auth context
				authCtx := &AuthContext{
					UserID:      authResult.User.ID,
					Username:    authResult.User.Username,
					Email:       authResult.User.Email,
					Role:        authResult.User.Role,
					Permissions: authResult.Permissions,
					SessionID:   authResult.SessionID,
					AuthMethod:  authResult.TokenType,
					ExpiresAt:   authResult.ExpiresAt,
					IsActive:    authResult.User.IsActive,
					IsVerified:  authResult.User.EmailVerified,
				}

				// Inject auth context into request
				ctx = context.WithValue(ctx, AuthContextKey, authCtx)
				ctx = context.WithValue(ctx, UserIDKey, authResult.User.ID)
				ctx = context.WithValue(ctx, UserKey, authResult.User)

				// Update user's last seen and online status
				go am.updateUserActivity(context.Background(), authResult.User.ID)

				next.ServeHTTP(w, r.WithContext(ctx))

			} else if required {
				// Authentication required but failed
				if am.config.LogFailedAuth {
					requestLogger.Warn("Authentication required but failed",
						zap.String("error", authResult.Error),
						zap.String("path", r.URL.Path),
						zap.String("request_id", requestID),
					)
				}

				am.writeAuthError(w, "Authentication required", http.StatusUnauthorized)
				return

			} else {
				// Authentication not required, proceed without auth context
				next.ServeHTTP(w, r)
			}
		})
	}
}

// RequireAuth requires authentication for the endpoint
func (am *AuthMiddleware) RequireAuth() func(http.Handler) http.Handler {
	return am.Authenticate(true)
}

// OptionalAuth provides optional authentication for the endpoint
func (am *AuthMiddleware) OptionalAuth() func(http.Handler) http.Handler {
	return am.Authenticate(false)
}

// ===============================
// AUTHORIZATION MIDDLEWARE
// ===============================

// Permission represents a system permission
type Permission struct {
	Resource string `json:"resource"` // "posts", "users", "admin", etc.
	Action   string `json:"action"`   // "create", "read", "update", "delete", "moderate"
	Scope    string `json:"scope"`    // "own", "any", "team", etc.
}

// RequirePermission requires specific permissions
func (am *AuthMiddleware) RequirePermission(permission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestLogger := GetRequestLogger(ctx)

			// Get auth context
			authCtx := GetAuthContext(ctx)
			if authCtx == nil {
				am.logger.Error("Authentication required", zap.String("path", r.URL.Path))
				am.writeAuthError(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check if user has required permission
			hasPermission := am.checkUserPermission(authCtx, permission, r)

			if am.config.LogPermissionChecks {
				requestLogger.Info("Permission check",
					zap.Int64("user_id", authCtx.UserID),
					zap.String("permission", fmt.Sprintf("%s:%s:%s", permission.Resource, permission.Action, permission.Scope)),
					zap.Bool("granted", hasPermission),
				)
			}

			if !hasPermission {
				requestLogger.Warn("Permission denied",
					zap.Int64("user_id", authCtx.UserID),
					zap.String("role", authCtx.Role),
					zap.String("required_permission", fmt.Sprintf("%s:%s:%s", permission.Resource, permission.Action, permission.Scope)),
				)

				am.writeAuthError(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole requires specific user role
func (am *AuthMiddleware) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r.Context())
			if authCtx == nil {
				am.logger.Error("Authentication required", zap.String("path", r.URL.Path))
				am.writeAuthError(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check if user has required role
			hasRole := false
			for _, role := range roles {
				if authCtx.Role == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				am.writeAuthError(w, "Insufficient role", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwnership requires ownership of a resource
func (am *AuthMiddleware) RequireOwnership(resourceType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r.Context())
			if authCtx == nil {
				am.logger.Error("Authentication required", zap.String("path", r.URL.Path))
				am.writeAuthError(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Check ownership
			isOwner, err := am.checkResourceOwnership(r, authCtx, resourceType)
			if err != nil {
				am.logger.Error("Failed to check resource ownership", zap.Error(err))
				am.writeAuthError(w, "Internal error", http.StatusInternalServerError)
				return
			}

			if !isOwner {
				am.writeAuthError(w, "Resource not owned by user", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireEmailVerified requires email verification
func (am *AuthMiddleware) RequireEmailVerified() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r.Context())
			if authCtx == nil {
				am.logger.Error("Authentication required", zap.String("path", r.URL.Path))
				am.writeAuthError(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			if !authCtx.IsVerified {
				am.writeAuthError(w, "Email verification required", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ===============================
// AUTHENTICATION METHODS
// ===============================

// authenticateRequest attempts authentication using multiple methods
func (am *AuthMiddleware) authenticateRequest(r *http.Request) *AuthResult {
	// Try JWT authentication first
	if am.config.EnableJWT {
		if result := am.authenticateJWT(r); result.Authenticated {
			return result
		}
	}

	// Try session authentication
	if am.config.EnableSessions {
		if result := am.authenticateSession(r); result.Authenticated {
			return result
		}
	}

	// Try OAuth authentication
	if am.config.EnableOAuth {
		if result := am.authenticateOAuth(r); result.Authenticated {
			return result
		}
	}

	// Try API key authentication
	if am.config.EnableAPIKeys {
		if result := am.authenticateAPIKey(r); result.Authenticated {
			return result
		}
	}

	return &AuthResult{
		Authenticated: false,
		Error:         "No valid authentication found",
	}
}

// authenticateJWT handles JWT authentication
func (am *AuthMiddleware) authenticateJWT(r *http.Request) *AuthResult {
	// Get JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return &AuthResult{Authenticated: false, Error: "No authorization header"}
	}

	// Parse Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return &AuthResult{Authenticated: false, Error: "Invalid authorization header format"}
	}

	tokenString := parts[1]

	// Parse and validate JWT token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtPublicKey, nil
	})

	if err != nil {
		return &AuthResult{Authenticated: false, Error: "Invalid JWT token"}
	}

	if !token.Valid {
		return &AuthResult{Authenticated: false, Error: "JWT token not valid"}
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return &AuthResult{Authenticated: false, Error: "Invalid JWT claims"}
	}

	// Get user ID from claims
	userIDFloat, ok := claims["sub"].(float64)
	if !ok {
		return &AuthResult{Authenticated: false, Error: "Invalid user ID in JWT"}
	}
	userID := int64(userIDFloat)

	// Get user from cache or database
	user, err := am.getUserFromCacheOrDB(context.Background(), userID)
	if err != nil {
		return &AuthResult{Authenticated: false, Error: "User not found"}
	}

	// Check if user is active
	if !user.IsActive {
		return &AuthResult{Authenticated: false, Error: "User account is inactive"}
	}

	// Get expiration time
	exp, ok := claims["exp"].(float64)
	var expiresAt time.Time
	if ok {
		expiresAt = time.Unix(int64(exp), 0)
	}

	// Get user permissions
	permissions := am.getUserPermissions(user)

	return &AuthResult{
		Authenticated: true,
		User:          user,
		TokenType:     "jwt",
		ExpiresAt:     expiresAt,
		Permissions:   permissions,
	}
}

// authenticateSession handles session-based authentication
func (am *AuthMiddleware) authenticateSession(r *http.Request) *AuthResult {
	var sessionToken string

	// Try Authorization header first (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			sessionToken = parts[1]
		}
	}

	// Fallback to cookie if no Bearer token
	if sessionToken == "" {
		cookie, err := r.Cookie(am.config.SessionName)
		if err != nil || cookie.Value == "" {
			return &AuthResult{Authenticated: false, Error: "No session token or cookie"}
		}
		sessionToken = cookie.Value
	}

	// Get session from database
	ctx := context.Background()
	session, err := am.sessionRepo.GetByToken(ctx, sessionToken)
	if err != nil {
		return &AuthResult{Authenticated: false, Error: "Invalid session token"}
	}

	if session == nil {
		return &AuthResult{Authenticated: false, Error: "Session not found"}
	}

	// Check session expiration
	if session.IsExpired() {
		return &AuthResult{Authenticated: false, Error: "Session expired"}
	}

	// Get user
	user, err := am.getUserFromCacheOrDB(ctx, session.UserID)
	if err != nil {
		return &AuthResult{Authenticated: false, Error: "User not found"}
	}

	// Check if user is active
	if !user.IsActive {
		return &AuthResult{Authenticated: false, Error: "User account is inactive"}
	}

	// Update session activity
	go am.refreshSessionActivity(context.Background(), sessionToken)

	// Get user permissions
	permissions := am.getUserPermissions(user)

	return &AuthResult{
		Authenticated: true,
		User:          user,
		SessionID:     sessionToken,
		TokenType:     "session",
		ExpiresAt:     session.ExpiresAt,
		Permissions:   permissions,
	}
}

// authenticateOAuth handles OAuth token authentication
func (am *AuthMiddleware) authenticateOAuth(r *http.Request) *AuthResult {
	// This would implement OAuth token validation
	// For now, return not authenticated
	return &AuthResult{Authenticated: false, Error: "OAuth not implemented"}
}

// authenticateAPIKey handles API key authentication
func (am *AuthMiddleware) authenticateAPIKey(r *http.Request) *AuthResult {
	// This would implement API key validation
	// For now, return not authenticated
	return &AuthResult{Authenticated: false, Error: "API key authentication not implemented"}
}

// ===============================
// PERMISSION SYSTEM
// ===============================

// checkUserPermission checks if user has required permission
func (am *AuthMiddleware) checkUserPermission(authCtx *AuthContext, permission Permission, r *http.Request) bool {
	// Admin role has all permissions
	if authCtx.Role == "admin" {
		return true
	}

	// Check role-based permissions
	rolePermissions := am.getRolePermissions(authCtx.Role)

	// Check if user has the specific permission
	requiredPerm := fmt.Sprintf("%s:%s:%s", permission.Resource, permission.Action, permission.Scope)

	for _, userPerm := range rolePermissions {
		if userPerm == requiredPerm || userPerm == "*" {
			return true
		}

		// Check wildcard permissions
		if am.matchesWildcardPermission(userPerm, requiredPerm) {
			return true
		}
	}

	// Check resource ownership for "own" scope
	if permission.Scope == "own" {
		isOwner, err := am.checkResourceOwnership(r, authCtx, permission.Resource)
		if err != nil {
			am.logger.Warn("Failed to check resource ownership", zap.Error(err))
			return false
		}
		return isOwner
	}

	return false
}

// getRolePermissions returns permissions for a given role
func (am *AuthMiddleware) getRolePermissions(role string) []string {
	switch role {
	case "admin":
		return []string{"*"} // Admin has all permissions
	case "moderator":
		return []string{
			"posts:read:any", "posts:update:any", "posts:delete:any", "posts:moderate:any",
			"comments:read:any", "comments:update:any", "comments:delete:any", "comments:moderate:any",
			"users:read:any", "users:moderate:any",
			"reports:read:any", "reports:handle:any",
		}
	case "reviewer":
		return []string{
			"posts:read:any", "posts:review:any",
			"comments:read:any", "comments:review:any",
			"applications:read:any", "applications:review:any",
		}
	case "user":
		return []string{
			"posts:create:own", "posts:read:any", "posts:update:own", "posts:delete:own",
			"comments:create:own", "comments:read:any", "comments:update:own", "comments:delete:own",
			"profile:read:own", "profile:update:own",
			"jobs:create:own", "jobs:read:any", "jobs:update:own", "jobs:delete:own",
			"applications:create:own", "applications:read:own",
		}
	default:
		return []string{
			"posts:read:any",
			"comments:read:any",
			"jobs:read:any",
		}
	}
}

// matchesWildcardPermission checks if a wildcard permission matches the required permission
func (am *AuthMiddleware) matchesWildcardPermission(userPerm, requiredPerm string) bool {
	userParts := strings.Split(userPerm, ":")
	requiredParts := strings.Split(requiredPerm, ":")

	if len(userParts) != 3 || len(requiredParts) != 3 {
		return false
	}

	// Check each part (resource:action:scope)
	for i := 0; i < 3; i++ {
		if userParts[i] != "*" && userParts[i] != requiredParts[i] {
			return false
		}
	}

	return true
}

// getUserPermissions gets all permissions for a user
func (am *AuthMiddleware) getUserPermissions(user *models.User) []string {
	return am.getRolePermissions(user.Role)
}

// checkResourceOwnership checks if user owns a specific resource
func (am *AuthMiddleware) checkResourceOwnership(r *http.Request, authCtx *AuthContext, resourceType string) (bool, error) {
	// Extract resource ID from URL
	resourceID := am.extractResourceID(r, resourceType)
	if resourceID == "" {
		return false, fmt.Errorf("resource ID not found")
	}

	// Convert to int64
	id, err := strconv.ParseInt(resourceID, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid resource ID")
	}

	ctx := context.Background()

	// Check ownership based on resource type
	switch resourceType {
	case "posts":
		post, err := am.getPostFromCache(ctx, id)
		if err != nil {
			return false, err
		}
		return post.UserID == authCtx.UserID, nil

	case "comments":
		comment, err := am.getCommentFromCache(ctx, id)
		if err != nil {
			return false, err
		}
		return comment.UserID == authCtx.UserID, nil

	case "jobs":
		job, err := am.getJobFromCache(ctx, id)
		if err != nil {
			return false, err
		}
		return job.EmployerID == authCtx.UserID, nil

	default:
		return false, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

// ===============================
// HELPER METHODS
// ===============================

// getUserFromCacheOrDB gets user from cache or database
func (am *AuthMiddleware) getUserFromCacheOrDB(ctx context.Context, userID int64) (*models.User, error) {
	if am.config.CacheUserData {
		// Try cache first
		cacheKey := fmt.Sprintf("user:%d", userID)
		if cachedUser, found := am.cache.Get(ctx, cacheKey); found {
			if user, ok := cachedUser.(*models.User); ok {
				return user, nil
			}
		}
	}

	// Get from database
	user, err := am.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Cache the user
	if am.config.CacheUserData && user != nil {
		cacheKey := fmt.Sprintf("user:%d", userID)
		am.cache.Set(ctx, cacheKey, user, am.config.UserCacheTTL)
	}

	return user, nil
}

// updateUserActivity updates user's last seen and online status
func (am *AuthMiddleware) updateUserActivity(ctx context.Context, userID int64) {
	if err := am.userRepo.UpdateLastSeen(ctx, userID); err != nil {
		am.logger.Warn("Failed to update user last seen", zap.Error(err), zap.Int64("user_id", userID))
	}

	if err := am.userRepo.SetOnlineStatus(ctx, userID, true); err != nil {
		am.logger.Warn("Failed to update user online status", zap.Error(err), zap.Int64("user_id", userID))
	}
}

// refreshSessionActivity refreshes session activity timestamp
func (am *AuthMiddleware) refreshSessionActivity(ctx context.Context, sessionToken string) {
	if err := am.sessionRepo.RefreshActivity(ctx, sessionToken); err != nil {
		am.logger.Warn("Failed to refresh session activity", zap.Error(err))
	}
}

// extractResourceID extracts resource ID from request
func (am *AuthMiddleware) extractResourceID(r *http.Request, resourceType string) string {
	// Try URL path parameter first
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}

	// Try path segments
	pathSegments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, segment := range pathSegments {
		if segment == resourceType && i+1 < len(pathSegments) {
			return pathSegments[i+1]
		}
	}

	return ""
}

// getPostFromCache gets post from cache (you'd implement this based on your post service)
func (am *AuthMiddleware) getPostFromCache(ctx context.Context, postID int64) (*models.Post, error) {
	// This would integrate with your post service
	// For now, return a placeholder
	return &models.Post{ID: postID, UserID: 1}, nil
}

// getCommentFromCache gets comment from cache
func (am *AuthMiddleware) getCommentFromCache(ctx context.Context, commentID int64) (*models.Comment, error) {
	// This would integrate with your comment service
	return &models.Comment{ID: commentID, UserID: 1}, nil
}

// getJobFromCache gets job from cache
func (am *AuthMiddleware) getJobFromCache(ctx context.Context, jobID int64) (*models.Job, error) {
	// This would integrate with your job service
	return &models.Job{ID: jobID, EmployerID: 1}, nil
}

// writeAuthError writes authentication error response
func (am *AuthMiddleware) writeAuthError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "AUTHENTICATION_ERROR",
			"message": message,
		},
		"timestamp": time.Now().Unix(),
	}

	response, _ := json.Marshal(errorResponse)
	w.Write(response)
}

// initializeJWTKeys initializes JWT signing keys
func (am *AuthMiddleware) initializeJWTKeys() error {
	// If we have a symmetric key (HS256)
	if am.config.JWTSecret != "" {
		return nil // No key parsing needed for symmetric keys
	}

	// Parse RSA private key
	if am.config.JWTPrivateKey != "" {
		block, _ := pem.Decode([]byte(am.config.JWTPrivateKey))
		if block == nil {
			return fmt.Errorf("failed to parse PEM block containing private key")
		}

		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse RSA private key: %w", err)
		}
		am.jwtPrivateKey = privateKey
	}

	// Parse RSA public key
	if am.config.JWTPublicKey != "" {
		block, _ := pem.Decode([]byte(am.config.JWTPublicKey))
		if block == nil {
			return fmt.Errorf("failed to parse PEM block containing public key")
		}

		publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse RSA public key: %w", err)
		}

		rsaPublicKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("public key is not RSA")
		}
		am.jwtPublicKey = rsaPublicKey
	}

	return nil
}

// ===============================
// CONTEXT HELPERS
// ===============================

// Context keys
type contextKey string

const (
	AuthContextKey   contextKey = "auth_context"
	UserIDKey        contextKey = "userID"
	UserKey          contextKey = "user"
	SanitizedDataKey contextKey = "sanitized_data"
	ValidatedFilesKey contextKey = "validated_files"
)

// GetAuthContext extracts auth context from request context
func GetAuthContext(ctx context.Context) *AuthContext {
	if authCtx, ok := ctx.Value(AuthContextKey).(*AuthContext); ok {
		return authCtx
	}
	return nil
}

// GetUserID extracts user ID from request context
func GetUserID(ctx context.Context) int64 {
	return contextutils.GetUserID(ctx)
}

// GetUser extracts user from request context
func GetUser(ctx context.Context) *models.User {
	if user, ok := ctx.Value(UserKey).(*models.User); ok {
		return user
	}
	return nil
}

// RequireUser gets authenticated user or panics (for handlers that require auth)
func RequireUser(ctx context.Context) *models.User {
	user := GetUser(ctx)
	if user == nil {
		panic("user not found in context - ensure authentication middleware is applied")
	}
	return user
}

// ===============================
// CONVENIENCE MIDDLEWARE FUNCTIONS
// ===============================

// CreateAuthMiddlewareStack creates a complete authentication middleware stack
func CreateAuthMiddlewareStack(
	config *AuthConfig,
	cache cache.Cache,
	sessionRepo repositories.SessionRepository,
	userRepo repositories.UserRepository,
	authService services.AuthService,
	logger *zap.Logger,
) (func(http.Handler) http.Handler, error) {
	auth, err := NewAuthMiddleware(config, cache, sessionRepo, userRepo, authService, logger)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		// Stack multiple auth-related middleware
		handler := next
		handler = auth.OptionalAuth()(handler) // Add optional auth to all routes
		return handler
	}, nil
}

// Common permission shortcuts
func RequirePostOwnership(am *AuthMiddleware) func(http.Handler) http.Handler {
	return am.RequireOwnership("posts")
}

func RequireCommentOwnership(am *AuthMiddleware) func(http.Handler) http.Handler {
	return am.RequireOwnership("comments")
}

func RequireJobOwnership(am *AuthMiddleware) func(http.Handler) http.Handler {
	return am.RequireOwnership("jobs")
}

func RequireAdmin(am *AuthMiddleware) func(http.Handler) http.Handler {
	return am.RequireRole("admin")
}

func RequireModerator(am *AuthMiddleware) func(http.Handler) http.Handler {
	return am.RequireRole("admin", "moderator")
}
