package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"
)

// CommunityStats represents community-wide statistics
type CommunityStats struct {
	MostActiveUsers []models.User `json:"most_active_users"`
	TopContributors []models.User `json:"top_contributors"`
	RecentBadges    []UserBadge   `json:"recent_badges"`
}

// UserBadge represents a badge awarded to a user
type UserBadge struct {
	ID            int       `json:"id"`
	BadgeName     string    `json:"badge_name"`
	Description   string    `json:"description"`
	Icon          string    `json:"icon"`
	Color         string    `json:"color"`
	Username      string    `json:"username"`
	ProfileURL    *string   `json:"profile_url,omitempty"`
	EarnedAt      time.Time `json:"earned_at"`
	EarnedAtHuman string    `json:"earned_at_human,omitempty"`
}

// LeaderboardUser represents a user in the leaderboard
type LeaderboardUser struct {
	ID                 int    `json:"id"`
	Username           string `json:"username"`
	ProfileURL         string `json:"profile_url"`
	ReputationPoints   int    `json:"reputation_points"`
	TotalContributions int    `json:"total_contributions"`
	PostsCount         int    `json:"posts_count"`
	QuestionsCount     int    `json:"questions_count"`
	CommentsCount      int    `json:"comments_count"`
	LikesReceived      int    `json:"likes_received"`
	BadgeCount         int    `json:"badge_count"`
	Level              string `json:"level"`
	LevelColor         string `json:"level_color"`
}

// UpdateUserStats updates a user's statistics and reputation
// Takes a context to support cancellation and timeouts
func UpdateUserStats(ctx context.Context, userID int) error {
	// Calculate user stats
	var postsCount, questionsCount, commentsCount, likesReceived, likesGiven int

	// Posts count
	err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM posts WHERE user_id = $1", userID).Scan(&postsCount)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while getting posts count: %w", err)
		}
		log.Printf("Error calculating posts count: %v", err)
		postsCount = 0
	}

	// Questions count
	err = database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM questions WHERE user_id = $1", userID).Scan(&questionsCount)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while getting questions count: %w", err)
		}
		log.Printf("Error calculating questions count: %v", err)
		questionsCount = 0
	}

	// Comments count
	err = database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM comments WHERE user_id = $1", userID).Scan(&commentsCount)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while getting comments count: %w", err)
		}
		log.Printf("Error calculating comments count: %v", err)
		commentsCount = 0
	}

	// Likes received (on posts and questions)
	likesQuery := `
		SELECT COALESCE(SUM(likes), 0) FROM (
			SELECT COUNT(*) as likes 
			FROM post_reactions pr 
			JOIN posts p ON pr.post_id = p.id 
			WHERE p.user_id = $1 AND pr.reaction = 'like'
			UNION ALL
			SELECT COUNT(*) as likes 
			FROM question_reactions qr 
			JOIN questions q ON qr.question_id = q.id 
			WHERE q.user_id = $1 AND qr.reaction = 'like'
		) as total_likes`
	err = database.DB.QueryRowContext(ctx, likesQuery, userID).Scan(&likesReceived)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while calculating likes received: %w", err)
		}
		log.Printf("Error calculating likes received: %v", err)
		likesReceived = 0
	}

	// Likes given
	likesGivenQuery := `
		SELECT COALESCE(SUM(likes_given), 0) FROM (
			SELECT COUNT(*) as likes_given FROM post_reactions WHERE user_id = $1 AND reaction = 'like'
			UNION ALL
			SELECT COUNT(*) as likes_given FROM question_reactions WHERE user_id = $1 AND reaction = 'like'
		) as total_given`
	err = database.DB.QueryRowContext(ctx, likesGivenQuery, userID).Scan(&likesGiven)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while calculating likes given: %w", err)
		}
		log.Printf("Error calculating likes given: %v", err)
		likesGiven = 0
	}

	// Check if context is done before proceeding with calculations
	if ctx.Err() != nil {
		return fmt.Errorf("operation canceled before calculating reputation: %w", ctx.Err())
	}

	// Calculate reputation points (simplified for example)
	reputationPoints := (postsCount * 5) + (questionsCount * 3) + (commentsCount * 2) + (likesReceived * 1)

	// Update user stats in database with context
	_, err = database.DB.ExecContext(ctx, `
		INSERT INTO user_stats (user_id, posts_count, questions_count, comments_count, 
			likes_received, likes_given, reputation_points, last_updated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			posts_count = EXCLUDED.posts_count,
			questions_count = EXCLUDED.questions_count,
			comments_count = EXCLUDED.comments_count,
			likes_received = EXCLUDED.likes_received,
			likes_given = EXCLUDED.likes_given,
			reputation_points = EXCLUDED.reputation_points,
			last_updated = NOW()
	`,
		userID, postsCount, questionsCount, commentsCount,
		likesReceived, likesGiven, reputationPoints,
	)

	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return fmt.Errorf("context error while updating user stats: %w", err)
		}
		return fmt.Errorf("failed to update user stats: %w", err)
	}

	// Check and award badges
	return CheckAndAwardBadges(ctx, userID, postsCount, questionsCount, commentsCount, likesReceived, reputationPoints)
}

