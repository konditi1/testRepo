package web

import (
	"context"
	"evalhub/internal/database"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func ListPostsHandler(w http.ResponseWriter, r *http.Request) {
	posts, err := GetAllPosts("")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving posts: %v", err))
		return
	}
	userID := r.Context().Value(userIDKey)
	isAuthenticated := userID != nil
	currentUserID := 0
	if isAuthenticated {
		currentUserID = userID.(int)
	}
	var username string
	if isAuthenticated {
		username = getUsername(currentUserID)
	}
	for i := range posts {
		posts[i].Username = getUsername(int(posts[i].UserID))
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
		posts[i].CategoryArray = strings.Split(posts[i].Category, ",")
		posts[i].IsOwner = (posts[i].UserID == int64(currentUserID))
	}
	data := map[string]interface{}{
		"Title":      "All Posts - Forum",
		"IsLoggedIn": isAuthenticated,
		"Username":   username,
		"Posts":      posts,
	}
	err = templates.ExecuteTemplate(w, "post-list", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

func CreatePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		categories, err := GetCategories()
		if err != nil {
			log.Printf("Failed to load categories: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to load categories: %v", err))
			return
		}
		data := map[string]interface{}{
			"Title":      "Create Post - Forum",
			"IsLoggedIn": true,
			"Categories": categories,
		}
		err = templates.ExecuteTemplate(w, "create-post", data)
		if err != nil {
			log.Printf("Failed to render create-post template: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}
	if r.Method == http.MethodPost {
		// Create context for the request with timeout
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to parse form data: %v", err))
			return
		}
		userID, ok := r.Context().Value(userIDKey).(int)
		if !ok {
			log.Println("User ID not found in context")
			RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}
		title := r.FormValue("title")
		content := r.FormValue("content")
		category := r.Form["category[]"]
		if title == "" || content == "" || len(category) == 0 {
			log.Println("Missing required fields")
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("title, content, and category are required"))
			return
		}
		var imageURL string
		var imagePublicID string
		file, header, err := r.FormFile("image")
		if err == nil && header != nil {
			defer file.Close()
			// Initialize Cloudinary service
			cloudinary, err := utils.GetCloudinaryService()
			if err != nil {
				log.Printf("Failed to initialize Cloudinary service: %v", err)
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to initialize file upload service: %v", err))
				return
			}
			// Validate file
			if err := cloudinary.ValidateFile(ctx, header); err != nil {
				log.Printf("File validation failed: %v", err)
				RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("image validation failed: %v", err))
				return
			}
			// Generate a unique folder path for the post image
			uploadFolder := fmt.Sprintf("evalhub/posts/%d", time.Now().UnixNano())
			// Upload to Cloudinary
			uploadResult, err := cloudinary.UploadFile(ctx, header, uploadFolder)
			if err != nil {
				log.Printf("Failed to upload image to Cloudinary: %v", err)
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to upload image: %v", err))
				return
			}
			imageURL = uploadResult.URL
			imagePublicID = uploadResult.PublicID
			log.Printf("Image successfully uploaded to Cloudinary. URL: %s, Public ID: %s", imageURL, imagePublicID)
		} else if err != http.ErrMissingFile {
			log.Printf("Error retrieving file: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to process image: %v", err))
			return
		}
		// Create post with or without image
		var createErr error
		var postID int
		now := time.Now()
		if imageURL != "" && imagePublicID != "" {
			createErr = CreatePostWithImageCloudinary(userID, title, content, category, imageURL, imagePublicID)
		} else {
			categoryStr := strings.Join(category, ",")
			createErr = CreatePost(userID, title, content, categoryStr)
		}
		if createErr != nil {
			log.Printf("Error creating post in database: %v", createErr)
			// Clean up uploaded image if post creation failed
			if imagePublicID != "" {
				cloudinary, _ := utils.GetCloudinaryService()
				if delErr := cloudinary.DeleteFile(ctx, imagePublicID); delErr != nil {
					log.Printf("Warning: Failed to delete orphaned image: %v", delErr)
				}
			}
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create post: %v", createErr))
			return
		}

		// Update user stats after successful post creation
		go UpdateUserStats(ctx, userID)

		// Get the ID of the created post for notifications
		postID, err = GetLatestPostIDByUser(ctx, userID, now)
		if err == nil && postID > 0 {
			go NotifyPostCreated(postID, userID, title)
		} else {
			log.Printf("Warning: Failed to get post ID for notifications: %v", err)
		}
		log.Printf("Post successfully created with image URL: %s", imageURL)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

// Helper function to get the latest post ID by user around creation time
func GetLatestPostIDByUser(ctx context.Context, userID int, createdAt time.Time) (int, error) {
	var postID int
	// Look for the post created within a 5-second window
	startTime := createdAt.Add(-5 * time.Second)
	endTime := createdAt.Add(5 * time.Second)

	query := `SELECT id FROM posts WHERE user_id = $1 AND created_at BETWEEN $2 AND $3 ORDER BY created_at DESC LIMIT 1`
	err := database.DB.QueryRowContext(ctx, query, userID, startTime, endTime).Scan(&postID)
	return postID, err
}

