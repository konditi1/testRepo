package web

import (
	"net/http"
	"evalhub/internal/services"
)

// JobHandlers contains all job-related HTTP handlers with their dependencies
type JobHandlers struct {
	jobService services.JobService
}

// NewJobHandlers creates a new instance of JobHandlers with the required services
func NewJobHandlers(js services.JobService) *JobHandlers {
	return &JobHandlers{
		jobService: js,
	}
}

// RegisterRoutes registers all job-related routes
func (h *JobHandlers) RegisterRoutes(mux *http.ServeMux, authMiddleware func(http.Handler) http.Handler) {
	// Job-related routes
	mux.Handle("/jobs", authMiddleware(http.HandlerFunc(h.JobsHandler)))
	mux.Handle("/create-job", authMiddleware(http.HandlerFunc(h.CreateJobHandler)))
	mux.Handle("/view-job", authMiddleware(http.HandlerFunc(h.ViewJobHandler)))
	mux.Handle("/apply-job", authMiddleware(http.HandlerFunc(h.ApplyJobHandler)))
	mux.Handle("/my-jobs", authMiddleware(http.HandlerFunc(h.MyJobsHandler)))
	mux.Handle("/my-applications", authMiddleware(http.HandlerFunc(h.MyApplicationsHandler)))
}
