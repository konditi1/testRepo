// file: internal/handlers/web/handlers.go
package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"unicode"
)

var templates *template.Template
var errorTemplates *template.Template

type contextKey string

const userIDKey contextKey = "user_id"

func InitTemplates(baseDir string) error {
	funcMap := template.FuncMap{
		"title": func(s string) string {
			if s == "" {
				return s
			}
			runes := []rune(s)
			runes[0] = unicode.ToUpper(runes[0])
			return string(runes)
		},
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		// Add the replace function
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		// Add some other useful template functions
		"toLower": func(s string) string {
			return strings.ToLower(s)
		},
		"toUpper": func(s string) string {
			return strings.ToUpper(s)
		},
		"formatTime": func(t time.Time) string {
			return t.Format("Jan 2, 2006 15:04")
		},
		"truncate": func(s string, length int) string {
			if len(s) <= length {
				return s
			}
			return s[:length] + "..."
		},
	}

	var err error
	templates, err = template.New("").Funcs(funcMap).ParseGlob(filepath.Join(baseDir, "templates", "*.html"))
	if err != nil {
		return fmt.Errorf("failed to parse templates: %v", err)
	}

	errorTemplates, err = template.ParseGlob(filepath.Join(baseDir, "templates", "error", "*.html"))
	if err != nil {
		return fmt.Errorf("failed to parse error templates: %v", err)
	}

	for _, t := range errorTemplates.Templates() {
		if tmpl := templates.Lookup(t.Name()); tmpl != nil {
			templates, err = templates.AddParseTree(t.Name(), t.Tree)
			if err != nil {
				return fmt.Errorf("failed to add parse tree template %s: %v", t.Name(), err)
			}
		}
	}

	return nil
}

