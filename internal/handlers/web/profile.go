package web

import (
	"context"
	"database/sql"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ProfileHandler handles profile management (view and update)
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(int)
	if !ok {
		RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	if r.Method == http.MethodGet {
		// Fetch user details
		user, err := getUserByID(userID)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
			return
		}

		data := map[string]interface{}{
			"Title":      "Manage Profile - EvalHub",
			"IsLoggedIn": true,
			"Username":   getUsername(userID),
			"User":       user,
		}
		err = templates.ExecuteTemplate(w, "profile", data)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
		}
		return
	}

	if r.Method == http.MethodPost {
		// Create a context for the update process
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		// Parse multipart form (max 32MB)
		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
			user, err := getUserByID(userID) // Re-fetch user to repopulate form
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
				return
			}
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      "Failed to process form data. Please ensure files are valid and under size limits (Profile: 10MB, CV: 5MB).",
				"User":       user,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Retrieve form values
		email := utils.SanitizeString(r.FormValue("email"))
		username := utils.SanitizeString(r.FormValue("username"))
		firstName := utils.SanitizeString(r.FormValue("first_name"))
		lastName := utils.SanitizeString(r.FormValue("last_name"))
		affiliation := utils.SanitizeString(r.FormValue("affiliation"))
		bio := utils.SanitizeString(r.FormValue("bio"))
		yearsExperienceStr := utils.SanitizeString(r.FormValue("years_experience"))
		coreCompetencies := utils.SanitizeString(r.FormValue("core_competencies"))
		expertise := utils.SanitizeString(r.FormValue("expertise"))

		// Validate required fields
		if email == "" || username == "" || firstName == "" || lastName == "" || expertise == "" {
			user, err := getUserByID(userID)
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
				return
			}
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      "All required fields must be filled",
				"User":       user,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Validate years of experience
		var yearsExperience int
		if yearsExperienceStr != "" {
			yearsExperience, err = strconv.Atoi(yearsExperienceStr)
			if err != nil || yearsExperience < 0 || yearsExperience > 50 {
				user, err := getUserByID(userID)
				if err != nil {
					RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
					return
				}
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      "Years of experience must be a number between 0 and 50",
					"User":       user,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}
		}

		// Validate core competencies
		if coreCompetencies != "" {
			competencies := strings.Split(coreCompetencies, ",")
			for _, comp := range competencies {
				comp = strings.TrimSpace(comp)
				if len(comp) < 2 {
					user, err := getUserByID(userID)
					if err != nil {
						RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
						return
					}
					data := map[string]interface{}{
						"Title":      "Manage Profile - EvalHub",
						"IsLoggedIn": true,
						"Username":   getUsername(userID),
						"Error":      "Each core competency must be at least 2 characters long",
						"User":       user,
					}
					templates.ExecuteTemplate(w, "profile", data)
					return
				}
			}
			coreCompetencies = strings.Join(competencies, ",")
		}

		// Validate email
		if err := utils.ValidateEmail(email); err != nil {
			validationErr := err
			
			// Now get the user
			userData, err := getUserByID(userID)
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
				return
			}

			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      validationErr.Error(),
				"User":       userData,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Fetch current user data to retrieve existing files if not uploaded new ones
		currentUser, err := getUserByID(userID)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
			return
		}

		// Initialize Cloudinary service
		cloudinary, err := utils.GetCloudinaryService()
		if err != nil {
			log.Printf("Failed to initialize Cloudinary service: %v", err)
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      "Failed to initialize file upload service",
				"User":       currentUser,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Variables to store Cloudinary URLs and public IDs
		profileURL := currentUser.ProfileURL
		profilePublicID := currentUser.ProfilePublicID
		cvURL := currentUser.CVURL
		cvPublicID := currentUser.CVPublicID

		// Handle profile picture upload
		file, handler, err := r.FormFile("profile")
		if err == nil {
			defer file.Close()

			// Validate file
			if err := cloudinary.ValidateFile(ctx, handler); err != nil {
				log.Printf("Profile image validation failed: %v", err)
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      fmt.Sprintf("Profile picture validation failed: %v", err),
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Additional image-specific validation
			if !strings.HasPrefix(handler.Header.Get("Content-Type"), "image/") {
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      "Profile picture must be an image file",
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Generate a unique folder path for the profile image
			uploadFolder := fmt.Sprintf("evalhub/profiles/%s", username)

			// Upload to Cloudinary
			uploadResult, err := cloudinary.UploadFile(ctx, handler, uploadFolder)
			if err != nil {
				log.Printf("Failed to upload profile image to Cloudinary: %v", err)
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      "Failed to upload profile picture",
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Delete old profile picture from Cloudinary if it exists
			if currentUser.ProfilePublicID != nil && *currentUser.ProfilePublicID != "" {
				if delErr := cloudinary.DeleteFile(ctx, *currentUser.ProfilePublicID); delErr != nil {
					log.Printf("Warning: Failed to delete old profile image: %v", delErr)
				} else {
					log.Printf("Successfully deleted old profile image: %s", *currentUser.ProfilePublicID)
				}
			}

			// Update profile URL and public ID
			profileURL = &uploadResult.URL
			profilePublicID = &uploadResult.PublicID

			log.Printf("Profile image successfully uploaded to Cloudinary. URL: %s, Public ID: %s", *profileURL, *profilePublicID)
		} else if err != http.ErrMissingFile {
			log.Printf("Error retrieving profile picture: %v", err)
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      "Error processing profile picture",
				"User":       currentUser,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Handle CV upload
		file, handler, err = r.FormFile("cv")
		if err == nil {
			defer file.Close()

			// Validate file
			if err := cloudinary.ValidateFile(ctx, handler); err != nil {
				log.Printf("CV validation failed: %v", err)
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      fmt.Sprintf("CV validation failed: %v", err),
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Additional PDF-specific validation
			if handler.Header.Get("Content-Type") != "application/pdf" {
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      "CV must be a PDF file",
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Generate a unique folder path for the CV
			uploadFolder := fmt.Sprintf("evalhub/cvs/%s", username)

			// Upload to Cloudinary
			uploadResult, err := cloudinary.UploadFile(ctx, handler, uploadFolder)
			if err != nil {
				log.Printf("Failed to upload CV to Cloudinary: %v", err)
				data := map[string]interface{}{
					"Title":      "Manage Profile - EvalHub",
					"IsLoggedIn": true,
					"Username":   getUsername(userID),
					"Error":      "Failed to upload CV",
					"User":       currentUser,
				}
				templates.ExecuteTemplate(w, "profile", data)
				return
			}

			// Delete old CV from Cloudinary if it exists
			if currentUser.CVPublicID != nil && *currentUser.CVPublicID != "" {
				if delErr := cloudinary.DeleteFile(ctx, *currentUser.CVPublicID); delErr != nil {
					log.Printf("Warning: Failed to delete old CV: %v", delErr)
				} else {
					log.Printf("Successfully deleted old CV: %s", *currentUser.CVPublicID)
				}
			}

			// Update CV URL and public ID
			if uploadResult.URL != "" {
				cvURL = &uploadResult.URL
				cvPublicID = &uploadResult.PublicID
			}

			log.Printf("CV successfully uploaded to Cloudinary. URL: %s, Public ID: %s", safeDerefString(cvURL), safeDerefString(cvPublicID))
		} else if err != http.ErrMissingFile {
			log.Printf("Error retrieving CV: %v", err)
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      "Error processing CV",
				"User":       currentUser,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Update user in database with Cloudinary URLs and public IDs
		query := `
			UPDATE users 
			SET email = $1, username = $2, first_name = $3, last_name = $4, 
				affiliation = $5, bio = $6, years_experience = $7, 
				core_competencies = $8, expertise = $9, 
				profile_url = $10, profile_public_id = $11,
				cv_url = $12, cv_public_id = $13
			WHERE id = $14`
		_, err = database.DB.ExecContext(r.Context(), query, email, username, firstName, lastName,
			affiliation, bio, yearsExperience, coreCompetencies, expertise,
			profileURL, profilePublicID, cvURL, cvPublicID, userID)
		if err != nil {
			log.Printf("Database error: %v", err)
			var errorMessage string
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				if strings.Contains(err.Error(), "email") {
					errorMessage = "This email is already registered"
				} else {
					errorMessage = "This username is already taken"
				}
			} else {
				errorMessage = "An error occurred while updating profile"
			}
			user, err := getUserByID(userID)
			if err != nil {
				RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
				return
			}
			data := map[string]interface{}{
				"Title":      "Manage Profile - EvalHub",
				"IsLoggedIn": true,
				"Username":   getUsername(userID),
				"Error":      errorMessage,
				"User":       user,
			}
			templates.ExecuteTemplate(w, "profile", data)
			return
		}

		// Render success message
		user, err := getUserByID(userID)
		if err != nil {
			RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error retrieving user details: %v", err))
			return
		}
		data := map[string]interface{}{
			"Title":      "Manage Profile - EvalHub",
			"IsLoggedIn": true,
			"Username":   getUsername(userID),
			"Success":    "Profile updated successfully",
			"User":       user,
		}
		templates.ExecuteTemplate(w, "profile", data)
	}
}

// ViewProfileHandler handles public profile viewing
func ViewProfileHandler(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("username parameter is required"))
		return
	}

	// Fetch user by username
	user, err := getUserByUsername(username)
	if err != nil || user == nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("user not found %v", err))
		return
	}

	// Check if viewer is the profile owner
	userID, _ := r.Context().Value(userIDKey).(int)
	isOwnProfile := userID == int(user.ID)

	data := map[string]interface{}{
		"Title":        fmt.Sprintf("%s's Profile - EvalHub", user.Username),
		"IsLoggedIn":   userID != 0,
		"Username":     getUsername(userID),
		"User":         user,
		"IsOwnProfile": isOwnProfile,
	}
	err = templates.ExecuteTemplate(w, "view-profile", data)
	if err != nil {
		RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("error rendering template: %v", err))
	}
}

