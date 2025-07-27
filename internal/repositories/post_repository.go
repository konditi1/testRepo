// file: internal/repositories/post_repository.go
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

// postRepository implements PostRepository with advanced optimizations
type postRepository struct {
	*BaseRepository
}

// NewPostRepository creates a new optimized post repository
func NewPostRepository(db *database.Manager, logger *zap.Logger) PostRepository {
	return &postRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// BASIC CRUD OPERATIONS
// ===============================

// Create creates a new post with proper validation
func (r *postRepository) Create(ctx context.Context, post *models.Post) error {
	query := `
		INSERT INTO posts (
			user_id, title, content, category, status,
			image_url, image_public_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	err := r.QueryRowContext(
		ctx, query,
		post.UserID, post.Title, post.Content, post.Category,
		post.Status, post.ImageURL, post.ImagePublicID,
	).Scan(&post.ID, &post.CreatedAt, &post.UpdatedAt)

	if err != nil {
		r.GetLogger().Error("Failed to create post",
			zap.Error(err),
			zap.Int64("user_id", post.UserID),
			zap.String("title", post.Title),
		)
		return fmt.Errorf("failed to create post: %w", err)
	}

	// Initialize engagement metrics
	post.LikesCount = 0
	post.DislikesCount = 0
	post.CommentsCount = 0
	post.ViewsCount = 0

	r.GetLogger().Info("Post created successfully",
		zap.Int64("post_id", post.ID),
		zap.Int64("user_id", post.UserID),
		zap.String("title", post.Title),
	)

	return nil
}

// GetByID retrieves a post by ID with all related data (prevents N+1)
func (r *postRepository) GetByID(ctx context.Context, id int64, userID *int64) (*models.Post, error) {
	query := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category, p.status,
			p.image_url, p.image_public_id, p.created_at, p.updated_at,
			-- Author information (JOIN to prevent N+1)
			u.username, u.display_name, u.profile_url,
			-- Engagement metrics (computed)
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			-- User-specific reaction (if userID provided)
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		-- Aggregate reaction counts to prevent N+1
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		-- Aggregate comment counts to prevent N+1
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		-- User-specific reaction (conditional join)
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $2
		WHERE p.id = $1 AND p.status != 'deleted' AND u.is_active = true`

	var post models.Post
	var userReaction sql.NullString

	scanArgs := []interface{}{
		&post.ID, &post.UserID, &post.Title, &post.Content,
		&post.Category, &post.Status, &post.ImageURL, &post.ImagePublicID,
		&post.CreatedAt, &post.UpdatedAt,
		&post.Username, &post.DisplayName, &post.AuthorProfileURL,
		&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
		&userReaction,
	}

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{id, *userID}
	} else {
		queryArgs = []interface{}{id, nil}
	}

	err := r.QueryRowContext(ctx, query, queryArgs...).Scan(scanArgs...)
	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get post by ID: %w", err)
	}

	// Set user-specific fields
	if userID != nil {
		post.IsOwner = post.UserID == *userID
		if userReaction.Valid {
			post.UserReaction = &userReaction.String
		}
	}

	// Generate helper fields
	post.Preview = r.generatePreview(post.Content)
	post.CategoryArray = strings.Split(post.Category, ",")
	post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)
	post.UpdatedAtHuman = r.formatTimeHuman(post.UpdatedAt)

	return &post, nil
}

// Update updates a post's information
func (r *postRepository) Update(ctx context.Context, post *models.Post) error {
	query := `
		UPDATE posts SET
			title = $2, content = $3, category = $4,
			image_url = $5, image_public_id = $6,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $7 AND status != 'deleted'
		RETURNING updated_at`

	err := r.QueryRowContext(
		ctx, query,
		post.ID, post.Title, post.Content, post.Category,
		post.ImageURL, post.ImagePublicID, post.UserID,
	).Scan(&post.UpdatedAt)

	if err != nil {
		if r.IsNotFound(err) {
			return fmt.Errorf("post not found or not owned by user")
		}
		return fmt.Errorf("failed to update post: %w", err)
	}

	r.GetLogger().Info("Post updated successfully",
		zap.Int64("post_id", post.ID),
		zap.Int64("user_id", post.UserID),
	)

	return nil
}

