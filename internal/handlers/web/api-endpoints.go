package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"log"
	"net/http"
	"strconv"
)

// ChatMessagesAPIHandler handles API requests for chat messages
func ChatMessagesAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Context().Value(userIDKey).(int)
	recipientIDStr := r.URL.Query().Get("recipient_id")

	if recipientIDStr == "" {
		http.Error(w, "recipient_id is required", http.StatusBadRequest)
		return
	}

	recipientID, err := strconv.Atoi(recipientIDStr)
	if err != nil {
		http.Error(w, "Invalid recipient_id", http.StatusBadRequest)
		return
	}

	messages, err := getChatMessages(userID, recipientID)
	if err != nil {
		log.Printf("Error fetching chat messages: %v", err)
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"messages": messages,
		"success":  true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// JobsAPIHandler handles API requests for jobs listing
func JobsAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Context().Value(userIDKey).(int)

	jobs, err := getAllJobsForAPI(userID)
	if err != nil {
		log.Printf("Error fetching jobs: %v", err)
		http.Error(w, "Failed to fetch jobs", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"jobs":    jobs,
		"success": true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MyJobsAPIHandler handles API requests for user's posted jobs
func MyJobsAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Context().Value(userIDKey).(int)

	jobs, err := getMyJobsForAPI(userID)
	if err != nil {
		log.Printf("Error fetching my jobs: %v", err)
		http.Error(w, "Failed to fetch jobs", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"jobs":    jobs,
		"success": true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MyApplicationsAPIHandler handles API requests for user's job applications
func MyApplicationsAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Context().Value(userIDKey).(int)

	applications, err := getMyApplicationsForAPI(userID)
	if err != nil {
		log.Printf("Error fetching my applications: %v", err)
		http.Error(w, "Failed to fetch applications", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"applications": applications,
		"success":      true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func getChatMessages(userID, recipientID int) ([]models.Message, error) {
	query := `
		SELECT id, sender_id, recipient_id, content, created_at, read_at
		FROM messages 
		WHERE (sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1)
		ORDER BY created_at ASC
		LIMIT 100`

	rows, err := database.DB.QueryContext(context.Background(), query, userID, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		var readAt sql.NullTime

		err := rows.Scan(&msg.ID, &msg.SenderID, &msg.RecipientID, &msg.Content, &msg.CreatedAt, &readAt)
		if err != nil {
			log.Printf("Error scanning message: %v", err)
			continue
		}

		if readAt.Valid {
			msg.ReadAt = &readAt.Time
		}

		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

func getAllJobsForAPI(userID int) ([]models.Job, error) {
	query := `
		SELECT j.id, j.title, j.description, j.location, j.employment_type, j.salary_range,
		       j.application_deadline, j.status, j.created_at, j.employer_id,
		       u.username as employer_username, u.company as employer_company,
		       COALESCE(app_count.count, 0) as applications_count,
		       CASE WHEN j.employer_id = $1 THEN true ELSE false END as is_owner,
		       CASE WHEN user_app.id IS NOT NULL THEN true ELSE false END as has_applied
		FROM jobs j
		LEFT JOIN users u ON j.employer_id = u.id
		LEFT JOIN (
			SELECT job_id, COUNT(*) as count 
			FROM job_applications 
			GROUP BY job_id
		) app_count ON j.id = app_count.job_id
		LEFT JOIN job_applications user_app ON j.id = user_app.job_id AND user_app.applicant_id = $1
		WHERE j.status = 'active'
		ORDER BY j.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var salaryRange, employerCompany sql.NullString

		err := rows.Scan(
			&job.ID, &job.Title, &job.Description, &job.Location, &job.EmploymentType,
			&salaryRange, &job.ApplicationDeadline, &job.Status, &job.CreatedAt,
			&job.EmployerID, &job.EmployerUsername, &employerCompany,
			&job.ApplicationsCount, &job.IsOwner, &job.HasApplied,
		)
		if err != nil {
			log.Printf("Error scanning job: %v", err)
			continue
		}

		if salaryRange.Valid {
			job.SalaryRange = &salaryRange.String
		}
		if employerCompany.Valid {
			job.EmployerCompany = &employerCompany.String
		}

		// Format dates
		job.CreatedAtHuman = utils.TimeAgo(job.CreatedAt)
		job.DeadlineHuman = job.ApplicationDeadline.Format("Jan 2, 2006")

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

func getMyJobsForAPI(userID int) ([]models.Job, error) {
	query := `
		SELECT j.id, j.title, j.description, j.location, j.employment_type, j.salary_range,
		       j.application_deadline, j.status, j.created_at, j.employer_id,
		       u.username as employer_username, u.company as employer_company,
		       COALESCE(app_count.count, 0) as applications_count
		FROM jobs j
		LEFT JOIN users u ON j.employer_id = u.id
		LEFT JOIN (
			SELECT job_id, COUNT(*) as count 
			FROM job_applications 
			GROUP BY job_id
		) app_count ON j.id = app_count.job_id
		WHERE j.employer_id = $1
		ORDER BY j.created_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var salaryRange, employerCompany sql.NullString

		err := rows.Scan(
			&job.ID, &job.Title, &job.Description, &job.Location, &job.EmploymentType,
			&salaryRange, &job.ApplicationDeadline, &job.Status, &job.CreatedAt,
			&job.EmployerID, &job.EmployerUsername, &employerCompany,
			&job.ApplicationsCount,
		)
		if err != nil {
			log.Printf("Error scanning job: %v", err)
			continue
		}

		if salaryRange.Valid {
			job.SalaryRange = &salaryRange.String
		}
		if employerCompany.Valid {
			job.EmployerCompany = &employerCompany.String
		}

		// Format dates
		job.CreatedAtHuman = utils.TimeAgo(job.CreatedAt)
		job.DeadlineHuman = job.ApplicationDeadline.Format("Jan 2, 2006")
		job.IsOwner = true // These are user's own jobs

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

func getMyApplicationsForAPI(userID int) ([]models.JobApplication, error) {
	query := `
		SELECT ja.id, ja.job_id, ja.status, ja.applied_at, ja.reviewed_at,
		       j.title as job_title,
		       u.username as employer_username, u.company as employer_company	
		FROM job_applications ja
		LEFT JOIN jobs j ON ja.job_id = j.id
		LEFT JOIN users u ON j.employer_id = u.id
		WHERE ja.applicant_id = $1
		ORDER BY ja.applied_at DESC`

	rows, err := database.DB.QueryContext(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []models.JobApplication
	for rows.Next() {
		var app models.JobApplication
		var reviewedAt sql.NullTime
		var employerCompany sql.NullString

		err := rows.Scan(
			&app.ID, &app.JobID, &app.Status, &app.AppliedAt, &reviewedAt,
			&app.JobTitle, &app.EmployerUsername, &employerCompany,
		)
		if err != nil {
			log.Printf("Error scanning application: %v", err)
			continue
		}

		if reviewedAt.Valid {
			app.ReviewedAt = &reviewedAt.Time
			app.ReviewedAtHuman = utils.TimeAgo(*app.ReviewedAt)
		}
		if employerCompany.Valid {
			app.EmployerCompany = &employerCompany.String
		}

		// Format dates
		app.AppliedAtHuman = utils.TimeAgo(app.AppliedAt)

		applications = append(applications, app)
	}

	return applications, rows.Err()
}