// DownloadCVHandler handles CV downloads
func DownloadCVHandler(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("username parameter is required"))
		return
	}

	// Fetch user by username
	user, err := getUserByUsername(username)
	if err != nil || user == nil {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("user not found"))
		return
	}

	if user.CVURL == nil || *user.CVURL == "" {
		RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("no CV available for this user"))
		return
	}

	// Redirect to the Cloudinary CV URL with headers to prompt download
	if user.CVURL == nil {
		http.Error(w, "CV not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_cv.pdf", user.Username))
	http.Redirect(w, r, *user.CVURL, http.StatusFound)
}

// getUserByID fetches user details by ID
func getUserByID(userID int) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, 
		       profile_url, profile_public_id, affiliation, bio, years_experience, 
		       cv_url, cv_public_id, core_competencies, expertise, role
		FROM users WHERE id = $1`
	row := database.DB.QueryRowContext(context.Background(), query, userID)

	user := &models.User{}
	var (
		profileURL, profilePublicID, affiliation, bio, cvURL, cvPublicID, coreCompetencies sql.NullString
		firstName, lastName                                                                sql.NullString
	)

	err := row.Scan(&user.ID, &user.Email, &user.Username, &firstName, &lastName,
		&profileURL, &profilePublicID, &affiliation, &bio, &user.YearsExperience,
		&cvURL, &cvPublicID, &coreCompetencies, &user.Expertise, &user.Role)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}

	// Handle NULLs
	if firstName.Valid {
		user.FirstName = &firstName.String
	}
	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if profileURL.Valid {
		user.ProfileURL = &profileURL.String
	}
	if profilePublicID.Valid {
		user.ProfilePublicID = &profilePublicID.String
	}
	if affiliation.Valid {
		user.Affiliation = &affiliation.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	if cvURL.Valid {
		user.CVURL = &cvURL.String
	}
	if cvPublicID.Valid {
		user.CVPublicID = &cvPublicID.String
	}
	if coreCompetencies.Valid {
		user.CoreCompetencies = &coreCompetencies.String
	}

	return user, nil
}

// safeDerefString safely dereferences a string pointer, returning an empty string if the pointer is nil
func safeDerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// getUserByUsername fetches user details by username
func getUserByUsername(username string) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, 
		       profile_url, profile_public_id, affiliation, bio, years_experience, 
		       cv_url, cv_public_id, core_competencies, expertise, role
		FROM users WHERE LOWER(username) = LOWER($1)
	`

	row := database.DB.QueryRowContext(context.Background(), query, username)

	user := &models.User{}

	// Define nullable variables for potentially NULL fields
	var (
		firstName, lastName                                                                sql.NullString
		profileURL, profilePublicID, affiliation, bio, cvURL, cvPublicID, coreCompetencies sql.NullString
	)

	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Username,
		&firstName,
		&lastName,
		&profileURL,
		&profilePublicID,
		&affiliation,
		&bio,
		&user.YearsExperience,
		&cvURL,
		&cvPublicID,
		&coreCompetencies,
		&user.Expertise,
		&user.Role,
	)

	if err == sql.ErrNoRows {
		log.Println("user not found DEBUG", err)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Assign valid nullable fields to struct
	if firstName.Valid {
		user.FirstName = &firstName.String
	}
	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if profileURL.Valid {
		user.ProfileURL = &profileURL.String
	}
	if profilePublicID.Valid {
		user.ProfilePublicID = &profilePublicID.String
	}
	if affiliation.Valid {
		user.Affiliation = &affiliation.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	if cvURL.Valid {
		user.CVURL = &cvURL.String
	}
	if cvPublicID.Valid {
		user.CVPublicID = &cvPublicID.String
	}
	if coreCompetencies.Valid {
		user.CoreCompetencies = &coreCompetencies.String
	}

	return user, nil
}
