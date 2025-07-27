package web

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
)

func GetLikesByUserID(userID int) ([]models.Post, error) {
	query := `
	SELECT p.id, p.user_id, p.title, p.content, p.category, p.image_url, p.created_at, p.updated_at,
	   COALESCE(likes.count, 0) AS likes,
       COALESCE(dislikes.count, 0) AS dislikes,
       COALESCE(comments.count, 0) AS comments_count
	FROM posts p
	LEFT JOIN (
		SELECT post_id, COUNT(*) AS count
		FROM post_reactions
		WHERE reaction = 'like'
		GROUP BY post_id
	) AS likes ON p.id = likes.post_id
	LEFT JOIN (
		SELECT post_id, COUNT(*) AS count
		FROM post_reactions
		WHERE reaction = 'dislike'
		GROUP BY post_id
	) AS dislikes ON p.id = dislikes.post_id
	LEFT JOIN (
			SELECT post_id, COUNT(*) AS count
			FROM comments
			GROUP BY post_id
	) AS comments ON p.id = comments.post_id
	WHERE p.id IN (SELECT post_id FROM post_reactions WHERE user_id = $1 AND reaction = 'like')
	ORDER BY p.created_at DESC`
	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []models.Post
	for rows.Next() {
		var post models.Post
		var imageURL sql.NullString // Handle nullable image_url
		err := rows.Scan(
			&post.ID,
			&post.UserID,
			&post.Title,
			&post.Content,
			&post.Category,
			&imageURL, // Added to retrieve the image URL
			&post.CreatedAt,
			&post.UpdatedAt,
			&post.LikesCount,
			&post.DislikesCount,
			&post.CommentsCount,
		)
		if err != nil {
			return nil, err
		}
		// Handle NULL image_url
		if imageURL.Valid {
			post.ImageURL = &imageURL.String
		}
		// Truncate content for preview and format time
		post.Preview = utils.TruncateContent(post.Content, 30) // Limit to 30 words
		post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)    // Populate human-readable time
		posts = append(posts, post)
	}
	return posts, nil
}