// Delete soft deletes a post
func (r *postRepository) Delete(ctx context.Context, id int64) error {
	query := `
		UPDATE posts 
		SET status = 'deleted', updated_at = CURRENT_TIMESTAMP 
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete post: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("post not found")
	}

	return nil
}

// ===============================
// LISTING AND FILTERING
// ===============================

// List retrieves posts with pagination and user context
func (r *postRepository) List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1`

	whereClause := "p.status = 'published' AND u.is_active = true"
	whereArgs := []interface{}{}

	// Add user ID for user-specific data
	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}

	// Build paginated query
	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list posts: %w", err)
	}
	defer rows.Close()

	posts, lastCursor := r.scanPostRows(rows, userID)

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
	}, nil
}

// GetByUserID retrieves posts by a specific user
func (r *postRepository) GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category, p.status,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id`

	whereClause := "p.user_id = $1 AND p.status != 'deleted' AND u.is_active = true"
	whereArgs := []interface{}{userID}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get posts by user ID: %w", err)
	}
	defer rows.Close()

	var posts []*models.Post
	var lastCursor string

	for rows.Next() {
		var post models.Post

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.Status, &post.ImageURL,
			&post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
		)
		if err != nil {
			continue
		}

		// Set ownership
		post.IsOwner = true // All posts belong to the user in this query

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)
		post.UpdatedAtHuman = r.formatTimeHuman(post.UpdatedAt)

		posts = append(posts, &post)
		lastCursor = r.encodeCursor(post.CreatedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
	}, nil
}

// GetByStatus retrieves posts by status
func (r *postRepository) GetByStatus(ctx context.Context, status string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1`

	whereClause := "p.status = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, status)

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get posts by status: %w", err)
	}
	defer rows.Close()

	posts, lastCursor := r.scanPostRows(rows, userID)

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
		Filters:    map[string]any{"status": status},
	}, nil
}

// GetByCategory retrieves posts by category
func (r *postRepository) GetByCategory(ctx context.Context, category string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1`

	whereClause := "p.status = 'published' AND u.is_active = true AND p.category = $2"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, category)

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get posts by category: %w", err)
	}
	defer rows.Close()

	posts, lastCursor := r.scanPostRows(rows, userID)

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
		Filters:    map[string]any{"category": category},
	}, nil
}

// GetTrending retrieves trending posts based on engagement
func (r *postRepository) GetTrending(ctx context.Context, limit int, userID *int64) ([]*models.Post, error) {
	query := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction,
			-- Trending score calculation
			(
				COALESCE(pr_stats.likes_count, 0) * 3 +
				COALESCE(c_stats.comments_count, 0) * 2 +
				COALESCE(p.views_count, 0) * 0.1 +
				CASE WHEN p.created_at > CURRENT_TIMESTAMP - INTERVAL '7 days' THEN 10 ELSE 0 END
			) as trending_score
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1
		WHERE p.status = 'published' AND u.is_active = true
		AND p.created_at > CURRENT_TIMESTAMP - INTERVAL '30 days'
		ORDER BY trending_score DESC, p.created_at DESC
		LIMIT $2`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{*userID, limit}
	} else {
		queryArgs = []interface{}{nil, limit}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending posts: %w", err)
	}
	defer rows.Close()

	var posts []*models.Post
	for rows.Next() {
		var post models.Post
		var userReaction sql.NullString
		var trendingScore float64

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.ImageURL, &post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
			&userReaction, &trendingScore,
		)
		if err != nil {
			continue
		}

		// Set user-specific fields
		if userID != nil {
			post.IsOwner = post.UserID == *userID
			if userReaction.Valid {
				post.UserReaction = &userReaction.String
			}
		}

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)

		posts = append(posts, &post)
	}

	return posts, nil
}

