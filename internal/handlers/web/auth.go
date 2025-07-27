// // file: internal/handlers/web/auth.go

// package web

// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"strconv"
// 	"strings"
// 	"time"

// 	"evalhub/internal/models"
// 	"evalhub/internal/repositories"
// 	"evalhub/internal/services"
// 	"evalhub/internal/utils"

// 	"go.uber.org/zap"
// )

// var authService services.AuthService // TODO: Wire up in main or router

// // SignUp handles GET and POST requests for the signup page
// // Assume authService is available as a global or injected variable
// func SignUp(w http.ResponseWriter, r *http.Request) {
// 	if r.Method == http.MethodGet {
// 		data := map[string]interface{}{
// 			"Title": "Sign Up - EvalHub",
// 		}
// 		templates.ExecuteTemplate(w, "signup", data)
// 		return
// 	}

// 	if r.Method == http.MethodPost {
// 		// Create a context for the entire signup process
// 		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
// 		defer cancel()

// 		// Check Content-Type to determine form type
// 		contentType := r.Header.Get("Content-Type")
// 		var email, username, firstName, lastName, affiliation, bio, expertise, role, password string
// 		var profileURL, profilePublicID, cvURL, cvPublicID, coreCompetencies string
// 		var yearsExperience int
// 		var err error

// 		if strings.HasPrefix(contentType, "multipart/form-data") {
// 			// Parse multipart form (max 32MB)
// 			err = r.ParseMultipartForm(32 << 20) // 32MB limit to accommodate CV and profile picture
// 			if err != nil {
// 				log.Printf("Failed to parse multipart form: %v", err)
// 				data := map[string]interface{}{
// 					"Title": "Sign Up - EvalHub",
// 					"Error": "Failed to process form data. Please ensure files are valid and under size limits (Profile: 10MB, CV: 5MB).",
// 				}
// 				templates.ExecuteTemplate(w, "signup", data)
// 				return
// 			}

// 			// Retrieve form values
// 			email = utils.SanitizeString(r.FormValue("email"))
// 			username = utils.SanitizeString(r.FormValue("username"))
// 			firstName = utils.SanitizeString(r.FormValue("first_name"))
// 			lastName = utils.SanitizeString(r.FormValue("last_name"))
// 			affiliation = utils.SanitizeString(r.FormValue("affiliation"))
// 			bio = utils.SanitizeString(r.FormValue("bio"))
// 			yearsExperienceStr := utils.SanitizeString(r.FormValue("years_experience"))
// 			if yearsExperienceStr != "" {
// 				yearsExperience, err = strconv.Atoi(yearsExperienceStr)
// 				if err != nil || yearsExperience < 0 || yearsExperience > 50 {
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Years of experience must be a number between 0 and 50",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}
// 			}
// 			coreCompetencies = utils.SanitizeString(r.FormValue("core_competencies"))
// 			expertise = utils.SanitizeString(r.FormValue("expertise"))
// 			role = utils.SanitizeString(r.FormValue("role"))
// 			password = r.FormValue("password")

// 			// Initialize Cloudinary service
// 			cloudinary, err := utils.GetCloudinaryService()
// 			if err != nil {
// 				log.Printf("Failed to initialize Cloudinary service: %v", err)
// 				data := map[string]interface{}{
// 					"Title": "Sign Up - EvalHub",
// 					"Error": "Failed to initialize file upload service",
// 				}
// 				templates.ExecuteTemplate(w, "signup", data)
// 				return
// 			}

// 			// Get logger for structured logging
// 			logger := cloudinary.Logger.With(
// 				zap.String("handler", "SignUp"),
// 				zap.String("username", username),
// 				zap.String("email", email),
// 			)

// 			// Handle profile picture upload
// 			profileFile, profileHandler, err := r.FormFile("profile")
// 			if err == nil { // File was uploaded
// 				defer profileFile.Close()

// 				logger.Info("Processing profile picture upload",
// 					zap.String("filename", profileHandler.Filename),
// 					zap.Int64("size", profileHandler.Size))

