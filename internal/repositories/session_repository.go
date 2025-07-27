package repositories

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// sessionRepository implements SessionRepository with enhanced security features
type sessionRepository struct {
	*BaseRepository
}

// NewSessionRepository creates a new optimized session repository
func NewSessionRepository(db *database.Manager, logger *zap.Logger) SessionRepository {
	return &sessionRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// BASIC CRUD OPERATIONS
// ===============================

// Create creates a new session with proper security tracking
func (r *sessionRepository) Create(ctx context.Context, session *models.Session) error {
	query := `
		INSERT INTO sessions (
			user_id, session_token, expires_at, last_activity
		) VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING id, last_activity`

	err := r.QueryRowContext(
		ctx, query,
		session.UserID, session.SessionToken, session.ExpiresAt,
	).Scan(&session.ID, &session.LastActivity)

	if err != nil {
		r.GetLogger().Error("Failed to create session",
			zap.Error(err),
			zap.Int64("user_id", session.UserID),
			zap.String("token_prefix", r.truncateToken(session.SessionToken)),
		)
		return fmt.Errorf("failed to create session: %w", err)
	}

	r.GetLogger().Info("Session created successfully",
		zap.Int64("session_id", session.ID),
		zap.Int64("user_id", session.UserID),
		zap.Time("expires_at", session.ExpiresAt),
	)

	return nil
}

// GetByToken retrieves a session by token with user information
func (r *sessionRepository) GetByToken(ctx context.Context, token string) (*models.Session, error) {
	query := `
		SELECT 
			s.id, s.user_id, s.session_token, s.expires_at, s.last_activity,
			-- User information (JOIN to get role and status)
			u.role, u.is_active, u.username, u.email
		FROM sessions s
		INNER JOIN users u ON s.user_id = u.id
		WHERE s.session_token = $1 
		AND s.expires_at > CURRENT_TIMESTAMP
		AND u.is_active = true`

	var session models.Session
	var userRole, username, email string
	var userActive bool

	err := r.QueryRowContext(ctx, query, token).Scan(
		&session.ID, &session.UserID, &session.SessionToken,
		&session.ExpiresAt, &session.LastActivity,
		&userRole, &userActive, &username, &email,
	)

	if err != nil {
		if r.IsNotFound(err) {
			r.GetLogger().Debug("Session not found or expired",
				zap.String("token_prefix", r.truncateToken(token)),
			)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by token: %w", err)
	}

	// Set user role for authorization
	session.UserRole = userRole
	session.IsExpiredFlag = session.ExpiresAt.Before(time.Now())

	r.GetLogger().Debug("Session retrieved successfully",
		zap.Int64("session_id", session.ID),
		zap.Int64("user_id", session.UserID),
		zap.String("username", username),
		zap.String("role", userRole),
	)

	return &session, nil
}

// GetByUserID retrieves all sessions for a specific user
func (r *sessionRepository) GetByUserID(ctx context.Context, userID int64) ([]*models.Session, error) {
	query := `
		SELECT 
			id, user_id, session_token, expires_at, last_activity
		FROM sessions 
		WHERE user_id = $1 
		ORDER BY last_activity DESC`

	rows, err := r.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions by user ID: %w", err)
	}
	defer rows.Close()

	var sessions []*models.Session
	now := time.Now()

	for rows.Next() {
		var session models.Session
		err := rows.Scan(
			&session.ID, &session.UserID, &session.SessionToken,
			&session.ExpiresAt, &session.LastActivity,
		)
		if err != nil {
			continue
		}

		// Mark expired sessions
		session.IsExpiredFlag = session.ExpiresAt.Before(now)

		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// Update updates session information (mainly for extending expiry)
func (r *sessionRepository) Update(ctx context.Context, session *models.Session) error {
	query := `
		UPDATE sessions SET
			expires_at = $2, last_activity = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING last_activity`

	err := r.QueryRowContext(
		ctx, query,
		session.ID, session.ExpiresAt,
	).Scan(&session.LastActivity)

	if err != nil {
		if r.IsNotFound(err) {
			return fmt.Errorf("session not found")
		}
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// Delete removes a session (logout)
func (r *sessionRepository) Delete(ctx context.Context, token string) error {
	query := `DELETE FROM sessions WHERE session_token = $1`
	
	result, err := r.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found")
	}

	r.GetLogger().Info("Session deleted successfully",
		zap.String("token_prefix", r.truncateToken(token)),
	)

	return nil
}

// ===============================
// SESSION MANAGEMENT
// ===============================

// DeleteByUserID removes all sessions for a user (logout from all devices)
func (r *sessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	query := `DELETE FROM sessions WHERE user_id = $1`
	
	result, err := r.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete sessions by user ID: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	
	r.GetLogger().Info("All user sessions deleted",
		zap.Int64("user_id", userID),
		zap.Int64("sessions_deleted", rowsAffected),
	)

	return nil
}

// DeleteExpired removes all expired sessions (cleanup job)
func (r *sessionRepository) DeleteExpired(ctx context.Context) error {
	query := `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`
	
	result, err := r.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	
	r.GetLogger().Info("Expired sessions cleaned up",
		zap.Int64("sessions_deleted", rowsAffected),
	)

	return nil
}

// RefreshActivity updates the last activity timestamp for a session
func (r *sessionRepository) RefreshActivity(ctx context.Context, token string) error {
	query := `
		UPDATE sessions 
		SET last_activity = CURRENT_TIMESTAMP 
		WHERE session_token = $1 
		AND expires_at > CURRENT_TIMESTAMP`
	
	result, err := r.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to refresh session activity: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("session not found or expired")
	}

	return nil
}

// CountActiveSessions counts active sessions for a user
func (r *sessionRepository) CountActiveSessions(ctx context.Context, userID int64) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM sessions 
		WHERE user_id = $1 
		AND expires_at > CURRENT_TIMESTAMP`
	
	var count int
	err := r.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// ===============================
// CLEANUP OPERATIONS
// ===============================

// CleanupExpiredSessions performs comprehensive session cleanup
func (r *sessionRepository) CleanupExpiredSessions(ctx context.Context) (int, error) {
	return r.cleanupSessionsOlderThan(ctx, time.Now())
}

// GetExpiredSessions retrieves sessions that are older than specified time
func (r *sessionRepository) GetExpiredSessions(ctx context.Context, olderThan time.Time) ([]*models.Session, error) {
	query := `
		SELECT 
			s.id, s.user_id, s.session_token, s.expires_at, s.last_activity,
			u.username
		FROM sessions s
		INNER JOIN users u ON s.user_id = u.id
		WHERE s.expires_at <= $1 
		OR s.last_activity <= $2
		ORDER BY s.expires_at ASC
		LIMIT 1000` // Limit for safety

	// Consider sessions expired if they haven't been active for 30 days
	inactiveThreshold := olderThan.Add(-30 * 24 * time.Hour)

	rows, err := r.QueryContext(ctx, query, olderThan, inactiveThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get expired sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		var session models.Session
		var username string

		err := rows.Scan(
			&session.ID, &session.UserID, &session.SessionToken,
			&session.ExpiresAt, &session.LastActivity, &username,
		)
		if err != nil {
			continue
		}

		session.IsExpiredFlag = true
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// ===============================
// SECURITY OPERATIONS
// ===============================

// GetActiveSessions retrieves active sessions with detailed information
func (r *sessionRepository) GetActiveSessions(ctx context.Context, userID int64, includeExpired bool) ([]*models.Session, error) {
	var whereClause strings.Builder
	whereClause.WriteString("s.user_id = $1")
	
	if !includeExpired {
		whereClause.WriteString(" AND s.expires_at > CURRENT_TIMESTAMP")
	}

	query := fmt.Sprintf(`
		SELECT 
			s.id, s.user_id, s.session_token, s.expires_at, s.last_activity,
			u.role, u.username
		FROM sessions s
		INNER JOIN users u ON s.user_id = u.id
		WHERE %s
		ORDER BY s.last_activity DESC`, whereClause.String())

	rows, err := r.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*models.Session
	now := time.Now()

	for rows.Next() {
		var session models.Session
		var username string

		err := rows.Scan(
			&session.ID, &session.UserID, &session.SessionToken,
			&session.ExpiresAt, &session.LastActivity,
			&session.UserRole, &username,
		)
		if err != nil {
			continue
		}

		session.IsExpiredFlag = session.ExpiresAt.Before(now)
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// InvalidateOldSessions removes sessions older than a certain threshold
func (r *sessionRepository) InvalidateOldSessions(ctx context.Context, userID int64, keepLatest int) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Get session IDs to keep (most recent ones)
		keepQuery := `
			SELECT id 
			FROM sessions 
			WHERE user_id = $1 
			ORDER BY last_activity DESC 
			LIMIT $2`

		rows, err := tx.QueryContext(ctx, keepQuery, userID, keepLatest)
		if err != nil {
			return err
		}
		defer rows.Close()

		var keepIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				keepIDs = append(keepIDs, id)
			}
		}
		rows.Close()

		if len(keepIDs) == 0 {
			return nil // No sessions to process
		}

		// Build placeholders for NOT IN clause
		placeholders := make([]string, len(keepIDs))
		args := make([]interface{}, len(keepIDs)+1)
		args[0] = userID
		
		for i, id := range keepIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args[i+1] = id
		}

		// Delete old sessions
		deleteQuery := fmt.Sprintf(`
			DELETE FROM sessions 
			WHERE user_id = $1 
			AND id NOT IN (%s)`, strings.Join(placeholders, ","))

		result, err := tx.ExecContext(ctx, deleteQuery, args...)
		if err != nil {
			return err
		}

		rowsAffected, _ := result.RowsAffected()
		
		r.GetLogger().Info("Old sessions invalidated",
			zap.Int64("user_id", userID),
			zap.Int("kept_sessions", keepLatest),
			zap.Int64("deleted_sessions", rowsAffected),
		)

		return nil
	})
}

// GetSessionStatistics provides session analytics
func (r *sessionRepository) GetSessionStatistics(ctx context.Context) (map[string]interface{}, error) {
	query := `
		SELECT 
			COUNT(*) as total_sessions,
			COUNT(CASE WHEN expires_at > CURRENT_TIMESTAMP THEN 1 END) as active_sessions,
			COUNT(CASE WHEN expires_at <= CURRENT_TIMESTAMP THEN 1 END) as expired_sessions,
			COUNT(CASE WHEN last_activity > CURRENT_TIMESTAMP - INTERVAL '1 hour' THEN 1 END) as recent_activity,
			COUNT(CASE WHEN last_activity > CURRENT_TIMESTAMP - INTERVAL '1 day' THEN 1 END) as daily_active,
			COUNT(DISTINCT user_id) as unique_users,
			AVG(EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - last_activity))/3600) as avg_hours_since_activity
		FROM sessions`

	var stats struct {
		Total          int     `json:"total_sessions"`
		Active         int     `json:"active_sessions"`
		Expired        int     `json:"expired_sessions"`
		RecentActivity int     `json:"recent_activity"`
		DailyActive    int     `json:"daily_active"`
		UniqueUsers    int     `json:"unique_users"`
		AvgHoursIdle   float64 `json:"avg_hours_since_activity"`
	}

	err := r.QueryRowContext(ctx, query).Scan(
		&stats.Total, &stats.Active, &stats.Expired,
		&stats.RecentActivity, &stats.DailyActive,
		&stats.UniqueUsers, &stats.AvgHoursIdle,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get session statistics: %w", err)
	}

	return map[string]interface{}{
		"total_sessions":           stats.Total,
		"active_sessions":          stats.Active,
		"expired_sessions":         stats.Expired,
		"recent_activity":          stats.RecentActivity,
		"daily_active_sessions":    stats.DailyActive,
		"unique_active_users":      stats.UniqueUsers,
		"avg_hours_since_activity": stats.AvgHoursIdle,
	}, nil
}

// ===============================
// BATCH OPERATIONS
// ===============================

// BulkDelete removes multiple sessions by IDs
func (r *sessionRepository) BulkDelete(ctx context.Context, sessionIDs []int64) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	query := `DELETE FROM sessions WHERE id = ANY($1)`
	result, err := r.ExecContext(ctx, query, sessionIDs)
	if err != nil {
		return fmt.Errorf("failed to bulk delete sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	r.GetLogger().Info("Bulk session deletion completed",
		zap.Int("requested", len(sessionIDs)),
		zap.Int64("deleted", rowsAffected),
	)

	return nil
}

// ===============================
// PRIVATE HELPER METHODS
// ===============================

// cleanupSessionsOlderThan removes sessions based on various criteria
func (r *sessionRepository) cleanupSessionsOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	// Remove sessions that are either expired or haven't been active for 30 days
	inactiveThreshold := cutoff.Add(-30 * 24 * time.Hour)
	
	query := `
		DELETE FROM sessions 
		WHERE expires_at <= $1 
		OR last_activity <= $2`
	
	result, err := r.ExecContext(ctx, query, cutoff, inactiveThreshold)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	
	r.GetLogger().Info("Session cleanup completed",
		zap.Time("cutoff", cutoff),
		zap.Time("inactive_threshold", inactiveThreshold),
		zap.Int64("sessions_deleted", rowsAffected),
	)

	return int(rowsAffected), nil
}

// truncateToken safely truncates session token for logging (security)
func (r *sessionRepository) truncateToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// ===============================
// SCHEDULED CLEANUP METHODS
// ===============================

// RunScheduledCleanup performs regular session maintenance
func (r *sessionRepository) RunScheduledCleanup(ctx context.Context) error {
	r.GetLogger().Info("Starting scheduled session cleanup")

	// 1. Remove expired sessions
	expiredCount, err := r.CleanupExpiredSessions(ctx)
	if err != nil {
		r.GetLogger().Error("Failed to cleanup expired sessions", zap.Error(err))
		return err
	}

	// 2. Remove very old inactive sessions (more than 90 days)
	veryOldThreshold := time.Now().Add(-90 * 24 * time.Hour)
	oldCount, err := r.cleanupSessionsOlderThan(ctx, veryOldThreshold)
	if err != nil {
		r.GetLogger().Error("Failed to cleanup very old sessions", zap.Error(err))
		return err
	}

	// 3. Get final statistics
	stats, err := r.GetSessionStatistics(ctx)
	if err != nil {
		r.GetLogger().Warn("Failed to get session statistics after cleanup", zap.Error(err))
	}

	r.GetLogger().Info("Scheduled session cleanup completed",
		zap.Int("expired_sessions_removed", expiredCount),
		zap.Int("old_sessions_removed", oldCount),
		zap.Any("final_stats", stats),
	)

	return nil
}
