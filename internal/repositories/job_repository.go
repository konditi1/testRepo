// file: internal/repositories/job_repository.go
package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/models"

	"go.uber.org/zap"
)

// jobRepository implements JobRepository with high-performance patterns
type jobRepository struct {
	*BaseRepository
}

// NewJobRepository creates a new instance of JobRepository
func NewJobRepository(db *database.Manager, logger *zap.Logger) JobRepository {
	return &jobRepository{
		BaseRepository: NewBaseRepository(db, logger),
	}
}

// ===============================
// BASIC CRUD OPERATIONS
// ===============================

// Create creates a new job posting
func (r *jobRepository) Create(ctx context.Context, job *models.Job) error {
	query := `
		INSERT INTO jobs (
			employer_id, title, description, requirements, responsibilities,
			employment_type, location, salary_range, is_remote,
			application_deadline, start_date, status, tags
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at`

	err := r.QueryRowContext(
		ctx, query,
		job.EmployerID, job.Title, job.Description, job.Requirements, job.Responsibilities,
		job.EmploymentType, job.Location, job.SalaryRange, job.IsRemote,
		job.ApplicationDeadline, job.StartDate, job.Status, job.Tags,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)

	if err != nil {
		r.GetLogger().Error("Failed to create job",
			zap.Error(err),
			zap.Int64("employer_id", job.EmployerID),
			zap.String("title", job.Title),
		)
		return fmt.Errorf("failed to create job: %w", err)
	}

	r.GetLogger().Info("Job created successfully",
		zap.Int64("job_id", job.ID),
		zap.Int64("employer_id", job.EmployerID),
		zap.String("title", job.Title),
	)

	return nil
}

// GetByID retrieves a job by ID with employer information
func (r *jobRepository) GetByID(ctx context.Context, jobID int64, userID *int64) (*models.Job, error) {
	query := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.requirements, j.responsibilities,
			j.employment_type, j.location, j.salary_range, j.is_remote,
			j.application_deadline, j.start_date, j.status, j.views_count, j.applications_count,
			j.tags, j.created_at, j.updated_at, j.published_at,
			-- Employer information
			u.username as employer_username, u.email as employer_email, u.display_name as employer_company,
			-- User-specific fields
			CASE WHEN $2 IS NOT NULL AND j.employer_id = $2 THEN true ELSE false END as is_owner,
			CASE WHEN $2 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $2
		WHERE j.id = $1 AND u.is_active = true`

	var job models.Job
	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{jobID, *userID}
	} else {
		queryArgs = []interface{}{jobID, nil}
	}

	err := r.QueryRowContext(ctx, query, queryArgs...).Scan(
		&job.ID, &job.EmployerID, &job.Title, &job.Description, &job.Requirements, &job.Responsibilities,
		&job.EmploymentType, &job.Location, &job.SalaryRange, &job.IsRemote,
		&job.ApplicationDeadline, &job.StartDate, &job.Status, &job.ViewsCount, &job.ApplicationsCount,
		&job.Tags, &job.CreatedAt, &job.UpdatedAt, &job.PublishedAt,
		&job.EmployerUsername, &job.EmployerEmail, &job.EmployerCompany,
		&job.IsOwner, &job.HasApplied,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job by ID: %w", err)
	}

	// Generate helper fields
	job.CreatedAtHuman = r.formatTimeHuman(job.CreatedAt)
	if job.ApplicationDeadline != nil {
		job.DeadlineHuman = r.formatTimeHuman(*job.ApplicationDeadline)
	}
	if job.StartDate != nil {
		job.StartDateHuman = r.formatTimeHuman(*job.StartDate)
	}

	return &job, nil
}

// Update updates an existing job
func (r *jobRepository) Update(ctx context.Context, job *models.Job) error {
	query := `
		UPDATE jobs SET
			title = $2, description = $3, requirements = $4, responsibilities = $5,
			employment_type = $6, location = $7, salary_range = $8, is_remote = $9,
			application_deadline = $10, start_date = $11, status = $12, tags = $13,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND employer_id = $14
		RETURNING updated_at`

	err := r.QueryRowContext(
		ctx, query,
		job.ID, job.Title, job.Description, job.Requirements, job.Responsibilities,
		job.EmploymentType, job.Location, job.SalaryRange, job.IsRemote,
		job.ApplicationDeadline, job.StartDate, job.Status, job.Tags, job.EmployerID,
	).Scan(&job.UpdatedAt)

	if err != nil {
		if r.IsNotFound(err) {
			return fmt.Errorf("job not found or not owned by employer")
		}
		return fmt.Errorf("failed to update job: %w", err)
	}

	r.GetLogger().Info("Job updated successfully",
		zap.Int64("job_id", job.ID),
		zap.Int64("employer_id", job.EmployerID),
	)

	return nil
}

// Delete removes a job by ID
func (r *jobRepository) Delete(ctx context.Context, id int64) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// First delete all applications
		_, err := tx.ExecContext(ctx, "DELETE FROM job_applications WHERE job_id = $1", id)
		if err != nil {
			return fmt.Errorf("failed to delete job applications: %w", err)
		}

		// Then delete the job
		result, err := tx.ExecContext(ctx, "DELETE FROM jobs WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("failed to delete job: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return fmt.Errorf("job not found")
		}

		return nil
	})
}

// ===============================
// LISTING AND FILTERING
// ===============================

// List retrieves a paginated list of jobs
func (r *jobRepository) List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	whereClause := "j.status = 'active' AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
	}, nil
}

// GetByEmployerID retrieves paginated jobs for a specific employer
func (r *jobRepository) GetByEmployerID(ctx context.Context, employerID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			true as is_owner, false as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id`

	whereClause := "j.employer_id = $1 AND u.is_active = true"
	whereArgs := []interface{}{employerID}

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by employer: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, &employerID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"employer_id": employerID},
	}, nil
}