// CheckAndAwardBadges checks if user has earned any new badges
func CheckAndAwardBadges(ctx context.Context, userID, postsCount, questionsCount, commentsCount, likesReceived, reputationPoints int) error {
	// Get all badges
	badges, err := GetAllBadges()
	if err != nil {
		log.Printf("Error getting badges: %v", err)
		return err
	}

	// Get user's current badges
	userBadges, err := GetUserBadges(userID)
	if err != nil {
		log.Printf("Error getting user badges: %v", err)
		return err
	}

	// Create a map of badge IDs the user already has
	userBadgeMap := make(map[int]bool)
	for _, badge := range userBadges {
		userBadgeMap[badge.ID] = true
	}

	// Check each badge's criteria
	for _, badge := range badges {
		// Skip if user already has this badge
		if userBadgeMap[badge.ID] {
			continue
		}

		// Check criteria
		var earned bool
		switch badge.CriteriaType {
		case "posts":
			earned = postsCount >= badge.CriteriaValue
		case "questions":
			earned = questionsCount >= badge.CriteriaValue
		case "comments":
			earned = commentsCount >= badge.CriteriaValue
		case "likes_received":
			earned = likesReceived >= badge.CriteriaValue
		case "reputation":
			earned = reputationPoints >= badge.CriteriaValue
		}

		if earned {
			// Award the badge using ExecContext
			_, err := database.DB.ExecContext(ctx, "INSERT INTO user_badges (user_id, badge_id) VALUES ($1, $2)", userID, badge.ID)
			if err != nil {
				log.Printf("Error awarding badge %s to user %d: %v", badge.Name, userID, err)
			} else {
				log.Printf("Awarded badge '%s' to user %d", badge.Name, userID)
				// Could send notification here
			}
		}
	}
	return nil
}

