// file: internal/repositories/user_repository.go
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

// userRepository implements UserRepository with high-performance patterns
type userRepository struct {
	*BaseRepository
}

// NewUserRepository creates a new optimized user repository
func NewUserRepository(db *database.Manager, logger *zap.Logger) UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// BASIC CRUD OPERATIONS
// ===============================

// Create creates a new user with proper validation and constraints
func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (
			email, username, password_hash, first_name, last_name,
			job_title, affiliation, bio, years_experience, core_competencies,
			expertise, profile_url, profile_public_id, cv_url, cv_public_id,
			website_url, linkedin_profile, twitter_handle, role, email_notifications
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		) RETURNING id, created_at, updated_at, last_seen, display_name`

	err := r.QueryRowContext(
		ctx, query,
		user.Email, user.Username, user.PasswordHash,
		user.FirstName, user.LastName, user.JobTitle,
		user.Affiliation, user.Bio, user.YearsExperience,
		user.CoreCompetencies, user.Expertise,
		user.ProfileURL, user.ProfilePublicID,
		user.CVURL, user.CVPublicID,
		user.WebsiteURL, user.LinkedinProfile, user.TwitterHandle,
		user.Role, user.EmailNotifications,
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

	r.GetLogger().Info("User created successfully",
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
	)

	return nil
}

// GetByID retrieves a user by ID with optional stats
func (r *userRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.first_name, u.last_name,
			u.display_name, u.job_title, u.affiliation, u.bio,
			u.years_experience, u.core_competencies, u.expertise,
			u.profile_url, u.profile_public_id, u.cv_url, u.cv_public_id,
			u.website_url, u.linkedin_profile, u.twitter_handle,
			u.role, u.is_verified, u.is_active, u.is_online,
			u.email_notifications, u.created_at, u.updated_at,
			u.last_seen, u.email_verified_at, u.password_changed_at,
			-- User statistics (optional join)
			COALESCE(us.reputation_points, 0) as reputation_points,
			COALESCE(us.total_contributions, 0) as total_contributions,
			COALESCE(us.posts_count, 0) as posts_count,
			COALESCE(us.questions_count, 0) as questions_count,
			COALESCE(us.comments_count, 0) as comments_count
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id
		WHERE u.id = $1 AND u.is_active = true`

	var user models.User
	err := r.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Username,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.JobTitle, &user.Affiliation, &user.Bio,
		&user.YearsExperience, &user.CoreCompetencies, &user.Expertise,
		&user.ProfileURL, &user.ProfilePublicID,
		&user.CVURL, &user.CVPublicID,
		&user.WebsiteURL, &user.LinkedinProfile, &user.TwitterHandle,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.EmailNotifications, &user.CreatedAt, &user.UpdatedAt,
		&user.LastSeen, &user.EmailVerifiedAt, &user.PasswordChangedAt,
		&user.ReputationPoints, &user.TotalContributions,
		&user.PostsCount, &user.QuestionsCount, &user.CommentsCount,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	// Calculate level based on reputation
	user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)

	return &user, nil
}

