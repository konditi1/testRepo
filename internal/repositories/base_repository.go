package repositories

import (
	"context"
	"database/sql"
	"encoding/base64"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// BaseRepository provides common database operations with optimized patterns
type BaseRepository struct {
	db     *database.Manager
	logger *zap.Logger
}

// NewBaseRepository creates a new enhanced base repository
func NewBaseRepository(db *database.Manager, logger *zap.Logger) *BaseRepository {
	return &BaseRepository{
		db:     db,
		logger: logger,
	}
}

// ===============================
// CORE DATABASE OPERATIONS
// ===============================

// ExecContext executes a query with enhanced logging and metrics
func (r *BaseRepository) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	result, err := r.db.ExecContext(ctx, query, args...)
	
	// Log slow queries
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		r.logger.Warn("Slow query detected",
			zap.String("query", r.truncateQuery(query)),
			zap.Duration("duration", duration),
			zap.Any("args", args),
		)
	}
	
	if err != nil {
		r.logger.Error("Query execution failed",
			zap.String("query", r.truncateQuery(query)),
			zap.Error(err),
			zap.Any("args", args),
		)
	}
	
	return result, err
}

// QueryContext executes a query that returns rows
func (r *BaseRepository) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := r.db.QueryContext(ctx, query, args...)
	
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		r.logger.Warn("Slow query detected",
			zap.String("query", r.truncateQuery(query)),
			zap.Duration("duration", duration),
			zap.Any("args", args),
		)
	}
	
	if err != nil {
		r.logger.Error("Query execution failed",
			zap.String("query", r.truncateQuery(query)),
			zap.Error(err),
			zap.Any("args", args),
		)
	}
	
	return rows, err
}

// QueryRowContext executes a query that returns a single row
func (r *BaseRepository) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	row := r.db.QueryRowContext(ctx, query, args...)
	
	duration := time.Since(start)
	if duration > 50*time.Millisecond {
		r.logger.Warn("Slow single-row query detected",
			zap.String("query", r.truncateQuery(query)),
			zap.Duration("duration", duration),
			zap.Any("args", args),
		)
	}
	
	return row
}

// BeginTx starts a new transaction with enhanced context
func (r *BaseRepository) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, opts)
}

// ===============================
// PAGINATION HELPERS
// ===============================

// BuildPaginatedQuery constructs a paginated query with proper ordering
func (r *BaseRepository) BuildPaginatedQuery(baseQuery, whereClause, orderBy string, params models.PaginationParams) (string, []interface{}, error) {
	var args []interface{}
	argIndex := 1
	
	// Add WHERE clause if provided
	query := baseQuery
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	
	// Handle ordering
	if params.Sort == "" {
		params.Sort = "created_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	
	// Validate sort and order parameters
	validSorts := map[string]bool{
		"created_at": true, "updated_at": true, "title": true, 
		"likes_count": true, "username": true, "id": true,
	}
	validOrders := map[string]bool{"asc": true, "desc": true}
	
	if !validSorts[params.Sort] {
		params.Sort = "created_at"
	}
	if !validOrders[params.Order] {
		params.Order = "desc"
	}
	
	// Add ORDER BY
	query += fmt.Sprintf(" ORDER BY %s %s", params.Sort, strings.ToUpper(params.Order))
	
	// Handle cursor-based pagination if cursor is provided
	if params.Cursor != "" {
		cursorValue, err := r.decodeCursor(params.Cursor)
		if err == nil && cursorValue != "" {
			operator := ">"
			if params.Order == "desc" {
				operator = "<"
			}
			
			if whereClause != "" {
				query = strings.Replace(query, "WHERE", fmt.Sprintf("WHERE (%s %s $%d) AND", params.Sort, operator, argIndex), 1)
			} else {
				query += fmt.Sprintf(" WHERE %s %s $%d", params.Sort, operator, argIndex)
			}
			
			args = append(args, cursorValue)
			argIndex++
		}
	}
	
	// Add LIMIT
	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, params.Limit)
	argIndex++
	
	// Add OFFSET for offset-based pagination
	if params.Cursor == "" && params.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, params.Offset)
	}
	
	return query, args, nil
}

// BuildCountQuery creates a count query from a base query
func (r *BaseRepository) BuildCountQuery(baseQuery, whereClause string) string {
	// Extract the FROM clause and everything after it
	fromIndex := strings.Index(strings.ToUpper(baseQuery), "FROM")
	if fromIndex == -1 {
		return ""
	}
	
	fromClause := baseQuery[fromIndex:]
	
	// Remove ORDER BY, LIMIT, OFFSET clauses for counting
	orderByIndex := strings.Index(strings.ToUpper(fromClause), "ORDER BY")
	if orderByIndex != -1 {
		fromClause = fromClause[:orderByIndex]
	}
	
	countQuery := "SELECT COUNT(*) " + fromClause
	
	if whereClause != "" {
		countQuery += " WHERE " + whereClause
	}
	
	return countQuery
}

// GetTotalCount executes a count query
func (r *BaseRepository) GetTotalCount(ctx context.Context, countQuery string, args ...interface{}) (int64, error) {
	var total int64
	err := r.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	return total, err
}

// BuildPaginationMeta creates pagination metadata
func (r *BaseRepository) BuildPaginationMeta(params models.PaginationParams, total int64, hasMore bool, lastCursor string) models.PaginationMeta {
	currentPage := (params.Offset / params.Limit) + 1
	if currentPage < 1 {
		currentPage = 1
	}
	
	totalPages := int((total + int64(params.Limit) - 1) / int64(params.Limit))
	
	meta := models.PaginationMeta{
		CurrentPage:  currentPage,
		TotalPages:   totalPages,
		TotalItems:   total,
		ItemsPerPage: params.Limit,
		HasNext:      hasMore,
		HasPrev:      params.Offset > 0,
	}
	
	if hasMore && lastCursor != "" {
		meta.NextCursor = lastCursor
	}
	
	return meta
}

