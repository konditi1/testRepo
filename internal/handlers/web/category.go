// file: internal/handlers/web/category.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"evalhub/internal/database"
	"evalhub/internal/utils"
)

// GetCategories retrieves all categories from the database.
func GetCategories() ([]string, error) {
	ctx := context.Background()
	query := `SELECT name FROM categories ORDER BY name ASC`
	rows, err := database.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying categories: %v", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("error scanning category: %v", err)
		}
		categories = append(categories, name)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating categories: %v", err)
	}
	return categories, nil
}

// PostsByCategoryHandler handles category filtering
func PostsByCategoryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	category := r.URL.Query().Get("category")
	if category == "" {
		http.Error(w, "category parameter is required", http.StatusBadRequest)
		return
	}

	userID := ctx.Value(userIDKey)
	currentUserID := 0
	if userID != nil {
		currentUserID = userID.(int)
	}

	posts, err := GetAllPosts(category)
	if err != nil {
		http.Error(w, fmt.Sprintf("error retrieving posts: %v", err), http.StatusInternalServerError)
		return
	}

	for i := range posts {
		// Get username
		var username string
		err := database.DB.QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", posts[i].UserID).Scan(&username)
		if err != nil {
			if err == sql.ErrNoRows {
				posts[i].Username = "Unknown"
			} else {
				log.Printf("Error fetching username for user %d: %v", posts[i].UserID, err)
				posts[i].Username = "Unknown"
			}
		} else {
			posts[i].Username = username
		}

		// Get comments count
		commentsCount, err := GetCommentsCount(int(posts[i].ID))
		if err != nil {
			log.Printf("Error getting comment count for post %d: %v", posts[i].ID, err)
			posts[i].CommentsCount = 0
		} else {
			posts[i].CommentsCount = commentsCount
		}

		// Get reaction counts
		likes, dislikes, err := GetReactionCounts(ctx, int(posts[i].ID))
		if err != nil {
			log.Printf("Error getting reaction counts for post %d: %v", posts[i].ID, err)
		}
		posts[i].LikesCount = likes
		posts[i].DislikesCount = dislikes

		// Set ownership and format content
		posts[i].IsOwner = (int(posts[i].UserID) == currentUserID)
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"posts":   posts,
	})
}

// GetCategoriesHandler handles the HTTP request to retrieve all categories with post counts.
func GetCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	categories, err := GetCategories()
	if err != nil {
		http.Error(w, fmt.Sprintf("error retrieving categories: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(categories); err != nil {
		log.Printf("Error encoding categories to JSON: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