func EditPostHandler(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.URL.Query().Get("id")
	postID, err := strconv.Atoi(postIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Post ID: %v", err))
		return
	}
	if r.Method == http.MethodGet {
		post, err := GetPost(postID)
		if err != nil || post == nil {
			RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("post not found: %v", err))
			return
		}
		categories, err := GetCategories()
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to load categories: %v", err))
			return
		}
		fmt.Printf("Post data: %+v\n", post)
		data := map[string]interface{}{
			"Title":      "Edit Post - Forum",
			"IsLoggedIn": true,
			"Post":       post,
			"Categories": categories,
		}
		fmt.Printf("Template data: %+v\n", data)
		err = templates.ExecuteTemplate(w, "edit-post", data)
		if err != nil {
			fmt.Printf("Error executing edit-post template: %v\n", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}
	if r.Method == http.MethodPost {
		// Create context for the request with timeout
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		// Parse multipart form for file uploads
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to parse form data: %v", err))
			return
		}

		title := r.FormValue("title")
		content := r.FormValue("content")
		category := r.Form["category[]"]

		// Check if there's a new image to upload
		file, header, err := r.FormFile("image")
		var newImageURL string
		var newImagePublicID string

		if err == nil && header != nil {
			defer file.Close()

			// Get the existing post to find its image public ID (if any)
			existingPost, err := GetPost(postID)
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to retrieve existing post: %v", err))
				return
			}

			// Initialize Cloudinary service
			cloudinary, err := utils.GetCloudinaryService()
			if err != nil {
				log.Printf("Failed to initialize Cloudinary service: %v", err)
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to initialize file upload service: %v", err))
				return
			}

			// Validate file
			if err := cloudinary.ValidateFile(ctx, header); err != nil {
				log.Printf("File validation failed: %v", err)
				RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("image validation failed: %v", err))
				return
			}

			// Generate a unique folder path for the post image
			uploadFolder := fmt.Sprintf("evalhub/posts/%d", time.Now().UnixNano())

			// Upload to Cloudinary
			uploadResult, err := cloudinary.UploadFile(ctx, header, uploadFolder)
			if err != nil {
				log.Printf("Failed to upload image to Cloudinary: %v", err)
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to upload image: %v", err))
				return
			}

			newImageURL = uploadResult.URL
			newImagePublicID = uploadResult.PublicID

			log.Printf("New image successfully uploaded to Cloudinary. URL: %s, Public ID: %s", newImageURL, newImagePublicID)

			// Delete the old image if it exists
			if existingPost.ImagePublicID != nil && *existingPost.ImagePublicID != "" {
				if delErr := cloudinary.DeleteFile(ctx, *existingPost.ImagePublicID); delErr != nil {
					log.Printf("Warning: Failed to delete old image during post update: %v", delErr)
				} else {
					log.Printf("Successfully deleted old image: %s", *existingPost.ImagePublicID)
				}
			}
		} else if err != http.ErrMissingFile {
			log.Printf("Error retrieving file: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to process image: %v", err))
			return
		}

		// Update the post with or without new image
		if newImageURL != "" && newImagePublicID != "" {
			// Update with new image
			err = UpdatePostWithImage(postID, title, content, category, newImageURL, newImagePublicID)
		} else {
			// Update without changing image
			err = UpdatePost(postID, title, content, category)
		}

		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error updating post: %v", err))
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func DeletePostHandler(w http.ResponseWriter, r *http.Request) {
	// Create context for the delete operation
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	postIDStr := r.URL.Query().Get("id")
	postID, err := strconv.Atoi(postIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Post ID: %v", err))
		return
	}

	// Get the post to retrieve the image public ID (if any)
	post, err := GetPost(postID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to retrieve post: %v", err))
		return
	}

	// Delete the post from the database
	err = DeletePost(postID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error deleting post: %v", err))
		return
	}

	// Clean up image from Cloudinary if it exists
	if post.ImagePublicID != nil && *post.ImagePublicID != "" {
		cloudinary, err := utils.GetCloudinaryService()
		if err != nil {
			log.Printf("Warning: Failed to initialize Cloudinary service for image cleanup: %v", err)
		} else {
			if delErr := cloudinary.DeleteFile(ctx, *post.ImagePublicID); delErr != nil {
				log.Printf("Warning: Failed to delete image after post deletion: %v", delErr)
			} else {
				log.Printf("Successfully deleted image: %s", *post.ImagePublicID)
			}
		}
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func ViewPostHandler(w http.ResponseWriter, r *http.Request) {
	postID := r.URL.Query().Get("id")
	log.Println("Viewing post ID:", postID)

	id, err := strconv.Atoi(postID)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid post ID: %s", r.URL.Path))
		return
	}
	post, err := GetPostByID(id)
	if err != nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("page not found: %s", r.URL.Path))
		return
	}
	
	ctx := r.Context()
	likes, dislikes, err := GetReactionCounts(ctx, id)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error getting reaction counts: %v", err))
		return
	}
	post.LikesCount = likes
	post.DislikesCount = dislikes
	commentsCount, err := GetCommentsCount(id)
	if err != nil {
		commentsCount = 0
	}
	post.CommentsCount = commentsCount
	comments, err := GetCommentsByPostID(id)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving comments: %v", err))
		return
	}
	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		fmt.Printf("Error getting category counts: %v\n", err)
		categoryCounts = make(map[string]int)
	}
	userID := r.Context().Value(userIDKey)
	isAuthenticated := userID != nil
	currentUserID := 0

	if isAuthenticated {
		currentUserID = userID.(int)
	}
	var username string
	if isAuthenticated {
		username = getUsername(currentUserID)
	}
	for i := range comments {
		comments[i].Username = getUsername(int(comments[i].UserID))
		comments[i].CreatedAtHuman = utils.TimeAgo(comments[i].CreatedAt)
		// Add ownership flag for comments
		comments[i].IsOwner = (comments[i].UserID == int64(currentUserID))
	}

	// Set ownership flag for the post
	post.IsOwner = (post.UserID == int64(currentUserID))
	imageURL := ""
	if post.ImageURL != nil {
		imageURL = *post.ImageURL
	}
	imagePublicID := ""
	if post.ImagePublicID != nil {
		imagePublicID = *post.ImagePublicID
	}
	log.Printf("Post details - ID: %d, ImageURL: %s, ImagePublicID: %s",
		id, imageURL, imagePublicID)
	data := map[string]interface{}{
		"Title":           post.Title + " - Forum",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"Post":            post,
		"Comments":        comments,
		"IsAuthenticated": isAuthenticated,
		"UserID":          userID,
		"Categories":      categoryCounts,
	}
	err = templates.ExecuteTemplate(w, "view-post", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

func LikePostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	postIDStr := r.URL.Query().Get("id")
	postID, err := strconv.Atoi(postIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Post ID: %v", err))
		return
	}

	err = ToggleReaction(ctx, userID, postID, "like")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to like post: %v", err))
		return
	}

	likes, dislikes, _ := GetReactionCounts(ctx, postID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

func DislikePostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	postIDStr := r.URL.Query().Get("id")
	postID, err := strconv.Atoi(postIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Post ID: %v", err))
		return
	}

	err = ToggleReaction(ctx, userID, postID, "dislike")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to dislike post: %v", err))
		return
	}

	likes, dislikes, _ := GetReactionCounts(ctx, postID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

func UserPostHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(int)
	if !ok {
		log.Println("User ID not found in context")
		RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}
	posts, err := GetPostsByUserID(userID)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to retrieve posts: %v", err))
		return
	}
	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving category counts: %v", err))
		return
	}
	isAuthenticated := userID != 0
	var username string
	if isAuthenticated {
		username = getUsername(userID)
	}
	for i := range posts {
		posts[i].Username = getUsername(int(posts[i].UserID))
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
		posts[i].CategoryArray = strings.Split(posts[i].Category, ",")
		// Set ownership flag
		posts[i].IsOwner = true // User's own posts
	}
	data := map[string]interface{}{
		"Title":           "Your Posts - Forum",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
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

func SearchPostsHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	// Sanitize the search query
	query = strings.TrimSpace(utils.SanitizeString(query))
	if len(query) < 3 {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("search query must be at least 3 characters long"))
		return
	}
	// Fetch posts matching the search query
	posts, err := SearchPosts(query)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error searching posts: %v", err))
		return
	}
	userID := r.Context().Value(userIDKey)
	isAuthenticated := userID != nil
	currentUserID := 0
	if isAuthenticated {
		currentUserID = userID.(int)
	}
	var username string
	if isAuthenticated {
		username = getUsername(currentUserID)
	}
	// Process posts for display
	for i := range posts {
		posts[i].Username = getUsername(int(posts[i].UserID))
		if len(posts[i].Content) > 200 {
			posts[i].Preview = posts[i].Content[:200] + "..."
		} else {
			posts[i].Preview = posts[i].Content
		}
		posts[i].CreatedAtHuman = utils.TimeAgo(posts[i].CreatedAt)
		posts[i].CategoryArray = strings.Split(posts[i].Category, ",")
		posts[i].IsOwner = (posts[i].UserID == int64(currentUserID))
	}
	// Fetch category counts for sidebar
	categoryCounts, err := GetCategoryPostCounts()
	if err != nil {
		log.Printf("Error retrieving category counts: %v", err)
		categoryCounts = make(map[string]int)
	}
	data := map[string]interface{}{
		"Title":           fmt.Sprintf("Search Results for '%s' - Forum", query),
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"Posts":           posts,
		"Categories":      categoryCounts,
		"IsAuthenticated": isAuthenticated,
		"SearchQuery":     query,
	}
	err = templates.ExecuteTemplate(w, "dashboard", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}
