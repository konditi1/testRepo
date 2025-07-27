// internal/repositories/comment_repository.go
package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"

	"go.uber.org/zap"
)

// commentRepository implements CommentRepository with high-performance patterns
type commentRepository struct {
	*BaseRepository
}

// NewCommentRepository creates a new instance of CommentRepository
func NewCommentRepository(db *database.Manager, logger *zap.Logger) CommentRepository {
	return &commentRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// BASIC CRUD OPERATIONS
// ===============================

// Create creates a new comment with proper validation
func (r *commentRepository) Create(ctx context.Context, comment *models.Comment) error {
	// Validate that exactly one parent is set
	parentCount := 0
	if comment.PostID != nil {
		parentCount++
	}
	if comment.QuestionID != nil {
		parentCount++
	}
	if comment.DocumentID != nil {
		parentCount++
	}

	if parentCount != 1 {
		return fmt.Errorf("comment must have exactly one parent (post, question, or document)")
	}

	query := `
		INSERT INTO comments (
			user_id, post_id, question_id, document_id, content
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err := r.QueryRowContext(
		ctx, query,
		comment.UserID, comment.PostID, comment.QuestionID,
		comment.DocumentID, comment.Content,
	).Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)

	if err != nil {
		r.GetLogger().Error("Failed to create comment",
			zap.Error(err),
			zap.Int64("user_id", comment.UserID),
			zap.Any("post_id", comment.PostID),
			zap.Any("question_id", comment.QuestionID),
			zap.Any("document_id", comment.DocumentID),
		)
		return fmt.Errorf("failed to create comment: %w", err)
	}

	// Initialize engagement metrics
	comment.LikesCount = 0
	comment.DislikesCount = 0

	r.GetLogger().Info("Comment created successfully",
		zap.Int64("comment_id", comment.ID),
		zap.Int64("user_id", comment.UserID),
	)

	return nil
}

// GetByID retrieves a comment by ID with author information
func (r *commentRepository) GetByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error) {
	query := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			-- Author information (JOIN to prevent N+1)
			u.username, u.display_name, u.profile_url,
			-- Engagement metrics (computed)
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			-- User-specific reaction (if userID provided)
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		-- Aggregate reaction counts to prevent N+1
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		-- User-specific reaction (conditional join)
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $2
		WHERE c.id = $1 AND u.is_active = true`

	var comment models.Comment
	var userReaction sql.NullString

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{id, *userID}
	} else {
		queryArgs = []interface{}{id, nil}
	}

	err := r.QueryRowContext(ctx, query, queryArgs...).Scan(
		&comment.ID, &comment.UserID, &comment.PostID, &comment.QuestionID, &comment.DocumentID,
		&comment.Content, &comment.CreatedAt, &comment.UpdatedAt,
		&comment.Username, &comment.AuthorProfileURL,
		&comment.LikesCount, &comment.DislikesCount,
		&userReaction,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get comment by ID: %w", err)
	}

	// Set user-specific fields
	if userID != nil {
		comment.IsOwner = comment.UserID == *userID
		if userReaction.Valid {
			comment.UserReaction = &userReaction.String
		}
	}

	// Generate helper fields
	comment.CreatedAtHuman = r.formatTimeHuman(comment.CreatedAt)
	comment.UpdatedAtHuman = r.formatTimeHuman(comment.UpdatedAt)

	return &comment, nil
}

// Update updates a comment's content
func (r *commentRepository) Update(ctx context.Context, comment *models.Comment) error {
	query := `
		UPDATE comments SET
			content = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $3
		RETURNING updated_at`

	err := r.QueryRowContext(
		ctx, query,
		comment.ID, comment.Content, comment.UserID,
	).Scan(&comment.UpdatedAt)

	if err != nil {
		if r.IsNotFound(err) {
			return fmt.Errorf("comment not found or not owned by user")
		}
		return fmt.Errorf("failed to update comment: %w", err)
	}

	r.GetLogger().Info("Comment updated successfully",
		zap.Int64("comment_id", comment.ID),
		zap.Int64("user_id", comment.UserID),
	)

	return nil
}

