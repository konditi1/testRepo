// file: internal/services/job_service.go
package services

import (
	"context"
	"evalhub/internal/models"
	"evalhub/internal/repositories"
	"fmt"
	"time"
)

type jobService struct {
	repo repositories.JobRepository
}

// NewJobService creates a new job service
func NewJobService(repo repositories.JobRepository) JobService {
	return &jobService{repo: repo}
}

// CreateJob creates a new job posting
func (s *jobService) CreateJob(ctx context.Context, req *CreateJobRequest) (*models.Job, error) {
	// Validate request
	if req.Title == "" || req.Description == "" || req.Location == "" {
		return nil, NewValidationError("title, description, and location are required", nil)
	}

	// Handle currency safely
	currency := ""
	if req.Currency != nil {
		currency = *req.Currency
	}

	// Map request to job model
	salaryRangeStr := fmt.Sprintf("%d-%d %s", req.SalaryMin, req.SalaryMax, currency)
	job := &models.Job{
		EmployerID:          req.EmployerID,
		Title:               req.Title,
		Description:         req.Description,
		Requirements:        &req.Requirements, // Pointer to string remove it in future it can cause panic if its nil
		Location:            &req.Location,
		EmploymentType:      req.EmploymentType,
		SalaryRange:         &salaryRangeStr,
		IsRemote:            req.Remote,
		ApplicationDeadline: req.ApplicationDeadline,
		StartDate:           nil, // You might want to add this to the request
		Status:              "active",
		Tags:                req.Skills,
	}

	// Create job in repository
	err := s.repo.Create(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// GetJobByID retrieves a job by its ID
func (s *jobService) GetJobByID(ctx context.Context, jobID int64, currentUserID *int64) (*models.Job, error) {
	if jobID <= 0 {
		return nil, NewValidationError("invalid job ID", nil)
	}

	job, err := s.repo.GetByID(ctx, jobID, currentUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if job == nil {
		return nil, NewNotFoundError("job not found")
	}

	// Increment view count
	go func() {
		if err := s.repo.IncrementViews(context.Background(), jobID); err != nil {
			// Log error but don't fail the request
		}
	}()

	return job, nil
}

// UpdateJob updates an existing job
func (s *jobService) UpdateJob(ctx context.Context, req *UpdateJobRequest) (*models.Job, error) {
	// Get existing job to verify ownership
	existingJob, err := s.repo.GetByID(ctx, req.JobID, &req.EmployerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if existingJob == nil {
		return nil, NewNotFoundError("job not found")
	}

	if existingJob.EmployerID != req.EmployerID {
		return nil, NewForbiddenError("you can only update your own jobs")
	}

	// Update fields
	if req.Title != nil {
		existingJob.Title = *req.Title
	}
	if req.Description != nil {
		existingJob.Description = *req.Description
	}
	if req.Location != nil {
		existingJob.Location = req.Location
	}
	if req.EmploymentType != nil {
		existingJob.EmploymentType = *req.EmploymentType
	}
	if req.Remote != nil {
		existingJob.IsRemote = *req.Remote
	}
	if req.ApplicationDeadline != nil {
		existingJob.ApplicationDeadline = req.ApplicationDeadline
	}
	if req.Status != nil {
		existingJob.Status = *req.Status
	}
	if req.Skills != nil {
		existingJob.Tags = req.Skills
	}

	// Update salary range if provided
	if req.SalaryMin != nil && req.SalaryMax != nil && req.Currency != nil {
		salaryRange := fmt.Sprintf("%d-%d %s", *req.SalaryMin, *req.SalaryMax, *req.Currency)
		existingJob.SalaryRange = &salaryRange
	}

	err = s.repo.Update(ctx, existingJob)
	if err != nil {
		return nil, fmt.Errorf("failed to update job: %w", err)
	}

	return existingJob, nil
}

// DeleteJob deletes a job
func (s *jobService) DeleteJob(ctx context.Context, jobID, userID int64) error {
	// Verify ownership
	job, err := s.repo.GetByID(ctx, jobID, &userID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job == nil {
		return NewNotFoundError("job not found")
	}

	if job.EmployerID != userID {
		return NewForbiddenError("you can only delete your own jobs")
	}

	return s.repo.Delete(ctx, jobID)
}

// ListJobs retrieves a paginated list of jobs
func (s *jobService) ListJobs(ctx context.Context, req *ListJobsRequest) (*models.PaginatedResponse[*models.Job], error) {
	params := models.PaginationParams{
		Limit:  req.Pagination.Limit,
		Offset: req.Pagination.Offset,
		Cursor: req.Pagination.Cursor,
		Sort:   *req.SortBy,
		Order:  *req.SortOrder,
	}

	return s.repo.List(ctx, params, req.UserID)
}

// SearchJobs searches for jobs based on criteria
func (s *jobService) SearchJobs(ctx context.Context, req *SearchJobsRequest) (*models.PaginatedResponse[*models.Job], error) {
	params := models.PaginationParams{
		Limit:  req.Pagination.Limit,
		Offset: req.Pagination.Offset,
		Cursor: req.Pagination.Cursor,
	}

	if len(req.Skills) > 0 {
		return s.repo.SearchBySkills(ctx, req.Skills, params, req.UserID)
	}

	return s.repo.Search(ctx, req.Query, params, req.UserID)
}

// GetJobsByEmployer retrieves jobs posted by a specific employer
func (s *jobService) GetJobsByEmployer(ctx context.Context, req *GetJobsByEmployerRequest) (*models.PaginatedResponse[*models.Job], error) {
	params := models.PaginationParams{
		Limit:  req.Pagination.Limit,
		Offset: req.Pagination.Offset,
		Cursor: req.Pagination.Cursor,
	}

	return s.repo.GetByEmployerID(ctx, req.EmployerID, params)
}

// GetFeaturedJobs retrieves featured jobs
func (s *jobService) GetFeaturedJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	return s.repo.GetFeatured(ctx, limit, userID)
}

// GetRecentJobs retrieves recently posted jobs
func (s *jobService) GetRecentJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	return s.repo.GetRecent(ctx, limit, userID)
}

// GetPopularJobs retrieves popular jobs based on views/applications
func (s *jobService) GetPopularJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
	return s.repo.GetPopularJobs(ctx, limit, userID)
}

// ApplyForJob handles job applications
func (s *jobService) ApplyForJob(ctx context.Context, req *ApplyForJobRequest) (*models.JobApplication, error) {
	// Check if user already applied
	applied, err := s.repo.HasUserApplied(ctx, req.JobID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to check application status: %w", err)
	}

	if applied {
		return nil, NewValidationError("you have already applied to this job", err)
	}

	// Verify job exists and is active
	job, err := s.repo.GetByID(ctx, req.JobID, &req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if job == nil {
		return nil, NewNotFoundError("job not found")
	}

	if job.Status != "active" {
		return nil, NewValidationError("this job is no longer accepting applications", err)
	}

	if job.ApplicationDeadline != nil && job.ApplicationDeadline.Before(time.Now()) {
		return nil, NewValidationError("application deadline has passed", err)
	}

	// Create application
	application := &models.JobApplication{
		JobID:       req.JobID,
		ApplicantID: req.UserID,
		CoverLetter: *req.CoverLetter,
		Status:      "pending",
		AppliedAt:   time.Now(),
	}

	err = s.repo.CreateApplication(ctx, application)
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	return application, nil
}

// GetJobApplications retrieves applications for a job
func (s *jobService) GetJobApplications(ctx context.Context, req *GetJobApplicationsRequest) (*models.PaginatedResponse[*models.JobApplication], error) {
	// Verify job ownership
	job, err := s.repo.GetByID(ctx, req.JobID, &req.EmployerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if job == nil {
		return nil, NewNotFoundError("job not found")
	}

	if job.EmployerID != req.EmployerID {
		return nil, NewForbiddenError("you can only view applications for your own jobs")
	}

	params := models.PaginationParams{
		Limit:  req.Pagination.Limit,
		Offset: req.Pagination.Offset,
		Cursor: req.Pagination.Cursor,
	}

	return s.repo.GetApplicationsByJob(ctx, req.JobID, params)
}

// HasUserApplied checks if a user has applied to a job
func (s *jobService) HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error) {
	return s.repo.HasUserApplied(ctx, jobID, userID)
}

// ReviewApplication handles application review
func (s *jobService) ReviewApplication(ctx context.Context, req *ReviewApplicationRequest) error {
	// Get application to verify ownership
	application, err := s.repo.GetApplicationByID(ctx, req.ApplicationID)
	if err != nil {
		return fmt.Errorf("failed to get application: %w", err)
	}

	if application == nil {
		return NewNotFoundError("application not found")
	}

	// Verify job ownership
	job, err := s.repo.GetByID(ctx, application.JobID, &req.ReviewerID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job.EmployerID != req.ReviewerID {
		return NewForbiddenError("you can only review applications for your own jobs")
	}

	return s.repo.UpdateApplicationStatus(ctx, req.ApplicationID, req.Status, req.Notes)
}

// ShortlistApplicant shortlists an applicant
func (s *jobService) ShortlistApplicant(ctx context.Context, applicationID, reviewerID int64) error {
	return s.ReviewApplication(ctx, &ReviewApplicationRequest{
		ApplicationID: applicationID,
		ReviewerID:    reviewerID,
		Status:        "shortlisted",
	})
}

// RejectApplication rejects a job application
func (s *jobService) RejectApplication(ctx context.Context, req *RejectApplicationRequest) error {
	return s.ReviewApplication(ctx, &ReviewApplicationRequest{
		ApplicationID: req.ApplicationID,
		ReviewerID:    req.ReviewerID,
		Status:        "rejected",
		Notes:         req.Notes,
	})
}

// AcceptApplication accepts a job application
func (s *jobService) AcceptApplication(ctx context.Context, req *AcceptApplicationRequest) error {
	return s.ReviewApplication(ctx, &ReviewApplicationRequest{
		ApplicationID: req.ApplicationID,
		ReviewerID:    req.ReviewerID,
		Status:        "accepted",
		Notes:         req.Notes,
	})
}

// GetJobStats retrieves job statistics
func (s *jobService) GetJobStats(ctx context.Context, employerID int64) (*JobStatsResponse, error) {
	stats, err := s.repo.GetJobStats(ctx, employerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job stats: %w", err)
	}

	return &JobStatsResponse{
		EmployerID:         stats.EmployerID,
		TotalJobs:          stats.TotalJobs,
		ActiveJobs:         stats.ActiveJobs,
		ClosedJobs:         stats.ClosedJobs,
		TotalApplications:  stats.TotalApplications,
		TotalViews:         stats.TotalViews,
		FilledJobs:         stats.FilledJobs,
		AverageTimeToFill:  0, // Calculate if needed
	}, nil
}

// GetApplicationStats retrieves application statistics for a job
func (s *jobService) GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStatsResponse, error) {
	stats, err := s.repo.GetApplicationStats(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application stats: %w", err)
	}

	var conversionRate float64
	if stats.TotalApplications > 0 {
		conversionRate = float64(stats.AcceptedApplications) / float64(stats.TotalApplications) * 100
	}

	return &ApplicationStatsResponse{
		JobID:                   stats.JobID,
		TotalApplications:       stats.TotalApplications,
		PendingApplications:     stats.PendingApplications,
		ReviewedApplications:    stats.ReviewedApplications,
		ShortlistedApplications: stats.ShortlistedApplications,
		AcceptedApplications:    stats.AcceptedApplications,
		RejectedApplications:    stats.RejectedApplications,
		ConversionRate:          conversionRate,
	}, nil
}

// Additional methods that were missing from the interface but used in handlers
func (s *jobService) GetAllJobsWithDetails(ctx context.Context, currentUserID int64) ([]models.Job, error) {
	// This should be replaced with ListJobs for proper pagination
	req := &ListJobsRequest{
		Pagination: models.PaginationParams{
			Limit: 100, // Set a reasonable default
		},
		UserID: &currentUserID,
	}
	
	result, err := s.ListJobs(ctx, req)
	if err != nil {
		return nil, err
	}
	
	// Convert to slice
	jobs := make([]models.Job, len(result.Data))
	for i, job := range result.Data {
		jobs[i] = *job
	}
	
	return jobs, nil
}

func (s *jobService) GetUserApplications(ctx context.Context, req *GetUserApplicationsRequest) (*models.PaginatedResponse[*models.JobApplication], error) {
	params := models.PaginationParams{
		Limit:  req.Pagination.Limit,
		Offset: req.Pagination.Offset,
		Cursor: req.Pagination.Cursor,
	}

	return s.repo.GetApplicationsByUser(ctx, req.UserID, params)
}

func (s *jobService) WithdrawApplication(ctx context.Context, applicationID, userID int64) error {
	// Get application to verify ownership
	application, err := s.repo.GetApplicationByID(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("failed to get application: %w", err)
	}

	if application == nil {
		return NewNotFoundError("application not found")
	}

	if application.ApplicantID != userID {
		return NewForbiddenError("you can only withdraw your own applications")
	}

	return s.repo.DeleteApplication(ctx, applicationID)
}


// // file: internal/services/job_service.go
// package services

// import (
// 	"context"
// 	"evalhub/internal/models"
// 	"evalhub/internal/repositories"
// )

// type jobService struct {
// 	repo repositories.JobRepository
// }

// // NewJobService creates a new job service
// func NewJobService(repo repositories.JobRepository) JobService {
// 	return &jobService{repo: repo}
// }

// // CreateJob creates a new job posting
// func (s *jobService) CreateJob(ctx context.Context, req *CreateJobRequest) (*models.Job, error) {
// 	// TODO: Implement job creation logic
// 	// 1. Validate request
// 	// 2. Map request to job model
// 	// 3. Call repository to create job
// 	// 4. Return created job or error
// 	return nil, nil
// }

// // GetJobByID retrieves a job by its ID
// func (s *jobService) GetJobByID(ctx context.Context, jobID int64, currentUserID *int64) (*models.Job, error) {
// 	// TODO: Implement get job by ID logic
// 	// 1. Call repository to get job
// 	// 2. Check permissions if needed
// 	// 3. Return job or error
// 	return nil, nil
// }

// // UpdateJob updates an existing job
// func (s *jobService) UpdateJob(ctx context.Context, req *UpdateJobRequest) (*models.Job, error) {
// 	// TODO: Implement job update logic
// 	// 1. Validate request
// 	// 2. Check permissions
// 	// 3. Call repository to update job
// 	// 4. Return updated job or error
// 	return nil, nil
// }

// // DeleteJob deletes a job
// func (s *jobService) DeleteJob(ctx context.Context, jobID, userID int64) error {
// 	// TODO: Implement job deletion logic
// 	// 1. Check permissions
// 	// 2. Call repository to delete job
// 	// 3. Return error if any
// 	return nil
// }

// // ListJobs retrieves a paginated list of jobs
// func (s *jobService) ListJobs(ctx context.Context, req *ListJobsRequest) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement job listing logic
// 	// 1. Build query parameters from request
// 	// 2. Call repository to get paginated jobs
// 	// 3. Return paginated response or error
// 	return nil, nil
// }

// // SearchJobs searches for jobs based on criteria
// func (s *jobService) SearchJobs(ctx context.Context, req *SearchJobsRequest) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement job search logic
// 	// 1. Build search query from request
// 	// 2. Call repository to search jobs
// 	// 3. Return search results or error
// 	return nil, nil
// }

// // GetJobsByEmployer retrieves jobs posted by a specific employer
// func (s *jobService) GetJobsByEmployer(ctx context.Context, req *GetJobsByEmployerRequest) (*models.PaginatedResponse[*models.Job], error) {
// 	// TODO: Implement get jobs by employer logic
// 	// 1. Validate request
// 	// 2. Call repository to get employer's jobs
// 	// 3. Return paginated response or error
// 	return nil, nil
// }

// // GetFeaturedJobs retrieves featured jobs
// func (s *jobService) GetFeaturedJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
// 	// TODO: Implement get featured jobs logic
// 	// 1. Call repository to get featured jobs
// 	// 2. Return jobs or error
// 	return nil, nil
// }

// // GetRecentJobs retrieves recently posted jobs
// func (s *jobService) GetRecentJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
// 	// TODO: Implement get recent jobs logic
// 	// 1. Call repository to get recent jobs
// 	// 2. Return jobs or error
// 	return nil, nil
// }

// // GetPopularJobs retrieves popular jobs based on views/applications
// func (s *jobService) GetPopularJobs(ctx context.Context, limit int, userID *int64) ([]*models.Job, error) {
// 	// TODO: Implement get popular jobs logic
// 	// 1. Call repository to get popular jobs
// 	// 2. Return jobs or error
// 	return nil, nil
// }

// // ApplyForJob handles job applications
// func (s *jobService) ApplyForJob(ctx context.Context, req *ApplyForJobRequest) (*models.JobApplication, error) {
// 	// TODO: Implement job application logic
// 	// 1. Validate request
// 	// 2. Check if user can apply
// 	// 3. Create application record
// 	// 4. Return application or error
// 	return nil, nil
// }

// // GetJobApplications retrieves applications for a job
// func (s *jobService) GetJobApplications(ctx context.Context, req *GetJobApplicationsRequest) (*models.PaginatedResponse[*models.JobApplication], error) {
// 	// TODO: Implement get job applications logic
// 	// 1. Check permissions
// 	// 2. Call repository to get applications
// 	// 3. Return paginated response or error
// 	return nil, nil
// }

// // HasUserApplied checks if a user has applied to a job
// func (s *jobService) HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error) {
// 	// TODO: Implement check if user has applied
// 	// 1. Call repository to check application
// 	// 2. Return result or error
// 	return false, nil
// }

// // ReviewApplication handles application review
// func (s *jobService) ReviewApplication(ctx context.Context, req *ReviewApplicationRequest) error {
// 	// TODO: Implement application review logic
// 	// 1. Check permissions
// 	// 2. Update application status
// 	// 3. Return error if any
// 	return nil
// }

// // ShortlistApplicant shortlists an applicant
// func (s *jobService) ShortlistApplicant(ctx context.Context, applicationID, reviewerID int64) error {
// 	// TODO: Implement shortlist applicant logic
// 	// 1. Check permissions
// 	// 2. Update application status to shortlisted
// 	// 3. Return error if any
// 	return nil
// }

// // RejectApplication rejects a job application
// func (s *jobService) RejectApplication(ctx context.Context, req *RejectApplicationRequest) error {
// 	// TODO: Implement reject application logic
// 	// 1. Check permissions
// 	// 2. Update application status to rejected
// 	// 3. Return error if any
// 	return nil
// }

// // AcceptApplication accepts a job application
// func (s *jobService) AcceptApplication(ctx context.Context, req *AcceptApplicationRequest) error {
// 	// TODO: Implement accept application logic
// 	// 1. Check permissions
// 	// 2. Update application status to accepted
// 	// 3. Return error if any
// 	return nil
// }

// // GetJobStats retrieves job statistics
// func (s *jobService) GetJobStats(ctx context.Context, employerID int64) (*JobStatsResponse, error) {
// 	// TODO: Implement get job stats logic
// 	// 1. Check permissions
// 	// 2. Call repository to get stats
// 	// 3. Return stats or error
// 	return nil, nil
// }

// // GetApplicationStats retrieves application statistics for a job
// func (s *jobService) GetApplicationStats(ctx context.Context, jobID int64) (*ApplicationStatsResponse, error) {
// 	// TODO: Implement get application stats logic
// 	// 1. Check permissions
// 	// 2. Call repository to get stats
// 	// 3. Return stats or error
// 	return nil, nil
// }

// func (s *jobService) HasUserApplied(ctx context.Context, jobID, userID int64) (bool, error) {
// 	return s.repo.HasUserApplied(ctx, jobID, userID)
// }

// func (s *jobService) ApplyForJob(ctx context.Context, app *models.JobApplication) error {
// 	return s.repo.ApplyForJob(ctx, app)
// }

// func (s *jobService) GetJobApplications(ctx context.Context, jobID int64) ([]models.JobApplication, error) {
// 	return s.repo.GetJobApplications(ctx, jobID)
// }