// GetFeatured retrieves featured posts
func (r *postRepository) GetFeatured(ctx context.Context, limit int, userID *int64) ([]*models.Post, error) {
	// This could be based on admin selection, high engagement, or other criteria
	query := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1
		WHERE p.status = 'published' AND u.is_active = true
		AND COALESCE(pr_stats.likes_count, 0) >= 5  -- Minimum likes for featured
		ORDER BY pr_stats.likes_count DESC, p.created_at DESC
		LIMIT $2`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{*userID, limit}
	} else {
		queryArgs = []interface{}{nil, limit}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get featured posts: %w", err)
	}
	defer rows.Close()

	posts, _ := r.scanPostRows(rows, userID)
	return posts, nil
}

// GetDrafts retrieves draft posts for a user
func (r *postRepository) GetDrafts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category, p.status,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id`

	whereClause := "p.user_id = $1 AND p.status = 'draft' AND u.is_active = true"
	whereArgs := []interface{}{userID}

	if params.Sort == "" {
		params.Sort = "updated_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft posts: %w", err)
	}
	defer rows.Close()

	var posts []*models.Post
	var lastCursor string

	for rows.Next() {
		var post models.Post

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.Status, &post.ImageURL,
			&post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
		)
		if err != nil {
			continue
		}

		// Set ownership (all drafts belong to the user)
		post.IsOwner = true

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)
		post.UpdatedAtHuman = r.formatTimeHuman(post.UpdatedAt)

		posts = append(posts, &post)
		lastCursor = r.encodeCursor(post.UpdatedAt)
	}

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
		Filters:    map[string]any{"user_id": userID, "status": "draft"},
	}, nil
}

// ===============================
// SEARCH OPERATIONS
// ===============================

