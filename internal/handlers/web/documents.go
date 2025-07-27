// file: internal/handlers/web/documents.go
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
	"strconv"
	"strings"
	"time"
)

// UploadDocumentHandler handles document uploads
func UploadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	userID := r.Context().Value(userIDKey).(int)

	// Create context for the request with timeout
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()

	err := r.ParseMultipartForm(25 << 20) // 25MB max file size
	if err != nil {
		log.Printf("Failed to parse multipart form: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to parse form data: %v", err))
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	tags := r.FormValue("tags")

	if title == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("document title is required"))
		return
	}

	file, header, err := r.FormFile("document")
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("document file is required"))
		return
	}
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
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("document validation failed: %v", err))
		return
	}

	// Generate a unique folder path for the document
	uploadFolder := fmt.Sprintf("evalhub/documents/%d", time.Now().UnixNano())

	// Upload to Cloudinary
	uploadResult, err := cloudinary.UploadFile(ctx, header, uploadFolder)
	if err != nil {
		log.Printf("Failed to upload document to Cloudinary: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to upload document: %v", err))
		return
	}

	fileURL := uploadResult.URL
	filePublicID := uploadResult.PublicID
	fileType := getFileType(header.Filename)

	// Create document in database
	docID, err := CreateDocument(userID, title, description, fileURL, filePublicID, fileType, tags)
	if err != nil {
		log.Printf("Error creating document in database: %v", err)
		// Clean up uploaded file if document creation failed
		if filePublicID != "" {
			cloudinary, _ := utils.GetCloudinaryService()
			if delErr := cloudinary.DeleteFile(ctx, filePublicID); delErr != nil {
				log.Printf("Warning: Failed to delete orphaned document file: %v", delErr)
			}
		}
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to save document: %v", err))
		return
	}

	log.Printf("Document successfully uploaded with ID: %d, URL: %s", docID, fileURL)

	// Return to the referring page or dashboard
	referer := r.Header.Get("Referer")
	if referer != "" {
		http.Redirect(w, r, referer, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

// GetAllDocumentsWithProfiles retrieves all documents with uploader profile information
func GetAllDocumentsWithProfiles(tag string) ([]models.Document, error) {
	var rows *sql.Rows
	var err error

	// Improved database query with more debugging
	log.Printf("Running GetAllDocumentsWithProfiles with tag: %s", tag)

	baseQuery := `
        SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
               d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
               d.download_count, d.created_at, d.updated_at,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM documents d
        LEFT JOIN users u ON d.user_id = u.id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_reactions WHERE reaction = 'like' GROUP BY document_id
        ) AS likes ON d.id = likes.document_id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_reactions WHERE reaction = 'dislike' GROUP BY document_id
        ) AS dislikes ON d.id = dislikes.document_id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_comments GROUP BY document_id
        ) AS comments ON d.id = comments.document_id`

	ctx := context.Background()
	if tag != "" {
		rows, err = database.DB.QueryContext(ctx, baseQuery+" WHERE d.tags LIKE $1 ORDER BY d.created_at DESC", "%"+tag+"%")
	} else {
		rows, err = database.DB.QueryContext(ctx, baseQuery + " ORDER BY d.created_at DESC")
	}

	if err != nil {
		log.Printf("Database error in GetAllDocumentsWithProfiles: %v", err)
		return nil, err
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		var fileURL, filePublicID, tags, profileURL sql.NullString
		var isPublic sql.NullBool

		err := rows.Scan(&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
			&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
			&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
			&doc.Likes, &doc.Dislikes, &doc.CommentsCount,
			&doc.Username, &profileURL)

		if err != nil {
			log.Printf("Error scanning document row: %v", err)
			continue
		}

		// Handle NULL fields
		if fileURL.Valid {
			doc.FileURL = fileURL.String
		}
		if filePublicID.Valid {
			doc.FilePublicID = filePublicID.String
		}
		if tags.Valid {
			doc.Tags = tags.String
			doc.TagsArray = strings.Split(tags.String, ",")
		}
		if isPublic.Valid {
			doc.IsPublic = isPublic.Bool
		}
		if profileURL.Valid {
			doc.AuthorProfileURL = profileURL.String
		}

		doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
		doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating through document rows: %v", err)
		return nil, err
	}

	log.Printf("GetAllDocumentsWithProfiles found %d documents", len(documents))
	return documents, nil
}

// GetDocumentsByType retrieves documents filtered by file type
func GetDocumentsByType(fileType string) ([]models.Document, error) {
	baseQuery := `
        SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
               d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
               d.download_count, d.created_at, d.updated_at,
               COALESCE(likes.count, 0) AS likes,
               COALESCE(dislikes.count, 0) AS dislikes,
               COALESCE(comments.count, 0) AS comments_count,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM documents d
        LEFT JOIN users u ON d.user_id = u.id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_reactions WHERE reaction = 'like' GROUP BY document_id
        ) AS likes ON d.id = likes.document_id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_reactions WHERE reaction = 'dislike' GROUP BY document_id
        ) AS dislikes ON d.id = dislikes.document_id
        LEFT JOIN (
            SELECT document_id, COUNT(*) AS count FROM document_comments GROUP BY document_id
        ) AS comments ON d.id = comments.document_id
        WHERE d.file_type = $1
        ORDER BY d.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), baseQuery, fileType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		var fileURL, filePublicID, tags, profileURL sql.NullString
		var isPublic sql.NullBool

		err := rows.Scan(&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
			&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
			&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
			&doc.Likes, &doc.Dislikes, &doc.CommentsCount,
			&doc.Username, &profileURL)

		if err != nil {
			continue
		}

		// Handle NULL fields
		if fileURL.Valid {
			doc.FileURL = fileURL.String
		}
		if filePublicID.Valid {
			doc.FilePublicID = filePublicID.String
		}
		if tags.Valid {
			doc.Tags = tags.String
			doc.TagsArray = strings.Split(tags.String, ",")
		}
		if isPublic.Valid {
			doc.IsPublic = isPublic.Bool
		}
		if profileURL.Valid {
			doc.AuthorProfileURL = profileURL.String
		}

		doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
		doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return documents, nil
}

// CreateDocument creates a new document in the database
func CreateDocument(userID int, title, description, fileURL, filePublicID, fileType, tags string) (int, error) {
	var documentID int
	query := `
			INSERT INTO documents (
				user_id, title, description, file_url, file_public_id, 
				file_type, tags, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id`

	now := time.Now()
	err := database.DB.QueryRowContext(
		context.Background(),
		query, userID, title, description, fileURL, filePublicID,
		fileType, tags, now, now,
	).Scan(&documentID)

	return documentID, err
}

// GetDocumentByID retrieves a document by its ID
func GetDocumentByID(id int) (*models.Document, error) {
	query := `
			SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
				   d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
				   d.download_count, d.created_at, d.updated_at,
				   u.username, COALESCE(u.profile_url, '') as profile_url,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'like') as likes,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'dislike') as dislikes,
				   (SELECT COUNT(*) FROM document_comments WHERE document_id = d.id) as comments_count
			FROM documents d
			LEFT JOIN users u ON d.user_id = u.id
			WHERE d.id = $1`

	var doc models.Document
	var fileURL, filePublicID, tags, profileURL sql.NullString
	var isPublic sql.NullBool

	err := database.DB.QueryRowContext(context.Background(), query, id).Scan(
		&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
		&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
		&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
		&doc.Username, &profileURL, &doc.Likes, &doc.Dislikes, &doc.CommentsCount)

	if err != nil {
		return nil, err
	}

	// Handle NULL fields
	if fileURL.Valid {
		doc.FileURL = fileURL.String
	}
	if filePublicID.Valid {
		doc.FilePublicID = filePublicID.String
	}
	if tags.Valid {
		doc.Tags = tags.String
		doc.TagsArray = strings.Split(tags.String, ",")
	}
	if isPublic.Valid {
		doc.IsPublic = isPublic.Bool
	}
	if profileURL.Valid {
		doc.AuthorProfileURL = profileURL.String
	}

	doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
	doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)

	return &doc, nil
}

// GetDocumentsByUserID retrieves all documents uploaded by a specific user
func GetDocumentsByUserID(userID int) ([]models.Document, error) {
	query := `
			SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
				   d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
				   d.download_count, d.created_at, d.updated_at,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'like') as likes,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'dislike') as dislikes,
				   (SELECT COUNT(*) FROM document_comments WHERE document_id = d.id) as comments_count,
				   u.username, COALESCE(u.profile_url, '') as profile_url
			FROM documents d
			LEFT JOIN users u ON d.user_id = u.id
			WHERE d.user_id = $1
			ORDER BY d.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		var fileURL, filePublicID, tags, profileURL sql.NullString
		var isPublic sql.NullBool

		err := rows.Scan(&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
			&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
			&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
			&doc.Likes, &doc.Dislikes, &doc.CommentsCount,
			&doc.Username, &profileURL)

		if err != nil {
			return nil, err
		}

		// Handle NULL fields
		if fileURL.Valid {
			doc.FileURL = fileURL.String
		}
		if filePublicID.Valid {
			doc.FilePublicID = filePublicID.String
		}
		if tags.Valid {
			doc.Tags = tags.String
			doc.TagsArray = strings.Split(tags.String, ",")
		}
		if isPublic.Valid {
			doc.IsPublic = isPublic.Bool
		}
		if profileURL.Valid {
			doc.AuthorProfileURL = profileURL.String
		}

		doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
		doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)
		doc.IsOwner = true // User's own documents

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return documents, nil
}