// ===============================
// BATCH OPERATIONS
// ===============================

// BulkInsert performs optimized bulk insert operations
func (r *BaseRepository) BulkInsert(ctx context.Context, table string, columns []string, values [][]interface{}) (*BulkInsertResult, error) {
	if len(values) == 0 {
		return &BulkInsertResult{}, nil
	}
	
	// Build bulk insert query
	placeholders := make([]string, len(values))
	args := make([]interface{}, 0, len(values)*len(columns))
	argIndex := 1
	
	for i, row := range values {
		rowPlaceholders := make([]string, len(columns))
		for j := range columns {
			if i*len(columns)+j < len(row) {
				rowPlaceholders[j] = fmt.Sprintf("$%d", argIndex)
				args = append(args, row[j])
				argIndex++
			} else {
				rowPlaceholders[j] = "NULL"
			}
		}
		placeholders[i] = "(" + strings.Join(rowPlaceholders, ", ") + ")"
	}
	
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s RETURNING id",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	
	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return &BulkInsertResult{
			Failed: len(values),
			Errors: []error{err},
		}, err
	}
	defer rows.Close()
	
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	
	return &BulkInsertResult{
		Inserted: len(ids),
		Failed:   len(values) - len(ids),
		IDs:      ids,
	}, nil
}

// BulkUpdate performs optimized bulk update operations
func (r *BaseRepository) BulkUpdate(ctx context.Context, table string, updates map[string]interface{}, whereClause string, args ...interface{}) (*BatchUpdateResult, error) {
	setParts := make([]string, 0, len(updates))
	setArgs := make([]interface{}, 0, len(updates))
	argIndex := len(args) + 1
	
	for column, value := range updates {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", column, argIndex))
		setArgs = append(setArgs, value)
		argIndex++
	}
	
	query := fmt.Sprintf(
		"UPDATE %s SET %s",
		table,
		strings.Join(setParts, ", "),
	)
	
	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	
	allArgs := append(args, setArgs...)
	result, err := r.ExecContext(ctx, query, allArgs...)
	if err != nil {
		return &BatchUpdateResult{
			Failed: 1,
			Errors: []error{err},
		}, err
	}
	
	rowsAffected, _ := result.RowsAffected()
	return &BatchUpdateResult{
		Updated: int(rowsAffected),
	}, nil
}

// ===============================
// TRANSACTION HELPERS
// ===============================

// WithTransaction executes a function within a database transaction
func (r *BaseRepository) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()
	
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			r.logger.Error("Failed to rollback transaction",
				zap.Error(rbErr),
				zap.Error(err),
			)
		}
		return err
	}
	
	return tx.Commit()
}

// ===============================
// UTILITY METHODS
// ===============================

// encodeCursor creates a base64 encoded cursor
func (r *BaseRepository) encodeCursor(value interface{}) string {
	var str string
	switch v := value.(type) {
	case int64:
		str = strconv.FormatInt(v, 10)
	case time.Time:
		str = v.Format(time.RFC3339Nano)
	case string:
		str = v
	default:
		str = fmt.Sprintf("%v", v)
	}
	return base64.URLEncoding.EncodeToString([]byte(str))
}

// decodeCursor decodes a base64 cursor
func (r *BaseRepository) decodeCursor(cursor string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// truncateQuery truncates long queries for logging
func (r *BaseRepository) truncateQuery(query string) string {
	const maxLength = 200
	if len(query) <= maxLength {
		return query
	}
	return query[:maxLength] + "..."
}

// BuildWhereClause constructs WHERE conditions dynamically
func (r *BaseRepository) BuildWhereClause(conditions map[string]interface{}) (string, []interface{}) {
	if len(conditions) == 0 {
		return "", nil
	}
	
	var clauses []string
	var args []interface{}
	argIndex := 1
	
	for column, value := range conditions {
		switch v := value.(type) {
		case nil:
			clauses = append(clauses, fmt.Sprintf("%s IS NULL", column))
		case []interface{}:
			if len(v) > 0 {
				placeholders := make([]string, len(v))
				for i, item := range v {
					placeholders[i] = fmt.Sprintf("$%d", argIndex)
					args = append(args, item)
					argIndex++
				}
				clauses = append(clauses, fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")))
			}
		default:
			clauses = append(clauses, fmt.Sprintf("%s = $%d", column, argIndex))
			args = append(args, value)
			argIndex++
		}
	}
	
	return strings.Join(clauses, " AND "), args
}

// IsNotFound checks if error is a "not found" error
func (r *BaseRepository) IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}

// HandleNotFound converts sql.ErrNoRows to nil for optional queries
func (r *BaseRepository) HandleNotFound(err error) error {
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

// ===============================
// PERFORMANCE HELPERS
// ===============================

// PreloadAssociations helps prevent N+1 queries by preloading related data
func (r *BaseRepository) PreloadAssociations(ctx context.Context, query string, args []interface{}, processor func(*sql.Rows) error) error {
	rows, err := r.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	
	return processor(rows)
}

// GetDB returns the underlying database manager for advanced operations
func (r *BaseRepository) GetDB() *database.Manager {
	return r.db
}

// GetLogger returns the logger instance
func (r *BaseRepository) GetLogger() *zap.Logger {
	return r.logger
}