// 				// Additional image-specific validation
// 				contentType := profileHandler.Header.Get("Content-Type")
// 				if !strings.HasPrefix(contentType, "image/") {
// 					logger.Warn("Invalid file type for profile picture",
// 						zap.String("content_type", contentType))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Profile picture must be an image file",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				// Validate file
// 				if err := cloudinary.ValidateFile(ctx, profileHandler); err != nil {
// 					logger.Warn("Profile picture validation failed", zap.Error(err))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": fmt.Sprintf("Profile picture validation failed: %v", err),
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				// Upload to Cloudinary
// 				uploadFolder := fmt.Sprintf("evalhub/profiles/%s", username)
// 				uploadResult, err := cloudinary.UploadFile(ctx, profileHandler, uploadFolder)
// 				if err != nil {
// 					logger.Error("Failed to upload profile picture", zap.Error(err))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Failed to upload profile picture",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				profileURL = uploadResult.URL
// 				profilePublicID = uploadResult.PublicID

// 				logger.Info("Successfully uploaded profile picture",
// 					zap.String("url", profileURL),
// 					zap.String("public_id", profilePublicID))
// 			} else if err != http.ErrMissingFile {
// 				logger.Error("Error retrieving profile picture", zap.Error(err))
// 				data := map[string]interface{}{
// 					"Title": "Sign Up - EvalHub",
// 					"Error": "Error processing profile picture",
// 				}
// 				templates.ExecuteTemplate(w, "signup", data)
// 				return
// 			}

// 			// Handle CV upload
// 			cvFile, cvHandler, err := r.FormFile("cv")
// 			if err == nil { // File was uploaded
// 				defer cvFile.Close()

// 				logger.Info("Processing CV upload",
// 					zap.String("filename", cvHandler.Filename),
// 					zap.Int64("size", cvHandler.Size))

// 				// Additional PDF-specific validation
// 				contentType := cvHandler.Header.Get("Content-Type")
// 				if contentType != "application/pdf" {
// 					logger.Warn("Invalid file type for CV",
// 						zap.String("content_type", contentType))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "CV must be a PDF file",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				// Validate file
// 				if err := cloudinary.ValidateFile(ctx, cvHandler); err != nil {
// 					logger.Warn("CV validation failed", zap.Error(err))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": fmt.Sprintf("CV validation failed: %v", err),
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				// Upload to Cloudinary
// 				uploadFolder := fmt.Sprintf("evalhub/cvs/%s", username)
// 				uploadResult, err := cloudinary.UploadFile(ctx, cvHandler, uploadFolder)
// 				if err != nil {
// 					logger.Error("Failed to upload CV", zap.Error(err))
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Failed to upload CV",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}

// 				cvURL = uploadResult.URL
// 				cvPublicID = uploadResult.PublicID

// 				logger.Info("Successfully uploaded CV",
// 					zap.String("url", cvURL),
// 					zap.String("public_id", cvPublicID))
// 			} else if err != http.ErrMissingFile {
// 				logger.Error("Error retrieving CV", zap.Error(err))
// 				data := map[string]interface{}{
// 					"Title": "Sign Up - EvalHub",
// 					"Error": "Error processing CV",
// 				}
// 				templates.ExecuteTemplate(w, "signup", data)
// 				return
// 			}
// 		} else {
// 			// Fallback to regular form parsing
// 			err = r.ParseForm()
// 			if err != nil {
// 				log.Printf("Failed to parse form: %v", err)
// 				data := map[string]interface{}{
// 					"Title": "Sign Up - EvalHub",
// 					"Error": "Failed to process form data",
// 				}
// 				templates.ExecuteTemplate(w, "signup", data)
// 				return
// 			}
// 			email = utils.SanitizeString(r.FormValue("email"))
// 			username = utils.SanitizeString(r.FormValue("username"))
// 			firstName = utils.SanitizeString(r.FormValue("first_name"))
// 			lastName = utils.SanitizeString(r.FormValue("last_name"))
// 			affiliation = utils.SanitizeString(r.FormValue("affiliation"))
// 			bio = utils.SanitizeString(r.FormValue("bio"))
// 			yearsExperienceStr := utils.SanitizeString(r.FormValue("years_experience"))
// 			if yearsExperienceStr != "" {
// 				yearsExperience, err = strconv.Atoi(yearsExperienceStr)
// 				if err != nil || yearsExperience < 0 || yearsExperience > 50 {
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Years of experience must be a number between 0 and 50",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}
// 			}
// 			coreCompetencies = utils.SanitizeString(r.FormValue("core_competencies"))
// 			expertise = utils.SanitizeString(r.FormValue("expertise"))
// 			role = utils.SanitizeString(r.FormValue("role"))
// 			password = r.FormValue("password")
// 		}

