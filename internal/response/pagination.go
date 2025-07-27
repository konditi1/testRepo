// File: internal/response/pagination.go
package response

import (
	"context"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// ===============================
// PAGINATION CONFIGURATION
// ===============================

// PaginationConfig holds pagination configuration
type PaginationConfig struct {
	DefaultPageSize int    `json:"default_page_size"`
	MaxPageSize     int    `json:"max_page_size"`
	PageParam       string `json:"page_param"`
	SizeParam       string `json:"size_param"`
	SortParam       string `json:"sort_param"`
	OrderParam      string `json:"order_param"`
}

// DefaultPaginationConfig returns default pagination configuration
func DefaultPaginationConfig() *PaginationConfig {
	return &PaginationConfig{
		DefaultPageSize: 20,
		MaxPageSize:     100,
		PageParam:       "page",
		SizeParam:       "page_size",
		SortParam:       "sort",
		OrderParam:      "order",
	}
}

// ===============================
// PAGINATION TYPES
// ===============================

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort,omitempty"`
	Order    string `json:"order,omitempty"`
	Offset   int    `json:"offset"`
}

// PaginationResult represents the result of pagination
type PaginationResult struct {
	Items      interface{}      `json:"items"`
	Pagination *PaginationMeta  `json:"pagination"`
	Links      *PaginationLinks `json:"links,omitempty"`
}

// PaginationLinks represents pagination navigation links
type PaginationLinks struct {
	First    string `json:"first,omitempty"`
	Previous string `json:"previous,omitempty"`
	Next     string `json:"next,omitempty"`
	Last     string `json:"last,omitempty"`
	Self     string `json:"self"`
}

// CursorPaginationParams represents cursor-based pagination parameters
type CursorPaginationParams struct {
	Cursor   string `json:"cursor,omitempty"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort,omitempty"`
	Order    string `json:"order,omitempty"`
}

// CursorPaginationResult represents cursor-based pagination result
type CursorPaginationResult struct {
	Items      interface{} `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	PrevCursor string      `json:"prev_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
	PageSize   int         `json:"page_size"`
}

// ===============================
// PAGINATION PARSER
// ===============================

// PaginationParser helps parse pagination parameters from requests
type PaginationParser struct {
	config *PaginationConfig
}

// NewPaginationParser creates a new pagination parser
func NewPaginationParser(config *PaginationConfig) *PaginationParser {
	if config == nil {
		config = DefaultPaginationConfig()
	}
	return &PaginationParser{config: config}
}

// ParseFromQuery parses pagination parameters from query string
func (p *PaginationParser) ParseFromQuery(query url.Values) (*PaginationParams, error) {
	params := &PaginationParams{
		Page:     1,
		PageSize: p.config.DefaultPageSize,
		Order:    "desc", // Default order
	}

	// Parse page number
	if pageStr := query.Get(p.config.PageParam); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return nil, fmt.Errorf("invalid page parameter: %s", pageStr)
		}
		if page < 1 {
			return nil, fmt.Errorf("page must be greater than 0")
		}
		params.Page = page
	}

	// Parse page size
	if sizeStr := query.Get(p.config.SizeParam); sizeStr != "" {
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid page_size parameter: %s", sizeStr)
		}
		if size < 1 {
			return nil, fmt.Errorf("page_size must be greater than 0")
		}
		if size > p.config.MaxPageSize {
			return nil, fmt.Errorf("page_size cannot exceed %d", p.config.MaxPageSize)
		}
		params.PageSize = size
	}

	// Parse sort field
	if sort := query.Get(p.config.SortParam); sort != "" {
		// Validate sort field against allowed values
		if err := p.validateSortField(sort); err != nil {
			return nil, err
		}
		params.Sort = sort
	}

	// Parse order direction
	if order := query.Get(p.config.OrderParam); order != "" {
		order = strings.ToLower(order)
		if order != "asc" && order != "desc" {
			return nil, fmt.Errorf("order must be either 'asc' or 'desc'")
		}
		params.Order = order
	}

	// Calculate offset
	params.Offset = (params.Page - 1) * params.PageSize

	return params, nil
}