// GetLeaderboard returns the top users by reputation
func GetLeaderboard(ctx context.Context, limit int, period string) ([]LeaderboardUser, error) {
	var query string
	var args []interface{}

	switch period {
	case "week":
		query = `
			SELECT u.id, u.username, COALESCE(u.profile_url, '') as profile_url,
			       COALESCE(us.reputation_points, 0) as reputation_points,
			       COALESCE(us.total_contributions, 0) as total_contributions,
			       COALESCE(us.posts_count, 0) as posts_count,
			       COALESCE(us.questions_count, 0) as questions_count,
			       COALESCE(us.comments_count, 0) as comments_count,
			       COALESCE(us.likes_received, 0) as likes_received,
			       COALESCE(badge_count.count, 0) as badge_count
			FROM users u
			LEFT JOIN user_stats us ON u.id = us.user_id
			LEFT JOIN (
				SELECT user_id, COUNT(*) as count 
				FROM user_badges 
				GROUP BY user_id
			) badge_count ON u.id = badge_count.user_id
			WHERE u.is_active = true AND us.last_updated >= NOW() - INTERVAL '7 days'
			ORDER BY us.reputation_points DESC NULLS LAST, us.total_contributions DESC NULLS LAST
			LIMIT $1`
		args = append(args, limit)
	case "month":
		query = `
			SELECT u.id, u.username, COALESCE(u.profile_url, '') as profile_url,
			       COALESCE(us.reputation_points, 0) as reputation_points,
			       COALESCE(us.total_contributions, 0) as total_contributions,
			       COALESCE(us.posts_count, 0) as posts_count,
			       COALESCE(us.questions_count, 0) as questions_count,
			       COALESCE(us.comments_count, 0) as comments_count,
			       COALESCE(us.likes_received, 0) as likes_received,
			       COALESCE(badge_count.count, 0) as badge_count
			FROM users u
			LEFT JOIN user_stats us ON u.id = us.user_id
			LEFT JOIN (
				SELECT user_id, COUNT(*) as count 
				FROM user_badges 
				GROUP BY user_id
			) badge_count ON u.id = badge_count.user_id
			WHERE u.is_active = true AND us.last_updated >= NOW() - INTERVAL '30 days'
			ORDER BY us.reputation_points DESC NULLS LAST, us.total_contributions DESC NULLS LAST
			LIMIT $1`
		args = append(args, limit)
	default: // all time
		query = `
			SELECT u.id, u.username, COALESCE(u.profile_url, '') as profile_url,
			       COALESCE(us.reputation_points, 0) as reputation_points,
			       COALESCE(us.total_contributions, 0) as total_contributions,
			       COALESCE(us.posts_count, 0) as posts_count,
			       COALESCE(us.questions_count, 0) as questions_count,
			       COALESCE(us.comments_count, 0) as comments_count,
			       COALESCE(us.likes_received, 0) as likes_received,
			       COALESCE(badge_count.count, 0) as badge_count
			FROM users u
			LEFT JOIN user_stats us ON u.id = us.user_id
			LEFT JOIN (
				SELECT user_id, COUNT(*) as count 
				FROM user_badges 
				GROUP BY user_id
			) badge_count ON u.id = badge_count.user_id
			WHERE u.is_active = true
			ORDER BY us.reputation_points DESC NULLS LAST, us.total_contributions DESC NULLS LAST
			LIMIT $1`
		args = append(args, limit)
	}

	rows, err := database.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []LeaderboardUser
	for rows.Next() {
		var user LeaderboardUser
		var profileURL sql.NullString

		err := rows.Scan(&user.ID, &user.Username, &profileURL,
			&user.ReputationPoints, &user.TotalContributions,
			&user.PostsCount, &user.QuestionsCount, &user.CommentsCount,
			&user.LikesReceived, &user.BadgeCount)
		if err != nil {
			log.Printf("Error scanning leaderboard user: %v", err)
			continue
		}

		if profileURL.Valid {
			user.ProfileURL = profileURL.String
		}

		// Determine user level
		user.Level, user.LevelColor = GetUserLevel(user.ReputationPoints)

		users = append(users, user)
	}

	return users, nil
}

// GetUserLevel returns the user's level and color based on reputation
func GetUserLevel(reputation int) (string, string) {
	switch {
	case reputation >= 1000:
		return "Expert", "#eab308"
	case reputation >= 500:
		return "Advanced", "#8b5cf6"
	case reputation >= 200:
		return "Intermediate", "#3b82f6"
	case reputation >= 50:
		return "Beginner", "#10b981"
	default:
		return "Newcomer", "#6b7280"
	}
}