// Search searches posts by title and content
func (r *postRepository) Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction,
			-- Search ranking
			ts_rank(
				to_tsvector('english', p.title || ' ' || p.content),
				plainto_tsquery('english', $2)
			) as search_rank
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1`

	whereClause := `
		p.status = 'published' AND u.is_active = true
		AND (
			to_tsvector('english', p.title || ' ' || p.content) @@ plainto_tsquery('english', $2)
			OR p.title ILIKE $3
			OR p.content ILIKE $3
		)`

	searchTerm := "%" + strings.ToLower(query) + "%"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, query, searchTerm)

	// Override sort to use search ranking
	params.Sort = "search_rank"
	params.Order = "desc"

	sqlQuery, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, sqlQuery, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search posts: %w", err)
	}
	defer rows.Close()

	var posts []*models.Post
	var lastCursor string

	for rows.Next() {
		var post models.Post
		var userReaction sql.NullString
		var searchRank float64

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.ImageURL, &post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
			&userReaction, &searchRank,
		)
		if err != nil {
			continue
		}

		// Set user-specific fields
		if userID != nil {
			post.IsOwner = post.UserID == *userID
			if userReaction.Valid {
				post.UserReaction = &userReaction.String
			}
		}

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)

		posts = append(posts, &post)
		lastCursor = r.encodeCursor(post.CreatedAt)
	}

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
		Filters:    map[string]any{"query": query},
	}, nil
}

// SearchByTags searches posts by tags (if implemented)
func (r *postRepository) SearchByTags(ctx context.Context, tags []string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Post], error) {
	// This would require a tags system - placeholder implementation
	// For now, search in content for tag-like terms
	query := strings.Join(tags, " ")
	return r.Search(ctx, query, params, userID)
}

// ===============================
// ENGAGEMENT OPERATIONS
// ===============================

// AddReaction adds or updates a user's reaction to a post
func (r *postRepository) AddReaction(ctx context.Context, postID, userID int64, reactionType string) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Upsert reaction
		query := `
			INSERT INTO post_reactions (post_id, user_id, reaction, created_at, updated_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (post_id, user_id)
			DO UPDATE SET 
				reaction = EXCLUDED.reaction,
				updated_at = EXCLUDED.updated_at`

		_, err := tx.ExecContext(ctx, query, postID, userID, reactionType)
		return err
	})
}

// RemoveReaction removes a user's reaction from a post
func (r *postRepository) RemoveReaction(ctx context.Context, postID, userID int64) error {
	query := `DELETE FROM post_reactions WHERE post_id = $1 AND user_id = $2`
	_, err := r.ExecContext(ctx, query, postID, userID)
	return err
}

// GetUserReaction gets a user's reaction to a post
func (r *postRepository) GetUserReaction(ctx context.Context, postID, userID int64) (*string, error) {
	query := `SELECT reaction FROM post_reactions WHERE post_id = $1 AND user_id = $2`

	var reaction string
	err := r.QueryRowContext(ctx, query, postID, userID).Scan(&reaction)
	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &reaction, nil
}

// GetReactionCounts gets like and dislike counts for a post
func (r *postRepository) GetReactionCounts(ctx context.Context, postID int64) (likes, dislikes int, err error) {
	query := `
		SELECT 
			COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes,
			COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes
		FROM post_reactions 
		WHERE post_id = $1`

	err = r.QueryRowContext(ctx, query, postID).Scan(&likes, &dislikes)
	return likes, dislikes, err
}

// ===============================
// BATCH OPERATIONS
// ===============================

// GetByIDs retrieves multiple posts by IDs (prevents N+1)
func (r *postRepository) GetByIDs(ctx context.Context, ids []int64, userID *int64) ([]*models.Post, error) {
	if len(ids) == 0 {
		return []*models.Post{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)

	if userID != nil {
		args[0] = *userID
	} else {
		args[0] = nil
	}

	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(`
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			ur.reaction as user_reaction
		FROM posts p
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1
		WHERE p.id IN (%s) AND p.status != 'deleted' AND u.is_active = true
		ORDER BY p.created_at DESC`, strings.Join(placeholders, ","))

	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get posts by IDs: %w", err)
	}
	defer rows.Close()

	posts, _ := r.scanPostRows(rows, userID)
	return posts, nil
}

// BulkUpdateStatus updates status for multiple posts
func (r *postRepository) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}

	query := `UPDATE posts SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = ANY($2)`
	_, err := r.ExecContext(ctx, query, status, ids)
	return err
}

// IncrementViews increments view count for a post
func (r *postRepository) IncrementViews(ctx context.Context, postID int64) error {
	query := `
		UPDATE posts 
		SET views_count = COALESCE(views_count, 0) + 1
		updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`
	_, err := r.ExecContext(ctx, query, postID)
	if err != nil {
		r.GetLogger().Error("Failed to increment views count",
			zap.Error(err),
			zap.Int64("post_id", postID),
		)
	}
	return err
}

// ModeratePost handles moderation actions on a post
func (r *postRepository) ModeratePost(ctx context.Context, postID, moderatorID int64, action, reason string) error {
	// Validate action
	validActions := map[string]bool{
		"approve": true,
		"reject":  true,
		"hide":    true,
		"delete":  true,
	}

	if !validActions[action] {
		return fmt.Errorf("invalid moderation action: %s", action)
	}

	// Start transaction
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// 1. Update post status and moderation info
		updateQuery := `
			UPDATE posts 
			SET 
				status = $1,
				moderated_at = NOW(),
				moderated_by = $2,
				moderation_reason = $3,
				updated_at = NOW()
			WHERE id = $4
			RETURNING id`

		var updatedID int64
		err := tx.QueryRowContext(
			ctx,
			updateQuery,
			action,
			moderatorID,
			reason,
			postID,
		).Scan(&updatedID)

		if err != nil {
			r.GetLogger().Error("Failed to update post moderation status",
				zap.Error(err),
				zap.Int64("post_id", postID),
				zap.Int64("moderator_id", moderatorID),
				zap.String("action", action),
			)
			return fmt.Errorf("failed to update post status: %w", err)
		}

		// 2. Log the moderation action
		logQuery := `
			INSERT INTO moderation_logs 
			(post_id, moderator_id, action, reason, created_at)
			VALUES ($1, $2, $3, $4, NOW())`

		_, err = tx.ExecContext(
			ctx,
			logQuery,
			postID,
			moderatorID,
			action,
			reason,
		)

		if err != nil {
			r.GetLogger().Error("Failed to log moderation action",
				zap.Error(err),
				zap.Int64("post_id", postID),
				zap.Int64("moderator_id", moderatorID),
			)
			// Don't fail the entire operation if logging fails
		}

		r.GetLogger().Info("Post moderated successfully",
			zap.Int64("post_id", postID),
			zap.Int64("moderator_id", moderatorID),
			zap.String("action", action),
		)

		return nil
	})
}

// IncrementShareCount increments the share count for a post
func (r *postRepository) IncrementShareCount(ctx context.Context, postID int64) error {
	query := `UPDATE posts SET shares_count = COALESCE(shares_count, 0) + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.ExecContext(ctx, query, postID)
	if err != nil {
		r.GetLogger().Error("Failed to increment share count",
			zap.Error(err),
			zap.Int64("post_id", postID),
		)
		return fmt.Errorf("failed to increment share count: %w", err)
	}
	return nil
}

