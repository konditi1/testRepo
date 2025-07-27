package web

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func CreateQuestionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		categories, err := GetCategories()
		if err != nil {
			log.Printf("Failed to load categories: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to load categories: %v", err))
			return
		}
		data := map[string]interface{}{
			"Title":      "Create Question - Forum",
			"IsLoggedIn": true,
			"Categories": categories,
		}
		err = templates.ExecuteTemplate(w, "create-question", data)
		if err != nil {
			log.Printf("Failed to render create-question template: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}
	if r.Method == http.MethodPost {
		// Create context for the upload process
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
		title := utils.SanitizeString(r.FormValue("title"))
		content := utils.SanitizeString(r.FormValue("content"))
		category := r.Form["category[]"]
		targetGroup := utils.SanitizeString(r.FormValue("target_group"))
		if title == "" || len(category) == 0 {
			log.Println("Missing required fields")
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("title and category are required"))
			return
		}
		validGroups := []string{"All", "Data Analysis", "M&E in Science", "M&E in Health", "M&E in Education", "M&E in Social Sciences", "M&E in Climate Change", "M&E in Agriculture"}
		isValidGroup := false
		for _, group := range validGroups {
			if targetGroup == group {
				isValidGroup = true
				break
			}
		}
		if !isValidGroup {
			log.Println("Invalid target group")
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid target group"))
			return
		}
		var fileURL string
		var filePublicID string
		file, header, err := r.FormFile("attachment")
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
				RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("attachment validation failed: %v", err))
				return
			}
			// Additional file type validation
			fileType := header.Header.Get("Content-Type")
			isValidType := false
			validTypes := []string{"image/jpeg", "image/jpg", "image/png", "image/gif", "image/svg+xml", "application/pdf"}
			for _, validType := range validTypes {
				if fileType == validType {
					isValidType = true
					break
				}
			}
			if !isValidType {
				log.Printf("Unsupported file type: %s", fileType)
				RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("unsupported file type. Allowed types: JPEG, PNG, GIF, SVG, PDF"))
				return
			}
			// Generate a unique folder path for the question attachment
			uploadFolder := fmt.Sprintf("evalhub/questions/%d", time.Now().UnixNano())
			// Upload to Cloudinary
			uploadResult, err := cloudinary.UploadFile(ctx, header, uploadFolder)
			if err != nil {
				log.Printf("Failed to upload file to Cloudinary: %v", err)
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to upload attachment: %v", err))
				return
			}
			fileURL = uploadResult.URL
			filePublicID = uploadResult.PublicID
			log.Printf("File successfully uploaded to Cloudinary. URL: %s, Public ID: %s", fileURL, filePublicID)
		} else if err != http.ErrMissingFile {
			log.Printf("Error retrieving file: %v", err)
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to process attachment: %v", err))
			return
		}
		// Insert question with Cloudinary URLs and public IDs
		query := `
			INSERT INTO questions (user_id, title, content, category, file_url, file_public_id, target_group, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		now := time.Now()
		_, err = database.DB.ExecContext(context.Background(),
			query,
			userID,
			title,
			content,
			strings.Join(category, ","),
			fileURL,
			filePublicID,
			targetGroup,
			now,
			now,
		)
		if err != nil {
			log.Printf("Error creating question in database: %v", err)
			// If database insert fails but we uploaded a file, clean it up
			if filePublicID != "" {
				cloudinary, _ := utils.GetCloudinaryService()
				if delErr := cloudinary.DeleteFile(ctx, filePublicID); delErr != nil {
					log.Printf("Warning: Failed to delete orphaned file after question creation failure: %v", delErr)
				} else {
					log.Printf("Successfully deleted orphaned file: %s", filePublicID)
				}
			}
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create question: %v", err))
			return
		}
		
		// Update user stats after successful question creation
		go UpdateUserStats(ctx, userID)
		
		// Get the ID of the created question for notifications
		questionID, getErr := GetLatestQuestionIDByUser(userID, now)
		if getErr == nil && questionID > 0 {
			go NotifyQuestionCreated(questionID, userID, title)
		} else {
			log.Printf("Warning: Failed to get question ID for notifications: %v", getErr)
		}
		log.Printf("Question successfully created with file URL: %s", fileURL)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

// Helper function to get the latest question ID by user around creation time
func GetLatestQuestionIDByUser(userID int, createdAt time.Time) (int, error) {
	var questionID int
	// Look for the question created within a 5-second window
	startTime := createdAt.Add(-5 * time.Second)
	endTime := createdAt.Add(5 * time.Second)

	query := `SELECT id FROM questions WHERE user_id = $1 AND created_at BETWEEN $2 AND $3 ORDER BY created_at DESC LIMIT 1`
	err := database.DB.QueryRowContext(context.Background(), query, userID, startTime, endTime).Scan(&questionID)
	return questionID, err
}

// Replace the GetAllQuestions function in question_handlers.go with this complete version
func GetAllQuestions() ([]models.Question, error) {
	query := `
		SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, q.file_public_id, q.target_group, 
			q.created_at, q.updated_at, u.username,
			COALESCE(likes.count, 0) as likes,
			COALESCE(dislikes.count, 0) as dislikes,
			COALESCE(comments.count, 0) as comments_count
		FROM questions q
		JOIN users u ON q.user_id = u.id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'like' GROUP BY question_id
		) AS likes ON q.id = likes.question_id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'dislike' GROUP BY question_id
		) AS dislikes ON q.id = dislikes.question_id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM comments WHERE question_id IS NOT NULL GROUP BY question_id
		) AS comments ON q.id = comments.question_id
		ORDER BY q.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), query)
	if err != nil {
		// If there's an error with the complex query, try a simpler one
		simpleQuery := `
			SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, q.file_public_id, q.target_group, 
				q.created_at, q.updated_at, u.username,
				0 as likes, 0 as dislikes, 0 as comments_count
			FROM questions q
			JOIN users u ON q.user_id = u.id
			ORDER BY q.created_at DESC`

		rows, err = database.DB.QueryContext(context.Background(), simpleQuery)
		if err != nil {
			return nil, fmt.Errorf("error querying questions: %v", err)
		}
	}
	defer rows.Close()
	var questions []models.Question
	for rows.Next() {
		var q models.Question
		err := rows.Scan(
			&q.ID, &q.UserID, &q.Title, &q.Content, &q.Category, &q.FileURL, &q.FilePublicID,
			&q.TargetGroup, &q.CreatedAt, &q.UpdatedAt, &q.Username, &q.LikesCount, &q.DislikesCount, &q.CommentsCount,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning question: %v", err)
		}
		q.CreatedAtHuman = utils.TimeAgo(q.CreatedAt)
		q.CategoryArray = strings.Split(q.Category, ",")
		questions = append(questions, q)
	}
	return questions, nil
}

