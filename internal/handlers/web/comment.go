// internal/handlers/web/comment.go
package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
)

// CreateCommentHandler handles creating comments for both posts and questions
func CreateCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	userID := r.Context().Value(userIDKey).(int)
	content := r.FormValue("comment")
	postIDStr := r.FormValue("post_id")
	questionIDStr := r.FormValue("question_id")
	if content == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("comment content is required"))
		return
	}
	var err error
	var redirectURL string
	if postIDStr != "" {
		// Creating comment for a post
		postID, err := strconv.Atoi(postIDStr)
		if err != nil {
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid post ID"))
			return
		}
		err1 := CreateCommentForPost(userID, postID, content)
		if err1 != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create comment: %v", err1))
			return
		}
		redirectURL = fmt.Sprintf("/view-post?id=%d", postID)
	} else if questionIDStr != "" {
		// Creating comment for a question
		var questionID int
		questionID, err = strconv.Atoi(questionIDStr)
		if err != nil {
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid question ID"))
			return
		}
		err = CreateCommentForQuestion(userID, questionID, content)		
		redirectURL = fmt.Sprintf("/view-question?id=%d", questionID)
	} else {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("either post_id or question_id is required"))
		return
	}
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create comment: %v", err))
		return
	}

	// Comment created successfully - update user stats
	go UpdateUserStats(r.Context(), userID)

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// CreateCommentForPost creates a comment for a post
func CreateCommentForPost(userID, postID int, content string) error {
	username := getUsername(userID)
	query := `INSERT INTO comments (user_id, post_id, username, content) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, postID, username, content)

	// Add notification for comment creation
	if err == nil {
		go NotifyCommentCreated(0, userID, &postID, nil, content)
	}

	return err
}

// CreateCommentForQuestion creates a comment for a question
func CreateCommentForQuestion(userID, questionID int, content string) error {
	username := getUsername(userID)
	query := `INSERT INTO comments (user_id, question_id, username, content) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, questionID, username, content)

	// Add notification for comment creation
	if err == nil {
		go NotifyCommentCreated(0, userID, nil, &questionID, content)
	}

	return err
}

// EditCommentHandler handles editing comments (works for both post and question comments)
func EditCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	commentIDStr := r.URL.Query().Get("id")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid comment ID"))
		return
	}
	content := r.FormValue("content")
	if content == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("comment content is required"))
		return
	}
	// Check if user owns the comment
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	var commentUserID int64
	checkQuery := `SELECT user_id FROM comments WHERE id = $1`
	err = database.DB.QueryRowContext(ctx, checkQuery, commentID).Scan(&commentUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("comment not found"))
		} else {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("database error: %v", err))
		}
		return
	}
	if commentUserID != int64(userID) {
		RenderErrorPage(w, http.StatusForbidden, fmt.Errorf("you can only edit your own comments"))
		return
	}
	// Update the comment
	updateQuery := `UPDATE comments SET content = $1, updated_at = $2 WHERE id = $3`
	_, err = database.DB.ExecContext(ctx, updateQuery, content, time.Now(), commentID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to update comment: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// DeleteCommentHandler handles deleting comments (works for both post and question comments)
func DeleteCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	commentIDStr := r.URL.Query().Get("id")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid comment ID"))
		return
	}
	// Check if user owns the comment
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	var commentUserID int64
	checkQuery := `SELECT user_id FROM comments WHERE id = $1`
	err = database.DB.QueryRowContext(ctx, checkQuery, commentID).Scan(&commentUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("comment not found"))
		} else {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("database error: %v", err))
		}
		return
	}
	if commentUserID != int64(userID) {
		RenderErrorPage(w, http.StatusForbidden, fmt.Errorf("you can only delete your own comments"))
		return
	}
	// Delete the comment
	deleteQuery := `DELETE FROM comments WHERE id = $1`
	_, err = database.DB.ExecContext(ctx, deleteQuery, commentID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to delete comment: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GetCommentsCountByEntityID returns the number of comments for a post or question
func GetCommentsCountByEntityID(entityID int, entityType string) (int, error) {
	var query string
	ctx := context.Background()

	switch entityType {
	case "post":
		query = `SELECT COUNT(*) FROM comments WHERE post_id = $1`
	case "question":
		query = `SELECT COUNT(*) FROM comments WHERE question_id = $1`
	default:
		return 0, fmt.Errorf("invalid entity type")
	}

	var count int
	err := database.DB.QueryRowContext(ctx, query, entityID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get comments count: %w", err)
	}

	return count, nil
}

// LikeCommentHandler handles liking a comment
func LikeCommentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	commentIDStr := r.URL.Query().Get("id")
	commentID, err := strconv.Atoi(commentIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Comment ID: %v", err))
		return
	}

	err = ToggleCommentReaction(ctx, userID, commentID, "like")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to like comment: %v", err))
		return
	}

	// Return updated reaction counts
	likes, dislikes, _ := GetCommentReactionCounts(ctx, commentID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// DislikeCommentHandler handles disliking a comment
func DislikeCommentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	commentIDStr := r.URL.Query().Get("id")
	commentID, err := strconv.Atoi(commentIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Comment ID: %v", err))
		return
	}

	err = ToggleCommentReaction(ctx, userID, commentID, "dislike")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to dislike comment: %v", err))
		return
	}

	// Return updated reaction counts
	likes, dislikes, _ := GetCommentReactionCounts(ctx, commentID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// AddComment inserts a new comment into the database.
func AddComment(postID, userID int, username, content string) error {
	query := `INSERT INTO comments (post_id, user_id, username, content, created_at) 
			  VALUES ($1, $2, $3, $4, $5)`
	_, err := database.DB.ExecContext(context.Background(), query, postID, userID, username, content, time.Now())
	return err
}

// GetCommentByID retrieves a single comment by its ID
func GetCommentByID(commentID int) (*models.Comment, error) {
	ctx := context.Background()
	query := `SELECT id, post_id, user_id, username, content, created_at, updated_at 
	         FROM comments WHERE id = $1`

	comment := &models.Comment{}
	err := database.DB.QueryRowContext(ctx, query, commentID).Scan(
		&comment.ID,
		&comment.PostID,
		&comment.UserID,
		&comment.Username,
		&comment.Content,
		&comment.CreatedAt,
		&comment.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("comment not found")
		}
		return nil, fmt.Errorf("error fetching comment: %v", err)
	}
	return comment, nil
}