// ParseFromRequest parses pagination parameters from HTTP request
func (p *PaginationParser) ParseFromRequest(r *http.Request) (*PaginationParams, error) {
	return p.ParseFromQuery(r.URL.Query())
}

// ParseCursorFromQuery parses cursor pagination parameters from query string
func (p *PaginationParser) ParseCursorFromQuery(query url.Values) (*CursorPaginationParams, error) {
	params := &CursorPaginationParams{
		PageSize: p.config.DefaultPageSize,
		Order:    "desc",
	}

	// Parse cursor
	params.Cursor = query.Get("cursor")

	// Parse page size
	if sizeStr := query.Get(p.config.SizeParam); sizeStr != "" {
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid page_size parameter: %s", sizeStr)
		}
		if size < 1 || size > p.config.MaxPageSize {
			return nil, fmt.Errorf("page_size must be between 1 and %d", p.config.MaxPageSize)
		}
		params.PageSize = size
	}

	// Parse sort and order
	if sort := query.Get(p.config.SortParam); sort != "" {
		if err := p.validateSortField(sort); err != nil {
			return nil, err
		}
		params.Sort = sort
	}

	if order := query.Get(p.config.OrderParam); order != "" {
		order = strings.ToLower(order)
		if order != "asc" && order != "desc" {
			return nil, fmt.Errorf("order must be either 'asc' or 'desc'")
		}
		params.Order = order
	}

	return params, nil
}

// validateSortField validates sort field against allowed values
func (p *PaginationParser) validateSortField(sort string) error {
	// Define allowed sort fields - customize for your application
	allowedFields := []string{
		"id", "created_at", "updated_at", "title", "username",
		"email", "first_name", "last_name", "likes", "views",
		"comments_count", "status", "category", "popularity",
	}

	for _, field := range allowedFields {
		if sort == field {
			return nil
		}
	}

	return fmt.Errorf("invalid sort field: %s. Allowed fields: %s",
		sort, strings.Join(allowedFields, ", "))
}

// ===============================
// PAGINATION BUILDER
// ===============================

// PaginationBuilder helps build pagination responses
type PaginationBuilder struct {
	config *PaginationConfig
}

// NewPaginationBuilder creates a new pagination builder
func NewPaginationBuilder(config *PaginationConfig) *PaginationBuilder {
	if config == nil {
		config = DefaultPaginationConfig()
	}
	return &PaginationBuilder{config: config}
}

// BuildMeta creates pagination metadata
func (pb *PaginationBuilder) BuildMeta(params *PaginationParams, total int64) *PaginationMeta {
	totalPages := int((total + int64(params.PageSize) - 1) / int64(params.PageSize))

	return &PaginationMeta{
		Page:       params.Page,
		PageSize:   params.PageSize,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    params.Page < totalPages,
		HasPrev:    params.Page > 1,
	}
}

// BuildLinks creates pagination navigation links
func (pb *PaginationBuilder) BuildLinks(r *http.Request, params *PaginationParams, total int64) *PaginationLinks {
	totalPages := int((total + int64(params.PageSize) - 1) / int64(params.PageSize))
	baseURL := pb.getBaseURL(r)
	query := r.URL.Query()

	links := &PaginationLinks{
		Self: pb.buildLink(baseURL, query, params.Page, params.PageSize),
	}

	// First page link
	if params.Page > 1 {
		links.First = pb.buildLink(baseURL, query, 1, params.PageSize)
	}

	// Previous page link
	if params.Page > 1 {
		links.Previous = pb.buildLink(baseURL, query, params.Page-1, params.PageSize)
	}

	// Next page link
	if params.Page < totalPages {
		links.Next = pb.buildLink(baseURL, query, params.Page+1, params.PageSize)
	}

	// Last page link
	if params.Page < totalPages {
		links.Last = pb.buildLink(baseURL, query, totalPages, params.PageSize)
	}

	return links
}