// Delete a question and its attached file if any
func DeleteQuestion(questionID int) error {
	// First, get the file public ID if it exists
	var filePublicID string
	query := `SELECT file_public_id FROM questions WHERE id = $1`
	err := database.DB.QueryRowContext(context.Background(), query, questionID).Scan(&filePublicID)
	if err != nil {
		return fmt.Errorf("failed to get question details: %v", err)
	}
	// Delete the question from the database
	deleteQuery := `DELETE FROM questions WHERE id = $1`
	_, err = database.DB.ExecContext(context.Background(), deleteQuery, questionID)
	if err != nil {
		return fmt.Errorf("failed to delete question: %v", err)
	}
	// If there was an attached file, delete it from Cloudinary
	if filePublicID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cloudinary, err := utils.GetCloudinaryService()
		if err != nil {
			return fmt.Errorf("failed to initialize Cloudinary service for file cleanup: %v", err)
		}
		if err := cloudinary.DeleteFile(ctx, filePublicID); err != nil {
			// Log this error but don't fail the operation
			log.Printf("Warning: Failed to delete file from Cloudinary: %v", err)
		}
	}
	return nil
}

// Get a specific question by ID
func GetQuestionByID(questionID int) (*models.Question, error) {
	query := `
		SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, q.file_public_id, q.target_group, 
			q.created_at, q.updated_at, u.username
		FROM questions q
		JOIN users u ON q.user_id = u.id
		WHERE q.id = $1`
	var q models.Question
	err := database.DB.QueryRowContext(context.Background(), query, questionID).Scan(
		&q.ID, &q.UserID, &q.Title, &q.Content, &q.Category, &q.FileURL, &q.FilePublicID,
		&q.TargetGroup, &q.CreatedAt, &q.UpdatedAt, &q.Username,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get question: %v", err)
	}
	q.CreatedAtHuman = utils.TimeAgo(q.CreatedAt)
	q.CategoryArray = strings.Split(q.Category, ",")
	return &q, nil
}