// ToggleReaction toggles a like or dislike reaction on a post.
func ToggleReaction(ctx context.Context, userID, postID int, reaction string) error {
	var existingReaction string
	// Check if a reaction already exists
	query := `SELECT reaction FROM post_reactions WHERE user_id = $1 AND post_id = $2`
	err := database.DB.QueryRowContext(ctx, query, userID, postID).Scan(&existingReaction)
	if err == sql.ErrNoRows {
		// No existing reaction, insert a new one
		insertQuery := `INSERT INTO post_reactions (user_id, post_id, reaction) VALUES ($1, $2, $3)`
		_, err := database.DB.ExecContext(ctx, insertQuery, userID, postID, reaction)
		if err == nil && reaction == "like" {
			go NotifyLikeCreated(userID, &postID, nil, nil)

			// Update stats for the user who gave the like
			go UpdateUserStats(ctx, userID)

			// Update stats for the post author who received the like
			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM posts WHERE id = $1", postID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	} else if err != nil {
		return err // Return any other unexpected errors
	}
	if existingReaction == reaction {
		// If the reaction is the same, remove it		// Toggle the reaction
		updateQuery := `UPDATE post_reactions SET reaction = $1, updated_at = NOW() WHERE user_id = $2 AND post_id = $3`
		_, err = database.DB.ExecContext(ctx, updateQuery, reaction, userID, postID)

		// If toggling to like, update stats for both users
		if err == nil && reaction == "like" {
			go NotifyLikeCreated(userID, &postID, nil, nil)

			// Update stats for both users
			go UpdateUserStats(ctx, userID)

			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM posts WHERE id = $1", postID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	}
	// If the reaction is different, update it
	updateQuery := `UPDATE post_reactions SET reaction = $1, updated_at = NOW() WHERE user_id = $2 AND post_id = $3`
	_, err = database.DB.ExecContext(ctx, updateQuery, reaction, userID, postID)
	if err == nil && reaction == "like" {
		go NotifyLikeCreated(userID, &postID, nil, nil)

		// Update stats for the user who gave the like
		go UpdateUserStats(ctx, userID)

		// Update stats for the post author who received the like
		var authorID int
		if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM posts WHERE id = $1", postID).Scan(&authorID); dbErr == nil && authorID > 0 {
			go UpdateUserStats(ctx, authorID)
		}
	}
	return err
}

// GetReactionCounts returns the number of likes and dislikes for a post.
func GetReactionCounts(ctx context.Context, postID int) (int, int, error) {
	var likes, dislikes int
	likeQuery := `SELECT COUNT(*) FROM post_reactions WHERE post_id = $1 AND reaction = 'like'`
	err := database.DB.QueryRowContext(ctx, likeQuery, postID).Scan(&likes)
	if err != nil {
		return 0, 0, err
	}

	dislikeQuery := `SELECT COUNT(*) FROM post_reactions WHERE post_id = $1 AND reaction = 'dislike'`
	err = database.DB.QueryRowContext(ctx, dislikeQuery, postID).Scan(&dislikes)
	if err != nil {
		return 0, 0, err
	}

	return likes, dislikes, nil
}

// ToggleCommentReaction handles the like/dislike functionality for comments
func ToggleCommentReaction(ctx context.Context, userID, commentID int, reaction string) error {
	var existingReaction string
	query := `SELECT reaction FROM comment_reactions WHERE user_id = $1 AND comment_id = $2`
	err := database.DB.QueryRowContext(ctx, query, userID, commentID).Scan(&existingReaction)
	if err == sql.ErrNoRows {
		// No existing reaction, insert a new one
		insertQuery := `INSERT INTO comment_reactions (user_id, comment_id, reaction) VALUES ($1, $2, $3)`
		_, err := database.DB.ExecContext(ctx, insertQuery, userID, commentID, reaction)
		if err == nil && reaction == "like" {
			go NotifyLikeCreated(userID, nil, nil, &commentID)

			// Update stats for the user who gave the like
			go UpdateUserStats(ctx, userID)

			// Update stats for the comment author who received the like
			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM comments WHERE id = $1", commentID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	} else if err != nil {
		return err
	}
	if existingReaction == reaction {
		// Remove reaction if clicking the same button
		deleteQuery := `DELETE FROM comment_reactions WHERE user_id = $1 AND comment_id = $2`
		_, err := database.DB.ExecContext(ctx, deleteQuery, userID, commentID)
		// When removing a like, update stats for both users
		if err == nil && reaction == "like" {
			go UpdateUserStats(ctx, userID)

			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM comments WHERE id = $1", commentID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	} else {
		// Update existing reaction
		updateQuery := `UPDATE comment_reactions SET reaction = $1, updated_at = NOW() WHERE user_id = $2 AND comment_id = $3`
		_, err := database.DB.ExecContext(ctx, updateQuery, reaction, userID, commentID)
		if err == nil && reaction == "like" {
			go NotifyLikeCreated(userID, nil, nil, &commentID)

			// Update stats for the user who gave the like
			go UpdateUserStats(ctx, userID)

			// Update stats for the comment author who received the like
			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM comments WHERE id = $1", commentID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	}
}

// GetCommentReactionCounts returns the number of likes and dislikes for a comment
func GetCommentReactionCounts(ctx context.Context, commentID int) (likes int, dislikes int, err error) {
	likesQuery := `SELECT COUNT(*) FROM comment_reactions WHERE comment_id = $1 AND reaction = 'like'`
	dislikesQuery := `SELECT COUNT(*) FROM comment_reactions WHERE comment_id = $1 AND reaction = 'dislike'`
	
	err = database.DB.QueryRowContext(ctx, likesQuery, commentID).Scan(&likes)
	if err != nil {
		return 0, 0, err
	}

	err = database.DB.QueryRowContext(ctx, dislikesQuery, commentID).Scan(&dislikes)
	if err != nil {
		return 0, 0, err
	}
	
	return likes, dislikes, nil
}