// DeleteDocument removes a document from the database and storage
func DeleteDocument(documentID int) error {
	// First get the document to retrieve file public ID
	doc, err := GetDocumentByID(documentID)
	if err != nil {
		return err
	}

	// Delete from database
	query := `DELETE FROM documents WHERE id = $1`
	_, err = database.DB.ExecContext(context.Background(), query, documentID)
	if err != nil {
		return err
	}

	// Delete from Cloudinary
	if doc.FilePublicID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cloudinary, err := utils.GetCloudinaryService()
		if err != nil {
			log.Printf("Warning: Failed to initialize Cloudinary service for file cleanup: %v", err)
			return nil // Continue even if we can't clean up the file
		}

		if delErr := cloudinary.DeleteFile(ctx, doc.FilePublicID); delErr != nil {
			log.Printf("Warning: Failed to delete file after document deletion: %v", delErr)
		}
	}

	return nil
}

// ViewDocumentHandler handles displaying a document
func ViewDocumentHandler(w http.ResponseWriter, r *http.Request) {
	documentID := r.URL.Query().Get("id")
	id, err := strconv.Atoi(documentID)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid document ID: %s", r.URL.Path))
		return
	}

	document, err := GetDocumentByID(id)
	if err != nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("document not found: %s", r.URL.Path))
		return
	}

	comments, err := GetCommentsByDocumentID(id)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving comments: %v", err))
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

	// Set ownership flag for the document
	document.IsOwner = (document.UserID == currentUserID)

	data := map[string]interface{}{
		"Title":           document.Title + " - Document",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"Document":        document,
		"Comments":        comments,
		"IsAuthenticated": isAuthenticated,
		"UserID":          userID,
	}

	err = templates.ExecuteTemplate(w, "view-document", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}

	// Increment download count (viewed = downloaded in this case)
	go IncrementDocumentDownloadCount(id)
}

