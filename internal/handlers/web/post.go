// internal/handlers/web/post.go
package web

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"strings"
	"time"
)

// Updated to include profile information for posts
func GetAllPostsWithProfiles(category string) ([]models.Post, error) {
	var rows *sql.Rows
	var err error
	baseQuery := `
        SELECT p.id, p.user_id, p.title, p.content, p.category, p.image_url, 
               COALESCE(p.image_public_id, '') as image_public_id, p.created_at, p.updated_at,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM posts p
        LEFT JOIN users u ON p.user_id = u.id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'like' GROUP BY post_id
        ) AS likes ON p.id = likes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'dislike' GROUP BY post_id
        ) AS dislikes ON p.id = dislikes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM comments GROUP BY post_id
        ) AS comments ON p.id = comments.post_id`
	if category != "" {
		rows, err = database.DB.QueryContext(context.Background(), baseQuery+" WHERE p.category LIKE $1 ORDER BY p.created_at DESC", "%"+category+"%")
	} else {
		rows, err = database.DB.QueryContext(context.Background(), baseQuery+" ORDER BY p.created_at DESC")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []models.Post
	for rows.Next() {
		var post models.Post
		var imageURL, imagePublicID, profileURL sql.NullString
		err := rows.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
			&imageURL, &imagePublicID, &post.CreatedAt, &post.UpdatedAt,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount,
			&post.Username, &profileURL)
		if err != nil {
			continue
		}
		// Handle NULL fields
		if imageURL.Valid {
			post.ImageURL = &imageURL.String
		}
		if imagePublicID.Valid {
			post.ImagePublicID = &imagePublicID.String
		}
		if profileURL.Valid {
			post.AuthorProfileURL = &profileURL.String
		}
		post.Preview = utils.TruncateContent(post.Content, 30)
		post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)
		post.UpdatedAtHuman = utils.TimeAgo(post.UpdatedAt)
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return posts, nil
}

// Updated to include profile information
func GetPostsByUserIDWithProfiles(userID int) ([]models.Post, error) {
	query := `
        SELECT p.id, p.user_id, p.title, p.content, p.category, p.image_url, 
               COALESCE(p.image_public_id, '') as image_public_id, p.created_at, p.updated_at,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM posts p
        LEFT JOIN users u ON p.user_id = u.id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'like' GROUP BY post_id
        ) AS likes ON p.id = likes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'dislike' GROUP BY post_id
        ) AS dislikes ON p.id = dislikes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM comments GROUP BY post_id
        ) AS comments ON p.id = comments.post_id
        WHERE p.user_id = $1
        ORDER BY p.created_at DESC`
	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []models.Post
	for rows.Next() {
		var post models.Post
		var imageURL, imagePublicID, profileURL sql.NullString
		err := rows.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
			&imageURL, &imagePublicID, &post.CreatedAt, &post.UpdatedAt,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount,
			&post.Username, &profileURL)
		if err != nil {
			return nil, err
		}
		// Handle NULL fields
		if imageURL.Valid {
			post.ImageURL = &imageURL.String
		}
		if imagePublicID.Valid {
			post.ImagePublicID = &imagePublicID.String
		}
		if profileURL.Valid {
			post.AuthorProfileURL = &profileURL.String
		}
		post.Preview = utils.TruncateContent(post.Content, 30)
		post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)
		post.UpdatedAtHuman = utils.TimeAgo(post.UpdatedAt)
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return posts, nil
}

// Updated to include profile information for liked posts
func GetLikesByUserIDWithProfiles(userID int) ([]models.Post, error) {
	query := `
        SELECT p.id, p.user_id, p.title, p.content, p.category, p.image_url, 
               COALESCE(p.image_public_id, '') as image_public_id, p.created_at, p.updated_at,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM posts p
        LEFT JOIN users u ON p.user_id = u.id
        INNER JOIN post_reactions r ON p.id = r.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'like' GROUP BY post_id
        ) AS likes ON p.id = likes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM post_reactions WHERE reaction = 'dislike' GROUP BY post_id
        ) AS dislikes ON p.id = dislikes.post_id
        LEFT JOIN (
            SELECT post_id, COUNT(*) AS count FROM comments GROUP BY post_id
        ) AS comments ON p.id = comments.post_id
        WHERE r.user_id = $1 AND r.reaction = 'like'
        ORDER BY p.created_at DESC`
	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []models.Post
	for rows.Next() {
		var post models.Post
		var imageURL, imagePublicID, profileURL sql.NullString
		err := rows.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
			&imageURL, &imagePublicID, &post.CreatedAt, &post.UpdatedAt,
			&post.LikesCount, &post.DislikesCount, &post.CommentsCount,
			&post.Username, &profileURL)
		if err != nil {
			return nil, err
		}
		// Handle NULL fields
		if imageURL.Valid {
			post.ImageURL = &imageURL.String
		}
		if imagePublicID.Valid {
			post.ImagePublicID = &imagePublicID.String
		}
		if profileURL.Valid {
			post.AuthorProfileURL = &profileURL.String
		}
		post.Preview = utils.TruncateContent(post.Content, 30)
		post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)
		post.UpdatedAtHuman = utils.TimeAgo(post.UpdatedAt)
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return posts, nil
}

// Keep the original functions for backward compatibility but update GetAllPosts
func GetAllPosts(category string) ([]models.Post, error) {
	return GetAllPostsWithProfiles(category)
}

