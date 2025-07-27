package comments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"evalhub/internal/models"
	"evalhub/internal/response"
	"evalhub/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockCommentService is a simplified mock implementation for testing
type mockCommentService struct {
	t *testing.T
}

// Implement the minimal required methods for testing
func (m *mockCommentService) GetTrendingComments(ctx context.Context, req *services.GetTrendingCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Mock implementation for testing
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 1, Content: "Test comment 1"},
			{ID: 2, Content: "Test comment 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

// Implement all required methods from services.CommentService
func (m *mockCommentService) CreateComment(ctx context.Context, req *services.CreateCommentRequest) (*models.Comment, error) {
	return nil, nil
}

func (m *mockCommentService) GetCommentByID(ctx context.Context, id int64, userID *int64) (*models.Comment, error) {
	return nil, nil
}

func (m *mockCommentService) UpdateComment(ctx context.Context, req *services.UpdateCommentRequest) (*models.Comment, error) {
	return nil, nil
}

func (m *mockCommentService) DeleteComment(ctx context.Context, commentID, userID int64) error {
	return nil
}

func (m *mockCommentService) GetCommentsByPostID(ctx context.Context, postID int64, userID *int64, page, pageSize int, sortBy, order string) (*models.PaginatedResponse[*models.Comment], error) {
	return nil, nil
}

func (m *mockCommentService) GetCommentsByUserID(ctx context.Context, userID int64, page, pageSize int) (*models.PaginatedResponse[*models.Comment], error) {
	return nil, nil
}

// Implement other required methods with minimal implementations
func (m *mockCommentService) ToggleCommentLike(ctx context.Context, commentID, userID int64) (*models.Comment, error) {
	return nil, nil
}

func (m *mockCommentService) GetCommentReplies(ctx context.Context, commentID int64, userID *int64, page, pageSize int) (*models.PaginatedResponse[*models.Comment], error) {
	return nil, nil
}

func (m *mockCommentService) GetCommentStats(ctx context.Context, commentID int64) (*CommentStatsResponse, error) {
	return &CommentStatsResponse{
		CommentID:   commentID,
		LikeCount:   0,
		ReplyCount:  0,
		ReportCount: 0,
	}, nil
}

func (m *mockCommentService) GetCommentAnalytics(ctx context.Context, req *services.GetCommentAnalyticsRequest) (*services.CommentAnalyticsResponse, error) {
	return nil, nil
}

func (m *mockCommentService) GetRecentComments(ctx context.Context, req *services.GetRecentCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return nil, nil
}

func (m *mockCommentService) GetCommentsByDocument(ctx context.Context, req *services.GetCommentsByDocumentRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 1, Content: "Test document comment 1"},
			{ID: 2, Content: "Test document comment 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

func (m *mockCommentService) GetCommentsByPost(ctx context.Context, req *services.GetCommentsByPostRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 1, Content: "Test post comment 1"},
			{ID: 2, Content: "Test post comment 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

func (m *mockCommentService) GetCommentsByQuestion(ctx context.Context, req *services.GetCommentsByQuestionRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 3, Content: "Test question comment 1"},
			{ID: 4, Content: "Test question comment 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

func (m *mockCommentService) GetCommentsByUser(ctx context.Context, req *services.GetCommentsByUserRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 5, Content: "Test user comment 1"},
			{ID: 6, Content: "Test user comment 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

func (m *mockCommentService) GetModerationQueue(ctx context.Context, req *services.GetModerationQueueRequest) (*models.PaginatedResponse[*models.Comment], error) {
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{
			{ID: 7, Content: "Comment awaiting moderation 1"},
			{ID: 8, Content: "Comment awaiting moderation 2"},
		},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   2,
			TotalPages:   1,
		},
	}, nil
}

func (m *mockCommentService) ModerateComment(ctx context.Context, req *services.ModerateContentRequest) error {
	// In a real test, you might want to track that this was called
	// or update some internal state of the mock
	return nil
}

func (m *mockCommentService) ReactToComment(ctx context.Context, req *services.ReactToCommentRequest) error {
	// In a real test, you might want to track that this was called
	// or update some internal state of the mock
	return nil
}

func (m *mockCommentService) RemoveCommentReaction(ctx context.Context, commentID, userID int64) error {
	// In a real test, you might want to track that this was called
	// or update some internal state of the mock
	return nil
}

func (m *mockCommentService) ReportComment(ctx context.Context, req *services.ReportContentRequest) error {
	// In a real test, you might want to track that this was called
	// or update some internal state of the mock
	return nil
}

func (m *mockCommentService) SearchComments(ctx context.Context, req *services.SearchCommentsRequest) (*models.PaginatedResponse[*models.Comment], error) {
	// Return a mock paginated response with empty results
	return &models.PaginatedResponse[*models.Comment]{
		Data: []*models.Comment{},
		Pagination: models.PaginationMeta{
			CurrentPage:  1,
			ItemsPerPage: 10,
			TotalItems:   0,
			TotalPages:   1,
		},
	}, nil
}

// CommentStatsResponse is a mock implementation for testing
type CommentStatsResponse struct {
	CommentID   int64 `json:"comment_id"`
	LikeCount   int   `json:"like_count"`
	ReplyCount  int   `json:"reply_count"`
	ReportCount int   `json:"report_count"`
}

// mockServiceCollection implements the minimal interface needed for testing
type mockServiceCollection struct {
	commentService *mockCommentService
}

// GetCommentService returns the mock comment service
func (m *mockServiceCollection) GetCommentService() services.CommentService {
	return nil
}

func TestCommentController_GetTrendingComments(t *testing.T) {
	// Create a test response builder with default config
	responseBuilder := response.NewBuilder(
		&response.Config{
			PrettyJSON:         false, // Compact JSON in tests
			IncludeRequestID:   true,  // Include request ID for tracing
			IncludeTimestamp:   true,  // Include timestamp in responses
			IncludeVersion:     true,  // Include API version
			APIVersion:         "v1",  // Set API version
			IncludeErrorStack:  true,  // Show error stack traces in test
			MaskInternalErrors: false, // Show full error details in test
			EnableCompression:  false, // Disable compression in tests
			CacheHeaders:       false, // Disable cache headers in tests
			DefaultTemplate:    "",    // No default template for API tests
			ErrorTemplate:      "",    // No error template for API tests
		},
		nil, // logger is nil for tests
	)

	// Create mock comment service
	// mockSvc := &mockCommentService{t: t}

	// Create mock service (no longer need mockServiceCollection)

	// Create controller with test dependencies
	paginationConfig := &response.PaginationConfig{
		DefaultPageSize: 10,
		MaxPageSize:     100,
		PageParam:       "page",
		SizeParam:       "size",
		SortParam:       "sort",
		OrderParam:      "order",
	}
	_ = response.NewPaginationParser(paginationConfig) // We don't need the parser, just the config

	controller := &CommentController{
		serviceCollection: &services.ServiceCollection{
			// We only need to set the CommentService field that the controller will use
			CommentService: nil,
		},
		responseBuilder:   responseBuilder,
		paginationParser:  response.NewPaginationParser(paginationConfig),
		paginationBuilder: response.NewPaginationBuilder(paginationConfig),
		logger:            zap.NewNop(),
	}

	t.Run("successful request with default time range", func(t *testing.T) {
		// Create a request with query parameters
		req, err := http.NewRequest("GET", "/api/v1/comments/trending?page=1&page_size=10", nil)
		require.NoError(t, err)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Call the handler
		handler := http.HandlerFunc(controller.GetTrendingComments)
		handler.ServeHTTP(rr, req)

		// Check the status code
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse the response
		var responseBody map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseBody)
		require.NoError(t, err)

		// Verify the response structure
		assert.Contains(t, responseBody, "data")
		assert.Contains(t, responseBody, "meta")

		meta, ok := responseBody["meta"].(map[string]interface{})
		require.True(t, ok)

		pagination, ok := meta["pagination"].(map[string]interface{})
		require.True(t, ok)

		// Verify pagination metadata
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(10), pagination["page_size"])
		assert.Equal(t, float64(1), pagination["total"])
		assert.Equal(t, float64(1), pagination["total_pages"])
		assert.Equal(t, false, pagination["has_next"])
		assert.Equal(t, false, pagination["has_prev"])
	})

	t.Run("invalid time range format", func(t *testing.T) {
		// Create a request with invalid time range format
		req, err := http.NewRequest("GET", "/api/v1/comments/trending?range=invalid", nil)
		require.NoError(t, err)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Call the handler
		handler := http.HandlerFunc(controller.GetTrendingComments)
		handler.ServeHTTP(rr, req)

		// Check the status code
		assert.Equal(t, http.StatusBadRequest, rr.Code)

		// Parse the response
		var responseBody map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseBody)
		require.NoError(t, err)

		// Verify the error response
		errObj, ok := responseBody["error"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "VALIDATION_ERROR", errObj["type"])
	})
}