// BuildResult creates a complete pagination result
func (pb *PaginationBuilder) BuildResult(r *http.Request, items interface{}, params *PaginationParams, total int64) *PaginationResult {
	return &PaginationResult{
		Items:      items,
		Pagination: pb.BuildMeta(params, total),
		Links:      pb.BuildLinks(r, params, total),
	}
}

// BuildCursorResult creates a cursor-based pagination result
func (pb *PaginationBuilder) BuildCursorResult(items interface{}, params *CursorPaginationParams, nextCursor, prevCursor string, hasMore bool) *CursorPaginationResult {
	return &CursorPaginationResult{
		Items:      items,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
		HasMore:    hasMore,
		PageSize:   params.PageSize,
	}
}

// ===============================
// HELPER METHODS
// ===============================

// getBaseURL constructs the base URL for pagination links
func (pb *PaginationBuilder) getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// Check for forwarded protocol
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.Path)
}

// buildLink constructs a pagination link
func (pb *PaginationBuilder) buildLink(baseURL string, query url.Values, page, pageSize int) string {
	// Clone query parameters
	linkQuery := make(url.Values)
	for k, v := range query {
		linkQuery[k] = v
	}

	// Set pagination parameters
	linkQuery.Set(pb.config.PageParam, strconv.Itoa(page))
	linkQuery.Set(pb.config.SizeParam, strconv.Itoa(pageSize))

	return fmt.Sprintf("%s?%s", baseURL, linkQuery.Encode())
}

// ===============================
// INTEGRATION HELPERS
// ===============================

// ExtractPaginationFromModels extracts pagination info from your existing models.PaginatedResponse
// It returns the items, pagination parameters, total count, and any error
func ExtractPaginationFromModels(paginatedResponse interface{}) (interface{}, *PaginationParams, int64, error) {
	// Use reflection to inspect the paginatedResponse
	val := reflect.ValueOf(paginatedResponse)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// Check if the input is valid
	if !val.IsValid() {
		return nil, nil, 0, fmt.Errorf("invalid paginated response")
	}

	// Initialize default pagination params using the existing helper
	defaultConfig := DefaultPaginationConfig()
	params := &PaginationParams{
		Page:     1,
		PageSize: defaultConfig.DefaultPageSize,
		Order:    "desc",
	}

	var items interface{}
	var total int64

	// Try to extract common fields from the response
	if val.Kind() == reflect.Struct {
		// Try to get Items/Data field
		if itemsField := val.FieldByName("Items"); itemsField.IsValid() {
			items = itemsField.Interface()
		} else if dataField := val.FieldByName("Data"); dataField.IsValid() {
			items = dataField.Interface()
		}

		// Try to get pagination info
		if pageField := val.FieldByName("Page"); pageField.IsValid() && pageField.Kind() == reflect.Int {
			params.Page = int(pageField.Int())
		}

		if pageSizeField := val.FieldByName("PageSize"); pageSizeField.IsValid() && pageSizeField.Kind() == reflect.Int {
			params.PageSize = int(pageSizeField.Int())
		}

		if totalField := val.FieldByName("Total"); totalField.IsValid() && totalField.Kind() == reflect.Int64 {
			total = totalField.Int()
		}

		// Check for embedded pagination struct (common pattern in response structures)
		if paginationField := val.FieldByName("Pagination"); paginationField.IsValid() && paginationField.Kind() == reflect.Struct {
			if pageField := paginationField.FieldByName("Page"); pageField.IsValid() && pageField.Kind() == reflect.Int {
				params.Page = int(pageField.Int())
			}
			if pageSizeField := paginationField.FieldByName("PageSize"); pageSizeField.IsValid() && pageSizeField.Kind() == reflect.Int {
				params.PageSize = int(pageSizeField.Int())
			}
			if totalField := paginationField.FieldByName("Total"); totalField.IsValid() && totalField.Kind() == reflect.Int64 {
				total = totalField.Int()
			}
		}

		// If we couldn't find items directly, try to get it from a nested structure
		if items == nil {
			if dataField := val.FieldByName("Data"); dataField.IsValid() && dataField.Kind() == reflect.Struct {
				if nestedItems := dataField.FieldByName("Items"); nestedItems.IsValid() {
					items = nestedItems.Interface()
				}
			}
		}
	}

	// If we still don't have items, use the entire response
	if items == nil {
		items = paginatedResponse
	}

	// Validate pagination parameters using the existing validation function
	if err := ValidatePaginationParams(params); err != nil {
		// Reset to defaults if validation fails
		params.Page = 1
		params.PageSize = defaultConfig.DefaultPageSize
	}

	// Calculate offset using the existing helper
	params.Offset = CalculateOffset(params.Page, params.PageSize)

	return items, params, total, nil
}