// getUserWithProfile fetches user details including profile URL
func getUserWithProfile(ctx context.Context, userID int) (*models.User, error) {
	query := `
        SELECT id, username, profile_url
        FROM users WHERE id = $1`
	row := database.DB.QueryRowContext(ctx, query, userID)

	user := &models.User{}
	var profileURL sql.NullString

	err := row.Scan(&user.ID, &user.Username, &profileURL)
	if err != nil {
		return nil, err
	}

	if profileURL.Valid {
		url := profileURL.String
		user.ProfileURL = &url
	}

	return user, nil
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("page not found: %s", r.URL.Path))
		return
	}

	posts, err := GetAllPostsWithProfiles("")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving posts: %v", err))
		return
	}

	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving category counts: %v", err))
		return
	}

	userID := r.Context().Value(userIDKey)
	isAuthenticated := userID != nil
	var user *models.User
	var username string

	if isAuthenticated {
		user, err = getUserWithProfile(r.Context(), userID.(int))
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
			// Continue without profile picture but ensure user is not nil
			user = &models.User{
				ID:       int64(userID.(int)),
				Username: getUsername(userID.(int)),
			}
		}
		username = user.Username
	} else {
		// For non-authenticated users, create an empty user object
		user = &models.User{}
		username = ""
	}

	for i := range posts {
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
		posts[i].CategoryArray = strings.Split(posts[i].Category, ",")
	}

	data := map[string]interface{}{
		"Title":           "Melconnect - Home",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"User":            user,
		"Posts":           posts,
		"Categories":      categoryCounts,
		"IsAuthenticated": isAuthenticated,
	}

	err = templates.ExecuteTemplate(w, "dashboard", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("DashboardHandler invoked")
	userID := r.Context().Value(userIDKey).(int)

	posts, err := GetAllPostsWithProfiles("")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving posts: %v", err))
		return
	}

	questions, err := GetAllQuestionsWithProfiles()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving questions: %v", err))
		return
	}

	// Get recent documents for the dashboard
	documents, err := GetRecentDocuments(5, "")
	if err != nil {
		log.Printf("Error retrieving documents: %v", err)
		documents = []models.Document{} // Use empty slice on error
	}

	users, err := GetAllUsers(userID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving users: %v", err))
		return
	}

	// Get current user with profile
	user, err := getUserWithProfile(r.Context(), userID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(userID)}
	}

	for i := range posts {
		posts[i].CategoryArray = strings.Split(posts[i].Category, ",")
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
	}

	for i := range questions {
		questions[i].IsOwner = (questions[i].UserID == int64(userID))
	}

	// Set document ownership
	for i := range documents {
		documents[i].IsOwner = (documents[i].UserID == userID)
	}

	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving category counts: %v", err))
		return
	}

	data := map[string]interface{}{
		"Title":      "Dashboard - Melconnect",
		"IsLoggedIn": true,
		"Username":   user.Username,
		"User":       user,
		"Posts":      posts,
		"Questions":  questions,
		"Documents":  documents,
		"Categories": categoryCounts,
		"UserID":     userID,
		"Users":      users,
	}

	err = templates.ExecuteTemplate(w, "dashboard", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

func FilterLikesHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	posts, err := GetLikesByUserIDWithProfiles(userID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving posts: %v", err))
		return
	}

	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving category counts: %v", err))
		return
	}

	user, err := getUserWithProfile(r.Context(), userID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(userID)}
	}

	for i := range posts {
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
	}

	data := map[string]interface{}{
		"Title":           "Melconnect - Home",
		"IsLoggedIn":      true,
		"Username":        user.Username,
		"User":            user,
		"Posts":           posts,
		"Categories":      categoryCounts,
		"IsAuthenticated": true,
	}

	err = templates.ExecuteTemplate(w, "dashboard", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

// Keep the rest of the handlers but update them to include user profiles...

func ToggleReactions(ctx context.Context, userID, postID int, reactionType string) error {
	var existingReaction string
	query := `SELECT reaction_type FROM reactions WHERE user_id = $1 AND post_id = $2`
	err := database.DB.QueryRowContext(ctx, query, userID, postID).Scan(&existingReaction)

	query1 := `INSERT INTO reactions (user_id, post_id, reaction_type) VALUES ($1, $2, $3)`
	if err == sql.ErrNoRows {
		_, err = database.DB.ExecContext(ctx, query1, userID, postID, reactionType)
		return err
	}

	if err != nil {
		return err
	}

	deleteQuery := `DELETE FROM reactions WHERE user_id = $1 AND post_id = $2`
	if existingReaction == reactionType {
		_, err = database.DB.ExecContext(ctx, deleteQuery, userID, postID)
		return err
	}

	Updatequery := `UPDATE reactions SET reaction_type = $1 WHERE user_id = $2 AND post_id = $3`
	_, err = database.DB.ExecContext(ctx, Updatequery, reactionType, userID, postID)
	return err
}

func SearchPosts(query string) ([]models.Post, error) {
	rows, err := database.DB.QueryContext(context.Background(), `
		SELECT DISTINCT p.id, p.user_id, p.title, p.content, p.category, p.image_url, p.image_public_id, 
			p.created_at, p.updated_at
		FROM posts p
		LEFT JOIN comments c ON p.id = c.post_id
		WHERE p.title ILIKE $1 OR p.content ILIKE $2 OR c.content ILIKE $3
		ORDER BY p.created_at DESC`, "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to query posts: %v", err)
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var post models.Post
		var imageURL, imagePublicID sql.NullString
		err := rows.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.Category,
			&imageURL, &imagePublicID, &post.CreatedAt, &post.UpdatedAt)
		if err != nil {
			log.Printf("Error scanning post: %v", err)
			continue
		}
		if imageURL.Valid {
			post.ImageURL = &imageURL.String
		}
		if imagePublicID.Valid {
			post.ImagePublicID = &imagePublicID.String
		}
		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating posts: %v", err)
	}

	return posts, nil
}

func SearchSuggestions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(strings.TrimSpace(query)) < 3 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode([]string{})
		return
	}

	query = strings.TrimSpace(utils.SanitizeString(query))
	rows, err := database.DB.QueryContext(context.Background(), `
		SELECT DISTINCT title
		FROM (
			SELECT p.title
			FROM posts p
			WHERE p.title ILIKE $1 OR p.content ILIKE $1
			UNION
			SELECT LEFT(c.content, 50) AS title
			FROM comments c
			WHERE c.content ILIKE $1
		) AS results
		LIMIT 5`, "%"+query+"%")
	if err != nil {
		log.Printf("Error querying suggestions: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode([]string{})
		return
	}
	defer rows.Close()

	var suggestions []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			log.Printf("Error scanning suggestion: %v", err)
			continue
		}
		suggestions = append(suggestions, title)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(suggestions)
}