// GetByStatus retrieves paginated jobs by status
func (r *jobRepository) GetByStatus(ctx context.Context, status string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	whereClause := "j.status = $2 AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, status)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by status: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"status": status},
	}, nil
}

// GetByEmploymentType retrieves paginated jobs by employment type
func (r *jobRepository) GetByEmploymentType(ctx context.Context, empType string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	whereClause := "j.employment_type = $2 AND j.status = 'active' AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, empType)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by employment type: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"employment_type": empType},
	}, nil
}

// GetByLocation retrieves paginated jobs by location
func (r *jobRepository) GetByLocation(ctx context.Context, location string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	whereClause := "(j.location ILIKE $2 OR j.is_remote = true) AND j.status = 'active' AND u.is_active = true"
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, "%"+location+"%")

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by location: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"location": location},
	}, nil
}

// GetFeatured retrieves featured jobs
func (r *jobRepository) GetFeatured(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	query := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1
		WHERE j.status = 'active' AND u.is_active = true
		ORDER BY j.views_count DESC, j.applications_count DESC, j.created_at DESC
		LIMIT $2`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{*userID, limit}
	} else {
		queryArgs = []interface{}{nil, limit}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get featured jobs: %w", err)
	}
	defer rows.Close()

	jobs, _ := r.scanJobRows(rows, userID)
	return jobs, nil
}

// GetRecent retrieves the most recent jobs
func (r *jobRepository) GetRecent(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	query := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1
		WHERE j.status = 'active' AND u.is_active = true
		ORDER BY j.created_at DESC
		LIMIT $2`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{*userID, limit}
	} else {
		queryArgs = []interface{}{nil, limit}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent jobs: %w", err)
	}
	defer rows.Close()

	jobs, _ := r.scanJobRows(rows, userID)
	return jobs, nil
}

// ===============================
// SEARCH OPERATIONS
// ===============================

// Search searches for jobs based on the provided query
func (r *jobRepository) Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	searchTerm := "%" + query + "%"
	whereClause := `j.status = 'active' AND u.is_active = true AND (
		j.title ILIKE $2 OR 
		j.description ILIKE $2 OR 
		j.location ILIKE $2 OR
		array_to_string(j.tags, ' ') ILIKE $2
	)`
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}
	whereArgs = append(whereArgs, searchTerm)

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	finalQuery, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, finalQuery, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"query": query},
	}, nil
}

