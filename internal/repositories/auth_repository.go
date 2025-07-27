// file: internal/repositories/auth_repository.go
package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"

	"go.uber.org/zap"
)

// authRepository implements AuthRepository with security focus
type authRepository struct {
	*BaseRepository
}

// NewAuthRepository creates a new authentication repository
func NewAuthRepository(db *database.Manager, logger *zap.Logger) AuthRepository {
	return &authRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// USER OPERATIONS (AUTH-FOCUSED)
// ===============================

// CreateUser creates a new user account (auth-specific version)
func (r *authRepository) CreateUser(ctx context.Context, user *models.User) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Insert user with auth-specific fields
		query := `
			INSERT INTO users (
				email, username, password_hash, first_name, last_name,
				role, is_verified, is_active, expertise, email_notifications,
				password_changed_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_TIMESTAMP
			) RETURNING id, created_at, updated_at, last_seen, display_name`

		err := tx.QueryRowContext(
			ctx, query,
			user.Email, user.Username, user.PasswordHash,
			user.FirstName, user.LastName, user.Role,
			user.EmailVerified, user.IsActive, user.Expertise,
			user.EmailNotifications,
		).Scan(
			&user.ID, &user.CreatedAt, &user.UpdatedAt,
			&user.LastSeen, &user.DisplayName,
		)

		if err != nil {
			r.GetLogger().Error("Failed to create user",
				zap.Error(err),
				zap.String("email", user.Email),
				zap.String("username", user.Username),
			)
			return fmt.Errorf("failed to create user: %w", err)
		}

		// Create initial user stats entry
		statsQuery := `
			INSERT INTO user_stats (user_id, created_at, updated_at)
			VALUES ($1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`
		
		_, err = tx.ExecContext(ctx, statsQuery, user.ID)
		if err != nil {
			r.GetLogger().Warn("Failed to create user stats",
				zap.Error(err),
				zap.Int64("user_id", user.ID),
			)
			// Don't fail the transaction for this
		}

		r.GetLogger().Info("User created successfully",
			zap.Int64("user_id", user.ID),
			zap.String("username", user.Username),
			zap.String("email", user.Email),
		)

		return nil
	})
}

// GetUserByUsername retrieves user by username (with password hash for auth)
func (r *authRepository) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.password_hash, u.first_name, u.last_name,
			u.display_name, u.role, u.is_verified, u.is_active, u.is_online,
			u.email_notifications, u.created_at, u.updated_at, u.last_seen,
			u.email_verified_at, u.password_changed_at,
			-- Include basic stats for auth context
			COALESCE(us.reputation_points, 0) as reputation_points
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id
		WHERE u.username = $1`

	var user models.User
	err := r.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.EmailNotifications, &user.CreatedAt, &user.UpdatedAt, &user.LastSeen,
		&user.EmailVerifiedAt, &user.PasswordChangedAt,
		&user.ReputationPoints,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// GetUserByEmail retrieves user by email (for login)
func (r *authRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.password_hash, u.first_name, u.last_name,
			u.display_name, u.role, u.is_verified, u.is_active, u.is_online,
			u.created_at, u.updated_at, u.last_seen, u.password_changed_at
		FROM users u
		WHERE u.email = $1`

	var user models.User
	err := r.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.CreatedAt, &user.UpdatedAt, &user.LastSeen, &user.PasswordChangedAt,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}

// GetUserByID retrieves user by ID (for session validation)
func (r *authRepository) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.first_name, u.last_name,
			u.display_name, u.role, u.is_verified, u.is_active, u.is_online,
			u.created_at, u.updated_at, u.last_seen
		FROM users u
		WHERE u.id = $1 AND u.is_active = true`

	var user models.User
	err := r.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Username,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.CreatedAt, &user.UpdatedAt, &user.LastSeen,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return &user, nil
}

// VerifyUserEmail marks user email as verified
func (r *authRepository) VerifyUserEmail(ctx context.Context, userID int64) error {
	query := `
		UPDATE users 
		SET is_verified = true, email_verified_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to verify user email: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	r.GetLogger().Info("User email verified",
		zap.Int64("user_id", userID),
	)

	return nil
}

// UpdatePassword updates user password hash
func (r *authRepository) UpdatePassword(ctx context.Context, userID int64, newPasswordHash string) error {
	query := `
		UPDATE users 
		SET password_hash = $2, password_changed_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND is_active = true`

	result, err := r.ExecContext(ctx, query, userID, newPasswordHash)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found or inactive")
	}

	r.GetLogger().Info("User password updated",
		zap.Int64("user_id", userID),
	)

	return nil
}