// 		// Validate required fields
// 		if email == "" || username == "" || firstName == "" || lastName == "" || expertise == "" || role == "" || password == "" {
// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": "All required fields must be filled",
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Validate email format
// 		if err := utils.ValidateEmail(email); err != nil {
// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": err.Error(),
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Validate password strength
// 		if err := utils.ValidatePasswordStrength(password); err != nil {
// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": err.Error(),
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Validate role (only 'user' allowed)
// 		if role != "user" {
// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": "Invalid role selected",
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Validate core competencies (if provided)
// 		if coreCompetencies != "" {
// 			competencies := strings.Split(coreCompetencies, ",")
// 			for _, comp := range competencies {
// 				comp = strings.TrimSpace(comp)
// 				if len(comp) < 2 {
// 					data := map[string]interface{}{
// 						"Title": "Sign Up - EvalHub",
// 						"Error": "Each core competency must be at least 2 characters long",
// 					}
// 					templates.ExecuteTemplate(w, "signup", data)
// 					return
// 				}
// 			}
// 			coreCompetencies = strings.Join(competencies, ",")
// 		}

// 		// Hash password
// 		hashedPassword, err := utils.HashPassword(password)
// 		if err != nil {
// 			log.Printf("Error hashing password: %v", err)
// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": "Internal server error",
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Create user struct for registration
// 		user := &models.User{
// 			Email:            email,
// 			Username:         username,
// 			FirstName:        &firstName,
// 			LastName:         &lastName,
// 			ProfileURL:       &profileURL,
// 			ProfilePublicID:  &profilePublicID,
// 			Affiliation:      &affiliation,
// 			Bio:              &bio,
// 			YearsExperience:  int16(yearsExperience),
// 			CVPath:           cvURL, // CVURL mapped to CVPath for legacy compatibility
// 			CVPublicID:       &cvPublicID,
// 			CoreCompetencies: &coreCompetencies,
// 			Expertise:        expertise,
// 			Role:             role,
// 			PasswordHash:     hashedPassword,
// 		}

// 		repoUser := &repositories.User{
// 			ID:       int(user.ID),
// 			Username: user.Username,
// 			Password: user.PasswordHash,
// 			IsOnline: user.IsOnline,
// 		}
// 		err = authService.Register(ctx, repoUser)
// 		if err != nil {
// 			log.Printf("Database error: %v", err)
// 			var errorMessage string
// 			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
// 				if strings.Contains(err.Error(), "email") {
// 					errorMessage = "This email is already registered"
// 				} else {
// 					errorMessage = "This username is already taken"
// 				}
// 			} else if strings.Contains(err.Error(), "CHECK constraint failed") {
// 				errorMessage = "Invalid expertise or role selected"
// 			} else {
// 				errorMessage = "An error occurred while signing up"
// 			}

// 			// If database error and we already uploaded files to Cloudinary, clean them up
// 			if profilePublicID != "" || cvPublicID != "" {
// 				cloudinary, cldErr := utils.GetCloudinaryService()
// 				if cldErr == nil {
// 					logger := cloudinary.Logger.With(
// 						zap.String("handler", "SignUp"),
// 						zap.String("action", "CleanupFiles"),
// 					)