// SearchBySkills searches for jobs by skills/tags
func (r *jobRepository) SearchBySkills(ctx context.Context, skills []string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
	if len(skills) == 0 {
		return r.List(ctx, params, userID)
	}

	baseQuery := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1`

	// Build skill matching condition
	skillConditions := make([]string, len(skills))
	whereArgs := []interface{}{}

	if userID != nil {
		whereArgs = append(whereArgs, *userID)
	} else {
		whereArgs = append(whereArgs, nil)
	}

	argIndex := 2
	for i, skill := range skills {
		skillConditions[i] = fmt.Sprintf("j.tags && ARRAY[$%d]", argIndex)
		whereArgs = append(whereArgs, skill)
		argIndex++
	}

	whereClause := fmt.Sprintf("j.status = 'active' AND u.is_active = true AND (%s)",
		strings.Join(skillConditions, " OR "))

	if params.Sort == "" {
		params.Sort = "created_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search jobs by skills: %w", err)
	}
	defer rows.Close()

	jobs, lastCursor := r.scanJobRows(rows, userID)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(jobs) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.Job]{
		Data:       jobs,
		Pagination: meta,
		Filters:    map[string]any{"skills": skills},
	}, nil
}

// ===============================
// APPLICATION MANAGEMENT
// ===============================

// HasUserApplied checks if a user has applied for a job
func (r *jobRepository) HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM job_applications WHERE job_id = $1 AND applicant_id = $2)`

	var exists bool
	err := r.QueryRowContext(ctx, query, jobID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if user applied: %w", err)
	}

	return exists, nil
}

// CreateApplication creates a new job application
func (r *jobRepository) CreateApplication(ctx context.Context, application *models.JobApplication) error {
	query := `
		INSERT INTO job_applications (
			job_id, applicant_id, cover_letter, application_letter_url, application_letter_public_id, status
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, applied_at, updated_at`

	err := r.QueryRowContext(
		ctx, query,
		application.JobID, application.ApplicantID, application.CoverLetter,
		application.ApplicationLetterURL, application.ApplicationLetterPublicID, "pending",
	).Scan(&application.ID, &application.AppliedAt, &application.UpdatedAt)

	if err != nil {
		r.GetLogger().Error("Failed to create job application",
			zap.Error(err),
			zap.Int64("job_id", application.JobID),
			zap.Int64("applicant_id", application.ApplicantID),
		)
		return fmt.Errorf("failed to create job application: %w", err)
	}

	// Update applications count
	_, err = r.ExecContext(ctx,
		"UPDATE jobs SET applications_count = applications_count + 1 WHERE id = $1",
		application.JobID)
	if err != nil {
		r.GetLogger().Warn("Failed to update applications count",
			zap.Error(err),
			zap.Int64("job_id", application.JobID),
		)
	}

	r.GetLogger().Info("Job application created successfully",
		zap.Int64("application_id", application.ID),
		zap.Int64("job_id", application.JobID),
		zap.Int64("applicant_id", application.ApplicantID),
	)

	return nil
}

// GetApplication retrieves a job application by job ID and user ID
func (r *jobRepository) GetApplication(ctx context.Context, jobID, userID int64) (*models.JobApplication, error) {
	query := `
		SELECT 
			ja.id, ja.job_id, ja.applicant_id, ja.cover_letter,
			ja.application_letter_url, ja.application_letter_public_id,
			ja.status, ja.notes, ja.applied_at, ja.reviewed_at, ja.updated_at,
			-- Job information
			j.title as job_title,
			-- Employer information
			emp.username as employer_username, emp.display_name as employer_company,
			-- Applicant information
			app.username as applicant_username, app.email as applicant_email,
			CONCAT(COALESCE(app.first_name, ''), ' ', COALESCE(app.last_name, '')) as applicant_name,
			app.cv_url as applicant_cv_url
		FROM job_applications ja
		INNER JOIN jobs j ON ja.job_id = j.id
		INNER JOIN users emp ON j.employer_id = emp.id
		INNER JOIN users app ON ja.applicant_id = app.id
		WHERE ja.job_id = $1 AND ja.applicant_id = $2`

	var application models.JobApplication
	err := r.QueryRowContext(ctx, query, jobID, userID).Scan(
		&application.ID, &application.JobID, &application.ApplicantID, &application.CoverLetter,
		&application.ApplicationLetterURL, &application.ApplicationLetterPublicID,
		&application.Status, &application.Notes, &application.AppliedAt, &application.ReviewedAt, &application.UpdatedAt,
		&application.JobTitle,
		&application.EmployerUsername, &application.EmployerCompany,
		&application.ApplicantUsername, &application.ApplicantEmail,
		&application.ApplicantName, &application.ApplicantCVURL,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job application: %w", err)
	}

	// Generate helper fields
	application.AppliedAtHuman = r.formatTimeHuman(application.AppliedAt)
	if application.ReviewedAt != nil {
		application.ReviewedAtHuman = r.formatTimeHuman(*application.ReviewedAt)
	}

	return &application, nil
}