// SetUserOnlineStatus updates user online status
func (r *authRepository) SetUserOnlineStatus(ctx context.Context, userID int64, online bool) error {
	query := `
		UPDATE users 
		SET is_online = $2, last_seen = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	_, err := r.ExecContext(ctx, query, userID, online)
	if err != nil {
		return fmt.Errorf("failed to update online status: %w", err)
	}

	return nil
}

// ===============================
// SESSION OPERATIONS
// ===============================

// CreateSession creates a new user session
func (r *authRepository) CreateSession(ctx context.Context, session *models.Session) error {
	query := `
		INSERT INTO sessions (
			user_id, session_token, expires_at, ip_address, user_agent, is_active
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, last_activity`

	err := r.QueryRowContext(
		ctx, query,
		session.UserID, session.SessionToken, session.ExpiresAt,
		session.IPAddress, session.UserAgent, true,
	).Scan(&session.ID, &session.CreatedAt, &session.LastActivity)

	if err != nil {
		r.GetLogger().Error("Failed to create session",
			zap.Error(err),
			zap.Int64("user_id", session.UserID),
		)
		return fmt.Errorf("failed to create session: %w", err)
	}

	r.GetLogger().Info("Session created",
		zap.Int64("session_id", session.ID),
		zap.Int64("user_id", session.UserID),
	)

	return nil
}

// GetSessionByToken retrieves session by token with user info
func (r *authRepository) GetSessionByToken(ctx context.Context, token string) (*models.Session, error) {
	query := `
		SELECT 
			s.id, s.user_id, s.session_token, s.expires_at, s.last_activity,
			s.ip_address, s.user_agent, s.is_active, s.created_at,
			u.role as user_role
		FROM sessions s
		INNER JOIN users u ON s.user_id = u.id
		WHERE s.session_token = $1 AND s.is_active = true AND u.is_active = true`

	var session models.Session
	err := r.QueryRowContext(ctx, query, token).Scan(
		&session.ID, &session.UserID, &session.SessionToken,
		&session.ExpiresAt, &session.LastActivity,
		&session.IPAddress, &session.UserAgent, &session.IsActive,
		&session.CreatedAt, &session.UserRole,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by token: %w", err)
	}

	// Check if session is expired
	session.IsExpiredFlag = time.Now().After(session.ExpiresAt)

	return &session, nil
}

// UpdateSessionActivity updates session last activity
func (r *authRepository) UpdateSessionActivity(ctx context.Context, token string) error {
	query := `
		UPDATE sessions 
		SET last_activity = CURRENT_TIMESTAMP
		WHERE session_token = $1 AND is_active = true`

	_, err := r.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to update session activity: %w", err)
	}

	return nil
}

// DeleteSessionByToken deletes a specific session
func (r *authRepository) DeleteSessionByToken(ctx context.Context, token string) error {
	query := `DELETE FROM sessions WHERE session_token = $1`

	result, err := r.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found")
	}

	r.GetLogger().Info("Session deleted", zap.String("token", token[:10]+"..."))

	return nil
}

// DeleteAllUserSessions deletes all sessions for a user
func (r *authRepository) DeleteAllUserSessions(ctx context.Context, userID int64) error {
	query := `DELETE FROM sessions WHERE user_id = $1`

	result, err := r.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	r.GetLogger().Info("All user sessions deleted",
		zap.Int64("user_id", userID),
		zap.Int64("sessions_deleted", rowsAffected),
	)

	return nil
}

// CleanupExpiredSessions removes expired sessions
func (r *authRepository) CleanupExpiredSessions(ctx context.Context) (int, error) {
	query := `DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`

	result, err := r.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		r.GetLogger().Info("Expired sessions cleaned up",
			zap.Int64("sessions_deleted", rowsAffected),
		)
	}

	return int(rowsAffected), nil
}

// ===============================
// SECURITY OPERATIONS
// ===============================

// RecordLoginAttempt records a login attempt for security monitoring
func (r *authRepository) RecordLoginAttempt(ctx context.Context, email string, success bool, ipAddress string) error {
	// This would require a login_attempts table - for now, just log
	if success {
		r.GetLogger().Info("Successful login",
			zap.String("email", email),
			zap.String("ip_address", ipAddress),
		)
	} else {
		r.GetLogger().Warn("Failed login attempt",
			zap.String("email", email),
			zap.String("ip_address", ipAddress),
		)
	}

	// TODO: Implement login_attempts table tracking
	return nil
}

// GetRecentLoginAttempts gets recent failed login attempts for rate limiting
func (r *authRepository) GetRecentLoginAttempts(ctx context.Context, email string, since time.Time) (int, error) {
	// TODO: Implement with login_attempts table
	// For now, return 0 (no rate limiting)
	return 0, nil
}

// LockAccount locks a user account for security reasons
func (r *authRepository) LockAccount(ctx context.Context, userID int64, reason string) error {
	query := `
		UPDATE users 
		SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to lock account: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	// Delete all sessions when locking account
	err = r.DeleteAllUserSessions(ctx, userID)
	if err != nil {
		r.GetLogger().Warn("Failed to delete sessions when locking account",
			zap.Error(err),
			zap.Int64("user_id", userID),
		)
	}

	r.GetLogger().Warn("Account locked",
		zap.Int64("user_id", userID),
		zap.String("reason", reason),
	)

	return nil
}

// UnlockAccount unlocks a user account
func (r *authRepository) UnlockAccount(ctx context.Context, userID int64) error {
	query := `
		UPDATE users 
		SET is_active = true, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to unlock account: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	r.GetLogger().Info("Account unlocked",
		zap.Int64("user_id", userID),
	)

	return nil
}