// ===============================
// ANALYTICS
// ===============================

// GetPostStats retrieves detailed statistics for a post
func (r *postRepository) GetPostStats(ctx context.Context, postID int64) (*PostStats, error) {
	query := `
		SELECT 
			p.id,
			COALESCE(p.views_count, 0) as views_count,
			COALESCE(reactions.likes_count, 0) as likes_count,
			COALESCE(reactions.dislikes_count, 0) as dislikes_count,
			COALESCE(comments.comments_count, 0) as comments_count,
			0 as shares_count -- Placeholder for future feature
		FROM posts p
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			WHERE post_id = $1
			GROUP BY post_id
		) reactions ON p.id = reactions.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id = $1
			GROUP BY post_id
		) comments ON p.id = comments.post_id
		WHERE p.id = $1`

	var stats PostStats
	err := r.QueryRowContext(ctx, query, postID).Scan(
		&stats.PostID, &stats.ViewsCount, &stats.LikesCount,
		&stats.DislikesCount, &stats.CommentsCount, &stats.SharesCount,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get post stats: %w", err)
	}

	return &stats, nil
}

// GetUserPostStats retrieves post statistics for a user
func (r *postRepository) GetUserPostStats(ctx context.Context, userID int64) (*UserPostStats, error) {
	query := `
		SELECT 
			$1 as user_id,
			COUNT(*) as total_posts,
			COUNT(CASE WHEN status = 'published' THEN 1 END) as published_posts,
			COUNT(CASE WHEN status = 'draft' THEN 1 END) as draft_posts,
			COALESCE(SUM(views_count), 0) as total_views,
			COALESCE(likes_stats.total_likes, 0) as total_likes,
			COALESCE(comments_stats.total_comments, 0) as total_comments
		FROM posts p
		LEFT JOIN (
			SELECT 
				p.user_id,
				COUNT(pr.id) as total_likes
			FROM posts p
			LEFT JOIN post_reactions pr ON p.id = pr.post_id AND pr.reaction = 'like'
			WHERE p.user_id = $1
			GROUP BY p.user_id
		) likes_stats ON p.user_id = likes_stats.user_id
		LEFT JOIN (
			SELECT 
				p.user_id,
				COUNT(c.id) as total_comments
			FROM posts p
			LEFT JOIN comments c ON p.id = c.post_id
			WHERE p.user_id = $1
			GROUP BY p.user_id
		) comments_stats ON p.user_id = comments_stats.user_id
		WHERE p.user_id = $1
		GROUP BY p.user_id, likes_stats.total_likes, comments_stats.total_comments`

	var stats UserPostStats
	err := r.QueryRowContext(ctx, query, userID).Scan(
		&stats.UserID, &stats.TotalPosts, &stats.PublishedPosts,
		&stats.DraftPosts, &stats.TotalViews, &stats.TotalLikes, &stats.TotalComments,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return &UserPostStats{UserID: userID}, nil
		}
		return nil, fmt.Errorf("failed to get user post stats: %w", err)
	}

	return &stats, nil
}

// GetCategoryStats retrieves statistics by category
func (r *postRepository) GetCategoryStats(ctx context.Context) ([]*CategoryStats, error) {
	query := `
		SELECT 
			p.category,
			COUNT(*) as posts_count,
			0 as questions_count, -- This would come from questions table
			COALESCE(SUM(p.views_count), 0) as total_views,
			COUNT(DISTINCT p.user_id) as active_authors
		FROM posts p
		WHERE p.status = 'published'
		GROUP BY p.category
		ORDER BY posts_count DESC`

	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get category stats: %w", err)
	}
	defer rows.Close()

	var stats []*CategoryStats
	for rows.Next() {
		var stat CategoryStats
		err := rows.Scan(
			&stat.Category, &stat.PostsCount, &stat.QuestionsCount,
			&stat.TotalViews, &stat.ActiveAuthors,
		)
		if err != nil {
			continue
		}
		stats = append(stats, &stat)
	}

	return stats, nil
}