// Update a question with or without uploading a new file
func UpdateQuestion(ctx context.Context, questionID int, title, content string, category []string, targetGroup string, file *http.Request) error {
	// Get current question to check if it has a file
	currentQuestion, err := GetQuestionByID(questionID)
	if err != nil {
		return fmt.Errorf("failed to get current question: %v", err)
	}
	fileURL := currentQuestion.FileURL
	filePublicID := currentQuestion.FilePublicID
	// Check if a new file was uploaded
	newFile, header, err := file.FormFile("attachment")
	if err == nil && header != nil {
		defer newFile.Close()
		// Initialize Cloudinary service
		cloudinary, err := utils.GetCloudinaryService()
		if err != nil {
			return fmt.Errorf("failed to initialize Cloudinary service: %v", err)
		}
		// Validate the new file
		if err := cloudinary.ValidateFile(ctx, header); err != nil {
			return fmt.Errorf("file validation failed: %v", err)
		}
		// Upload the new file
		uploadFolder := fmt.Sprintf("evalhub/questions/%d", time.Now().UnixNano())
		uploadResult, err := cloudinary.UploadFile(ctx, header, uploadFolder)
		if err != nil {
			return fmt.Errorf("failed to upload file: %v", err)
		}
		// If there was an old file, delete it
		if currentQuestion.FilePublicID != nil && *currentQuestion.FilePublicID != "" {
			if err := cloudinary.DeleteFile(ctx, *currentQuestion.FilePublicID); err != nil {
				log.Printf("Warning: Failed to delete old file: %v", err)
			}
		}
		// Update the file URL and public ID
		fileURL = &uploadResult.URL
		filePublicID = &uploadResult.PublicID
	} else if err != http.ErrMissingFile {
		return fmt.Errorf("error processing attachment: %v", err)
	}
	// Update the question in the database
	updateQuery := `
		UPDATE questions 
		SET title = $1, content = $2, category = $3, file_url = $4, file_public_id = $5, target_group = $6, updated_at = $7
		WHERE id = $8`
	_, err = database.DB.ExecContext(ctx,
		updateQuery,
		title,
		content,
		strings.Join(category, ","),
		fileURL,
		filePublicID,
		targetGroup,
		time.Now(),
		questionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update question: %v", err)
	}
	return nil
}