// Delete deletes a comment (hard delete for comments)
func (r *commentRepository) Delete(ctx context.Context, id int64) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// First delete all reactions
		_, err := tx.ExecContext(ctx, "DELETE FROM comment_reactions WHERE comment_id = $1", id)
		if err != nil {
			return fmt.Errorf("failed to delete comment reactions: %w", err)
		}

		// Then delete the comment
		result, err := tx.ExecContext(ctx, "DELETE FROM comments WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("failed to delete comment: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return fmt.Errorf("comment not found")
		}

		return nil
	})
}

// ===============================
// LISTING OPERATIONS
// ===============================

// GetByPostID retrieves comments for a specific post
func (r *commentRepository) GetByPostID(ctx context.Context, postID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $1`

	whereClause := "c.post_id = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, postID)

	// Default sort by creation time for comments
	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "asc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments by post ID: %w", err)
	}
	defer rows.Close()

	comments, lastCursor := r.scanCommentRows(rows, userID)

	// Get total count
	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"post_id": postID},
	}, nil
}

// GetByQuestionID retrieves comments for a specific question
func (r *commentRepository) GetByQuestionID(ctx context.Context, questionID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $1`

	whereClause := "c.question_id = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, questionID)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "asc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments by question ID: %w", err)
	}
	defer rows.Close()

	comments, lastCursor := r.scanCommentRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"question_id": questionID},
	}, nil
}

// GetByDocumentID retrieves comments for a specific document
func (r *commentRepository) GetByDocumentID(ctx context.Context, documentID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $1`

	whereClause := "c.document_id = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, documentID)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "asc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments by document ID: %w", err)
	}
	defer rows.Close()

	comments, lastCursor := r.scanCommentRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"document_id": documentID},
	}, nil
}

// GetByUserID retrieves comments by a specific user
func (r *commentRepository) GetByUserID(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			-- Include parent context for user's comments
			p.title as post_title,
			q.title as question_title,
			d.title as document_title
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN posts p ON c.post_id = p.id
		LEFT JOIN questions q ON c.question_id = q.id
		LEFT JOIN documents d ON c.document_id = d.id`

	whereClause := "c.user_id = $1 AND u.is_active = true"
	whereArgs := []interface{}{userID}

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments by user ID: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	var lastCursor string

	for rows.Next() {
		var comment models.Comment
		var postTitle, questionTitle, documentTitle sql.NullString

		err := rows.Scan(
			&comment.ID, &comment.UserID, &comment.PostID, &comment.QuestionID, &comment.DocumentID,
			&comment.Content, &comment.CreatedAt, &comment.UpdatedAt,
			&comment.Username, &comment.AuthorProfileURL,
			&comment.LikesCount, &comment.DislikesCount,
			&postTitle, &questionTitle, &documentTitle,
		)
		if err != nil {
			continue
		}

		// Set ownership (all comments belong to the user)
		comment.IsOwner = true

		// Set context about what the comment is on
		switch {
		case comment.PostID != nil:
			comment.ContextType = "post"
			if postTitle.Valid {
				comment.ContextTitle = postTitle.String
			}
		case comment.QuestionID != nil:
			comment.ContextType = "question"
			if questionTitle.Valid {
				comment.ContextTitle = questionTitle.String
			}
		case comment.DocumentID != nil:
			comment.ContextType = "document"
			if documentTitle.Valid {
				comment.ContextTitle = documentTitle.String
			}
		}

		// Generate helper fields
		comment.CreatedAtHuman = r.formatTimeHuman(comment.CreatedAt)
		comment.UpdatedAtHuman = r.formatTimeHuman(comment.UpdatedAt)

		comments = append(comments, &comment)
		lastCursor = r.encodeCursor(comment.CreatedAt)
	}

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"user_id": userID},
	}, nil
}

// ===============================
// ENGAGEMENT OPERATIONS
// ===============================