// GetByUsername retrieves a user by username
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.password_hash, u.first_name, u.last_name,
			u.display_name, u.job_title, u.affiliation, u.bio,
			u.years_experience, u.core_competencies, u.expertise,
			u.profile_url, u.profile_public_id, u.cv_url, u.cv_public_id,
			u.website_url, u.linkedin_profile, u.twitter_handle,
			u.role, u.is_verified, u.is_active, u.is_online,
			u.email_notifications, u.created_at, u.updated_at,
			u.last_seen, u.email_verified_at, u.password_changed_at
		FROM users u
		WHERE u.username = $1 AND u.is_active = true`

	var user models.User
	err := r.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.JobTitle, &user.Affiliation, &user.Bio,
		&user.YearsExperience, &user.CoreCompetencies, &user.Expertise,
		&user.ProfileURL, &user.ProfilePublicID,
		&user.CVURL, &user.CVPublicID,
		&user.WebsiteURL, &user.LinkedinProfile, &user.TwitterHandle,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.EmailNotifications, &user.CreatedAt, &user.UpdatedAt,
		&user.LastSeen, &user.EmailVerifiedAt, &user.PasswordChangedAt,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// GetByGitHubID retrieves a user by their GitHub ID
func (r *userRepository) GetByGitHubID(ctx context.Context, githubID int64) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.password_hash, u.first_name, u.last_name,
			u.display_name, u.job_title, u.affiliation, u.bio,
			u.years_experience, u.core_competencies, u.expertise,
			u.profile_url, u.profile_public_id, u.cv_url, u.cv_public_id,
			u.website_url, u.linkedin_profile, u.twitter_handle,
			u.role, u.is_verified, u.is_active, u.is_online,
			u.email_notifications, u.created_at, u.updated_at,
			u.last_seen, u.email_verified_at, u.password_changed_at,
			COALESCE(us.reputation_points, 0) as reputation_points,
			COALESCE(us.total_contributions, 0) as total_contributions,
			COALESCE(us.posts_count, 0) as posts_count,
			COALESCE(us.questions_count, 0) as questions_count,
			COALESCE(us.comments_count, 0) as comments_count
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id
		WHERE u.github_id = $1 AND u.is_active = true`

	var user models.User
	err := r.QueryRowContext(ctx, query, githubID).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.FirstName, &user.LastName, &user.DisplayName,
		&user.JobTitle, &user.Affiliation, &user.Bio,
		&user.YearsExperience, &user.CoreCompetencies, &user.Expertise,
		&user.ProfileURL, &user.ProfilePublicID,
		&user.CVURL, &user.CVPublicID,
		&user.WebsiteURL, &user.LinkedinProfile, &user.TwitterHandle,
		&user.Role, &user.EmailVerified, &user.IsActive, &user.IsOnline,
		&user.EmailNotifications, &user.CreatedAt, &user.UpdatedAt,
		&user.LastSeen, &user.EmailVerifiedAt, &user.PasswordChangedAt,
		&user.ReputationPoints, &user.TotalContributions,
		&user.PostsCount, &user.QuestionsCount, &user.CommentsCount,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by GitHub ID: %w", err)
	}

	// Calculate level based on reputation
	user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)

	return &user, nil
}

// GetByEmail retrieves a user by email (for authentication)
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT 
			u.id, u.email, u.username, u.password_hash, u.first_name, u.last_name,
			u.display_name, u.role, u.is_verified, u.is_active, u.is_online,
			u.created_at, u.updated_at, u.last_seen, u.password_changed_at
		FROM users u
		WHERE u.email = $1 AND u.is_active = true`

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

// Update updates a user's information
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users SET
			first_name = $2, last_name = $3, job_title = $4,
			affiliation = $5, bio = $6, years_experience = $7,
			core_competencies = $8, expertise = $9,
			profile_url = $10, profile_public_id = $11,
			cv_url = $12, cv_public_id = $13,
			website_url = $14, linkedin_profile = $15, twitter_handle = $16,
			email_notifications = $17, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND is_active = true
		RETURNING updated_at, display_name`

	err := r.QueryRowContext(
		ctx, query,
		user.ID, user.FirstName, user.LastName, user.JobTitle,
		user.Affiliation, user.Bio, user.YearsExperience,
		user.CoreCompetencies, user.Expertise,
		user.ProfileURL, user.ProfilePublicID,
		user.CVURL, user.CVPublicID,
		user.WebsiteURL, user.LinkedinProfile, user.TwitterHandle,
		user.EmailNotifications,
	).Scan(&user.UpdatedAt, &user.DisplayName)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	r.GetLogger().Info("User updated successfully",
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
	)

	return nil
}