// ViewQuestionHandler displays a specific question with its comments
func ViewQuestionHandler(w http.ResponseWriter, r *http.Request) {
	questionID := r.URL.Query().Get("id")
	log.Println("Viewing question ID:", questionID)
	id, err := strconv.Atoi(questionID)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid question ID: %s", r.URL.Path))
		return
	}
	question, err := GetQuestionByIDWithReactions(id)
	if err != nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("page not found: %s", r.URL.Path))
		return
	}
	comments, err := GetCommentsByQuestionID(id)
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
		comments[i].IsOwner = (comments[i].UserID == int64(currentUserID))
	}
	question.IsOwner = (question.UserID == int64(currentUserID))
	data := map[string]interface{}{
		"Title":           question.Title + " - Forum",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"Question":        question,
		"Comments":        comments,
		"IsAuthenticated": isAuthenticated,
		"UserID":          userID,
		"Categories":      categoryCounts,
	}
	err = templates.ExecuteTemplate(w, "view-question", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

// GetQuestionByIDWithReactions retrieves a question with its reaction counts
func GetQuestionByIDWithReactions(id int) (*models.Question, error) {
	question := &models.Question{}
	query := `
		SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, q.file_public_id, q.target_group,
		       q.created_at, q.updated_at, u.username,
		       COALESCE(likes.count, 0) as likes,
		       COALESCE(dislikes.count, 0) as dislikes,
		       COALESCE(comments.count, 0) as comments_count
		FROM questions q
		LEFT JOIN users u ON q.user_id = u.id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'like' GROUP BY question_id
		) AS likes ON q.id = likes.question_id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'dislike' GROUP BY question_id
		) AS dislikes ON q.id = dislikes.question_id
		LEFT JOIN (
			SELECT question_id, COUNT(*) AS count FROM comments WHERE question_id IS NOT NULL GROUP BY question_id
		) AS comments ON q.id = comments.question_id
		WHERE q.id = $1`
	err := database.DB.QueryRowContext(context.Background(), query, id).Scan(
		&question.ID, &question.UserID, &question.Title, &question.Content, &question.Category,
		&question.FileURL, &question.FilePublicID, &question.TargetGroup, &question.CreatedAt,
		&question.UpdatedAt, &question.Username, &question.LikesCount, &question.DislikesCount, &question.CommentsCount)
	if err != nil {
		return nil, err
	}
	question.CreatedAtHuman = utils.TimeAgo(question.CreatedAt)
	question.CategoryArray = strings.Split(question.Category, ",")
	return question, nil
}

// GetCommentsByQuestionID retrieves all comments for a specific question
func GetCommentsByQuestionID(questionID int) ([]models.Comment, error) {
	query := `
		SELECT c.id, c.question_id, c.user_id, c.username, c.content, c.created_at,
		       COALESCE(likes.count, 0) as likes,
		       COALESCE(dislikes.count, 0) as dislikes
		FROM comments c
		LEFT JOIN (
			SELECT comment_id, COUNT(*) AS count FROM comment_reactions WHERE reaction = 'like' GROUP BY comment_id
		) AS likes ON c.id = likes.comment_id
		LEFT JOIN (
			SELECT comment_id, COUNT(*) AS count FROM comment_reactions WHERE reaction = 'dislike' GROUP BY comment_id
		) AS dislikes ON c.id = dislikes.comment_id
		WHERE c.question_id = $1
		ORDER BY c.created_at ASC`
	rows, err := database.DB.QueryContext(context.Background(), query, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []models.Comment
	for rows.Next() {
		var comment models.Comment
		var questionID sql.NullInt64
		err := rows.Scan(&comment.ID, &questionID, &comment.UserID, &comment.Username,
			&comment.Content, &comment.CreatedAt, &comment.LikesCount, &comment.DislikesCount)
		if err != nil {
			log.Printf("Error scanning comment: %v", err)
			continue
		}
		comments = append(comments, comment)
	}
	return comments, nil
}

// ToggleQuestionReaction handles like/dislike for questions
func ToggleQuestionReaction(ctx context.Context, userID, questionID int, reactionType string) error {
	var existingReaction string
	query := `SELECT reaction FROM question_reactions WHERE user_id = $1 AND question_id = $2`
	err := database.DB.QueryRowContext(ctx, query, userID, questionID).Scan(&existingReaction)
	if err == sql.ErrNoRows {
		// No existing reaction, create new one
		query1 := `INSERT INTO question_reactions (user_id, question_id, reaction) VALUES ($1, $2, $3)`
		_, err = database.DB.ExecContext(ctx, query1, userID, questionID, reactionType)
		if err == nil && reactionType == "like" {
			go NotifyLikeCreated(userID, nil, &questionID, nil)
			
			// Update stats for the user who gave the like
			go UpdateUserStats(ctx, userID)
			
			// Update stats for the question author who received the like
			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM questions WHERE id = $1", questionID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	}
	if err != nil {
		return err
	}
	// Existing reaction found
	if existingReaction == reactionType {
		// Same reaction, remove it
		deleteQuery := `DELETE FROM question_reactions WHERE user_id = $1 AND question_id = $2`
		_, err = database.DB.ExecContext(ctx, deleteQuery, userID, questionID)
		// When removing a like, update stats for both users
		if err == nil && reactionType == "like" {
			go UpdateUserStats(ctx, userID)
			
			var authorID int
			if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM questions WHERE id = $1", questionID).Scan(&authorID); dbErr == nil && authorID > 0 {
				go UpdateUserStats(ctx, authorID)
			}
		}
		return err
	}
	// Different reaction, update it
	updateQuery := `UPDATE question_reactions SET reaction = $1, updated_at = NOW() WHERE user_id = $2 AND question_id = $3`
	_, err = database.DB.ExecContext(ctx, updateQuery, reactionType, userID, questionID)
	if err == nil && reactionType == "like" {
		go NotifyLikeCreated(userID, nil, &questionID, nil)
		
		// Update stats for the user who gave the like
		go UpdateUserStats(ctx, userID)
		
		// Update stats for the question author who received the like
		var authorID int
		if dbErr := database.DB.QueryRowContext(ctx, "SELECT user_id FROM questions WHERE id = $1", questionID).Scan(&authorID); dbErr == nil && authorID > 0 {
			go UpdateUserStats(ctx, authorID)
		}
	}
	return err
}

// GetQuestionReactionCounts gets the like and dislike counts for a question
func GetQuestionReactionCounts(ctx context.Context, questionID int) (int, int, error) {
	var likes, dislikes int
	likeQuery := `SELECT COUNT(*) FROM question_reactions WHERE question_id = $1 AND reaction = 'like'`
	err := database.DB.QueryRowContext(ctx, likeQuery, questionID).Scan(&likes)
	if err != nil {
		return 0, 0, err
	}

	dislikeQuery := `SELECT COUNT(*) FROM question_reactions WHERE question_id = $1 AND reaction = 'dislike'`
	err = database.DB.QueryRowContext(ctx, dislikeQuery, questionID).Scan(&dislikes)
	if err != nil {
		return 0, 0, err
	}

	return likes, dislikes, nil
}

// LikeQuestionHandler handles liking a question
func LikeQuestionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	questionIDStr := r.URL.Query().Get("id")
	questionID, err := strconv.Atoi(questionIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Question ID: %v", err))
		return
	}

	err = ToggleQuestionReaction(ctx, userID, questionID, "like")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to like question: %v", err))
		return
	}

	likes, dislikes, _ := GetQuestionReactionCounts(ctx, questionID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// DislikeQuestionHandler handles disliking a question
func DislikeQuestionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := ctx.Value(userIDKey).(int)
	questionIDStr := r.URL.Query().Get("id")
	questionID, err := strconv.Atoi(questionIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Question ID: %v", err))
		return
	}

	err = ToggleQuestionReaction(ctx, userID, questionID, "dislike")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to dislike question: %v", err))
		return
	}

	likes, dislikes, _ := GetQuestionReactionCounts(ctx, questionID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// Add this function to question_handlers.go
func GetQuestionCommentsCount(questionID int) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM comments WHERE question_id = $1`
	err := database.DB.QueryRowContext(context.Background(), query, questionID).Scan(&count)
	return count, err
}

// GetAllQuestionsWithProfiles retrieves all questions with author profile pictures
func GetAllQuestionsWithProfiles() ([]models.Question, error) {
	query := `
        SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, 
               COALESCE(q.file_public_id, '') as file_public_id, q.target_group, 
               q.created_at, q.updated_at,
               u.username, COALESCE(u.profile_url, '') as profile_url,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count
        FROM questions q
        LEFT JOIN users u ON q.user_id = u.id
        LEFT JOIN (
            SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'like' GROUP BY question_id
        ) AS likes ON q.id = likes.question_id
        LEFT JOIN (
            SELECT question_id, COUNT(*) AS count FROM question_reactions WHERE reaction = 'dislike' GROUP BY question_id
        ) AS dislikes ON q.id = dislikes.question_id
        LEFT JOIN (
            SELECT question_id, COUNT(*) AS count FROM comments WHERE question_id IS NOT NULL GROUP BY question_id
        ) AS comments ON q.id = comments.question_id
        ORDER BY q.created_at DESC`
	rows, err := database.DB.QueryContext(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []models.Question
	for rows.Next() {
		var question models.Question
		var fileURL, filePublicID, content, profileURL sql.NullString
		err := rows.Scan(&question.ID, &question.UserID, &question.Title, &content,
			&question.Category, &fileURL, &filePublicID, &question.TargetGroup,
			&question.CreatedAt, &question.UpdatedAt, &question.Username, &profileURL,
			&question.LikesCount, &question.DislikesCount, &question.CommentsCount)
		if err != nil {
			log.Printf("Error scanning question: %v", err)
			continue
		}
		// Handle NULL fields
		if content.Valid {
			question.Content = &content.String
		}
		if fileURL.Valid {
			question.FileURL = &fileURL.String
		}
		if filePublicID.Valid {
			question.FilePublicID = &filePublicID.String
		}
		if profileURL.Valid {
			question.AuthorProfileURL = &profileURL.String
		}
		question.CreatedAtHuman = utils.TimeAgo(question.CreatedAt)
		question.CategoryArray = strings.Split(question.Category, ",")
		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return questions, nil
}

// GetQuestionByIDWithProfile retrieves a single question with author profile picture
func GetQuestionByIDWithProfile(id int) (*models.Question, error) {
	query := `
        SELECT q.id, q.user_id, q.title, q.content, q.category, q.file_url, 
               COALESCE(q.file_public_id, '') as file_public_id, q.target_group, 
               q.created_at, q.updated_at,
               u.username, COALESCE(u.profile_url, '') as profile_url,
               (SELECT COUNT(*) FROM question_reactions WHERE question_id = q.id AND reaction = 'like') as likes_count,
               (SELECT COUNT(*) FROM question_reactions WHERE question_id = q.id AND reaction = 'dislike') as dislikes_count,
               (SELECT COUNT(*) FROM comments WHERE question_id = q.id) as comments_count
        FROM questions q
        LEFT JOIN users u ON q.user_id = u.id
        WHERE q.id = $1`
	var question models.Question
	var fileURL, filePublicID, content, profileURL sql.NullString
	err := database.DB.QueryRowContext(context.Background(), query, id).Scan(
		&question.ID, &question.UserID, &question.Title, &content,
		&question.Category, &fileURL, &filePublicID, &question.TargetGroup,
		&question.CreatedAt, &question.UpdatedAt, &question.Username, &profileURL,
		&question.LikesCount, &question.DislikesCount, &question.CommentsCount)
	if err != nil {
		return nil, err
	}
	// Handle NULL fields
	if content.Valid {
		question.Content = &content.String
	}
	if fileURL.Valid {
		question.FileURL = &fileURL.String
	}
	if filePublicID.Valid {
		question.FilePublicID = &filePublicID.String
	}
	if profileURL.Valid {
		question.AuthorProfileURL = &profileURL.String
	}
	question.CreatedAtHuman = utils.TimeAgo(question.CreatedAt)
	question.CategoryArray = strings.Split(question.Category, ",")
	return &question, nil
}