// AddReaction adds or updates a user's reaction to a comment
func (r *commentRepository) AddReaction(ctx context.Context, commentID, userID int64, reactionType string) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Upsert reaction (PostgreSQL UPSERT)
		query := `
			INSERT INTO comment_reactions (comment_id, user_id, reaction)
			VALUES ($1, $2, $3)
			ON CONFLICT (comment_id, user_id)
			DO UPDATE SET reaction = EXCLUDED.reaction`

		_, err := tx.ExecContext(ctx, query, commentID, userID, reactionType)
		return err
	})
}

// RemoveReaction removes a user's reaction from a comment
func (r *commentRepository) RemoveReaction(ctx context.Context, commentID, userID int64) error {
	query := `DELETE FROM comment_reactions WHERE comment_id = $1 AND user_id = $2`
	_, err := r.ExecContext(ctx, query, commentID, userID)
	return err
}

// GetUserReaction gets a user's reaction to a comment
func (r *commentRepository) GetUserReaction(ctx context.Context, commentID, userID int64) (*string, error) {
	query := `SELECT reaction FROM comment_reactions WHERE comment_id = $1 AND user_id = $2`

	var reaction string
	err := r.QueryRowContext(ctx, query, commentID, userID).Scan(&reaction)
	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &reaction, nil
}

// GetReactionCounts gets the reaction counts for a comment
func (r *commentRepository) GetReactionCounts(ctx context.Context, commentID int64) (likes, dislikes int, err error) {
	query := `
		SELECT 
			COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
			COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
		FROM comment_reactions 
		WHERE comment_id = $1`

	err = r.QueryRowContext(ctx, query, commentID).Scan(&likes, &dislikes)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get reaction counts: %w", err)
	}

	return likes, dislikes, nil
}


// ===============================
// ANALYTICS OPERATIONS
// ===============================

// CountByPostID counts comments for a specific post
func (r *commentRepository) CountByPostID(ctx context.Context, postID int64) (int, error) {
	query := `SELECT COUNT(*) FROM comments WHERE post_id = $1`

	var count int
	err := r.QueryRowContext(ctx, query, postID).Scan(&count)
	return count, err
}

// CountByQuestionID counts comments for a specific question
func (r *commentRepository) CountByQuestionID(ctx context.Context, questionID int64) (int, error) {
	query := `SELECT COUNT(*) FROM comments WHERE question_id = $1`

	var count int
	err := r.QueryRowContext(ctx, query, questionID).Scan(&count)
	return count, err
}

// CountByDocumentID counts comments for a specific document
func (r *commentRepository) CountByDocumentID(ctx context.Context, documentID int64) (int, error) {
	query := `SELECT COUNT(*) FROM comments WHERE document_id = $1`

	var count int
	err := r.QueryRowContext(ctx, query, documentID).Scan(&count)
	return count, err
}

