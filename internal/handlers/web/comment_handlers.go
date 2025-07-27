// internal/handlers/web/comment_handlers.go
package web

import (
	"encoding/json"
	"evalhub/internal/models"
	"evalhub/internal/services"
	"evalhub/internal/utils"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"		
)

// CommentHandler handles HTTP requests for comments
type CommentHandler struct {
	commentService services.CommentService
}

// NewCommentHandler creates a new CommentHandler
func NewCommentHandler(commentService services.CommentService) *CommentHandler {
	return &CommentHandler{commentService: commentService}
}

// CreateCommentRequest represents the request body for creating a comment
type CreateCommentRequest struct {
	PostID  int    `json:"post_id"`
	Content string `json:"content"`
}

// UpdateCommentRequest represents the request body for updating a comment
type UpdateCommentRequest struct {
	Content string `json:"content"`
}

// CreateComment handles POST /api/comments
func (h *CommentHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(int64)
	if !ok || userID <= 0 {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert request to service layer request
	serviceReq := &services.CreateCommentRequest{
		UserID:  userID,
		Content: req.Content,
	}
	if req.PostID > 0 {
		postID := int64(req.PostID)
		serviceReq.PostID = &postID
	}

	// Call service layer
	comment, err := h.commentService.CreateComment(ctx, serviceReq)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case services.IsValidationError(err):
			status = http.StatusBadRequest
		case services.IsAuthenticationError(err) || services.IsErrorType(err, "UNAUTHORIZED"):
			status = http.StatusUnauthorized
		case services.IsAuthorizationError(err) || services.IsErrorType(err, "FORBIDDEN"):
			status = http.StatusForbidden
		case services.IsNotFoundError(err):
			status = http.StatusNotFound
		case services.IsErrorType(err, "CONFLICT"):
			status = http.StatusConflict
		}
		utils.RespondWithError(w, status, err.Error())
		return
	}

	utils.RespondWithJSON(w, http.StatusCreated, comment)
}

// UpdateComment handles PUT /api/comments/{id}
func (h *CommentHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(int64)
	if !ok || userID <= 0 {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Extract comment ID from URL path
	vars := mux.Vars(r)
	commentID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil || commentID <= 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}

	var req UpdateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert request to service layer request
	serviceReq := &services.UpdateCommentRequest{
		CommentID: commentID,
		UserID:    userID,
		Content:   req.Content,
	}

	// Call service layer
	comment, err := h.commentService.UpdateComment(ctx, serviceReq)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case services.IsValidationError(err):
			status = http.StatusBadRequest
		case services.IsAuthenticationError(err) || services.IsErrorType(err, "UNAUTHORIZED"):
			status = http.StatusUnauthorized
		case services.IsAuthorizationError(err) || services.IsErrorType(err, "FORBIDDEN"):
			status = http.StatusForbidden
		case services.IsNotFoundError(err):
			status = http.StatusNotFound
		case services.IsErrorType(err, "CONFLICT"):
			status = http.StatusConflict
		}
		utils.RespondWithError(w, status, err.Error())
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, comment)
}

// DeleteComment handles DELETE /api/comments/{id}
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := ctx.Value("userID").(int64)
	if !ok || userID <= 0 {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Extract comment ID from URL path
	vars := mux.Vars(r)
	commentID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil || commentID <= 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}

	err = h.commentService.DeleteComment(ctx, commentID, userID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case services.IsValidationError(err):
			status = http.StatusBadRequest
		case services.IsAuthenticationError(err) || services.IsErrorType(err, "UNAUTHORIZED"):
			status = http.StatusUnauthorized
		case services.IsAuthorizationError(err) || services.IsErrorType(err, "FORBIDDEN"):
			status = http.StatusForbidden
		case services.IsNotFoundError(err):
			status = http.StatusNotFound
		case services.IsErrorType(err, "CONFLICT"):
			status = http.StatusConflict
		}
		utils.RespondWithError(w, status, err.Error())
		return
	}

	utils.RespondWithJSON(w, http.StatusNoContent, nil)
}

// getPaginationParams extracts pagination parameters from the request
func getPaginationParams(r *http.Request) (int, int) {
	// Default values
	page := 1
	limit := 20

	// Get page from query params
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Get limit from query params
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	return page, limit
}

// ListCommentsByPost handles GET /api/posts/{post_id}/comments
func (h *CommentHandler) ListCommentsByPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract post ID from URL
	postID, err := strconv.ParseInt(mux.Vars(r)["post_id"], 10, 64)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid post ID")
		return
	}

	// Get pagination parameters
	page, limit := getPaginationParams(r)
	offset := (page - 1) * limit

	// Call service
	serviceReq := &services.GetCommentsByPostRequest{
		PostID: postID,
		Pagination: models.PaginationParams{
			Limit:  limit,
			Offset: offset,
		},
	}

	response, err := h.commentService.GetCommentsByPost(ctx, serviceReq)
	if err != nil {
		status := http.StatusInternalServerError
		if services.IsValidationError(err) {
			status = http.StatusBadRequest
		} else if services.IsNotFoundError(err) {
			status = http.StatusNotFound
		}
		utils.RespondWithError(w, status, err.Error())
		return
	}

	// Convert to paginated response
	pagination := models.PaginationMeta{
		CurrentPage:  page,
		ItemsPerPage: limit,
		TotalItems:   response.Pagination.TotalItems,
		TotalPages:   (int(response.Pagination.TotalItems) + limit - 1) / limit,
		HasNext:      (page * limit) < int(response.Pagination.TotalItems),
		HasPrev:      page > 1,
	}

	// Convert []*models.Comment to []models.Comment
	comments := make([]models.Comment, len(response.Data))
	for i, comment := range response.Data {
		comments[i] = *comment
	}

	utils.RespondWithJSON(w, http.StatusOK, models.PaginatedResponse[models.Comment]{
		Data:       comments,
		Pagination: pagination,
	})
}