// Delete soft deletes a user
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	query := `
		UPDATE users 
		SET is_active = false, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// ===============================
// BATCH OPERATIONS
// ===============================

// GetByIDs retrieves multiple users by IDs (prevents N+1 queries)
func (r *userRepository) GetByIDs(ctx context.Context, ids []int64) ([]*models.User, error) {
	if len(ids) == 0 {
		return []*models.User{}, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT 
			u.id, u.username, u.display_name, u.profile_url,
			u.role, u.expertise, u.is_online, u.last_seen,
			COALESCE(us.reputation_points, 0) as reputation_points
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id
		WHERE u.id IN (%s) AND u.is_active = true
		ORDER BY u.username`, strings.Join(placeholders, ","))

	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by IDs: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName,
			&user.ProfileURL, &user.Role, &user.Expertise,
			&user.IsOnline, &user.LastSeen, &user.ReputationPoints,
		)
		if err != nil {
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// UpdateLastSeen updates user's last seen timestamp
func (r *userRepository) UpdateLastSeen(ctx context.Context, userID int64) error {
	query := `UPDATE users SET last_seen = CURRENT_TIMESTAMP WHERE id = $1`
	_, err := r.ExecContext(ctx, query, userID)
	return err
}

// SetOnlineStatus updates user's online status
func (r *userRepository) SetOnlineStatus(ctx context.Context, userID int64, online bool) error {
	query := `
		UPDATE users 
		SET is_online = $2, last_seen = CURRENT_TIMESTAMP 
		WHERE id = $1`
	_, err := r.ExecContext(ctx, query, userID, online)
	return err
}

// BulkSetOffline sets multiple users offline (for cleanup)
func (r *userRepository) BulkSetOffline(ctx context.Context, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}

	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		query := `UPDATE users SET is_online = false WHERE id = ANY($1)`
		_, err := tx.ExecContext(ctx, query, userIDs)
		return err
	})
}

// ===============================
// SEARCH AND LISTING
// ===============================

// List retrieves users with pagination and filtering
func (r *userRepository) List(ctx context.Context, params models.PaginationParams, excludeID int64) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.expertise,
			u.profile_url, u.affiliation, u.bio, u.role,
			u.is_online, u.last_seen, u.created_at,
			COALESCE(us.reputation_points, 0) as reputation_points,
			COALESCE(us.total_contributions, 0) as total_contributions
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id`

	whereClause := "u.is_active = true AND u.id != $1"
	whereArgs := []interface{}{excludeID}

	// Build paginated query
	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	// Combine where args with pagination args
	finalArgs := append(whereArgs, args...)

	// Execute query
	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.Expertise,
			&user.ProfileURL, &user.Affiliation, &user.Bio, &user.Role,
			&user.IsOnline, &user.LastSeen, &user.CreatedAt,
			&user.ReputationPoints, &user.TotalContributions,
		)
		if err != nil {
			continue
		}

		user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)
		users = append(users, &user)

		// Generate cursor for pagination
		lastCursor = r.encodeCursor(user.CreatedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	// Build pagination metadata
	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
	}, nil
}

// Search searches users by various criteria
func (r *userRepository) Search(ctx context.Context, query string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.expertise,
			u.profile_url, u.affiliation, u.bio, u.role,
			u.is_online, u.last_seen, u.created_at,
			COALESCE(us.reputation_points, 0) as reputation_points
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id`

	searchTerm := "%" + strings.ToLower(query) + "%"
	whereClause := `
		u.is_active = true AND (
			LOWER(u.username) LIKE $1 OR 
			LOWER(u.display_name) LIKE $1 OR 
			LOWER(u.affiliation) LIKE $1 OR
			LOWER(u.bio) LIKE $1 OR
			LOWER(u.core_competencies) LIKE $1
		)`
	whereArgs := []interface{}{searchTerm}

	// Build paginated query
	sqlQuery, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, sqlQuery, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.Expertise,
			&user.ProfileURL, &user.Affiliation, &user.Bio, &user.Role,
			&user.IsOnline, &user.LastSeen, &user.CreatedAt,
			&user.ReputationPoints,
		)
		if err != nil {
			continue
		}

		user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)
		users = append(users, &user)
		lastCursor = r.encodeCursor(user.CreatedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
	}, nil
}