// 					if profilePublicID != "" {
// 						if delErr := cloudinary.DeleteFile(ctx, profilePublicID); delErr != nil {
// 							logger.Error("Failed to delete profile picture after failed signup",
// 								zap.Error(delErr),
// 								zap.String("public_id", profilePublicID))
// 						} else {
// 							logger.Info("Successfully deleted profile picture after failed signup",
// 								zap.String("public_id", profilePublicID))
// 						}
// 					}

// 					if cvPublicID != "" {
// 						if delErr := cloudinary.DeleteFile(ctx, cvPublicID); delErr != nil {
// 							logger.Error("Failed to delete CV after failed signup",
// 								zap.Error(delErr),
// 								zap.String("public_id", cvPublicID))
// 						} else {
// 							logger.Info("Successfully deleted CV after failed signup",
// 								zap.String("public_id", cvPublicID))
// 						}
// 					}
// 				}
// 			}

// 			data := map[string]interface{}{
// 				"Title": "Sign Up - EvalHub",
// 				"Error": errorMessage,
// 			}
// 			templates.ExecuteTemplate(w, "signup", data)
// 			return
// 		}

// 		// Redirect to login on success
// 		http.Redirect(w, r, "/login", http.StatusSeeOther)
// 	}
// }

// // Login handles user authentication
// // Assume authService is available as a global or injected variable
// func Login(w http.ResponseWriter, r *http.Request) {
// 	if r.Method == http.MethodGet {
// 		data := map[string]interface{}{
// 			"Title": "Login - EvalHub",
// 		}
// 		templates.ExecuteTemplate(w, "login", data)
// 		return
// 	}

// 	if r.Method == http.MethodPost {
// 		// Create a context for the login process
// 		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
// 		defer cancel()

// 		username := r.FormValue("username")
// 		password := r.FormValue("password")

// 		if username == "" || password == "" {
// 			RenderErrorPage(w, http.StatusBadRequest, fmt.Errorf("username and password are required"))
// 			return
// 		}

// 		_, err := authService.Login(ctx, username, password)
// 		if err != nil {
// 			data := map[string]interface{}{
// 				"Title": "Login - EvalHub",
// 				"Error": "Invalid username or password",
// 			}
// 			templates.ExecuteTemplate(w, "login", data)
// 			return
// 		}

// 		// Assume session management is handled within the service, but if a session token is returned, set it as a cookie
// 		// For now, simulate session token
// 		sessionToken := "session_token_placeholder" // Replace with actual token from service if available
// 		expiresAt := time.Now().Add(time.Hour * 24)
// 		http.SetCookie(w, &http.Cookie{
// 			Name:     "session_token",
// 			Value:    sessionToken,
// 			Expires:  expiresAt,
// 			HttpOnly: true,
// 			SameSite: http.SameSiteStrictMode,
// 			Secure:   r.TLS != nil,
// 			Path:     "/",
// 		})
// 		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
// 	}
// }

// // Logout terminates a user's session
// // Assume authService is available as a global or injected variable
// func Logout(w http.ResponseWriter, r *http.Request) {
// 	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
// 	defer cancel()

// 	_, err := r.Cookie("session_token")
// 	if err != nil {
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// Extract user ID from session token (simulate for now)
// 	userID := 0 // In real code, extract from session or use a helper/service
// 	err = authService.Logout(ctx, userID)
// 	if err != nil {
// 		RenderErrorPage(w, http.StatusInternalServerError, err)
// 		return
// 	}

// 	http.SetCookie(w, &http.Cookie{
// 		Name:     "session_token",
// 		Value:    "",
// 		Expires:  time.Now(),
// 		HttpOnly: true,
// 		SameSite: http.SameSiteStrictMode,
// 		Secure:   r.TLS != nil,
// 		Path:     "/",
// 	})

// 	http.Redirect(w, r, "/", http.StatusSeeOther)
// }


// this is the common auth handler using the same service
// file: internal/handlers/web/auth.go
package web