// GetAllBadges retrieves all available badges from the database
func GetAllBadges() ([]models.Badge, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	query := `
		SELECT id, name, description, icon, color, criteria_type, criteria_value, is_active, created_at, updated_at
		FROM badges
		WHERE is_active = true
		ORDER BY criteria_value ASC, name ASC
	`

	rows, err := database.DB.QueryContext(context.Background(), query)
	if err != nil {
		if err == sql.ErrNoRows {
			return []models.Badge{}, nil
		}
		return nil, fmt.Errorf("failed to fetch badges: %w", err)
	}
	defer rows.Close()

	var badges []models.Badge
	for rows.Next() {
		var badge models.Badge
		err := rows.Scan(
			&badge.ID,
			&badge.Name,
			&badge.Description,
			&badge.Icon,
			&badge.Color,
			&badge.CriteriaType,
			&badge.CriteriaValue,
			&badge.IsActive,
			&badge.CreatedAt,
			&badge.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning badge: %w", err)
		}
		badges = append(badges, badge)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating badges: %w", err)
	}

	return badges, nil
}

// GetUserBadges returns badges earned by a specific user
func GetUserBadges(userID int) ([]models.Badge, error) {
	// Create a new context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT b.id, b.name, b.description, b.icon, b.color, 
		       b.criteria_type, b.criteria_value, b.is_active, 
		       b.created_at, b.updated_at, ub.earned_at 
		FROM badges b
		JOIN user_badges ub ON b.id = ub.badge_id 
		WHERE ub.user_id = $1 AND b.is_active = true
		ORDER BY ub.earned_at DESC
	`

	// Use QueryContext with the timeout context
	rows, err := database.DB.QueryContext(ctx, query, userID)
	if err != nil {
		if err == context.DeadlineExceeded {
			log.Printf("Timeout fetching user badges for user %d", userID)
			return nil, fmt.Errorf("timeout fetching badges: %w", err)
		}
		log.Printf("Error querying user badges: %v", err)
		return nil, fmt.Errorf("error querying user badges: %w", err)
	}
	defer rows.Close()

	var badges []models.Badge
	for rows.Next() {
		var badge models.Badge
		var earnedAt time.Time

		// Scan the row into the badge model
		if err := rows.Scan(
			&badge.ID,
			&badge.Name,
			&badge.Description,
			&badge.Icon,
			&badge.Color,
			&badge.CriteriaType,
			&badge.CriteriaValue,
			&badge.IsActive,
			&badge.CreatedAt,
			&badge.UpdatedAt,
			&earnedAt,
		); err != nil {
			log.Printf("Error scanning badge for user %d: %v", userID, err)
			continue
		}
		badges = append(badges, badge)
	}

	// Check for any error that occurred during iteration
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating badges for user %d: %v", userID, err)
		return badges, fmt.Errorf("error iterating badges: %w", err)
	}

	return badges, nil
}

// GetRecentBadgeAwards returns recent badge awards across all users
func GetRecentBadgeAwards(limit int) ([]UserBadge, error) {
	// Create a new context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT ub.id, b.name as badge_name, b.description, b.icon, b.color,
		       u.username, u.profile_url, ub.earned_at
		FROM user_badges ub
		JOIN badges b ON ub.badge_id = b.id
		JOIN users u ON ub.user_id = u.id
		WHERE b.is_active = true AND u.is_active = true
		ORDER BY ub.earned_at DESC
		LIMIT $1`

	// Use QueryContext with the timeout context
	rows, err := database.DB.QueryContext(ctx, query, limit)
	if err != nil {
		if err == context.DeadlineExceeded {
			log.Printf("Timeout fetching recent badge awards (limit: %d)", limit)
			return nil, fmt.Errorf("timeout fetching recent badge awards: %w", err)
		}
		log.Printf("Error querying recent badge awards: %v", err)
		return nil, fmt.Errorf("error querying recent badge awards: %w", err)
	}
	defer rows.Close()

	var recentBadges []UserBadge
	for rows.Next() {
		var badge UserBadge
		var profileURL sql.NullString

		// Scan the row into the UserBadge struct
		if err := rows.Scan(
			&badge.ID,
			&badge.BadgeName,
			&badge.Description,
			&badge.Icon,
			&badge.Color,
			&badge.Username,
			&profileURL,
			&badge.EarnedAt,
		); err != nil {
			log.Printf("Error scanning recent badge award: %v", err)
			continue
		}

		// Handle nullable profile URL
		if profileURL.Valid && profileURL.String != "" {
			badge.ProfileURL = &profileURL.String
		}

		// Format the earned at time for display
		badge.EarnedAtHuman = utils.TimeAgo(badge.EarnedAt)
		recentBadges = append(recentBadges, badge)
	}

	// Check for any error that occurred during iteration
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating recent badge awards: %v", err)
		return recentBadges, fmt.Errorf("error iterating recent badge awards: %w", err)
	}

	return recentBadges, nil
}

