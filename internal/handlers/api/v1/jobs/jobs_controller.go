// file: internal/handlers/api/v1/jobs/jobs_controller.go
package jobs

import (
	"encoding/json"
	"evalhub/internal/models"
	"evalhub/internal/response"
	"evalhub/internal/services"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

type JobController struct {
	serviceCollection *services.ServiceCollection
	logger            *zap.Logger
	responseBuilder   *response.Builder
}

// NewJobController creates a new job controller
func NewJobController(serviceCollection *services.ServiceCollection, logger *zap.Logger, responseBuilder *response.Builder) *JobController {
	return &JobController{
		serviceCollection: serviceCollection,
		logger:            logger,
		responseBuilder:   responseBuilder,
	}
}

// CreateJob handles job creation
func (c *JobController) CreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	var req services.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.QuickError(w, r, services.NewValidationError("invalid request body", err))
		return
	}

	req.EmployerID = userID

	job, err := c.serviceCollection.JobService.CreateJob(r.Context(), &req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, job)
}

// ListJobs handles job listing
func (c *JobController) ListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	var userPtr *int64
	if userID != 0 {
		userPtr = &userID
	}

	req := &services.ListJobsRequest{
		Pagination:     c.getPaginationParams(r),
		UserID:         userPtr,
		Location:       c.getQueryParam(r, "location"),
		EmploymentType: c.getQueryParam(r, "employment_type"),
		SortBy:         c.getQueryParam(r, "sort_by"),
		SortOrder:      c.getQueryParam(r, "sort_order"),
	}

	// Parse remote filter
	if remoteStr := r.URL.Query().Get("remote"); remoteStr != "" {
		if remote, err := strconv.ParseBool(remoteStr); err == nil {
			req.Remote = &remote
		}
	}

	// Parse salary range
	if salaryMinStr := r.URL.Query().Get("salary_min"); salaryMinStr != "" {
		if salaryMin, err := strconv.Atoi(salaryMinStr); err == nil {
			req.SalaryMin = &salaryMin
		}
	}

	if salaryMaxStr := r.URL.Query().Get("salary_max"); salaryMaxStr != "" {
		if salaryMax, err := strconv.Atoi(salaryMaxStr); err == nil {
			req.SalaryMax = &salaryMax
		}
	}

	// Parse skills
	if skillsStr := r.URL.Query().Get("skills"); skillsStr != "" {
		req.Skills = strings.Split(skillsStr, ",")
	}

	jobs, err := c.serviceCollection.JobService.ListJobs(r.Context(), req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, jobs)
}

// GetJob handles retrieving a specific job
func (c *JobController) GetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	jobID := c.getJobIDFromPath(r)
	if jobID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid job ID", nil))
		return
	}

	userID := c.getUserID(r)
	var userPtr *int64
	if userID != 0 {
		userPtr = &userID
	}

	job, err := c.serviceCollection.JobService.GetJobByID(r.Context(), jobID, userPtr)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, job)
}

// UpdateJob handles job updates
func (c *JobController) UpdateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	jobID := c.getJobIDFromPath(r)
	if jobID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid job ID", nil))
		return
	}

	var req services.UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.QuickError(w, r, services.NewValidationError("invalid request body", err))
		return
	}

	req.JobID = jobID
	req.EmployerID = userID

	job, err := c.serviceCollection.JobService.UpdateJob(r.Context(), &req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, job)
}

// DeleteJob handles job deletion
func (c *JobController) DeleteJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	jobID := c.getJobIDFromPath(r)
	if jobID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid job ID", nil))
		return
	}

	err := c.serviceCollection.JobService.DeleteJob(r.Context(), jobID, userID)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, map[string]string{"message": "Job deleted successfully"})
}

// SearchJobs handles job search
func (c *JobController) SearchJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		response.QuickError(w, r, services.NewValidationError("search query is required", nil))
		return
	}

	userID := c.getUserID(r)
	var userPtr *int64
	if userID != 0 {
		userPtr = &userID
	}

	req := &services.SearchJobsRequest{
		Query:          query,
		UserID:         userPtr,
		Pagination:     c.getPaginationParams(r),
		Location:       c.getQueryParam(r, "location"),
		EmploymentType: c.getQueryParam(r, "employment_type"),
	}

	// Parse other filters similar to ListJobs
	if remoteStr := r.URL.Query().Get("remote"); remoteStr != "" {
		if remote, err := strconv.ParseBool(remoteStr); err == nil {
			req.Remote = &remote
		}
	}

	if skillsStr := r.URL.Query().Get("skills"); skillsStr != "" {
		req.Skills = strings.Split(skillsStr, ",")
	}

	jobs, err := c.serviceCollection.JobService.SearchJobs(r.Context(), req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, jobs)
}