// ===============================
// REPORT OPERATIONS
// ===============================

// AddReport adds a report for a post
func (r *postRepository) AddReport(ctx context.Context, postID, reporterID int64, reason, description string) error {
	query := `
		INSERT INTO post_reports 
		(post_id, reporter_id, reason, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'pending', NOW(), NOW())
		ON CONFLICT (post_id, reporter_id) 
		DO UPDATE SET 
			reason = EXCLUDED.reason,
			description = EXCLUDED.description,
			status = 'pending',
			updated_at = NOW()
		RETURNING id`

	var id int64
	err := r.db.QueryRowContext(
		ctx, query,
		postID, reporterID, reason, description,
	).Scan(&id)

	if err != nil {
		r.logger.Error("Failed to add post report",
			zap.Error(err),
			zap.Int64("post_id", postID),
			zap.Int64("reporter_id", reporterID),
		)
		return fmt.Errorf("failed to add post report: %w", err)
	}

	// Log the successful report
	r.logger.Info("Post report added successfully",
		zap.Int64("report_id", id),
		zap.Int64("post_id", postID),
		zap.Int64("reporter_id", reporterID),
	)

	return nil
}

// ===============================
// BOOKMARK OPERATIONS
// ===============================

// AddBookmark adds a bookmark for a user on a post
func (r *postRepository) AddBookmark(ctx context.Context, postID, userID int64) error {
	query := `
		INSERT INTO post_bookmarks (post_id, user_id, created_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (post_id, user_id) DO NOTHING`

	_, err := r.ExecContext(ctx, query, postID, userID)
	if err != nil {
		r.GetLogger().Error("Failed to add bookmark",
			zap.Error(err),
			zap.Int64("post_id", postID),
			zap.Int64("user_id", userID),
		)
		return fmt.Errorf("failed to add bookmark: %w", err)
	}

	r.GetLogger().Info("Bookmark added successfully",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// RemoveBookmark removes a bookmark for a user on a post
func (r *postRepository) RemoveBookmark(ctx context.Context, postID, userID int64) error {
	query := `DELETE FROM post_bookmarks WHERE post_id = $1 AND user_id = $2`

	result, err := r.ExecContext(ctx, query, postID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove bookmark: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("bookmark not found")
	}

	r.GetLogger().Info("Bookmark removed successfully",
		zap.Int64("post_id", postID),
		zap.Int64("user_id", userID),
	)

	return nil
}

// IsBookmarked checks if a user has bookmarked a post
func (r *postRepository) IsBookmarked(ctx context.Context, postID, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM post_bookmarks WHERE post_id = $1 AND user_id = $2)`

	var exists bool
	err := r.QueryRowContext(ctx, query, postID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check bookmark status: %w", err)
	}

	return exists, nil
}

// GetBookmarkedPosts retrieves all bookmarked posts for a user
func (r *postRepository) GetBookmarkedPosts(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Post], error) {
	baseQuery := `
		SELECT 
			p.id, p.user_id, p.title, p.content, p.category,
			p.image_url, p.created_at, p.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(pr_stats.likes_count, 0) as likes_count,
			COALESCE(pr_stats.dislikes_count, 0) as dislikes_count,
			COALESCE(c_stats.comments_count, 0) as comments_count,
			COALESCE(p.views_count, 0) as views_count,
			ur.reaction as user_reaction,
			pb.created_at as bookmarked_at
		FROM post_bookmarks pb
		INNER JOIN posts p ON pb.post_id = p.id
		INNER JOIN users u ON p.user_id = u.id
		LEFT JOIN (
			SELECT 
				post_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM post_reactions 
			GROUP BY post_id
		) pr_stats ON p.id = pr_stats.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) c_stats ON p.id = c_stats.post_id
		LEFT JOIN post_reactions ur ON p.id = ur.post_id AND ur.user_id = $1`

	whereClause := "pb.user_id = $1 AND p.status = 'published' AND u.is_active = true"
	whereArgs := []interface{}{userID}

	// Default sort by bookmark creation time
	if params.Sort == "" {
		params.Sort = "bookmarked_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get bookmarked posts: %w", err)
	}
	defer rows.Close()

	var posts []*models.Post
	var lastCursor string

	for rows.Next() {
		var post models.Post
		var userReaction sql.NullString
		var bookmarkedAt time.Time

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.ImageURL, &post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
			&userReaction, &bookmarkedAt,
		)
		if err != nil {
			continue
		}

		// Set user-specific fields
		post.IsOwner = post.UserID == userID
		if userReaction.Valid {
			post.UserReaction = &userReaction.String
		}

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)
		post.UpdatedAtHuman = r.formatTimeHuman(post.UpdatedAt)

		posts = append(posts, &post)
		lastCursor = r.encodeCursor(bookmarkedAt)
	}

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(posts) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Post]{
		Data:       posts,
		Pagination: meta,
		Filters:    map[string]any{"user_id": userID, "bookmarked": true},
	}, nil
}

// ===============================
// ANALYTICS
// ===============================

// GetPostAnalytics retrieves detailed post analytics over time for a user
func (r *postRepository) GetPostAnalytics(ctx context.Context, userID int64, days int) (*PostAnalytics, error) {
	startDate := time.Now().AddDate(0, 0, -days)

	// Get overall stats
	overallQuery := `
		SELECT 
			COUNT(*) as total_posts,
			COALESCE(SUM(p.views_count), 0) as total_views,
			COALESCE(likes_stats.total_likes, 0) as total_likes
		FROM posts p
		LEFT JOIN (
			SELECT 
				COUNT(pr.id) as total_likes
			FROM posts p2
			LEFT JOIN post_reactions pr ON p2.id = pr.post_id AND pr.reaction = 'like'
			WHERE p2.user_id = $1 AND p2.created_at >= $2
		) likes_stats ON true
		WHERE p.user_id = $1 AND p.created_at >= $2`

	var analytics PostAnalytics
	analytics.UserID = userID
	analytics.Days = days

	err := r.QueryRowContext(ctx, overallQuery, userID, startDate).Scan(
		&analytics.TotalPosts, &analytics.TotalViews, &analytics.TotalLikes,
	)
	if err != nil && !r.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get overall analytics: %w", err)
	}

	// Get daily stats
	dailyQuery := `
		SELECT 
			DATE(p.created_at) as date,
			COUNT(*) as posts_count,
			COALESCE(SUM(p.views_count), 0) as total_views,
			COALESCE(daily_likes.likes_count, 0) as total_likes
		FROM posts p
		LEFT JOIN (
			SELECT 
				DATE(p2.created_at) as date,
				COUNT(pr.id) as likes_count
			FROM posts p2
			LEFT JOIN post_reactions pr ON p2.id = pr.post_id AND pr.reaction = 'like'
			WHERE p2.user_id = $1 AND p2.created_at >= $2
			GROUP BY DATE(p2.created_at)
		) daily_likes ON DATE(p.created_at) = daily_likes.date
		WHERE p.user_id = $1 AND p.created_at >= $2
		GROUP BY DATE(p.created_at), daily_likes.likes_count
		ORDER BY DATE(p.created_at)`

	rows, err := r.QueryContext(ctx, dailyQuery, userID, startDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily analytics: %w", err)
	}
	defer rows.Close()

	var dailyStats []DailyPostStats
	for rows.Next() {
		var stat DailyPostStats
		err := rows.Scan(&stat.Date, &stat.PostsCount, &stat.TotalViews, &stat.TotalLikes)
		if err != nil {
			continue
		}
		dailyStats = append(dailyStats, stat)
	}
	analytics.DailyStats = dailyStats

	// Get top performing posts
	topPostsQuery := `
		SELECT 
			p.id, p.title, 
			COALESCE(p.views_count, 0) as views_count,
			COALESCE(likes.likes_count, 0) as likes_count,
			COALESCE(comments.comments_count, 0) as comments_count
		FROM posts p
		LEFT JOIN (
			SELECT post_id, COUNT(*) as likes_count
			FROM post_reactions 
			WHERE reaction = 'like'
			GROUP BY post_id
		) likes ON p.id = likes.post_id
		LEFT JOIN (
			SELECT post_id, COUNT(*) as comments_count
			FROM comments 
			WHERE post_id IS NOT NULL
			GROUP BY post_id
		) comments ON p.id = comments.post_id
		WHERE p.user_id = $1 AND p.created_at >= $2 AND p.status = 'published'
		ORDER BY (
			COALESCE(p.views_count, 0) * 0.1 + 
			COALESCE(likes.likes_count, 0) * 3 + 
			COALESCE(comments.comments_count, 0) * 2
		) DESC
		LIMIT 10`

	topRows, err := r.QueryContext(ctx, topPostsQuery, userID, startDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get top posts: %w", err)
	}
	defer topRows.Close()

	var topPosts []PostPerformance
	for topRows.Next() {
		var post PostPerformance
		err := topRows.Scan(
			&post.PostID, &post.Title, &post.ViewsCount,
			&post.LikesCount, &post.CommentsCount,
		)
		if err != nil {
			continue
		}
		topPosts = append(topPosts, post)
	}
	analytics.TopPosts = topPosts

	return &analytics, nil
}

// ===============================
// HELPER METHODS
// ===============================

// scanPostRows scans post rows and handles user-specific data
func (r *postRepository) scanPostRows(rows *sql.Rows, userID *int64) ([]*models.Post, string) {
	var posts []*models.Post
	var lastCursor string

	for rows.Next() {
		var post models.Post
		var userReaction sql.NullString

		err := rows.Scan(
			&post.ID, &post.UserID, &post.Title, &post.Content,
			&post.Category, &post.ImageURL, &post.CreatedAt, &post.UpdatedAt,
			&post.Username, &post.DisplayName, &post.AuthorProfileURL,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.ViewsCount,
			&userReaction,
		)
		if err != nil {
			continue
		}

		// Set user-specific fields
		if userID != nil {
			post.IsOwner = post.UserID == *userID
			if userReaction.Valid {
				post.UserReaction = &userReaction.String
			}
		}

		// Generate helper fields
		post.Preview = r.generatePreview(post.Content)
		post.CategoryArray = strings.Split(post.Category, ",")
		post.CreatedAtHuman = r.formatTimeHuman(post.CreatedAt)
		post.UpdatedAtHuman = r.formatTimeHuman(post.UpdatedAt)

		posts = append(posts, &post)
		lastCursor = r.encodeCursor(post.CreatedAt)
	}

	return posts, lastCursor
}

// generatePreview creates a preview from post content
func (r *postRepository) generatePreview(content string) string {
	const maxLength = 200
	if len(content) <= maxLength {
		return content
	}

	// Find the last complete word within the limit
	preview := content[:maxLength]
	if lastSpace := strings.LastIndex(preview, " "); lastSpace > maxLength/2 {
		preview = preview[:lastSpace]
	}

	return preview + "..."
}

// formatTimeHuman formats time in human-readable format
func (r *postRepository) formatTimeHuman(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < 0 {
		// Future time
		return formatFutureTime(t)
	}
	return formatPastTime(t)
}

func formatPastTime(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		secs := int(diff.Seconds())
		if secs <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d seconds ago", secs)
	case diff < time.Hour:
		mins := int(diff.Minutes())
		return pluralize(mins, "minute") + " ago"
	case diff < 24*time.Hour:
		hrs := int(diff.Hours())
		return pluralize(hrs, "hour") + " ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return pluralize(days, "day") + " ago"
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		return pluralize(weeks, "week") + " ago"
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		return pluralize(months, "month") + " ago"
	default:
		years := int(diff.Hours() / (24 * 365))
		return pluralize(years, "year") + " ago"
	}
}

func formatFutureTime(t time.Time) string {
	diff := time.Until(t)

	switch {
	case diff < time.Minute:
		return "in a few seconds"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		return "in " + pluralize(mins, "minute")
	case diff < 24*time.Hour:
		hrs := int(diff.Hours())
		return "in " + pluralize(hrs, "hour")
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return "in " + pluralize(days, "day")
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		return "in " + pluralize(weeks, "week")
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		return "in " + pluralize(months, "month")
	default:
		years := int(diff.Hours() / (24 * 365))
		return "in " + pluralize(years, "year")
	}
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