// API Handlers

// GetCommunityStatsHandler returns community statistics for the dashboard
func GetCommunityStatsHandler(w http.ResponseWriter, r *http.Request) {
	// Get top contributors
	topContributors, err := GetLeaderboard(r.Context(), 5, "all-time")
	if err != nil {
		log.Printf("Error getting leaderboard: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get recent badge awards
	recentBadges, err := GetRecentBadgeAwards(3)
	if err != nil {
		log.Printf("Error getting recent badges: %v", err)
		recentBadges = []UserBadge{} // Return empty slice instead of error
	}

	// Convert to response format
	stats := CommunityStats{
		TopContributors: make([]models.User, len(topContributors)),
		RecentBadges:    recentBadges,
	}

	// Convert LeaderboardUser to models.User
	for i, leader := range topContributors {
		stats.TopContributors[i] = models.User{
			ID:                 int64(leader.ID),
			Username:           leader.Username,
			ProfileURL:         &leader.ProfileURL,
			ReputationPoints:   leader.ReputationPoints,
			TotalContributions: leader.TotalContributions,
			BadgeCount:         leader.BadgeCount,
			Level:              leader.Level,
			LevelColor:         leader.LevelColor,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetUserStatsHandler returns detailed stats for a specific user
func GetUserStatsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(int)
	if !ok {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get user stats
	var stats LeaderboardUser
	query := `
		SELECT u.id, u.username, COALESCE(u.profile_url, '') as profile_url,
		       COALESCE(us.reputation_points, 0) as reputation_points,
		       COALESCE(us.total_contributions, 0) as total_contributions,
		       COALESCE(us.posts_count, 0) as posts_count,
		       COALESCE(us.questions_count, 0) as questions_count,
		       COALESCE(us.comments_count, 0) as comments_count,
		       COALESCE(us.likes_received, 0) as likes_received,
		       COALESCE(badge_count.count, 0) as badge_count
		FROM users u
		LEFT JOIN user_stats us ON u.id = us.user_id
		LEFT JOIN (
			SELECT user_id, COUNT(*) as count 
			FROM user_badges 
			GROUP BY user_id
		) badge_count ON u.id = badge_count.user_id
		WHERE u.id = $1`

	// Use context from request for database operations
	ctx := r.Context()
	err := database.DB.QueryRowContext(ctx, query, userID).Scan(
		&stats.ID, &stats.Username, &stats.ProfileURL,
		&stats.ReputationPoints, &stats.TotalContributions,
		&stats.PostsCount, &stats.QuestionsCount, &stats.CommentsCount,
		&stats.LikesReceived, &stats.BadgeCount,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			log.Printf("Error getting user stats: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Get user badges with context
	badges, err := GetUserBadges(userID)
	if err != nil {
		log.Printf("Error getting user badges: %v", err)
		badges = []models.Badge{} // Return empty slice instead of error
	}

	// Set level and color based on reputation
	stats.Level, stats.LevelColor = GetUserLevel(stats.ReputationPoints)

	response := map[string]interface{}{
		"user_stats": stats,
		"badges":     badges,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
