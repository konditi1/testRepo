
// file: internal/handlers/web/jobs.go
package web

import (
	"context"
	"evalhub/internal/models"
	"evalhub/internal/services"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// JobsHandler handles the jobs listing page
func (h *JobHandlers) JobsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)

	// Use the service instead of direct database calls
	jobs, err := h.jobService.GetAllJobsWithDetails(r.Context(), int64(userID))
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving jobs: %v", err))
		return
	}

	user, err := getUserWithProfile(r.Context(), userID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(userID)}
	}

	data := map[string]interface{}{
		"Title":      "Jobs - EvalHub",
		"IsLoggedIn": true,
		"Username":   user.Username,
		"User":       user,
		"Jobs":       jobs,
	}

	err = templates.ExecuteTemplate(w, "jobs", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// CreateJobHandler handles job creation (GET for form, POST for submission)
func (h *JobHandlers) CreateJobHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDKey).(int)
	user, err := getUserWithProfile(r.Context(), userID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(userID)}
	}

	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title":      "Post a Job - EvalHub",
			"IsLoggedIn": true,
			"Username":   user.Username,
			"User":       user,
		}
		err = templates.ExecuteTemplate(w, "create-job", data)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}

	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			log.Printf("Failed to parse form: %v", err)
			http.Error(w, "Failed to process form data", http.StatusBadRequest)
			return
		}

		// Get form values
		title := r.FormValue("title")
		description := r.FormValue("description")
		requirements := r.FormValue("requirements")
		responsibilities := r.FormValue("responsibilities")
		location := r.FormValue("location")
		employmentType := r.FormValue("employment_type")
		salaryRange := r.FormValue("salary_range")
		deadlineStr := r.FormValue("application_deadline")

		// Validate required fields
		if title == "" || description == "" || location == "" || employmentType == "" || deadlineStr == "" {
			data := map[string]interface{}{
				"Title":      "Post a Job - EvalHub",
				"IsLoggedIn": true,
				"Username":   user.Username,
				"User":       user,
				"Error":      "All required fields must be filled",
				"FormData": map[string]string{
					"title": title, "description": description, "requirements": requirements,
					"responsibilities": responsibilities, "location": location,
					"employment_type": employmentType, "salary_range": salaryRange,
					"application_deadline": deadlineStr,
				},
			}
			templates.ExecuteTemplate(w, "create-job", data)
			return
		}

		// Parse deadline
		deadline, err := time.Parse("2006-01-02", deadlineStr)
		if err != nil || deadline.Before(time.Now()) {
			data := map[string]interface{}{
				"Title":      "Post a Job - EvalHub",
				"IsLoggedIn": true,
				"Username":   user.Username,
				"User":       user,
				"Error":      "Please enter a valid future date for application deadline",
				"FormData": map[string]string{
					"title": title, "description": description, "requirements": requirements,
					"responsibilities": responsibilities, "location": location,
					"employment_type": employmentType, "salary_range": salaryRange,
					"application_deadline": deadlineStr,
				},
			}
			templates.ExecuteTemplate(w, "create-job", data)
			return
		}

		// Create job request using service types
		req := &services.CreateJobRequest{
			EmployerID:          int64(userID),
			Title:               title,
			Description:         description,
			Requirements:        requirements,
			Location:            location,
			EmploymentType:      employmentType,
			ApplicationDeadline: &deadline,
		}

		job, err := h.jobService.CreateJob(r.Context(), req)
		if err != nil {
			log.Printf("Service error: %v", err)
			data := map[string]interface{}{
				"Title":      "Post a Job - EvalHub",
				"IsLoggedIn": true,
				"Username":   user.Username,
				"User":       user,
				"Error":      "Failed to post job. Please try again.",
				"FormData": map[string]string{
					"title": title, "description": description, "requirements": requirements,
					"responsibilities": responsibilities, "location": location,
					"employment_type": employmentType, "salary_range": salaryRange,
					"application_deadline": deadlineStr,
				},
			}
			templates.ExecuteTemplate(w, "create-job", data)
			return
		}

		// Add notification for job posting
		if job.ID > 0 {
			go NotifyJobPosted(int(job.ID), int(userID), title)
		}

		// Redirect to jobs page
		http.Redirect(w, r, "/jobs", http.StatusSeeOther)
	}
}