// GetOnlineUsers retrieves currently online users
func (r *userRepository) GetOnlineUsers(ctx context.Context, limit int) ([]*models.User, error) {
	query := `
		SELECT id, username, display_name, profile_url, role
		FROM users 
		WHERE is_online = true AND is_active = true
		ORDER BY last_seen DESC
		LIMIT $1`

	rows, err := r.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get online users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName,
			&user.ProfileURL, &user.Role,
		)
		if err != nil {
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// ===============================
// FILTER METHODS
// ===============================

// GetByRole retrieves users by role with pagination
func (r *userRepository) GetByRole(ctx context.Context, role string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.expertise,
			u.profile_url, u.affiliation, u.role,
			u.is_online, u.created_at,
			COALESCE(us.reputation_points, 0) as reputation_points
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id`

	whereClause := "u.is_active = true AND u.role = $1"
	whereArgs := []interface{}{role}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by role: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.Expertise,
			&user.ProfileURL, &user.Affiliation, &user.Role,
			&user.IsOnline, &user.CreatedAt, &user.ReputationPoints,
		)
		if err != nil {
			continue
		}

		users = append(users, &user)
		lastCursor = r.encodeCursor(user.CreatedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
	}, nil
}

// GetByExpertise retrieves users by expertise level with pagination
func (r *userRepository) GetByExpertise(ctx context.Context, expertise string, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.expertise,
			u.profile_url, u.affiliation, u.years_experience,
			u.is_online, u.created_at,
			COALESCE(us.reputation_points, 0) as reputation_points
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id`

	whereClause := "u.is_active = true AND u.expertise = $1"
	whereArgs := []interface{}{expertise}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by expertise: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.Expertise,
			&user.ProfileURL, &user.Affiliation, &user.YearsExperience,
			&user.IsOnline, &user.CreatedAt, &user.ReputationPoints,
		)
		if err != nil {
			continue
		}

		users = append(users, &user)
		lastCursor = r.encodeCursor(user.CreatedAt)
	}

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
	}, nil
}

// ===============================
// ANALYTICS METHODS
// ===============================

// GetUserStats retrieves comprehensive user statistics
func (r *userRepository) GetUserStats(ctx context.Context, userID int64) (*UserStats, error) {
	query := `
		SELECT 
			us.user_id, us.reputation_points, us.posts_count,
			us.questions_count, us.comments_count, us.likes_given,
			us.likes_received, us.total_contributions, us.last_activity,
			u.created_at
		FROM user_stats us
		JOIN users u ON us.user_id = u.id
		WHERE us.user_id = $1`

	var stats UserStats
	err := r.QueryRowContext(ctx, query, userID).Scan(
		&stats.UserID, &stats.ReputationPoints, &stats.PostsCount,
		&stats.QuestionsCount, &stats.CommentsCount, &stats.LikesGiven,
		&stats.LikesReceived, &stats.TotalContributions, &stats.LastActivity,
		&stats.JoinedAt,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	// Calculate level and next level points
	stats.Level, _ = r.calculateUserLevel(stats.ReputationPoints)
	stats.NextLevelPoints = r.calculateNextLevelPoints(stats.ReputationPoints)

	return &stats, nil
}

// GetLeaderboard retrieves top users by reputation
func (r *userRepository) GetLeaderboard(ctx context.Context, limit int) ([]*models.User, error) {
	query := `
		SELECT 
			u.id, u.username, u.display_name, u.profile_url,
			u.expertise, u.affiliation, us.reputation_points,
			us.total_contributions, us.posts_count, us.questions_count
		FROM users u
		JOIN user_stats us ON u.id = us.user_id
		WHERE u.is_active = true
		ORDER BY us.reputation_points DESC, us.total_contributions DESC
		LIMIT $1`

	rows, err := r.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.ProfileURL,
			&user.Expertise, &user.Affiliation, &user.ReputationPoints,
			&user.TotalContributions, &user.PostsCount, &user.QuestionsCount,
		)
		if err != nil {
			continue
		}

		user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)
		users = append(users, &user)
	}

	return users, nil
}