// GetApplicationByID retrieves a job application by its ID
func (r *jobRepository) GetApplicationByID(ctx context.Context, applicationID int64) (*models.JobApplication, error) {
	query := `
		SELECT 
			ja.id, ja.job_id, ja.applicant_id, ja.cover_letter,
			ja.application_letter_url, ja.application_letter_public_id,
			ja.status, ja.notes, ja.applied_at, ja.reviewed_at, ja.updated_at,
			-- Job information
			j.title as job_title,
			-- Employer information
			emp.username as employer_username, emp.display_name as employer_company,
			-- Applicant information
			app.username as applicant_username, app.email as applicant_email,
			CONCAT(COALESCE(app.first_name, ''), ' ', COALESCE(app.last_name, '')) as applicant_name,
			app.cv_url as applicant_cv_url
		FROM job_applications ja
		INNER JOIN jobs j ON ja.job_id = j.id
		INNER JOIN users emp ON j.employer_id = emp.id
		INNER JOIN users app ON ja.applicant_id = app.id
		WHERE ja.id = $1`

	var application models.JobApplication
	err := r.QueryRowContext(ctx, query, applicationID).Scan(
		&application.ID, &application.JobID, &application.ApplicantID, &application.CoverLetter,
		&application.ApplicationLetterURL, &application.ApplicationLetterPublicID,
		&application.Status, &application.Notes, &application.AppliedAt, &application.ReviewedAt, &application.UpdatedAt,
		&application.JobTitle,
		&application.EmployerUsername, &application.EmployerCompany,
		&application.ApplicantUsername, &application.ApplicantEmail,
		&application.ApplicantName, &application.ApplicantCVURL,
	)

	if err != nil {
		if r.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job application by ID: %w", err)
	}

	// Generate helper fields
	application.AppliedAtHuman = r.formatTimeHuman(application.AppliedAt)
	if application.ReviewedAt != nil {
		application.ReviewedAtHuman = r.formatTimeHuman(*application.ReviewedAt)
	}

	return &application, nil
}

// GetApplicationsByJob retrieves paginated job applications for a specific job
func (r *jobRepository) GetApplicationsByJob(ctx context.Context, jobID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error) {
	baseQuery := `
		SELECT 
			ja.id, ja.job_id, ja.applicant_id, ja.cover_letter,
			ja.application_letter_url, ja.application_letter_public_id,
			ja.status, ja.notes, ja.applied_at, ja.reviewed_at, ja.updated_at,
			-- Job information
			j.title as job_title,
			-- Employer information
			emp.username as employer_username, emp.display_name as employer_company,
			-- Applicant information
			app.username as applicant_username, app.email as applicant_email,
			CONCAT(COALESCE(app.first_name, ''), ' ', COALESCE(app.last_name, '')) as applicant_name,
			app.cv_url as applicant_cv_url
		FROM job_applications ja
		INNER JOIN jobs j ON ja.job_id = j.id
		INNER JOIN users emp ON j.employer_id = emp.id
		INNER JOIN users app ON ja.applicant_id = app.id`

	whereClause := "ja.job_id = $1"
	whereArgs := []interface{}{jobID}

	if params.Sort == "" {
		params.Sort = "applied_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get job applications: %w", err)
	}
	defer rows.Close()

	applications, lastCursor := r.scanApplicationRows(rows)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(applications) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.JobApplication]{
		Data:       applications,
		Pagination: meta,
		Filters:    map[string]any{"job_id": jobID},
	}, nil
}