// ViewJobHandler handles viewing a specific job
func (h *JobHandlers) ViewJobHandler(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.URL.Query().Get("id")
	if jobIDStr == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("job ID is required"))
		return
	}

	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid job ID"))
		return
	}

	userID := int64(r.Context().Value(userIDKey).(int))

	job, err := h.jobService.GetJobByID(r.Context(), jobID, &userID)
	if err != nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("job not found: %v", err))
		return
	}

	var applications *models.PaginatedResponse[*models.JobApplication]
	if job.IsOwner {
		req := &services.GetJobApplicationsRequest{
			JobID:      jobID,
			EmployerID: userID,
			Pagination: models.PaginationParams{Limit: 50}, // Default limit
		}
		applications, err = h.jobService.GetJobApplications(r.Context(), req)
		if err != nil {
			log.Printf("Error fetching applications: %v", err)
		}
	}

	user, err := getUserWithProfile(r.Context(), int(userID))
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(int(userID))}
	}

	data := map[string]interface{}{
		"Title":        fmt.Sprintf("%s - Jobs", job.Title),
		"IsLoggedIn":   true,
		"Username":     user.Username,
		"User":         user,
		"Job":          job,
		"Applications": applications,
	}

	err = templates.ExecuteTemplate(w, "view-job", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// ApplyJobHandler handles job applications
func (h *JobHandlers) ApplyJobHandler(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.URL.Query().Get("id")
	if jobIDStr == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("job ID is required"))
		return
	}

	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid job ID"))
		return
	}

	userID := int64(r.Context().Value(userIDKey).(int))

	if r.Method == http.MethodGet {
		// Check if user already applied
		applied, err := h.jobService.HasUserApplied(r.Context(), jobID, userID)
		if err != nil || applied {
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("you have already applied to this job or error checking application status"))
			return
		}

		job, err := h.jobService.GetJobByID(r.Context(), jobID, &userID)
		if err != nil {
			RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("job not found"))
			return
		}

		user, err := getUserWithProfile(r.Context(), int(userID))
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
			user = &models.User{Username: getUsername(int(userID))}
		}

		data := map[string]interface{}{
			"Title":      fmt.Sprintf("Apply for %s", job.Title),
			"IsLoggedIn": true,
			"Username":   user.Username,
			"User":       user,
			"Job":        job,
		}

		err = templates.ExecuteTemplate(w, "apply-job", data)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}

	if r.Method == http.MethodPost {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		err := r.ParseMultipartForm(10 << 20) // 10MB limit
		if err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("failed to process form data"))
			return
		}

		coverLetter := r.FormValue("cover_letter")
		if coverLetter == "" {
			job, _ := h.jobService.GetJobByID(ctx, jobID, &userID)
			user, err := getUserWithProfile(r.Context(), int(userID))
			if err != nil {
				log.Printf("Error fetching user profile: %v", err)
				user = &models.User{Username: getUsername(int(userID))}
			}

			data := map[string]interface{}{
				"Title":      fmt.Sprintf("Apply for %s", job.Title),
				"IsLoggedIn": true,
				"Username":   user.Username,
				"User":       user,
				"Job":        job,
				"Error":      "Cover letter is required",
			}
			templates.ExecuteTemplate(w, "apply-job", data)
			return
		}

		// Create application request
		req := &services.ApplyForJobRequest{
			JobID:       jobID,
			UserID:      userID,
			CoverLetter: &coverLetter,
		}

		_, err = h.jobService.ApplyForJob(ctx, req)
		if err != nil {
			log.Printf("Error submitting application: %v", err)
			job, _ := h.jobService.GetJobByID(ctx, jobID, &userID)
			user, err := getUserWithProfile(r.Context(), int(userID))
			if err != nil {
				log.Printf("Error fetching user profile: %v", err)
				user = &models.User{Username: getUsername(int(userID))}
			}

			data := map[string]interface{}{
				"Title":      fmt.Sprintf("Apply for %s", job.Title),
				"IsLoggedIn": true,
				"Username":   user.Username,
				"User":       user,
				"Job":        job,
				"Error":      "Failed to submit application. Please try again.",
			}
			templates.ExecuteTemplate(w, "apply-job", data)
			return
		}

		// Redirect to job page with success message
		http.Redirect(w, r, fmt.Sprintf("/view-job?id=%d&applied=1", jobID), http.StatusSeeOther)
	}
}