// Rest of the original functions remain unchanged
func CreatePost(userID int, title, content, category string) error {
	query := `INSERT INTO posts (user_id, title, content, category) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, title, content, category)
	return err
}

func CreatePostWithImage(userID int, title, content string, category []string, imageURL string) error {
	categoryStr := strings.Join(category, ",")
	query := `INSERT INTO posts (user_id, title, content, category, image_url) VALUES ($1, $2, $3, $4, $5)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, title, content, categoryStr, imageURL)
	return err
}

func CreatePostWithImageCloudinary(userID int, title, content string, category []string, imageURL, imagePublicID string) error {
	categoryStr := strings.Join(category, ",")
	query := `
        INSERT INTO posts (user_id, title, content, category, image_url, image_public_id, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	now := time.Now()
	_, err := database.DB.ExecContext(context.Background(), query, userID, title, content, categoryStr, imageURL, imagePublicID, now, now)
	return err
}

func GetPost(postID int) (*models.Post, error) {
	query := `
        SELECT id, user_id, title, content, category, image_url, COALESCE(image_public_id, '') as image_public_id, created_at, updated_at
        FROM posts WHERE id = $1`
	row := database.DB.QueryRowContext(context.Background(), query, postID)
	var post models.Post
	var imageURL, imagePublicID sql.NullString
	err := row.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
		&imageURL, &imagePublicID, &post.CreatedAt, &post.UpdatedAt)
	if err != nil {
		return nil, err
	}
	// Handle NULL fields
	if imageURL.Valid {
		post.ImageURL = &imageURL.String
	}
	if imagePublicID.Valid {
		post.ImagePublicID = &imagePublicID.String
	}
	post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)
	post.UpdatedAtHuman = utils.TimeAgo(post.UpdatedAt)
	return &post, nil
}

func GetPostByID(id int) (*models.Post, error) {
	query := `
        SELECT p.id, p.user_id, p.title, p.content, p.category, p.created_at, p.image_url, 
               COALESCE(p.image_public_id, '') as image_public_id,
               u.username, COALESCE(u.profile_url, '') as profile_url,
               (SELECT COUNT(*) FROM post_reactions WHERE post_id = p.id AND reaction = 'like') as likes,
               (SELECT COUNT(*) FROM post_reactions WHERE post_id = p.id AND reaction = 'dislike') as dislikes,
               p.updated_at
        FROM posts p
        LEFT JOIN users u ON p.user_id = u.id
        WHERE p.id = $1`
	var post models.Post
	var imageURL, imagePublicID, profileURL sql.NullString
	err := database.DB.QueryRowContext(context.Background(), query, id).Scan(
		&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
		&post.CreatedAt, &imageURL, &imagePublicID, &post.Username, &profileURL,
		&post.LikesCount, &post.DislikesCount, &post.CommentsCount, &post.UpdatedAt)
	if err != nil {
		return nil, err
	}
	// Handle NULL fields
	if imageURL.Valid {
		post.ImageURL = &imageURL.String
	}
	if imagePublicID.Valid {
		post.ImagePublicID = &imagePublicID.String
	}
	if profileURL.Valid {
		post.AuthorProfileURL = &profileURL.String
	}
	post.CreatedAtHuman = utils.TimeAgo(post.CreatedAt)
	post.UpdatedAtHuman = utils.TimeAgo(post.UpdatedAt)
	return &post, nil
}

// Keep remaining functions unchanged...
func GetPostsByUserID(userID int) ([]models.Post, error) {
	return GetPostsByUserIDWithProfiles(userID)
}

func UpdatePost(postID int, title, content string, category []string) error {
	categoryStr := strings.Join(category, ",")
	query := `UPDATE posts SET title = $1, content = $2, category = $3, updated_at = $4 WHERE id = $5`
	_, err := database.DB.ExecContext(context.Background(), query, title, content, categoryStr, time.Now(), postID)
	return err
}

func UpdatePostWithImage(postID int, title, content string, category []string, imageURL, imagePublicID string) error {
	categoryStr := strings.Join(category, ",")
	query := `UPDATE posts SET title = $1, content = $2, category = $3, image_url = $4, image_public_id = $5, updated_at = $6 WHERE id = $7`
	_, err := database.DB.ExecContext(context.Background(), query, title, content, categoryStr, imageURL, imagePublicID, time.Now(), postID)
	return err
}

func UpdatePostWithImageLegacy(postID int, title, content string, category []string, imageURL string) error {
	categoryStr := strings.Join(category, ",")
	query := `UPDATE posts SET title = $1, content = $2, category = $3, image_url = $4, updated_at = $5 WHERE id = $6`
	_, err := database.DB.ExecContext(context.Background(), query, title, content, categoryStr, imageURL, time.Now(), postID)
	return err
}

func DeletePost(postID int) error {
	query := `DELETE FROM posts WHERE id = $1`
	_, err := database.DB.ExecContext(context.Background(), query, postID)
	return err
}

func GetCategoryPostCounts() (map[string]int, error) {
	query := `SELECT category FROM posts`
	rows, err := database.DB.QueryContext(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var categoryStr string
		if err := rows.Scan(&categoryStr); err != nil {
			return nil, err
		}
		categories := strings.Split(categoryStr, ",")
		for _, category := range categories {
			category = strings.TrimSpace(category)
			counts[category]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