// UpdateComment updates an existing comment
func UpdateComment(commentID int, content string) error {
	ctx := context.Background()
	query := `UPDATE comments SET content = $1, updated_at = NOW() WHERE id = $2`
	_, err := database.DB.ExecContext(ctx, query, content, commentID)
	if err != nil {
		return fmt.Errorf("error updating comment: %v", err)
	}
	return nil
}

// DeleteComment removes a comment
func DeleteComment(commentID int) error {
	ctx := context.Background()
	query := `DELETE FROM comments WHERE id = $1`
	_, err := database.DB.ExecContext(ctx, query, commentID)
	if err != nil {
		return fmt.Errorf("error deleting comment: %v", err)
	}
	return nil
}

// GetCommentsCount returns the number of comments for a post
func GetCommentsCount(postID int) (int, error) {
	ctx := context.Background()
	var count int
	query := `SELECT COUNT(*) FROM comments WHERE post_id = $1`

	err := database.DB.QueryRowContext(ctx, query, postID).Scan(&count)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("error getting comments count: %v", err)
	}
	return count, nil
}

// GetCommentsByPostID retrieves all comments for a specific post ID with user profile information.
func GetCommentsByPostID(postID int) ([]models.Comment, error) {
	ctx := context.Background()
	// Ensure utils is used to avoid unused import
	_ = utils.TimeAgo(time.Now())
	query := `
        SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username,
               (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = c.id AND reaction = 'like') as likes,
               (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = c.id AND reaction = 'dislike') as dislikes,
               u.avatar_url, u.profile_url
        FROM comments c
        LEFT JOIN users u ON c.user_id = u.id
        WHERE c.post_id = $1
        ORDER BY c.created_at DESC`

	rows, err := database.DB.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, fmt.Errorf("error querying comments: %v", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var comment models.Comment
		var avatarURL, profileURL sql.NullString

		err := rows.Scan(
			&comment.ID,
			&comment.PostID,
			&comment.UserID,
			&comment.Content,
			&comment.CreatedAt,
			&comment.Username,
			&comment.LikesCount,
			&comment.DislikesCount,
			&avatarURL,
			&profileURL,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning comment: %v", err)
		}

		// Handle NULL values for avatar and profile URLs
		if avatarURL.Valid {
			comment.AuthorProfileURL = &avatarURL.String
		}
		if profileURL.Valid && comment.AuthorProfileURL == nil {
			comment.AuthorProfileURL = &profileURL.String
		}

		comments = append(comments, comment)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating comments: %v", err)
	}

	return comments, nil
}

// GetCommentsByQuestionIDWithProfiles retrieves comments for a question including author profile information
func GetCommentsByQuestionIDWithProfiles(questionID int) ([]models.Comment, error) {
	ctx := context.Background()
	query := `
        SELECT c.id, c.question_id, c.user_id, c.content, c.created_at, u.username,
               (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = c.id AND reaction = 'like') as likes,
               (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = c.id AND reaction = 'dislike') as dislikes,
               u.avatar_url, u.profile_url
        FROM comments c
        LEFT JOIN users u ON c.user_id = u.id
        WHERE c.question_id = $1
        ORDER BY c.created_at DESC`

	rows, err := database.DB.QueryContext(ctx, query, questionID)
	if err != nil {
		return nil, fmt.Errorf("error querying question comments: %v", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var comment models.Comment
		var avatarURL, profileURL sql.NullString

		err := rows.Scan(
			&comment.ID,
			&comment.QuestionID,
			&comment.UserID,
			&comment.Content,
			&comment.CreatedAt,
			&comment.Username,
			&comment.LikesCount,
			&comment.DislikesCount,
			&avatarURL,
			&profileURL,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning comment: %v", err)
		}

		// Handle NULL values for avatar and profile URLs
		if avatarURL.Valid {
			comment.AuthorProfileURL = &avatarURL.String
		}
		if profileURL.Valid && comment.AuthorProfileURL == nil {
			comment.AuthorProfileURL = &profileURL.String
		}

		comments = append(comments, comment)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating comments: %v", err)
	}

	return comments, nil
}