// GetApplicationsByUser retrieves paginated job applications for a specific user
func (r *jobRepository) GetApplicationsByUser(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error) {
	baseQuery := `
		SELECT 
			ja.id, ja.job_id, ja.applicant_id, ja.cover_letter,
			ja.application_letter_url, ja.application_letter_public_id,
			ja.status, ja.notes, ja.applied_at, ja.reviewed_at, ja.updated_at,
			-- Job information
			j.title as job_title,
			-- Employer information
			emp.username as employer_username, emp.display_name as employer_company,
			-- Applicant information
			app.username as applicant_username, app.email as applicant_email,
			CONCAT(COALESCE(app.first_name, ''), ' ', COALESCE(app.last_name, '')) as applicant_name,
			app.cv_url as applicant_cv_url
		FROM job_applications ja
		INNER JOIN jobs j ON ja.job_id = j.id
		INNER JOIN users emp ON j.employer_id = emp.id
		INNER JOIN users app ON ja.applicant_id = app.id`

	whereClause := "ja.applicant_id = $1"
	whereArgs := []interface{}{userID}

	if params.Sort == "" {
		params.Sort = "applied_at"
		params.Order = "desc"
	}

	query, args, err := r.BuildPaginatedQuery(baseQuery, whereClause, "", params)
	if err != nil {
		return nil, err
	}

	finalArgs := append(whereArgs, args...)

	rows, err := r.QueryContext(ctx, query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get user applications: %w", err)
	}
	defer rows.Close()

	applications, lastCursor := r.scanApplicationRows(rows)

	countQuery := r.BuildCountQuery(baseQuery, whereClause)
	total, err := r.GetTotalCount(ctx, countQuery, whereArgs...)
	if err != nil {
		total = 0
	}

	hasMore := len(applications) == params.Limit
	meta := r.BuildPaginationMeta(params, total, hasMore, lastCursor)

	return &models.PaginatedResponse[*models.JobApplication]{
		Data:       applications,
		Pagination: meta,
		Filters:    map[string]any{"applicant_id": userID},
	}, nil
}

// UpdateApplication updates an existing job application
func (r *jobRepository) UpdateApplication(ctx context.Context, application *models.JobApplication) error {
	query := `
		UPDATE job_applications SET
			cover_letter = $2, application_letter_url = $3, application_letter_public_id = $4,
			status = $5, notes = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING updated_at`

	err := r.QueryRowContext(
		ctx, query,
		application.ID, application.CoverLetter, application.ApplicationLetterURL,
		application.ApplicationLetterPublicID, application.Status, application.Notes,
	).Scan(&application.UpdatedAt)

	if err != nil {
		if r.IsNotFound(err) {
			return fmt.Errorf("job application not found")
		}
		return fmt.Errorf("failed to update job application: %w", err)
	}

	r.GetLogger().Info("Job application updated successfully",
		zap.Int64("application_id", application.ID),
		zap.String("status", application.Status),
	)

	return nil
}