// CountByUserID counts comments by a specific user
func (r *commentRepository) CountByUserID(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM comments WHERE user_id = $1`

	var count int
	err := r.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// GetCommentStats gets detailed statistics for a comment
func (r *commentRepository) GetCommentStats(ctx context.Context, commentID int64) (*CommentStats, error) {
	query := `
		SELECT 
			c.id,
			COALESCE(likes.likes_count, 0) as likes_count,
			COALESCE(dislikes.dislikes_count, 0) as dislikes_count,
			COALESCE(replies.replies_count, 0) as replies_count,
			CASE WHEN qa.comment_id IS NOT NULL THEN true ELSE false END as is_accepted
		FROM comments c
		LEFT JOIN (
			SELECT comment_id, COUNT(*) as likes_count 
			FROM comment_reactions 
			WHERE reaction = 'like' AND comment_id = $1
			GROUP BY comment_id
		) likes ON c.id = likes.comment_id
		LEFT JOIN (
			SELECT comment_id, COUNT(*) as dislikes_count 
			FROM comment_reactions 
			WHERE reaction = 'dislike' AND comment_id = $1
			GROUP BY comment_id
		) dislikes ON c.id = dislikes.comment_id
		LEFT JOIN (
			SELECT parent_comment_id, COUNT(*) as replies_count 
			FROM comments 
			WHERE parent_comment_id = $1
			GROUP BY parent_comment_id
		) replies ON c.id = replies.parent_comment_id
		LEFT JOIN question_accepted_answers qa ON c.id = qa.comment_id
		WHERE c.id = $1`

	var stats CommentStats
	err := r.QueryRowContext(ctx, query, commentID).Scan(
		&stats.CommentID,
		&stats.LikesCount,
		&stats.DislikesCount,
		&stats.RepliesCount,
		&stats.IsAccepted,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get comment stats: %w", err)
	}

	return &stats, nil
}

// ===============================
// TRENDING COMMENTS
// ===============================

// GetTrendingComments retrieves comments with the highest engagement (likes + replies) within a time range
func (r *commentRepository) GetTrendingComments(ctx context.Context, startTime, endTime time.Time, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		WITH comment_engagement AS (
			SELECT 
				c.id,
				c.user_id,
				c.post_id,
				c.question_id,
				c.document_id,
				c.content,
				c.created_at,
				c.updated_at,
				u.username,
				u.display_name,
				u.profile_url,
				COALESCE(likes.likes_count, 0) as likes_count,
				COALESCE(dislikes.dislikes_count, 0) as dislikes_count,
				COALESCE(replies.replies_count, 0) as replies_count
			FROM comments c
			INNER JOIN users u ON c.user_id = u.id
			LEFT JOIN (
				SELECT comment_id, COUNT(*) as likes_count 
				FROM comment_reactions 
				WHERE reaction = 'like' AND created_at BETWEEN $1 AND $2
				GROUP BY comment_id
			) likes ON c.id = likes.comment_id
			LEFT JOIN (
				SELECT comment_id, COUNT(*) as dislikes_count 
				FROM comment_reactions 
				WHERE reaction = 'dislike' AND created_at BETWEEN $1 AND $2
				GROUP BY comment_id
			) dislikes ON c.id = dislikes.comment_id
			LEFT JOIN (
				SELECT parent_comment_id, COUNT(*) as replies_count 
				FROM comments 
				WHERE parent_comment_id IS NOT NULL 
				AND created_at BETWEEN $1 AND $2
				GROUP BY parent_comment_id
			) replies ON c.id = replies.parent_comment_id
			WHERE c.created_at BETWEEN $1 AND $2
			AND u.is_active = true
		)
		SELECT 
			id, user_id, post_id, question_id, document_id,
			content, created_at, updated_at,
			username, display_name, profile_url,
			likes_count, dislikes_count,
			user_reaction.reaction as user_reaction,
			(likes_count * 2 + replies_count) as engagement_score
		FROM comment_engagement
		LEFT JOIN (
			SELECT comment_id, reaction 
			FROM comment_reactions 
			WHERE user_id = $3
		) user_reaction ON comment_engagement.id = user_reaction.comment_id`

	// Set default sorting by engagement score descending if not specified
	if params.Sort == "" {
		params.Sort = "engagement_score"
		params.Order = "desc"
	}

	// Build the paginated query
	query, args, err := r.BuildPaginatedQuery(
		baseQuery,
		"",                               // No additional where clause needed as it's in the CTE
		"ORDER BY engagement_score DESC", // Default ordering
		params,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build trending comments query: %w", err)
	}

	// Prepend the time range and user ID to the args
	finalArgs := append([]interface{}{startTime, endTime, userID}, args...)

	// Execute the query
	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending comments: %w", err)
	}
	defer rows.Close()

	// Scan the rows into comments
	comments, lastCursor := r.scanCommentRows(rows, userID)

	// Get total count
	countQuery := `
		SELECT COUNT(*) 
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		WHERE c.created_at BETWEEN $1 AND $2
		AND u.is_active = true`

	total, err := r.GetTotalCount(ctx, countQuery, startTime, endTime)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters: map[string]any{
			"start_time": startTime,
			"end_time":   endTime,
		},
	}, nil
}