// ValidatePaginationParams validates pagination parameters
func ValidatePaginationParams(params *PaginationParams) error {
	if params.Page < 1 {
		return fmt.Errorf("page must be greater than 0")
	}

	if params.PageSize < 1 {
		return fmt.Errorf("page_size must be greater than 0")
	}

	if params.PageSize > 100 { // Max page size
		return fmt.Errorf("page_size cannot exceed 100")
	}

	return nil
}

// CalculateOffset calculates offset from page and page size
func CalculateOffset(page, pageSize int) int {
	return (page - 1) * pageSize
}

// CalculateTotalPages calculates total pages from total items and page size
func CalculateTotalPages(total int64, pageSize int) int {
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

// ===============================
// MIDDLEWARE HELPER
// ===============================

// PaginationMiddleware creates middleware that parses pagination parameters
func PaginationMiddleware(parser *PaginationParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Parse pagination parameters
			params, err := parser.ParseFromRequest(r)
			if err != nil {
				// Write validation error
				QuickError(w, r, &services.ValidationError{
					ServiceError: &services.ServiceError{
						Type:       "VALIDATION_ERROR",
						Message:    fmt.Sprintf("Invalid pagination parameters: %s", err.Error()),
						StatusCode: http.StatusBadRequest,
					},
				})
				return
			}

			// Add pagination params to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, "pagination_params", params)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetPaginationParams extracts pagination parameters from context
func GetPaginationParams(ctx context.Context) *PaginationParams {
	if params, ok := ctx.Value("pagination_params").(*PaginationParams); ok {
		return params
	}

	// Return default params if not found
	return &PaginationParams{
		Page:     1,
		PageSize: 20,
		Order:    "desc",
	}
}

// ===============================
// RESPONSE BUILDERS
// ===============================

// BuildPaginatedResponse creates a paginated API response
func (b *Builder) BuildPaginatedResponse(r *http.Request, items interface{}, params *PaginationParams, total int64) *APIResponse {
	pb := NewPaginationBuilder(nil)
	result := pb.BuildResult(r, items, params, total)

	meta := &ResponseMeta{
		Pagination: result.Pagination,
	}

	response := b.SuccessWithMeta(r.Context(), result.Items, meta)

	// Add links to extra metadata if needed
	if result.Links != nil {
		if response.Meta.Extra == nil {
			response.Meta.Extra = make(map[string]interface{})
		}
		response.Meta.Extra["links"] = result.Links
	}

	return response
}

// WritePaginatedResponse writes a paginated response
func (b *Builder) WritePaginatedResponse(w http.ResponseWriter, r *http.Request, items interface{}, params *PaginationParams, total int64) {
	response := b.BuildPaginatedResponse(r, items, params, total)
	b.WriteJSON(w, r, response, http.StatusOK)
}