// UpdateApplicationStatus updates the status of a job application
func (r *jobRepository) UpdateApplicationStatus(ctx context.Context, applicationID int64, status string, notes *string) error {
	query := `
		UPDATE job_applications SET
			status = $2, notes = $3, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	result, err := r.ExecContext(ctx, query, applicationID, status, notes)
	if err != nil {
		return fmt.Errorf("failed to update application status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("job application not found")
	}

	r.GetLogger().Info("Job application status updated",
		zap.Int64("application_id", applicationID),
		zap.String("status", status),
	)

	return nil
}

// DeleteApplication removes a job application
func (r *jobRepository) DeleteApplication(ctx context.Context, applicationID int64) error {
	return r.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Get job ID for updating count
		var jobID int64
		err := tx.QueryRowContext(ctx, "SELECT job_id FROM job_applications WHERE id = $1", applicationID).Scan(&jobID)
		if err != nil {
			if r.IsNotFound(err) {
				return fmt.Errorf("job application not found")
			}
			return fmt.Errorf("failed to get job ID: %w", err)
		}

		// Delete the application
		result, err := tx.ExecContext(ctx, "DELETE FROM job_applications WHERE id = $1", applicationID)
		if err != nil {
			return fmt.Errorf("failed to delete job application: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return fmt.Errorf("job application not found")
		}

		// Update applications count
		_, err = tx.ExecContext(ctx,
			"UPDATE jobs SET applications_count = applications_count - 1 WHERE id = $1 AND applications_count > 0",
			jobID)
		if err != nil {
			r.GetLogger().Warn("Failed to update applications count",
				zap.Error(err),
				zap.Int64("job_id", jobID),
			)
		}

		return nil
	})
}

// ===============================
// ANALYTICS
// ===============================

// GetJobStats retrieves statistics for a specific employer's jobs
func (r *jobRepository) GetJobStats(ctx context.Context, employerID int64) (*JobStats, error) {
	query := `
		SELECT 
			$1 as employer_id,
			COUNT(*) as total_jobs,
			COUNT(CASE WHEN status = 'active' THEN 1 END) as active_jobs,
			COUNT(CASE WHEN status = 'closed' THEN 1 END) as closed_jobs,
			COALESCE(SUM(applications_count), 0) as total_applications,
			COALESCE(SUM(views_count), 0) as total_views,
			COUNT(CASE WHEN status = 'filled' THEN 1 END) as filled_jobs
		FROM jobs
		WHERE employer_id = $1`

	var stats JobStats
	err := r.QueryRowContext(ctx, query, employerID).Scan(
		&stats.EmployerID,
		&stats.TotalJobs,
		&stats.ActiveJobs,
		&stats.ClosedJobs,
		&stats.TotalApplications,
		&stats.TotalViews,
		&stats.FilledJobs,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get job stats: %w", err)
	}

	return &stats, nil
}

// GetApplicationStats retrieves statistics for job applications
func (r *jobRepository) GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStats, error) {
	query := `
		SELECT 
			$1 as job_id,
			COUNT(*) as total_applications,
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_applications,
			COUNT(CASE WHEN status = 'reviewing' THEN 1 END) as reviewed_applications,
			COUNT(CASE WHEN status = 'shortlisted' THEN 1 END) as shortlisted_applications,
			COUNT(CASE WHEN status = 'accepted' THEN 1 END) as accepted_applications,
			COUNT(CASE WHEN status = 'rejected' THEN 1 END) as rejected_applications
		FROM job_applications
		WHERE job_id = $1`

	var stats ApplicationStats
	err := r.QueryRowContext(ctx, query, jobID).Scan(
		&stats.JobID,
		&stats.TotalApplications,
		&stats.PendingApplications,
		&stats.ReviewedApplications,
		&stats.ShortlistedApplications,
		&stats.AcceptedApplications,
		&stats.RejectedApplications,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get application stats: %w", err)
	}

	return &stats, nil
}

// IncrementViews increments the view count for a job
func (r *jobRepository) IncrementViews(ctx context.Context, jobID int64) error {
	query := `UPDATE jobs SET views_count = views_count + 1 WHERE id = $1`
	_, err := r.ExecContext(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("failed to increment job views: %w", err)
	}
	return nil
}

// GetPopularJobs gets the most popular jobs based on views and applications
func (r *jobRepository) GetPopularJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	query := `
		SELECT 
			j.id, j.employer_id, j.title, j.description, j.employment_type, j.location,
			j.salary_range, j.is_remote, j.application_deadline, j.status, j.views_count,
			j.applications_count, j.tags, j.created_at, j.updated_at,
			u.username as employer_username, u.display_name as employer_company,
			CASE WHEN $1 IS NOT NULL AND j.employer_id = $1 THEN true ELSE false END as is_owner,
			CASE WHEN $1 IS NOT NULL AND ja.applicant_id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		INNER JOIN users u ON j.employer_id = u.id
		LEFT JOIN job_applications ja ON j.id = ja.job_id AND ja.applicant_id = $1
		WHERE j.status = 'active' AND u.is_active = true
		ORDER BY (j.views_count * 0.7 + j.applications_count * 0.3) DESC, j.created_at DESC
		LIMIT $2`

	var queryArgs []interface{}
	if userID != nil {
		queryArgs = []interface{}{*userID, limit}
	} else {
		queryArgs = []interface{}{nil, limit}
	}

	rows, err := r.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get popular jobs: %w", err)
	}
	defer rows.Close()

	jobs, _ := r.scanJobRows(rows, userID)
	return jobs, nil
}

// ===============================
// HELPER METHODS
// ===============================

// scanJobRows scans job rows and handles user-specific data
func (r *jobRepository) scanJobRows(rows *sql.Rows, userID *int64) ([]*models.Job, string) {
	var jobs []*models.Job
	var lastCursor string

	for rows.Next() {
		var job models.Job

		err := rows.Scan(
			&job.ID, &job.EmployerID, &job.Title, &job.Description, &job.EmploymentType, &job.Location,
			&job.SalaryRange, &job.IsRemote, &job.ApplicationDeadline, &job.Status, &job.ViewsCount,
			&job.ApplicationsCount, &job.Tags, &job.CreatedAt, &job.UpdatedAt,
			&job.EmployerUsername, &job.EmployerCompany,
			&job.IsOwner, &job.HasApplied,
		)
		if err != nil {
			continue
		}

		// Generate helper fields
		job.CreatedAtHuman = r.formatTimeHuman(job.CreatedAt)
		if job.ApplicationDeadline != nil {
			job.DeadlineHuman = r.formatTimeHuman(*job.ApplicationDeadline)
		}
		if job.StartDate != nil {
			job.StartDateHuman = r.formatTimeHuman(*job.StartDate)
		}

		jobs = append(jobs, &job)
		lastCursor = r.encodeCursor(job.CreatedAt)
	}

	return jobs, lastCursor
}

// scanApplicationRows scans job application rows
func (r *jobRepository) scanApplicationRows(rows *sql.Rows) ([]*models.JobApplication, string) {
	var applications []*models.JobApplication
	var lastCursor string

	for rows.Next() {
		var application models.JobApplication

		err := rows.Scan(
			&application.ID, &application.JobID, &application.ApplicantID, &application.CoverLetter,
			&application.ApplicationLetterURL, &application.ApplicationLetterPublicID,
			&application.Status, &application.Notes, &application.AppliedAt, &application.ReviewedAt, &application.UpdatedAt,
			&application.JobTitle,
			&application.EmployerUsername, &application.EmployerCompany,
			&application.ApplicantUsername, &application.ApplicantEmail,
			&application.ApplicantName, &application.ApplicantCVURL,
		)
		if err != nil {
			continue
		}

		// Generate helper fields
		application.AppliedAtHuman = r.formatTimeHuman(application.AppliedAt)
		if application.ReviewedAt != nil {
			application.ReviewedAtHuman = r.formatTimeHuman(*application.ReviewedAt)
		}

		applications = append(applications, &application)
		lastCursor = r.encodeCursor(application.AppliedAt)
	}

	return applications, lastCursor
}

// formatTimeHuman formats time in human-readable format
func (r *jobRepository) formatTimeHuman(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}




// package repositories

// import (
// 	"context"
// 	"database/sql"
// 	"evalhub/internal/models"
// )

// //go:generate mockgen -destination=../../mocks/mock_job_repository.go -package=mocks evalhub/internal/repositories JobRepository

// type jobRepository struct {
// 	db *sql.DB
// }

// // NewJobRepository creates a new job repository
// func NewJobRepository(db *sql.DB) JobRepository {
// 	return &jobRepository{db: db}
// }

// func (r *jobRepository) GetAllWithDetails(ctx context.Context, currentUserID int64) ([]models.Job, error) {
// 	// Implement DB logic (move from handler)
// 	return nil, nil
// }

// func (r *jobRepository) GetByID(ctx context.Context, jobID int64, currentUserID *int64) (*models.Job, error) {
// 	// Implement DB logic (move from handler)
// 	return nil, nil
// }

// func (r *jobRepository) GetByEmployer(ctx context.Context, employerID int64) ([]models.Job, error) {
// 	// Implement DB logic (move from handler)
// 	return nil, nil
// }

// // GetByEmployerID retrieves paginated jobs for a specific employer
// func (r *jobRepository) GetByEmployerID(ctx context.Context, employerID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for getting paginated jobs by employer ID
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // GetByEmploymentType retrieves paginated jobs by employment type
// func (r *jobRepository) GetByEmploymentType(ctx context.Context, empType string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for getting paginated jobs by employment type
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // GetByStatus retrieves paginated jobs by status
// func (r *jobRepository) GetByStatus(ctx context.Context, status string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for getting paginated jobs by status
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // GetByLocation retrieves paginated jobs by location
// func (r *jobRepository) GetByLocation(ctx context.Context, location string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for getting paginated jobs by location
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// func (r *jobRepository) Create(ctx context.Context, job *models.Job) error {
// 	// Implement DB logic (move from handler)
// 	return nil
// }

// func (r *jobRepository) HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error) {
// 	// TODO: Implement DB logic for checking if user has applied
// 	return false, nil
// }

// func (r *jobRepository) ApplyForJob(ctx context.Context, app *models.JobApplication) error {
// 	// TODO: Implement DB logic for applying to a job
// 	return nil
// }

// func (r *jobRepository) GetJobApplications(ctx context.Context, jobID int64) ([]models.JobApplication, error) {
// 	// TODO: Implement DB logic for getting job applications
// 	return nil, nil
// }

// // Update updates an existing job
// func (r *jobRepository) Update(ctx context.Context, job *models.Job) error {
// 	// TODO: Implement DB logic for updating a job
// 	return nil
// }

// // Delete removes a job by its ID
// func (r *jobRepository) Delete(ctx context.Context, id int64) error {
// 	// TODO: Implement DB logic for deleting a job
// 	return nil
// }

// // GetApplicationByID retrieves a job application by its ID
// func (r *jobRepository) GetApplicationByID(ctx context.Context, applicationID int64) (*models.JobApplication, error) {
// 	// TODO: Implement DB logic for getting a job application by ID
// 	return nil, nil
// }

// // UpdateApplication updates an existing job application
// func (r *jobRepository) UpdateApplication(ctx context.Context, application *models.JobApplication) error {
// 	// TODO: Implement DB logic for updating a job application
// 	return nil
// }

// // GetApplicationByJobAndUser retrieves a job application by job ID and user ID
// func (r *jobRepository) GetApplicationByJobAndUser(ctx context.Context, jobID, userID int64) (*models.JobApplication, error) {
// 	// TODO: Implement DB logic for getting a job application by job ID and user ID
// 	return nil, nil
// }

// // GetApplicationsByJob retrieves paginated job applications for a specific job
// func (r *jobRepository) GetApplicationsByJob(ctx context.Context, jobID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error) {
// 	// TODO: Implement DB logic for getting paginated job applications by job ID
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.JobApplication]{
// 		Data: []*models.JobApplication{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // UpdateApplicationStatus updates the status of a job application
// func (r *jobRepository) UpdateApplicationStatus(ctx context.Context, applicationID int64, status string, notes *string) error {
// 	// TODO: Implement DB logic for updating application status
// 	return nil
// }

// // GetApplicationsByUser retrieves paginated job applications for a specific user
// func (r *jobRepository) GetApplicationsByUser(ctx context.Context, userID int64, params models.PaginationParams) (*models.PaginatedResponse[*models.JobApplication], error) {
// 	// TODO: Implement DB logic for getting paginated job applications by user ID
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.JobApplication]{
// 		Data: []*models.JobApplication{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // GetApplicationStats retrieves statistics for job applications
// func (r *jobRepository) GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStats, error) {
// 	// TODO: Implement DB logic for getting application stats
// 	return &ApplicationStats{
// 		JobID:              jobID,
// 		TotalApplications:  0,
// 		PendingApplications: 0,
// 		ShortlistedApplications: 0,
// 		AcceptedApplications: 0,
// 		RejectedApplications: 0,
// 	}, nil
// }

// // GetJobStats retrieves statistics for a specific employer's jobs
// func (r *jobRepository) GetJobStats(ctx context.Context, employerID int64) (*JobStats, error) {
// 	// TODO: Implement DB logic for getting job statistics
// 	return &JobStats{
// 		EmployerID:        employerID,
// 		TotalJobs:         0,
// 		ActiveJobs:        0,
// 		TotalApplications: 0,
// 		FilledJobs:        0,
// 	}, nil
// }

// // IncrementViews increments the view count for a job
// func (r *jobRepository) IncrementViews(ctx context.Context, jobID int64) error {
// 	// TODO: Implement DB logic for incrementing job views
// 	return nil
// }

// // List retrieves a paginated list of jobs
// func (r *jobRepository) List(ctx context.Context, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for listing jobs with pagination
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }

// // Search searches for jobs based on the provided query
// func (r *jobRepository) Search(ctx context.Context, query string, params models.PaginationParams, userID *int64) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement DB logic for searching jobs with pagination
// 	totalItems := int64(0)
// 	totalPages := 0
// 	if params.Limit > 0 {
// 		totalPages = int((totalItems + int64(params.Limit) - 1) / int64(params.Limit))
// 	}

// 	return &models.PaginatedResponse[*models.Job]{
// 		Data: []*models.Job{},
// 		Pagination: models.PaginationMeta{
// 			CurrentPage:  params.Offset/params.Limit + 1,
// 			TotalPages:   totalPages,
// 			TotalItems:   totalItems,
// 			ItemsPerPage: params.Limit,
// 			HasNext:      false,
// 			HasPrev:      params.Offset > 0,
// 			NextCursor:   params.Cursor,
// 		},
// 	}, nil
// }