// GetRecentComments retrieves the most recent comments across all content types
func (r *commentRepository) GetRecentComments(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	// Build the base query with proper filtering and ordering
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id, c.content, 
			c.created_at, c.updated_at, c.parent_comment_id,
			u.username, u.display_name, u.profile_url,
			c.is_edited, c.is_deleted, c.deleted_at,
			(
				SELECT COUNT(*) FROM comment_reactions cr 
				WHERE cr.comment_id = c.id AND cr.reaction = 'like'
			) as likes_count,
			(
				SELECT COUNT(*) FROM comment_reactions cr 
				WHERE cr.comment_id = c.id AND cr.reaction = 'dislike'
			) as dislikes_count,
			(
				SELECT COUNT(*) FROM comments cr 
				WHERE cr.parent_comment_id = c.id
			) as replies_count`

	// Add user-specific reaction if user is authenticated
	if userID != nil {
		baseQuery += `,
			(
				SELECT reaction FROM comment_reactions cr 
				WHERE cr.comment_id = c.id AND cr.user_id = $1
			) as user_reaction`
	}

	// Add FROM and JOIN clauses
	baseQuery += `
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		WHERE c.parent_comment_id IS NULL` // Only top-level comments

	// Add pagination
	var queryParams []interface{}
	if userID != nil {
		queryParams = append(queryParams, *userID)
	}

	// Add ordering and pagination
	orderClause := ` ORDER BY c.created_at DESC`
	limitOffset := ` LIMIT $` + strconv.Itoa(len(queryParams)+1) +
		` OFFSET $` + strconv.Itoa(len(queryParams)+2)

	// Execute the query
	query := baseQuery + orderClause + limitOffset

	// Add pagination parameters
	queryParams = append(queryParams, params.Limit+1, params.Offset)

	// Execute the query
	rows, err := r.QueryContext(ctx, query, queryParams...)
	if err != nil {
		r.logger.Error("failed to get recent comments",
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get recent comments: %w", err)
	}
	defer rows.Close()

	// Process the results
	var allComments []*models.Comment
	var lastCursor string

	// Get all comments first
	allComments, lastCursor = r.scanCommentRows(rows, userID)

	// Apply pagination
	var comments []*models.Comment
	limit := int(params.Limit)
	if limit > len(allComments) {
		limit = len(allComments)
	}
	comments = allComments[:limit]

	if err = rows.Err(); err != nil {
		r.logger.Error("error iterating over comment rows",
			zap.Error(err),
		)
		return nil, fmt.Errorf("error processing comment results: %w", err)
	}

	// Get total count for pagination
	countQuery := `SELECT COUNT(*) FROM comments c WHERE c.parent_comment_id IS NULL`
	total, err := r.GetTotalCount(ctx, countQuery)
	if err != nil {
		r.logger.Warn("failed to get total count of comments",
			zap.Error(err),
		)
		total = 0
	}

	// Determine if there are more results
	hasMore := len(allComments) > int(params.Limit)

	// Build pagination metadata
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
	}, nil
}

// GetReplies retrieves replies to a specific comment
func (r *commentRepository) GetReplies(ctx context.Context, parentCommentID int64, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $1`

	whereClause := "c.parent_comment_id = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, parentCommentID)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "asc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment replies: %w", err)
	}
	defer rows.Close()

	comments, lastCursor := r.scanCommentRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"parent_comment_id": parentCommentID},
	}, nil
}