// GetJobsByEmployer handles getting jobs by employer
func (c *JobController) GetJobsByEmployer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	employerID := c.getEmployerIDFromPath(r)
	if employerID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid employer ID", nil))
		return
	}

	req := &services.GetJobsByEmployerRequest{
		EmployerID: employerID,
		Pagination: c.getPaginationParams(r),
		Status:     c.getQueryParam(r, "status"),
	}

	jobs, err := c.serviceCollection.JobService.GetJobsByEmployer(r.Context(), req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, jobs)
}

// GetFeaturedJobs handles getting featured jobs
func (c *JobController) GetFeaturedJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	userID := c.getUserID(r)
	var userPtr *int64
	if userID != 0 {
		userPtr = &userID
	}

	jobs, err := c.serviceCollection.JobService.GetFeaturedJobs(r.Context(), limit, userPtr)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, jobs)
}

// ApplyForJob handles job applications
func (c *JobController) ApplyForJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	jobID := c.getJobIDFromPath(r)
	if jobID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid job ID", nil))
		return
	}

	var req services.ApplyForJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.QuickError(w, r, services.NewValidationError("invalid request body", err))
		return
	}

	req.JobID = jobID
	req.UserID = userID

	application, err := c.serviceCollection.JobService.ApplyForJob(r.Context(), &req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, application)
}

// GetJobApplications handles getting applications for a job
func (c *JobController) GetJobApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	jobID := c.getJobIDFromPath(r)
	if jobID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid job ID", nil))
		return
	}

	req := &services.GetJobApplicationsRequest{
		JobID:      jobID,
		EmployerID: userID,
		Pagination: c.getPaginationParams(r),
		Status:     c.getQueryParam(r, "status"),
	}

	applications, err := c.serviceCollection.JobService.GetJobApplications(r.Context(), req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, applications)
}

// GetUserApplications handles getting user's applications
func (c *JobController) GetUserApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	req := &services.GetUserApplicationsRequest{
		UserID:     userID,
		Pagination: c.getPaginationParams(r),
		Status:     c.getQueryParam(r, "status"),
	}

	applications, err := c.serviceCollection.JobService.GetUserApplications(r.Context(), req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, applications)
}

// ReviewApplication handles reviewing applications
func (c *JobController) ReviewApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	applicationID := c.getApplicationIDFromPath(r)
	if applicationID == 0 {
		response.QuickError(w, r, services.NewValidationError("invalid application ID", nil))
		return
	}

	var req services.ReviewApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.QuickError(w, r, services.NewValidationError("invalid request body", err))
		return
	}

	req.ApplicationID = applicationID
	req.ReviewerID = userID

	err := c.serviceCollection.JobService.ReviewApplication(r.Context(), &req)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, map[string]string{"message": "Application reviewed successfully"})
}

// GetJobStats handles getting job statistics
func (c *JobController) GetJobStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID := c.getUserID(r)
	if userID == 0 {
		response.QuickError(w, r, services.NewUnauthorizedError("user not authenticated"))
		return
	}

	stats, err := c.serviceCollection.JobService.GetJobStats(r.Context(), userID)
	if err != nil {
		response.QuickError(w, r, err)
		return
	}

	response.QuickSuccess(w, r, stats)
}

// Helper methods
func (c *JobController) getUserID(r *http.Request) int64 {
	if userID := r.Context().Value("user_id"); userID != nil {
		if id, ok := userID.(int64); ok {
			return id
		}
	}
	return 0
}

func (c *JobController) getJobIDFromPath(r *http.Request) int64 {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) >= 4 {
		if id, err := strconv.ParseInt(pathParts[3], 10, 64); err == nil {
			return id
		}
	}
	return 0
}

func (c *JobController) getEmployerIDFromPath(r *http.Request) int64 {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) >= 6 && pathParts[3] == "employer" {
		if id, err := strconv.ParseInt(pathParts[4], 10, 64); err == nil {
			return id
		}
	}
	return 0
}

func (c *JobController) getApplicationIDFromPath(r *http.Request) int64 {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) >= 6 && pathParts[4] == "applications" {
		if id, err := strconv.ParseInt(pathParts[5], 10, 64); err == nil {
			return id
		}
	}
	return 0
}

func (c *JobController) getPaginationParams(r *http.Request) models.PaginationParams {
	params := models.PaginationParams{
		Limit: 20, // Default limit
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			params.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			params.Offset = offset
		}
	}

	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		params.Cursor = cursor
	}

	return params
}

func (c *JobController) getQueryParam(r *http.Request, key string) *string {
	if value := r.URL.Query().Get(key); value != "" {
		return &value
	}
	return nil
}