// IncrementDocumentDownloadCount increases the download count for a document
func IncrementDocumentDownloadCount(documentID int) {
	query := `UPDATE documents SET download_count = download_count + 1 WHERE id = $1`
	_, err := database.DB.ExecContext(context.Background(), query, documentID)
	if err != nil {
		log.Printf("Error incrementing document download count: %v", err)
	}
}

// DeleteDocumentHandler handles document deletion
func DeleteDocumentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	documentID := r.URL.Query().Get("id")
	id, err := strconv.Atoi(documentID)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid document ID: %s", documentID))
		return
	}

	// Check ownership
	document, err := GetDocumentByID(id)
	if err != nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("document not found"))
		return
	}

	userID := r.Context().Value(userIDKey).(int)
	if document.UserID != userID {
		RenderErrorPage(w, http.StatusForbidden, fmt.Errorf("you can only delete your own documents"))
		return
	}

	err = DeleteDocument(id)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to delete document: %v", err))
		return
	}

	// Redirect to referring page or dashboard
	referer := r.Header.Get("Referer")
	if referer != "" && !strings.Contains(referer, fmt.Sprintf("view-document?id=%d", id)) {
		http.Redirect(w, r, referer, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

// LikeDocumentHandler handles document liking
func LikeDocumentHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	documentIDStr := r.URL.Query().Get("id")
	documentID, err := strconv.Atoi(documentIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Document ID: %v", err))
		return
	}

	err = ToggleDocumentReaction(userID, documentID, "like")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to like document: %v", err))
		return
	}

	likes, dislikes, _ := GetDocumentReactionCounts(documentID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// DislikeDocumentHandler handles document disliking
func DislikeDocumentHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	documentIDStr := r.URL.Query().Get("id")
	documentID, err := strconv.Atoi(documentIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid Document ID: %v", err))
		return
	}

	err = ToggleDocumentReaction(userID, documentID, "dislike")
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to dislike document: %v", err))
		return
	}

	likes, dislikes, _ := GetDocumentReactionCounts(documentID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"likes": %d, "dislikes": %d}`, likes, dislikes)))
}

// ToggleDocumentReaction toggles a like/dislike reaction on a document
func ToggleDocumentReaction(userID, documentID int, reactionType string) error {
	// Check if reaction exists
	var existingReaction string
	query := `SELECT reaction FROM document_reactions WHERE user_id = $1 AND document_id = $2`
	err := database.DB.QueryRowContext(context.Background(), query, userID, documentID).Scan(&existingReaction)

	if err == sql.ErrNoRows {
		// No reaction exists, create one
		insertQuery := `INSERT INTO document_reactions (user_id, document_id, reaction) VALUES ($1, $2, $3)`
		_, err = database.DB.ExecContext(context.Background(), insertQuery, userID, documentID, reactionType)
		return err
	}

	if err != nil {
		return err
	}

	// If the same reaction type, delete it (toggle off)
	if existingReaction == reactionType {
		deleteQuery := `DELETE FROM document_reactions WHERE user_id = $1 AND document_id = $2`
		_, err = database.DB.ExecContext(context.Background(), deleteQuery, userID, documentID)
		return err
	}

	// Different reaction type, update it
	updateQuery := `UPDATE document_reactions SET reaction = $1 WHERE user_id = $2 AND document_id = $3`
	_, err = database.DB.ExecContext(context.Background(), updateQuery, reactionType, userID, documentID)
	return err
}

// GetDocumentReactionCounts returns the number of likes and dislikes for a document
func GetDocumentReactionCounts(documentID int) (likes int, dislikes int, err error) {
	likesQuery := `SELECT COUNT(*) FROM document_reactions WHERE document_id = $1 AND reaction = 'like'`
	dislikesQuery := `SELECT COUNT(*) FROM document_reactions WHERE document_id = $1 AND reaction = 'dislike'`

	err = database.DB.QueryRowContext(context.Background(), likesQuery, documentID).Scan(&likes)
	if err != nil {
		return 0, 0, err
	}

	err = database.DB.QueryRowContext(context.Background(), dislikesQuery, documentID).Scan(&dislikes)
	if err != nil {
		return likes, 0, err
	}

	return likes, dislikes, nil
}

// GetCommentsByDocumentID retrieves comments for a document
func GetCommentsByDocumentID(documentID int) ([]models.DocumentComment, error) {
	query := `
			SELECT c.id, c.document_id, c.user_id, c.content, c.created_at,
				   u.username, COALESCE(u.profile_url, '') as profile_url
			FROM document_comments c
			LEFT JOIN users u ON c.user_id = u.id
			WHERE c.document_id = $1
			ORDER BY c.created_at ASC`

	rows, err := database.DB.QueryContext(context.Background(), query, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.DocumentComment
	for rows.Next() {
		var comment models.DocumentComment
		var profileURL sql.NullString

		err := rows.Scan(&comment.ID, &comment.DocumentID, &comment.UserID,
			&comment.Content, &comment.CreatedAt,
			&comment.Username, &profileURL)

		if err != nil {
			log.Printf("Error scanning comment: %v", err)
			continue
		}

		if profileURL.Valid {
			comment.AuthorProfileURL = profileURL.String
		}

		comment.CreatedAtHuman = utils.TimeAgo(comment.CreatedAt)
		comments = append(comments, comment)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return comments, nil
}

// CreateDocumentCommentHandler handles creating comments on documents
func CreateDocumentCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RenderErrorPage(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	userID := r.Context().Value(userIDKey).(int)
	content := r.FormValue("comment")
	documentIDStr := r.FormValue("document_id")

	if content == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("comment content is required"))
		return
	}

	if documentIDStr == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("document_id is required"))
		return
	}

	documentID, err := strconv.Atoi(documentIDStr)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid document ID"))
		return
	}

	username := getUsername(userID)

	err = CreateCommentForDocument(userID, documentID, username, content)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("failed to create comment: %v", err))
		return
	}

	// Comment created successfully - update user stats
	go UpdateUserStats(context.Background(), userID)

	// Redirect back to document
	http.Redirect(w, r, fmt.Sprintf("/view-document?id=%d", documentID), http.StatusSeeOther)
}

// CreateCommentForDocument creates a comment on a document
func CreateCommentForDocument(userID, documentID int, username, content string) error {
	query := `INSERT INTO document_comments (user_id, document_id, username, content) VALUES ($1, $2, $3, $4)`
	_, err := database.DB.ExecContext(context.Background(), query, userID, documentID, username, content)
	return err
}

// Helper function to determine file type from filename
func getFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "word"
	case ".xls", ".xlsx":
		return "excel"
	case ".ppt", ".pptx":
		return "powerpoint"
	case ".txt":
		return "text"
	case ".jpg", ".jpeg", ".png", ".gif":
		return "image"
	case ".mp4", ".mov", ".avi":
		return "video"
	case ".mp3", ".wav":
		return "audio"
	case ".zip", ".rar", ".7z":
		return "archive"
	default:
		return "other"
	}
}

// SearchDocuments searches documents by title, description, or tags
func SearchDocuments(query string) ([]models.Document, error) {
	searchQuery := `
			SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
				   d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
				   d.download_count, d.created_at, d.updated_at,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'like') as likes,
				   (SELECT COUNT(*) FROM document_reactions WHERE document_id = d.id AND reaction = 'dislike') as dislikes,
				   (SELECT COUNT(*) FROM document_comments WHERE document_id = d.id) as comments_count,
				   u.username, COALESCE(u.profile_url, '') as profile_url
			FROM documents d
			LEFT JOIN users u ON d.user_id = u.id
			WHERE d.title ILIKE $1 OR d.description ILIKE $1 OR d.tags ILIKE $1
			ORDER BY d.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), searchQuery, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		var fileURL, filePublicID, tags, profileURL sql.NullString
		var isPublic sql.NullBool

		err := rows.Scan(&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
			&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
			&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
			&doc.Likes, &doc.Dislikes, &doc.CommentsCount,
			&doc.Username, &profileURL)

		if err != nil {
			return nil, err
		}

		// Handle NULL fields
		if fileURL.Valid {
			doc.FileURL = fileURL.String
		}
		if filePublicID.Valid {
			doc.FilePublicID = filePublicID.String
		}
		if tags.Valid {
			doc.Tags = tags.String
			doc.TagsArray = strings.Split(tags.String, ",")
		}
		if isPublic.Valid {
			doc.IsPublic = isPublic.Bool
		}
		if profileURL.Valid {
			doc.AuthorProfileURL = profileURL.String
		}

		doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
		doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return documents, nil
}