// GetCommentThread retrieves the entire thread for a comment
func (r *commentRepository) GetCommentThread(ctx context.Context, commentID int64, userID *int64) ([]*models.Comment, error) {
	query := `
		WITH RECURSIVE comment_thread AS (
			-- Base case: get the root comment
			SELECT id, user_id, post_id, question_id, document_id, content,
				   created_at, updated_at, parent_comment_id, 0 as level
			FROM comments 
			WHERE id = $1
			
			UNION ALL
			
			-- Recursive case: get all replies
			SELECT c.id, c.user_id, c.post_id, c.question_id, c.document_id, c.content,
				   c.created_at, c.updated_at, c.parent_comment_id, ct.level + 1
			FROM comments c
			INNER JOIN comment_thread ct ON c.parent_comment_id = ct.id
		)
		SELECT 
			ct.id, ct.user_id, ct.post_id, ct.question_id, ct.document_id,
			ct.content, ct.created_at, ct.updated_at, ct.level,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comment_thread ct
		INNER JOIN users u ON ct.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON ct.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON ct.id = ur.comment_id AND ur.user_id = $2
		WHERE u.is_active = true
		ORDER BY ct.level, ct.created_at`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{commentID, *userID}
	} else {
		queryArgs = []interface{}{commentID, nil}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment thread: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	for rows.Next() {
		var comment models.Comment
		var level int
		var userReaction sql.NullString

		err := rows.Scan(
			&comment.ID, &comment.UserID, &comment.PostID, &comment.QuestionID, &comment.DocumentID,
			&comment.Content, &comment.CreatedAt, &comment.UpdatedAt, &level,
			&comment.Username, &comment.AuthorProfileURL,
			&comment.LikesCount, &comment.DislikesCount,
			&userReaction,
		)
		if err != nil {
			continue
		}

		if userID != nil {
			comment.IsOwner = comment.UserID == *userID
			if userReaction.Valid {
				comment.UserReaction = &userReaction.String
			}
		}

		comment.CreatedAtHuman = r.formatTimeHuman(comment.CreatedAt)
		comment.UpdatedAtHuman = r.formatTimeHuman(comment.UpdatedAt)

		comments = append(comments, &comment)
	}

	return comments, nil
}


// ===============================
// BATCH OPERATIONS
// ===============================