import (
	"context"
	"evalhub/internal/services"
	"evalhub/internal/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Global webHandler instance for backward compatibility
var webHandler *WebHandler

// InitWebHandler initializes the web handler with service collection
func InitWebHandler(serviceCollection *services.ServiceCollection, logger *zap.Logger) {
	webHandler = NewWebHandler(serviceCollection, logger)
}

// SignUp handles GET and POST requests for the signup page
func (h *WebHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title": "Sign Up - EvalHub",
		}
		templates.ExecuteTemplate(w, "signup", data)
		return
	}

	if r.Method == http.MethodPost {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		// Check Content-Type to determine form type
		contentType := r.Header.Get("Content-Type")
		var email, username, firstName, lastName, affiliation, bio, expertise, role, password string
		var profileURL, profilePublicID, cvURL, cvPublicID, coreCompetencies string
		var yearsExperience int
		var err error

		if strings.HasPrefix(contentType, "multipart/form-data") {
			// Parse multipart form (max 32MB)
			err = r.ParseMultipartForm(32 << 20) // 32MB limit
			if err != nil {
				h.logger.Error("Failed to parse multipart form", zap.Error(err))
				data := map[string]interface{}{
					"Title": "Sign Up - EvalHub",
					"Error": "Failed to process form data. Please ensure files are valid and under size limits.",
				}
				templates.ExecuteTemplate(w, "signup", data)
				return
			}

			// Retrieve form values
			email = utils.SanitizeString(r.FormValue("email"))
			username = utils.SanitizeString(r.FormValue("username"))
			firstName = utils.SanitizeString(r.FormValue("first_name"))
			lastName = utils.SanitizeString(r.FormValue("last_name"))
			affiliation = utils.SanitizeString(r.FormValue("affiliation"))
			bio = utils.SanitizeString(r.FormValue("bio"))
			yearsExperienceStr := utils.SanitizeString(r.FormValue("years_experience"))
			if yearsExperienceStr != "" {
				yearsExperience, err = strconv.Atoi(yearsExperienceStr)
				if err != nil || yearsExperience < 0 || yearsExperience > 50 {
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Years of experience must be a number between 0 and 50",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}
			}
			coreCompetencies = utils.SanitizeString(r.FormValue("core_competencies"))
			expertise = utils.SanitizeString(r.FormValue("expertise"))
			role = utils.SanitizeString(r.FormValue("role"))
			password = r.FormValue("password")

			// Handle file uploads using your existing Cloudinary setup
			cloudinary, err := utils.GetCloudinaryService()
			if err != nil {
				h.logger.Error("Failed to initialize Cloudinary service", zap.Error(err))
				data := map[string]interface{}{
					"Title": "Sign Up - EvalHub",
					"Error": "Failed to initialize file upload service",
				}
				templates.ExecuteTemplate(w, "signup", data)
				return
			}

			// Get logger for structured logging
			logger := cloudinary.Logger.With(
				zap.String("handler", "SignUp"),
				zap.String("username", username),
				zap.String("email", email),
			)

			// Handle profile picture upload
			profileFile, profileHandler, err := r.FormFile("profile")
			if err == nil { // File was uploaded
				defer profileFile.Close()

				logger.Info("Processing profile picture upload",
					zap.String("filename", profileHandler.Filename),
					zap.Int64("size", profileHandler.Size))

				// Additional image-specific validation
				contentType := profileHandler.Header.Get("Content-Type")
				if !strings.HasPrefix(contentType, "image/") {
					logger.Warn("Invalid file type for profile picture",
						zap.String("content_type", contentType))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Profile picture must be an image file",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				// Validate file
				if err := cloudinary.ValidateFile(ctx, profileHandler); err != nil {
					logger.Warn("Profile picture validation failed", zap.Error(err))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": fmt.Sprintf("Profile picture validation failed: %v", err),
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				// Upload to Cloudinary
				uploadFolder := fmt.Sprintf("evalhub/profiles/%s", username)
				uploadResult, err := cloudinary.UploadFile(ctx, profileHandler, uploadFolder)
				if err != nil {
					logger.Error("Failed to upload profile picture", zap.Error(err))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Failed to upload profile picture",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				profileURL = uploadResult.URL
				profilePublicID = uploadResult.PublicID

				logger.Info("Successfully uploaded profile picture",
					zap.String("url", profileURL),
					zap.String("public_id", profilePublicID))
			} else if err != http.ErrMissingFile {
				logger.Error("Error retrieving profile picture", zap.Error(err))
				data := map[string]interface{}{
					"Title": "Sign Up - EvalHub",
					"Error": "Error processing profile picture",
				}
				templates.ExecuteTemplate(w, "signup", data)
				return
			}

			// Handle CV upload
			cvFile, cvHandler, err := r.FormFile("cv")
			if err == nil { // File was uploaded
				defer cvFile.Close()

				logger.Info("Processing CV upload",
					zap.String("filename", cvHandler.Filename),
					zap.Int64("size", cvHandler.Size))

				// Additional PDF-specific validation
				contentType := cvHandler.Header.Get("Content-Type")
				if contentType != "application/pdf" {
					logger.Warn("Invalid file type for CV",
						zap.String("content_type", contentType))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "CV must be a PDF file",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				// Validate file
				if err := cloudinary.ValidateFile(ctx, cvHandler); err != nil {
					logger.Warn("CV validation failed", zap.Error(err))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": fmt.Sprintf("CV validation failed: %v", err),
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				// Upload to Cloudinary
				uploadFolder := fmt.Sprintf("evalhub/cvs/%s", username)
				uploadResult, err := cloudinary.UploadFile(ctx, cvHandler, uploadFolder)
				if err != nil {
					logger.Error("Failed to upload CV", zap.Error(err))
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Failed to upload CV",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}

				cvURL = uploadResult.URL
				cvPublicID = uploadResult.PublicID

				logger.Info("Successfully uploaded CV",
					zap.String("url", cvURL),
					zap.String("public_id", cvPublicID))
			} else if err != http.ErrMissingFile {
				logger.Error("Error retrieving CV", zap.Error(err))
				data := map[string]interface{}{
					"Title": "Sign Up - EvalHub",
					"Error": "Error processing CV",
				}
				templates.ExecuteTemplate(w, "signup", data)
				return
			}
		} else {
			// Fallback to regular form parsing
			err = r.ParseForm()
			if err != nil {
				h.logger.Error("Failed to parse form", zap.Error(err))
				data := map[string]interface{}{
					"Title": "Sign Up - EvalHub",
					"Error": "Failed to process form data",
				}
				templates.ExecuteTemplate(w, "signup", data)
				return
			}

			// Extract form values
			email = utils.SanitizeString(r.FormValue("email"))
			username = utils.SanitizeString(r.FormValue("username"))
			firstName = utils.SanitizeString(r.FormValue("first_name"))
			lastName = utils.SanitizeString(r.FormValue("last_name"))
			affiliation = utils.SanitizeString(r.FormValue("affiliation"))
			bio = utils.SanitizeString(r.FormValue("bio"))
			yearsExperienceStr := utils.SanitizeString(r.FormValue("years_experience"))
			if yearsExperienceStr != "" {
				yearsExperience, err = strconv.Atoi(yearsExperienceStr)
				if err != nil || yearsExperience < 0 || yearsExperience > 50 {
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Years of experience must be a number between 0 and 50",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}
			}
			coreCompetencies = utils.SanitizeString(r.FormValue("core_competencies"))
			expertise = utils.SanitizeString(r.FormValue("expertise"))
			role = utils.SanitizeString(r.FormValue("role"))
			password = r.FormValue("password")
		}

		// Validate required fields
		if email == "" || username == "" || firstName == "" || lastName == "" || expertise == "" || role == "" || password == "" {
			data := map[string]interface{}{
				"Title": "Sign Up - EvalHub",
				"Error": "All required fields must be filled",
			}
			templates.ExecuteTemplate(w, "signup", data)
			return
		}

		// Additional validation
		if err := utils.ValidateEmail(email); err != nil {
			data := map[string]interface{}{
				"Title": "Sign Up - EvalHub",
				"Error": err.Error(),
			}
			templates.ExecuteTemplate(w, "signup", data)
			return
		}

		if err := utils.ValidatePasswordStrength(password); err != nil {
			data := map[string]interface{}{
				"Title": "Sign Up - EvalHub",
				"Error": err.Error(),
			}
			templates.ExecuteTemplate(w, "signup", data)
			return
		}

		if role != "user" {
			data := map[string]interface{}{
				"Title": "Sign Up - EvalHub",
				"Error": "Invalid role selected",
			}
			templates.ExecuteTemplate(w, "signup", data)
			return
		}

		// Validate core competencies
		if coreCompetencies != "" {
			competencies := strings.Split(coreCompetencies, ",")
			for _, comp := range competencies {
				comp = strings.TrimSpace(comp)
				if len(comp) < 2 {
					data := map[string]interface{}{
						"Title": "Sign Up - EvalHub",
						"Error": "Each core competency must be at least 2 characters long",
					}
					templates.ExecuteTemplate(w, "signup", data)
					return
				}
			}
			coreCompetencies = strings.Join(competencies, ",")
		}

		// ✅ Step 1: Use AuthService to register user with basic info
		authService := h.GetAuthService()

		registerReq := &services.RegisterRequest{
			Email:           email,
			Username:        username,
			Password:        password,
			ConfirmPassword: password,
			FirstName:       firstName,
			LastName:        lastName,
			AcceptTerms:     true,
		}

		authResp, err := authService.Register(ctx, registerReq)
		if err != nil {
			h.logger.Error("Registration failed", zap.Error(err))

			// Handle service errors properly
			var errorMessage string
			if serviceErr := services.GetServiceError(err); serviceErr != nil {
				switch serviceErr.Type {
				case "VALIDATION_ERROR":
					errorMessage = serviceErr.Message
				case "BUSINESS_ERROR":
					if serviceErr.Code == "EMAIL_EXISTS" {
						errorMessage = "This email is already registered"
					} else if serviceErr.Code == "USERNAME_EXISTS" {
						errorMessage = "This username is already taken"
					} else {
						errorMessage = serviceErr.Message
					}
				default:
					errorMessage = "An error occurred while signing up"
				}
			} else {
				// Handle legacy error messages
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					if strings.Contains(err.Error(), "email") {
						errorMessage = "This email is already registered"
					} else {
						errorMessage = "This username is already taken"
					}
				} else if strings.Contains(err.Error(), "CHECK constraint failed") {
					errorMessage = "Invalid expertise or role selected"
				} else {
					errorMessage = "An error occurred while signing up"
				}
			}

			// Clean up uploaded files if registration failed
			if profilePublicID != "" || cvPublicID != "" {
				cloudinary, cldErr := utils.GetCloudinaryService()
				if cldErr == nil {
					logger := cloudinary.Logger.With(
						zap.String("handler", "SignUp"),
						zap.String("action", "CleanupFiles"),
					)

					if profilePublicID != "" {
						if delErr := cloudinary.DeleteFile(ctx, profilePublicID); delErr != nil {
							logger.Error("Failed to delete profile picture after failed signup",
								zap.Error(delErr),
								zap.String("public_id", profilePublicID))
						} else {
							logger.Info("Successfully deleted profile picture after failed signup",
								zap.String("public_id", profilePublicID))
						}
					}

					if cvPublicID != "" {
						if delErr := cloudinary.DeleteFile(ctx, cvPublicID); delErr != nil {
							logger.Error("Failed to delete CV after failed signup",
								zap.Error(delErr),
								zap.String("public_id", cvPublicID))
						} else {
							logger.Info("Successfully deleted CV after failed signup",
								zap.String("public_id", cvPublicID))
						}
					}
				}
			}

			data := map[string]interface{}{
				"Title": "Sign Up - EvalHub",
				"Error": errorMessage,
			}
			templates.ExecuteTemplate(w, "signup", data)
			return
		}

		// ✅ Step 2: Update user profile with extended fields
		if authResp.User != nil && (affiliation != "" || bio != "" || yearsExperience > 0 ||
			coreCompetencies != "" || expertise != "" || profileURL != "" || cvURL != "") {

			userService := h.GetUserService()

			// Convert years experience
			var yearsExp *int16
			if yearsExperience > 0 {
				exp := int16(yearsExperience)
				yearsExp = &exp
			}

			updateReq := &services.UpdateUserRequest{
				UserID:           authResp.User.ID,
				YearsExperience:  yearsExp,
				CoreCompetencies: &coreCompetencies,
				Expertise:        &expertise,
				Affiliation:      &affiliation,
				Bio:              &bio,
			}

			if _, err := userService.UpdateUser(ctx, updateReq); err != nil {
				h.logger.Warn("Failed to update user profile after registration",
					zap.Error(err),
					zap.Int64("user_id", authResp.User.ID))
				// Don't fail the registration for this, just log the warning
			}

			// Update profile/CV URLs directly in user model if uploaded
			if profileURL != "" || cvURL != "" {
				user := authResp.User
				if profileURL != "" {
					user.ProfileURL = &profileURL
					user.ProfilePublicID = &profilePublicID
				}
				if cvURL != "" {
					user.CVURL = &cvURL
					user.CVPublicID = &cvPublicID
				}

				// Update the user in database directly for file URLs
				// Note: This might need to be done through a repository call
				// depending on your UserService implementation
			}
		}

		// ✅ Step 3: Set session cookie using the returned token
		if authResp.AccessToken != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    authResp.AccessToken,
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				Secure:   r.TLS != nil,
				Path:     "/",
			})
		}

		// Redirect to login page on success (or dashboard if you prefer)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// Login handles user authentication
func (h *WebHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]interface{}{
			"Title": "Login - EvalHub",
		}
		templates.ExecuteTemplate(w, "login", data)
		return
	}

	if r.Method == http.MethodPost {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		username := r.FormValue("username")
		password := r.FormValue("password")
		remember := r.FormValue("remember") == "on"

		if username == "" || password == "" {
			data := map[string]interface{}{
				"Title": "Login - EvalHub",
				"Error": "Username and password are required",
			}
			templates.ExecuteTemplate(w, "login", data)
			return
		}

		// ✅ Use AuthService with proper LoginRequest
		authService := h.GetAuthService()

		loginReq := &services.LoginRequest{
			Login:    username,
			Password: password,
			Remember: remember,
		}

		authResp, err := authService.Login(ctx, loginReq)
		if err != nil {
			h.logger.Warn("Login failed", zap.Error(err))

			data := map[string]interface{}{
				"Title": "Login - EvalHub",
				"Error": "Invalid username or password",
			}
			templates.ExecuteTemplate(w, "login", data)
			return
		}

		// ✅ Set session cookie using the actual returned token
		if authResp.AccessToken != "" {
			sessionTTL := time.Duration(authResp.ExpiresIn) * time.Second
			if sessionTTL == 0 {
				sessionTTL = 24 * time.Hour // Fallback
			}

			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    authResp.AccessToken,
				Expires:  time.Now().Add(sessionTTL),
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				Secure:   r.TLS != nil,
				Path:     "/",
			})
		}

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

// Logout terminates a user's session
func (h *WebHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cookie, err := r.Cookie("session_token")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// ✅ Use AuthService with proper LogoutRequest
	authService := h.GetAuthService()

	logoutReq := &services.LogoutRequest{
		SessionToken: cookie.Value,
	}

	if err := authService.Logout(ctx, logoutReq); err != nil {
		h.logger.Error("Logout failed", zap.Error(err))
		// Continue with cookie clearing even if service call fails
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		Path:     "/",
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Backward compatibility functions - these call the methods on the global webHandler
func SignUp(w http.ResponseWriter, r *http.Request) {
	if webHandler == nil {
		log.Fatal("WebHandler not initialized. Call InitWebHandler() first.")
	}
	webHandler.SignUp(w, r)
}

func Login(w http.ResponseWriter, r *http.Request) {
	if webHandler == nil {
		log.Fatal("WebHandler not initialized. Call InitWebHandler() first.")
	}
	webHandler.Login(w, r)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if webHandler == nil {
		log.Fatal("WebHandler not initialized. Call InitWebHandler() first.")
	}
	webHandler.Logout(w, r)
}