// GetDocumentsHandler displays all documents
func GetDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	// Get tag and type filters
	tag := r.URL.Query().Get("tag")
	fileType := r.URL.Query().Get("type")
	searchQuery := r.URL.Query().Get("search")

	// Log debugging information
	log.Printf("GetDocumentsHandler called with tag=%s, type=%s, search=%s", tag, fileType, searchQuery)

	// Get documents with appropriate filters
	var documents []models.Document
	var err error

	if searchQuery != "" {
		// Search documents by query
		documents, err = SearchDocuments(searchQuery)
	} else if fileType != "" {
		// Filter documents by file type
		documents, err = GetDocumentsByType(fileType)
	} else {
		// Get all documents with optional tag filter
		documents, err = GetAllDocumentsWithProfiles(tag)
	}

	if err != nil {
		log.Printf("Error retrieving documents: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving documents: %v", err))
		return
	}

	// Log document count for debugging
	log.Printf("Retrieved %d documents", len(documents))

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

	// Mark ownership for documents
	for i := range documents {
		documents[i].IsOwner = (documents[i].UserID == currentUserID)
	}

	// Get document stats
	totalDocuments, totalDownloads, _ := GetDocumentStats()

	data := map[string]interface{}{
		"Title":           "Documents Repository",
		"IsLoggedIn":      isAuthenticated,
		"Username":        username,
		"Documents":       documents,
		"IsAuthenticated": isAuthenticated,
		"UserID":          userID,
		"CurrentTag":      tag,
		"FileType":        fileType,
		"SearchQuery":     searchQuery,
		"TotalDocuments":  totalDocuments,
		"TotalDownloads":  totalDownloads,
	}

	err = templates.ExecuteTemplate(w, "documents", data)
	if err != nil {
		log.Printf("Error rendering documents template: %v", err)
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		return
	}
}