// GetCommentsForModeration retrieves comments that need moderation based on status and priority
func (r *commentRepository) GetCommentsForModeration(ctx context.Context, status *string, priority *string, params models.PaginationParams) (*models.PaginatedResponse[*models.Comment], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Build base query
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id, c.content, 
			c.created_at, c.updated_at, c.parent_comment_id, c.status, c.priority,
			u.id, u.username, u.display_name, u.profile_url,
			c.is_edited, c.is_deleted, c.deleted_at,
			(
				SELECT COUNT(*) FROM comment_reactions cr 
				WHERE cr.comment_id = c.id AND cr.reaction = 'like'
			) as likes_count,
			(
				SELECT COUNT(*) FROM comment_reactions cr 
				WHERE cr.comment_id = c.id AND cr.reaction = 'dislike'
			) as dislikes_count,
			(
				SELECT COUNT(*) FROM comments child 
				WHERE child.parent_comment_id = c.id AND child.is_deleted = false
			) as reply_count
		FROM comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.is_deleted = false`

	// Add status filter if provided
	args := []interface{}{}
	argNum := 1

	if status != nil && *status != "" {
		baseQuery += fmt.Sprintf(" AND c.status = $%d", argNum)
		args = append(args, *status)
		argNum++
	} else {
		// Default to showing only pending and flagged comments if no status is specified
		baseQuery += " AND c.status IN ('pending', 'flagged')"
	}

	// Add priority filter if provided
	if priority != nil && *priority != "" {
		baseQuery += fmt.Sprintf(" AND c.priority = $%d", argNum)
		args = append(args, *priority)
		argNum++
	}

	// Add ordering and pagination
	orderClause := `
		ORDER BY 
			CASE 
				WHEN c.priority = 'high' THEN 1
				WHEN c.priority = 'medium' THEN 2
				ELSE 3
			END,
		c.created_at ASC
		LIMIT $` + strconv.Itoa(argNum) + ` OFFSET $` + strconv.Itoa(argNum+1)

	// Calculate offset if not provided
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, params.Limit, offset)

	// Execute query
	rows, err := tx.QueryContext(ctx, baseQuery+orderClause, args...)
	if err != nil {
		r.logger.Error("failed to query comments for moderation",
			zap.Error(err),
			zap.String("status", safeDerefString(status, "")),
			zap.String("priority", safeDerefString(priority, "")),
		)
		return nil, fmt.Errorf("failed to query comments for moderation: %w", err)
	}
	defer rows.Close()

	// Process results
	comments, _ := r.scanCommentRows(rows, nil) // Don't need user-specific data for moderation queue

	// Get total count for pagination
	countQuery := `
		SELECT COUNT(*)
		FROM comments c
		WHERE c.is_deleted = false`

	// Add the same filters as the main query
	countArgs := []interface{}{}
	argNum = 1

	if status != nil && *status != "" {
		countQuery += fmt.Sprintf(" AND c.status = $%d", argNum)
		countArgs = append(countArgs, *status)
	} else {
		countQuery += " AND c.status IN ('pending', 'flagged')"
	}

	if priority != nil && *priority != "" {
		if len(countArgs) > 0 {
			argNum = 2
		}
		countQuery += fmt.Sprintf(" AND c.priority = $%d", argNum)
		countArgs = append(countArgs, *priority)
	}

	var total int
	err = tx.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		r.logger.Error("failed to count comments for moderation",
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to count comments for moderation: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Calculate pagination metadata
	itemsPerPage := int(params.Limit)
	if itemsPerPage <= 0 {
		itemsPerPage = 10 // Default page size
	}
	currentPage := (params.Offset / params.Limit) + 1
	totalPages := (total + itemsPerPage - 1) / itemsPerPage
	if totalPages < 1 {
		totalPages = 1
	}

	return &models.PaginatedResponse[*models.Comment]{
		Data: comments,
		Pagination: models.PaginationMeta{
			CurrentPage:  currentPage,
			ItemsPerPage: itemsPerPage,
			TotalItems:   int64(total),
			TotalPages:   totalPages,
			HasNext:      currentPage < totalPages,
			HasPrev:      currentPage > 1,
		},
	}, nil
}

// BulkUpdateStatus updates the status of multiple comments (for moderation)
func (r *commentRepository) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	if len(ids) == 0 {
		return nil
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending":  true,
		"approved": true,
		"rejected": true,
		"flagged":  true,
		"hidden":   true,
	}

	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}

	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Build the query with proper placeholders
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids)+1)
		
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+2) // Start from $2 since $1 is status
			args[i+1] = id
		}
		args[0] = status // Status is the first parameter

		query := fmt.Sprintf(`
			UPDATE comments 
			SET status = $1, updated_at = CURRENT_TIMESTAMP 
			WHERE id IN (%s)`, strings.Join(placeholders, ","))

		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("failed to bulk update comment status: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		r.GetLogger().Info("Bulk updated comment statuses",
			zap.String("status", status),
			zap.Int64("affected_rows", rowsAffected),
			zap.Int("comment_count", len(ids)),
		)

		return nil
	})
}

// safeDerefString safely dereferences a string pointer, returning an empty string if nil
func safeDerefString(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}

// GetLatestByPostIDs gets the latest comments for multiple posts (prevents N+1)
func (r *commentRepository) GetLatestByPostIDs(ctx context.Context, postIDs []int64, limit int) ([]*models.Comment, error) {
	if len(postIDs) == 0 {
		return []*models.Comment{}, nil
	}

	// Use window function to get latest comments per post
	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs)+1)

	for i, id := range postIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	args[len(postIDs)] = limit

	query := fmt.Sprintf(`
		SELECT 
			c.id, c.user_id, c.post_id, c.content, c.created_at,
			u.username, u.display_name, u.profile_url,
			ROW_NUMBER() OVER (PARTITION BY c.post_id ORDER BY c.created_at DESC) as rn
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		WHERE c.post_id IN (%s) AND u.is_active = true
		ORDER BY c.post_id, c.created_at DESC`, strings.Join(placeholders, ","))

	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest comments by post IDs: %w", err)
	}
	defer rows.Close()

	var comments []*models.Comment
	for rows.Next() {
		var comment models.Comment
		var rowNum int

		err := rows.Scan(
			&comment.ID, &comment.UserID, &comment.PostID,
			&comment.Content, &comment.CreatedAt,
			&comment.Username, &comment.AuthorProfileURL,
			&rowNum,
		)
		if err != nil {
			continue
		}

		// Only include up to 'limit' comments per post
		if rowNum <= limit {
			comment.CreatedAtHuman = r.formatTimeHuman(comment.CreatedAt)
			comments = append(comments, &comment)
		}
	}

	return comments, nil
}

// BulkDelete deletes multiple comments (for moderation)
func (r *commentRepository) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// First delete all reactions
		_, err := tx.ExecContext(ctx, "DELETE FROM comment_reactions WHERE comment_id = ANY($1)", ids)
		if err != nil {
			return fmt.Errorf("failed to delete comment reactions: %w", err)
		}

		// Then delete the comments
		_, err = tx.ExecContext(ctx, "DELETE FROM comments WHERE id = ANY($1)", ids)
		if err != nil {
			return fmt.Errorf("failed to delete comments: %w", err)
		}

		return nil
	})
}

// ===============================
// SEARCH OPERATIONS
// ===============================

// Search searches comments across all content types
func (r *commentRepository) Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Comment], error) {
	baseQuery := `
		SELECT 
			c.id, c.user_id, c.post_id, c.question_id, c.document_id,
			c.content, c.created_at, c.updated_at,
			u.username, u.display_name, u.profile_url,
			COALESCE(cr_stats.likes_count, 0) as likes_count,
			COALESCE(cr_stats.dislikes_count, 0) as dislikes_count,
			ur.reaction as user_reaction
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN (
			SELECT 
				comment_id,
				COUNT(CASE WHEN reaction = 'like' THEN 1 END) as likes_count,
				COUNT(CASE WHEN reaction = 'dislike' THEN 1 END) as dislikes_count
			FROM comment_reactions 
			GROUP BY comment_id
		) cr_stats ON c.id = cr_stats.comment_id
		LEFT JOIN comment_reactions ur ON c.id = ur.comment_id AND ur.user_id = $1`

	whereClause := "u.is_active = true AND c.content ILIKE $2"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, "%"+query+"%")

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	finalQuery, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, finalQuery, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search comments: %w", err)
	}
	defer rows.Close()

	comments, lastCursor := r.scanCommentRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(comments) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Comment]{
		Data:       comments,
		Pagination: meta,
		Filters:    map[string]any{"query": query},
	}, nil
}

// ===============================
// HELPER METHODS
// ===============================

// scanCommentRows scans comment rows and handles user-specific data
func (r *commentRepository) scanCommentRows(rows *sql.Rows, userID *int64) ([]*models.Comment, string) {
	var comments []*models.Comment
	var lastCursor string

	for rows.Next() {
		var comment models.Comment
		var userReaction sql.NullString

		err := rows.Scan(
			&comment.ID, &comment.UserID, &comment.PostID, &comment.QuestionID, &comment.DocumentID,
			&comment.Content, &comment.CreatedAt, &comment.UpdatedAt,
			&comment.Username, &comment.AuthorProfileURL,
			&comment.LikesCount, &comment.DislikesCount,
			&userReaction,
		)
		if err != nil {
			continue
		}

		// Set user-specific fields
		if userID != nil {
			comment.IsOwner = comment.UserID == *userID
			if userReaction.Valid {
				comment.UserReaction = &userReaction.String
			}
		}

		// Generate helper fields
		comment.CreatedAtHuman = r.formatTimeHuman(comment.CreatedAt)
		comment.UpdatedAtHuman = r.formatTimeHuman(comment.UpdatedAt)

		comments = append(comments, &comment)
		lastCursor = r.encodeCursor(comment.CreatedAt)
	}

	return comments, lastCursor
}

// formatTimeHuman formats time in human-readable format
func (r *commentRepository) formatTimeHuman(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}