// MyJobsHandler handles employer's posted jobs
func (h *JobHandlers) MyJobsHandler(w http.ResponseWriter, r *http.Request) {
	userID := int64(r.Context().Value(userIDKey).(int))

	req := &services.GetJobsByEmployerRequest{
		EmployerID: userID,
		Pagination: models.PaginationParams{Limit: 50}, // Default limit
	}

	result, err := h.jobService.GetJobsByEmployer(r.Context(), req)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving jobs: %v", err))
		return
	}

	// Convert to slice for template
	jobs := make([]models.Job, len(result.Data))
	for i, job := range result.Data {
		jobs[i] = *job
	}

	user, err := getUserWithProfile(r.Context(), int(userID))
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(int(userID))}
	}

	data := map[string]interface{}{
		"Title":      "My Posted Jobs - EvalHub",
		"IsLoggedIn": true,
		"Username":   user.Username,
		"User":       user,
		"Jobs":       jobs,
	}

	err = templates.ExecuteTemplate(w, "my-jobs", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// MyApplicationsHandler handles user's job applications
func (h *JobHandlers) MyApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	userID := int64(r.Context().Value(userIDKey).(int))

	req := &services.GetUserApplicationsRequest{
		UserID:     userID,
		Pagination: models.PaginationParams{Limit: 50}, // Default limit
	}

	result, err := h.jobService.GetUserApplications(r.Context(), req)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving applications: %v", err))
		return
	}

	// Convert to slice for template
	applications := make([]models.JobApplication, len(result.Data))
	for i, app := range result.Data {
		applications[i] = *app
	}

	user, err := getUserWithProfile(r.Context(), int(userID))
	if err != nil {	
		log.Printf("Error fetching user profile: %v", err)
		user = &models.User{Username: getUsername(int(userID))}
	}

	data := map[string]interface{}{
		"Title":        "My Applications - EvalHub",
		"IsLoggedIn":   true,
		"Username":     user.Username,
		"User":         user,
		"Applications": applications,
	}

	err = templates.ExecuteTemplate(w, "my-applications", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// Remove all the helper functions at the bottom of the file (GetAllJobsWithDetails, etc.)
// as they should now be handled by the service layer




// // file: internal/handlers/web/jobs.go
// package web

// import (
// 	"context"
// 	"evalhub/internal/models"
// 	"evalhub/internal/services"
// 	"fmt"
// 	"log"
// 	"net/http"
// )

// var jobService services.JobService // TODO: Wire up in main or router

// // JobsHandler handles the jobs listing page
// func JobsHandler(w http.ResponseWriter, r *http.Request) {
// 	userID := r.Context().Value(userIDKey).(int)

// 	jobs, err := jobService.GetAllJobsWithDetails(r.Context(), userID)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving jobs: %v", err))
// 		return
// 	}
// 	user, err := getUserWithProfile(userID)
// 	if err != nil {
// 		log.Printf("Error fetching user profile: %v", err)
// 		user = &models.User{Username: getUsername(userID)}
// 	}
// 	data := map[string]interface{}{
// 		"Title":      "Jobs - EvalHub",
// 		"IsLoggedIn": true,
// 		"Username":   user.Username,
// 		"User":       user,
// 		"Jobs":       jobs,
// 	}
// 	err = templates.ExecuteTemplate(w, "jobs", data)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 	}
// }

// // CreateJobHandler handles job creation (GET for form, POST for submission)
// func CreateJobHandler(w http.ResponseWriter, r *http.Request) {
// 	userID := r.Context().Value(userIDKey).(int)
// 	user, err := getUserWithProfile(userID)
// 	if err != nil {
// 		log.Printf("Error fetching user profile: %v", err)
// 		user = &models.User{Username: getUsername(userID)}
// 	}
// 	if r.Method == http.MethodGet {
// 		data := map[string]interface{}{
// 			"Title":      "Post a Job - EvalHub",
// 			"IsLoggedIn": true,
// 			"Username":   user.Username,
// 			"User":       user,
// 		}
// 		err = templates.ExecuteTemplate(w, "create-job", data)
// 		if err != nil {
// 			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 		}
// 		return
// 	}
// 	if r.Method == http.MethodPost {
// 		err := r.ParseForm()
// 		if err != nil {
// 			log.Printf("Failed to parse form: %v", err)
// 			http.Error(w, "Failed to process form data", http.StatusBadRequest)
// 			return
// 		}
// 		// Get form values (assume sanitization is handled in service or utils)
// 		title := r.FormValue("title")
// 		description := r.FormValue("description")
// 		requirements := r.FormValue("requirements")
// 		responsibilities := r.FormValue("responsibilities")
// 		location := r.FormValue("location")
// 		employmentType := r.FormValue("employment_type")
// 		salaryRange := r.FormValue("salary_range")
// 		deadlineStr := r.FormValue("application_deadline")
// 		// Validate required fields
// 		if title == "" || description == "" || location == "" || employmentType == "" || deadlineStr == "" {
// 			data := map[string]interface{}{
// 				"Title":      "Post a Job - EvalHub",
// 				"IsLoggedIn": true,
// 				"Username":   user.Username,
// 				"User":       user,
// 				"Error":      "All required fields must be filled",
// 				"FormData": map[string]string{
// 					"title": title, "description": description, "requirements": requirements,
// 					"responsibilities": responsibilities, "location": location,
// 					"employment_type": employmentType, "salary_range": salaryRange,
// 					"application_deadline": deadlineStr,
// 				},
// 			}
// 			templates.ExecuteTemplate(w, "create-job", data)
// 			return
// 		}
// 		// Parse deadline
// 		deadline, err := time.Parse("2006-01-02", deadlineStr)
// 		if err != nil || deadline.Before(time.Now()) {
// 			data := map[string]interface{}{
// 				"Title":      "Post a Job - EvalHub",
// 				"IsLoggedIn": true,
// 				"Username":   user.Username,
// 				"User":       user,
// 				"Error":      "Please enter a valid future date for application deadline",
// 				"FormData": map[string]string{
// 					"title": title, "description": description, "requirements": requirements,
// 					"responsibilities": responsibilities, "location": location,
// 					"employment_type": employmentType, "salary_range": salaryRange,
// 					"application_deadline": deadlineStr,
// 				},
// 			}
// 			templates.ExecuteTemplate(w, "create-job", data)
// 			return
// 		}
// 		// Create job struct
// 		job := &models.Job{
// 			EmployerID:      userID,
// 			Title:           title,
// 			Description:     description,
// 			Requirements:    requirements,
// 			Responsibilities: responsibilities,
// 			Location:        location,
// 			EmploymentType:  employmentType,
// 			SalaryRange:     salaryRange,
// 			ApplicationDeadline: deadline,
// 		}
// 		err = jobService.CreateJob(r.Context(), job)
// 		if err != nil {
// 			log.Printf("Database error: %v", err)
// 			data := map[string]interface{}{
// 				"Title":      "Post a Job - EvalHub",
// 				"IsLoggedIn": true,
// 				"Username":   user.Username,
// 				"User":       user,
// 				"Error":      "Failed to post job. Please try again.",
// 				"FormData": map[string]string{
// 					"title": title, "description": description, "requirements": requirements,
// 					"responsibilities": responsibilities, "location": location,
// 					"employment_type": employmentType, "salary_range": salaryRange,
// 					"application_deadline": deadlineStr,
// 				},
// 			}
// 			templates.ExecuteTemplate(w, "create-job", data)
// 			return
// 		}

// 		// Add notification for job posting
// 		if job.ID > 0 {
// 			go NotifyJobPosted(job.ID, userID, title)
// 		}

// 		// Redirect to jobs page
// 		http.Redirect(w, r, "/jobs", http.StatusSeeOther)
// 	}
// }

// // ViewJobHandler handles viewing a specific job
// func ViewJobHandler(w http.ResponseWriter, r *http.Request) {
// 	jobIDStr := r.URL.Query().Get("id")
// 	if jobIDStr == "" {
// 		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("job ID is required"))
// 		return
// 	}
// 	jobID, err := strconv.Atoi(jobIDStr)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid job ID"))
// 		return
// 	}
// 	userID := r.Context().Value(userIDKey).(int)
// 	job, err := jobService.GetJobByID(r.Context(), jobID, userID)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("job not found: %v", err))
// 		return
// 	}
// 	var applications []models.JobApplication
// 	if job.IsOwner {
// 		applications, err = jobService.GetJobApplications(r.Context(), jobID)
// 		if err != nil {
// 			log.Printf("Error fetching applications: %v", err)
// 		}
// 	}

// 	user, err := getUserWithProfile(userID)
// 	if err != nil {
// 		log.Printf("Error fetching user profile: %v", err)
// 		user = &models.User{Username: getUsername(userID)}
// 	}
// 	data := map[string]interface{}{
// 		"Title":        fmt.Sprintf("%s - Jobs", job.Title),
// 		"IsLoggedIn":   true,
// 		"Username":     user.Username,
// 		"User":         user,
// 		"Job":          job,
// 		"Applications": applications,
// 	}
// 	err = templates.ExecuteTemplate(w, "view-job", data)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 	}
// }

// // ApplyJobHandler handles job applications
// func ApplyJobHandler(w http.ResponseWriter, r *http.Request) {
// 	jobIDStr := r.URL.Query().Get("id")
// 	if jobIDStr == "" {
// 		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("job ID is required"))
// 		return
// 	}
// 	jobID, err := strconv.Atoi(jobIDStr)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("invalid job ID"))
// 		return
// 	}
// 	userID := r.Context().Value(userIDKey).(int)

// 	if r.Method == http.MethodGet {
// 		// Delegate duplicate application check to jobService
// 		applied, err := jobService.HasUserApplied(r.Context(), jobID, userID)
// 		if err != nil || applied {
// 			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("you have already applied to this job or error checking application status"))
// 			return
// 		}
// 		job, err := jobService.GetJobByID(r.Context(), jobID, userID)
// 		if err != nil {
// 			RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("job not found"))
// 			return
// 		}
// 		user, err := getUserWithProfile(userID)
// 		if err != nil {
// 			log.Printf("Error fetching user profile: %v", err)
// 			user = &models.User{Username: getUsername(userID)}
// 		}
// 		data := map[string]interface{}{
// 			"Title":      fmt.Sprintf("Apply for %s", job.Title),
// 			"IsLoggedIn": true,
// 			"Username":   user.Username,
// 			"User":       user,
// 			"Job":        job,
// 		}
// 		err = templates.ExecuteTemplate(w, "apply-job", data)
// 		if err != nil {
// 			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 		}
// 		return
// 	}
// 	if r.Method == http.MethodPost {
// 		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
// 		defer cancel()
// 		err := r.ParseMultipartForm(10 << 20) // 10MB limit
// 		if err != nil {
// 			log.Printf("Failed to parse multipart form: %v", err)
// 			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("failed to process form data"))
// 			return
// 		}
// 		coverLetter := r.FormValue("cover_letter")
// 		if coverLetter == "" {
// 			job, _ := jobService.GetJobByID(ctx, jobID, userID)
// 			user, err := getUserWithProfile(userID)
// 		if err != nil {
// 			log.Printf("Error fetching user profile: %v", err)
// 			user = &models.User{Username: getUsername(userID)}
// 		}
// 			data := map[string]interface{}{
// 				"Title":      fmt.Sprintf("Apply for %s", job.Title),
// 				"IsLoggedIn": true,
// 				"Username":   user.Username,
// 				"User":       user,
// 				"Job":        job,
// 				"Error":      "Cover letter is required",
// 			}
// 			templates.ExecuteTemplate(w, "apply-job", data)
// 			return
// 		}
// 		// TODO: Handle application letter upload and validation in service layer
// 		app := &models.JobApplication{
// 			JobID:       jobID,
// 			ApplicantID: userID,
// 			CoverLetter: coverLetter,
// 			// ApplicationLetterURL, ApplicationLetterPublicID, etc.
// 		}
// 		err = jobService.ApplyForJob(ctx, app)
// 		if err != nil {
// 			log.Printf("Error submitting application: %v", err)
// 			job, _ := jobService.GetJobByID(ctx, jobID, userID)
// 			user, err := getUserWithProfile(userID)
// 		if err != nil {
// 			log.Printf("Error fetching user profile: %v", err)
// 			user = &models.User{Username: getUsername(userID)}
// 		}
// 			data := map[string]interface{}{
// 				"Title":      fmt.Sprintf("Apply for %s", job.Title),
// 				"IsLoggedIn": true,
// 				"Username":   user.Username,
// 				"User":       user,
// 				"Job":        job,
// 				"Error":      "Failed to submit application. Please try again.",
// 			}
// 			templates.ExecuteTemplate(w, "apply-job", data)
// 			return
// 		}
// 		// TODO: Add notification for job application in service layer
// 		// Redirect to job page with success message
// 		http.Redirect(w, r, fmt.Sprintf("/view-job?id=%d&applied=1", jobID), http.StatusSeeOther)
// 	}
// }

// // MyJobsHandler handles employer's posted jobs
// func MyJobsHandler(w http.ResponseWriter, r *http.Request) {
// 	userID := r.Context().Value(userIDKey).(int)

// 	jobs, err := GetJobsByEmployer(userID)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving jobs: %v", err))
// 		return
// 	}
// 	user, err := getUserWithProfile(userID)
// 	if err != nil {
// 		log.Printf("Error fetching user profile: %v", err)
// 		user = &models.User{Username: getUsername(userID)}
// 	}
// 	data := map[string]interface{}{
// 		"Title":      "My Posted Jobs - EvalHub",
// 		"IsLoggedIn": true,
// 		"Username":   user.Username,
// 		"User":       user,
// 		"Jobs":       jobs,
// 	}
// 	err = templates.ExecuteTemplate(w, "my-jobs", data)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 	}
// }

// // MyApplicationsHandler handles user's job applications
// func MyApplicationsHandler(w http.ResponseWriter, r *http.Request) {
// 	userID := r.Context().Value(userIDKey).(int)

// 	applications, err := GetApplicationsByUser(userID)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving applications: %v", err))
// 		return
// 	}
// 	user, err := getUserWithProfile(userID)
// 	if err != nil {
// 		log.Printf("Error fetching user profile: %v", err)
// 		user = &models.User{Username: getUsername(userID)}
// 	}
// 	data := map[string]interface{}{
// 		"Title":        "My Applications - EvalHub",
// 		"IsLoggedIn":   true,
// 		"Username":     user.Username,
// 		"User":         user,
// 		"Applications": applications,
// 	}
// 	err = templates.ExecuteTemplate(w, "my-applications", data)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
// 	}
// }

// // Helper functions
// // GetAllJobsWithDetails retrieves all active jobs with details
// func GetAllJobsWithDetails(currentUserID int) ([]models.Job, error) {
// 	query := `
// 		SELECT j.id, j.employer_id, j.title, j.description, j.location, 
// 		       j.employment_type, j.salary_range, j.application_deadline, 
// 		       j.status, j.created_at, u.username, u.affiliation,
// 		       COUNT(ja.id) as applications_count,
// 		       EXISTS(SELECT 1 FROM job_applications WHERE job_id = j.id AND applicant_id = $1) as has_applied
// 		FROM jobs j
// 		JOIN users u ON j.employer_id = u.id
// 		LEFT JOIN job_applications ja ON j.id = ja.job_id
// 		WHERE j.status = 'active' AND j.application_deadline > CURRENT_DATE
// 		GROUP BY j.id, u.username, u.affiliation
// 		ORDER BY j.created_at DESC`
// 	rows, err := database.DB.Query(query, currentUserID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()
// 	var jobs []models.Job
// 	for rows.Next() {
// 		var job models.Job
// 		var affiliation sql.NullString

// 		err := rows.Scan(&job.ID, &job.EmployerID, &job.Title, &job.Description,
// 			&job.Location, &job.EmploymentType, &job.SalaryRange, &job.ApplicationDeadline,
// 			&job.Status, &job.CreatedAt, &job.EmployerUsername, &affiliation,
// 			&job.ApplicationsCount, &job.HasApplied)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if affiliation.Valid {
// 			job.EmployerCompany = affiliation.String
// 		}
// 		job.CreatedAtHuman = utility.TimeAgo(job.CreatedAt)
// 		job.DeadlineHuman = job.ApplicationDeadline.Format("January 2, 2006")
// 		job.IsOwner = (job.EmployerID == currentUserID)
// 		jobs = append(jobs, job)
// 	}
// 	return jobs, nil
// }

// // GetJobByID retrieves a specific job by ID
// func GetJobByID(jobID, currentUserID int) (*models.Job, error) {
// 	query := `
// 		SELECT j.id, j.employer_id, j.title, j.description, j.requirements,
// 		       j.responsibilities, j.location, j.employment_type, j.salary_range,
// 		       j.application_deadline, j.status, j.created_at, j.updated_at,
// 		       u.username, u.email, u.affiliation,
// 		       COUNT(ja.id) as applications_count,
// 		       EXISTS(SELECT 1 FROM job_applications WHERE job_id = j.id AND applicant_id = $2) as has_applied
// 		FROM jobs j
// 		JOIN users u ON j.employer_id = u.id
// 		LEFT JOIN job_applications ja ON j.id = ja.job_id
// 		WHERE j.id = $1
// 		GROUP BY j.id, u.username, u.email, u.affiliation`
// 	row := database.DB.QueryRow(query, jobID, currentUserID)

// 	var job models.Job
// 	var affiliation sql.NullString
// 	var requirements, responsibilities sql.NullString

// 	err := row.Scan(&job.ID, &job.EmployerID, &job.Title, &job.Description,
// 		&requirements, &responsibilities, &job.Location, &job.EmploymentType,
// 		&job.SalaryRange, &job.ApplicationDeadline, &job.Status, &job.CreatedAt,
// 		&job.UpdatedAt, &job.EmployerUsername, &job.EmployerEmail, &affiliation,
// 		&job.ApplicationsCount, &job.HasApplied)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if requirements.Valid {
// 		job.Requirements = requirements.String
// 	}
// 	if responsibilities.Valid {
// 		job.Responsibilities = responsibilities.String
// 	}
// 	if affiliation.Valid {
// 		job.EmployerCompany = affiliation.String
// 	}
// 	job.CreatedAtHuman = utility.TimeAgo(job.CreatedAt)
// 	job.DeadlineHuman = job.ApplicationDeadline.Format("January 2, 2006")
// 	job.IsOwner = (job.EmployerID == currentUserID)
// 	return &job, nil
// }

// // GetJobsByEmployer retrieves jobs posted by a specific employer
// func GetJobsByEmployer(employerID int) ([]models.Job, error) {
// 	query := `
// 		SELECT j.id, j.employer_id, j.title, j.description, j.location,
// 		       j.employment_type, j.salary_range, j.application_deadline,
// 		       j.status, j.created_at, COUNT(ja.id) as applications_count
// 		FROM jobs j
// 		LEFT JOIN job_applications ja ON j.id = ja.job_id
// 		WHERE j.employer_id = $1
// 		GROUP BY j.id
// 		ORDER BY j.created_at DESC`
// 	rows, err := database.DB.Query(query, employerID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()
// 	var jobs []models.Job
// 	for rows.Next() {
// 		var job models.Job
// 		err := rows.Scan(&job.ID, &job.EmployerID, &job.Title, &job.Description,
// 			&job.Location, &job.EmploymentType, &job.SalaryRange, &job.ApplicationDeadline,
// 			&job.Status, &job.CreatedAt, &job.ApplicationsCount)
// 		if err != nil {
// 			return nil, err
// 		}
// 		job.CreatedAtHuman = utility.TimeAgo(job.CreatedAt)
// 		job.DeadlineHuman = job.ApplicationDeadline.Format("January 2, 2006")
// 		job.IsOwner = true
// 		jobs = append(jobs, job)
// 	}
// 	return jobs, nil
// }

// // GetJobApplications retrieves applications for a specific job
// func GetJobApplications(jobID int) ([]models.JobApplication, error) {
// 	query := `
// 		SELECT ja.id, ja.job_id, ja.applicant_id, ja.cover_letter,
// 		       ja.application_letter_url, ja.status, ja.applied_at,
// 		       ja.reviewed_at, ja.notes, u.username, u.email,
// 		       u.first_name, u.last_name, u.cv_url
// 		FROM job_applications ja
// 		JOIN users u ON ja.applicant_id = u.id
// 		WHERE ja.job_id = $1
// 		ORDER BY ja.applied_at DESC`
// 	rows, err := database.DB.Query(query, jobID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()
// 	var applications []models.JobApplication
// 	for rows.Next() {
// 		var app models.JobApplication
// 		var firstName, lastName sql.NullString
// 		var reviewedAt sql.NullTime
// 		var notes, appLetterURL, cvURL sql.NullString
// 		err := rows.Scan(&app.ID, &app.JobID, &app.ApplicantID, &app.CoverLetter,
// 			&appLetterURL, &app.Status, &app.AppliedAt, &reviewedAt, &notes,
// 			&app.ApplicantUsername, &app.ApplicantEmail, &firstName, &lastName, &cvURL)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if firstName.Valid && lastName.Valid {
// 			app.ApplicantName = firstName.String + " " + lastName.String
// 		} else {
// 			app.ApplicantName = app.ApplicantUsername
// 		}
// 		if appLetterURL.Valid {
// 			app.ApplicationLetterURL = appLetterURL.String
// 		}
// 		if reviewedAt.Valid {
// 			app.ReviewedAt = reviewedAt.Time
// 			app.ReviewedAtHuman = utility.TimeAgo(app.ReviewedAt)
// 		}
// 		if notes.Valid {
// 			app.Notes = notes.String
// 		}
// 		if cvURL.Valid {
// 			app.ApplicantCVURL = cvURL.String
// 		}
// 		app.AppliedAtHuman = utility.TimeAgo(app.AppliedAt)
// 		applications = append(applications, app)
// 	}
// 	return applications, nil
// }

// // GetApplicationsByUser retrieves applications submitted by a specific user
// func GetApplicationsByUser(userID int) ([]models.JobApplication, error) {
// 	query := `
// 		SELECT ja.id, ja.job_id, ja.applicant_id, ja.status, ja.applied_at,
// 		       ja.reviewed_at, j.title, u.username, u.affiliation
// 		FROM job_applications ja
// 		JOIN jobs j ON ja.job_id = j.id
// 		JOIN users u ON j.employer_id = u.id
// 		WHERE ja.applicant_id = $1
// 		ORDER BY ja.applied_at DESC`
// 	rows, err := database.DB.Query(query, userID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()
// 	var applications []models.JobApplication
// 	for rows.Next() {
// 		var app models.JobApplication
// 		var reviewedAt sql.NullTime
// 		var employerCompany sql.NullString
// 		err := rows.Scan(&app.ID, &app.JobID, &app.ApplicantID, &app.Status,
// 			&app.AppliedAt, &reviewedAt, &app.JobTitle, &app.EmployerUsername, &employerCompany)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if reviewedAt.Valid {
// 			app.ReviewedAt = reviewedAt.Time
// 			app.ReviewedAtHuman = utility.TimeAgo(app.ReviewedAt)
// 		}
// 		if employerCompany.Valid {
// 			app.EmployerCompany = employerCompany.String
// 		}
// 		app.AppliedAtHuman = utility.TimeAgo(app.AppliedAt)
// 		applications = append(applications, app)
// 	}
// 	return applications, nil
// }