// GetDocumentsJSONHandler returns documents as JSON for AJAX requests
func GetDocumentsJSONHandler(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 5 // Default limit

	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	tag := r.URL.Query().Get("tag")

	// Add debugging log
	log.Printf("GetDocumentsJSONHandler called with limit=%d, tag=%s", limit, tag)

	// Get recent documents
	documents, err := GetRecentDocuments(limit, tag)
	if err != nil {
		log.Printf("Error getting recent documents: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Failed to retrieve documents",
		})
		return
	}

	// Log document count
	log.Printf("Retrieved %d recent documents", len(documents))

	// Get total documents and downloads
	totalDocuments, totalDownloads, err := GetDocumentStats()
	if err != nil {
		log.Printf("Error retrieving document stats: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"documents":       documents,
		"total_documents": totalDocuments,
		"total_downloads": totalDownloads,
	})
}

// GetRecentDocuments retrieves a limited number of recent documents
func GetRecentDocuments(limit int, tag string) ([]models.Document, error) {
	var query string
	var args []interface{}

	baseQuery := `
        SELECT d.id, d.user_id, d.title, d.description, d.file_url, 
               d.file_public_id, d.file_type, d.version, d.tags, d.is_public,
               d.download_count, d.created_at, d.updated_at,
               u.username, COALESCE(u.profile_url, '') as profile_url
        FROM documents d
        LEFT JOIN users u ON d.user_id = u.id`

	if tag != "" {
		query = baseQuery + " WHERE d.tags LIKE $1 ORDER BY d.created_at DESC LIMIT $2"
		args = []interface{}{"%" + tag + "%", limit}
	} else {
		query = baseQuery + " ORDER BY d.created_at DESC LIMIT $1"
		args = []interface{}{limit}
	}

	rows, err := database.DB.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		var fileURL, filePublicID, tags, profileURL sql.NullString
		var isPublic sql.NullBool

		err := rows.Scan(&doc.ID, &doc.UserID, &doc.Title, &doc.Description,
			&fileURL, &filePublicID, &doc.FileType, &doc.Version, &tags, &isPublic,
			&doc.DownloadCount, &doc.CreatedAt, &doc.UpdatedAt,
			&doc.Username, &profileURL)

		if err != nil {
			return nil, err
		}

		// Handle NULL fields
		if fileURL.Valid {
			doc.FileURL = fileURL.String
		}
		if filePublicID.Valid {
			doc.FilePublicID = filePublicID.String
		}
		if tags.Valid {
			doc.Tags = tags.String
			doc.TagsArray = strings.Split(tags.String, ",")
		}
		if isPublic.Valid {
			doc.IsPublic = isPublic.Bool
		}
		if profileURL.Valid {
			doc.AuthorProfileURL = profileURL.String
		}

		doc.CreatedAtHuman = utils.TimeAgo(doc.CreatedAt)
		doc.UpdatedAtHuman = utils.TimeAgo(doc.UpdatedAt)

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return documents, nil
}

// GetDocumentStats returns total document count and download count
func GetDocumentStats() (int, int, error) {
	var totalDocuments, totalDownloads int

	err := database.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM documents").Scan(&totalDocuments)
	if err != nil {
		return 0, 0, err
	}

	err = database.DB.QueryRowContext(context.Background(), "SELECT COALESCE(SUM(download_count), 0) FROM documents").Scan(&totalDownloads)
	if err != nil {
		return totalDocuments, 0, err
	}

	return totalDocuments, totalDownloads, nil
}