// GetActiveUsers retrieves users active since a specific time
func (r *userRepository) GetActiveUsers(ctx context.Context, since time.Time) ([]*models.User, error) {
	query := `
		SELECT id, username, display_name, profile_url, last_seen
		FROM users 
		WHERE last_seen > $1 AND is_active = true
		ORDER BY last_seen DESC`

	rows, err := r.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName,
			&user.ProfileURL, &user.LastSeen,
		)
		if err != nil {
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// CountByRole counts users by role
func (r *userRepository) CountByRole(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT role, COUNT(*) 
		FROM users 
		WHERE is_active = true 
		GROUP BY role`

	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count users by role: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var role string
		var count int
		if err := rows.Scan(&role, &count); err == nil {
			counts[role] = count
		}
	}

	return counts, nil
}


// ===============================
// SOCIAL FEATURES (MISSING)
// ===============================

// FollowUser creates a follow relationship between two users
func (r *userRepository) FollowUser(ctx context.Context, followerID, followeeID int64) error {
	// Prevent self-following
	if followerID == followeeID {
		return fmt.Errorf("users cannot follow themselves")
	}

	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Check if both users exist and are active
		var followerExists, followeeExists bool
		
		err := tx.QueryRowContext(ctx, 
			"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_active = true)", 
			followerID).Scan(&followerExists)
		if err != nil {
			return fmt.Errorf("failed to check follower existence: %w", err)
		}
		
		err = tx.QueryRowContext(ctx, 
			"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_active = true)", 
			followeeID).Scan(&followeeExists)
		if err != nil {
			return fmt.Errorf("failed to check followee existence: %w", err)
		}

		if !followerExists {
			return fmt.Errorf("follower user not found")
		}
		if !followeeExists {
			return fmt.Errorf("followee user not found")
		}

		// Insert the follow relationship (ON CONFLICT DO NOTHING to handle duplicates)
		query := `
			INSERT INTO user_follows (follower_id, followee_id, created_at)
			VALUES ($1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (follower_id, followee_id) DO NOTHING`

		_, err = tx.ExecContext(ctx, query, followerID, followeeID)
		if err != nil {
			return fmt.Errorf("failed to create follow relationship: %w", err)
		}

		// Update follower count in user_stats (if exists)
		updateFolloweeStatsQuery := `
			UPDATE user_stats 
			SET followers_count = (
				SELECT COUNT(*) FROM user_follows WHERE followee_id = $1
			)
			WHERE user_id = $1`
		
		_, err = tx.ExecContext(ctx, updateFolloweeStatsQuery, followeeID)
		if err != nil {
			r.GetLogger().Warn("Failed to update followee stats",
				zap.Error(err),
				zap.Int64("followee_id", followeeID),
			)
		}

		// Update following count in user_stats (if exists)
		updateFollowerStatsQuery := `
			UPDATE user_stats 
			SET following_count = (
				SELECT COUNT(*) FROM user_follows WHERE follower_id = $1
			)
			WHERE user_id = $1`
		
		_, err = tx.ExecContext(ctx, updateFollowerStatsQuery, followerID)
		if err != nil {
			r.GetLogger().Warn("Failed to update follower stats",
				zap.Error(err),
				zap.Int64("follower_id", followerID),
			)
		}

		r.GetLogger().Info("User follow relationship created",
			zap.Int64("follower_id", followerID),
			zap.Int64("followee_id", followeeID),
		)

		return nil
	})
}

// UnfollowUser removes a follow relationship between two users
func (r *userRepository) UnfollowUser(ctx context.Context, followerID, followeeID int64) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Delete the follow relationship
		query := `DELETE FROM user_follows WHERE follower_id = $1 AND followee_id = $2`
		
		result, err := tx.ExecContext(ctx, query, followerID, followeeID)
		if err != nil {
			return fmt.Errorf("failed to remove follow relationship: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return fmt.Errorf("follow relationship not found")
		}

		// Update follower count in user_stats
		updateFolloweeStatsQuery := `
			UPDATE user_stats 
			SET followers_count = (
				SELECT COUNT(*) FROM user_follows WHERE followee_id = $1
			)
			WHERE user_id = $1`
		
		_, err = tx.ExecContext(ctx, updateFolloweeStatsQuery, followeeID)
		if err != nil {
			r.GetLogger().Warn("Failed to update followee stats after unfollow",
				zap.Error(err),
				zap.Int64("followee_id", followeeID),
			)
		}

		// Update following count in user_stats
		updateFollowerStatsQuery := `
			UPDATE user_stats 
			SET following_count = (
				SELECT COUNT(*) FROM user_follows WHERE follower_id = $1
			)
			WHERE user_id = $1`
		
		_, err = tx.ExecContext(ctx, updateFollowerStatsQuery, followerID)
		if err != nil {
			r.GetLogger().Warn("Failed to update follower stats after unfollow",
				zap.Error(err),
				zap.Int64("follower_id", followerID),
			)
		}

		r.GetLogger().Info("User follow relationship removed",
			zap.Int64("follower_id", followerID),
			zap.Int64("followee_id", followeeID),
		)

		return nil
	})
}

// GetFollowers retrieves all users who follow the specified user
func (r *userRepository) GetFollowers(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.profile_url,
			u.expertise, u.affiliation, u.bio, u.role,
			u.is_online, u.last_seen, uf.created_at as followed_at,
			COALESCE(us.reputation_points, 0) as reputation_points,
			COALESCE(us.total_contributions, 0) as total_contributions
		FROM user_follows uf
		INNER JOIN users u ON uf.follower_id = u.id
		LEFT JOIN user_stats us ON u.id = us.user_id`

	whereClause := "uf.followee_id = $1 AND u.is_active = true"
	whereArgs := []interface{}{userID}

	// Default sort by follow date (most recent first)
	if params.Sort == "" {
		params.Sort = "followed_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get followers: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		var followedAt time.Time

		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.ProfileURL,
			&user.Expertise, &user.Affiliation, &user.Bio, &user.Role,
			&user.IsOnline, &user.LastSeen, &followedAt,
			&user.ReputationPoints, &user.TotalContributions,
		)
		if err != nil {
			continue
		}

		// Calculate user level
		user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)
		
		users = append(users, &user)
		lastCursor = r.encodeCursor(followedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
		Filters:    map[string]any{"user_id": userID, "type": "followers"},
	}, nil
}

// GetFollowing retrieves all users that the specified user follows
func (r *userRepository) GetFollowing(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.User], error) {
	baseQuery := `
		SELECT 
			u.id, u.username, u.display_name, u.profile_url,
			u.expertise, u.affiliation, u.bio, u.role,
			u.is_online, u.last_seen, uf.created_at as followed_at,
			COALESCE(us.reputation_points, 0) as reputation_points,
			COALESCE(us.total_contributions, 0) as total_contributions
		FROM user_follows uf
		INNER JOIN users u ON uf.followee_id = u.id
		LEFT JOIN user_stats us ON u.id = us.user_id`

	whereClause := "uf.follower_id = $1 AND u.is_active = true"
	whereArgs := []interface{}{userID}

	// Default sort by follow date (most recent first)
	if params.Sort == "" {
		params.Sort = "followed_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get following: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	var lastCursor string

	for rows.Next() {
		var user models.User
		var followedAt time.Time

		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.ProfileURL,
			&user.Expertise, &user.Affiliation, &user.Bio, &user.Role,
			&user.IsOnline, &user.LastSeen, &followedAt,
			&user.ReputationPoints, &user.TotalContributions,
		)
		if err != nil {
			continue
		}

		// Calculate user level
		user.Level, user.LevelColor = r.calculateUserLevel(user.ReputationPoints)
		
		users = append(users, &user)
		lastCursor = r.encodeCursor(followedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(users) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.User]{
		Data:       users,
		Pagination: meta,
		Filters:    map[string]any{"user_id": userID, "type": "following"},
	}, nil
}

// IsFollowing checks if one user follows another
func (r *userRepository) IsFollowing(ctx context.Context, followerID, followeeID int64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM user_follows 
			WHERE follower_id = $1 AND followee_id = $2
		)`

	var isFollowing bool
	err := r.QueryRowContext(ctx, query, followerID, followeeID).Scan(&isFollowing)
	if err != nil {
		return false, fmt.Errorf("failed to check follow status: %w", err)
	}

	return isFollowing, nil
}

// ===============================
// HELPER METHODS
// ===============================

// calculateUserLevel determines user level based on reputation points
func (r *userRepository) calculateUserLevel(points int) (string, string) {
	switch {
	case points >= 5000:
		return "Expert", "#8b5cf6"
	case points >= 2000:
		return "Advanced", "#3b82f6"
	case points >= 500:
		return "Intermediate", "#10b981"
	case points >= 100:
		return "Beginner", "#f59e0b"
	default:
		return "Newcomer", "#6b7280"
	}
}

// calculateNextLevelPoints calculates points needed for next level
func (r *userRepository) calculateNextLevelPoints(current int) int {
	levels := []int{100, 500, 2000, 5000}
	for _, threshold := range levels {
		if current < threshold {
			return threshold - current
		}
	}
	return 0 // Already at max level
}
