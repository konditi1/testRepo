// ===============================
// COMPLETE UPDATED FILE: internal/router/api_v1_integration.go
// ===============================

package router

import (
	"evalhub/internal/handlers/api/v1/auth"
	"evalhub/internal/handlers/api/v1/comments" // 🆕 ADD THIS IMPORT
	"evalhub/internal/handlers/api/v1/jobs"
	"evalhub/internal/handlers/api/v1/posts"
	"evalhub/internal/handlers/api/v1/users"

	"evalhub/internal/middleware"
	"evalhub/internal/response"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// AddAPIv1Routes adds API v1 routes with enhanced role-based security
// 🆕 UPDATED FUNCTION SIGNATURE - ADD responseBuilder PARAMETER
func AddAPIv1Routes(
	mux *http.ServeMux,
	serviceCollection *services.ServiceCollection,
	authMiddleware *middleware.AuthMiddleware,
	responseBuilder *response.Builder, // 🆕 ADD THIS PARAMETER
	logger *zap.Logger,
) {
	// Create controllers using existing service collection
	authController := auth.NewAuthController(serviceCollection, logger, responseBuilder)
	userController := users.NewUserController(serviceCollection, logger, responseBuilder)
	postController := posts.NewPostController(serviceCollection, logger, responseBuilder)
	commentController := comments.NewCommentController(serviceCollection, logger, responseBuilder)
	jobController := jobs.NewJobController(serviceCollection, logger, responseBuilder)

	// ===============================
	// PUBLIC AUTH ENDPOINTS (No auth required)
	// ===============================

	// Registration and login endpoints
	mux.Handle("/api/v1/auth/register", createAPIHandler(authController.Register))
	mux.Handle("/api/v1/auth/login", createAPIHandler(authController.Login))
	mux.Handle("/api/v1/auth/refresh", createAPIHandler(authController.RefreshToken))

	// Password management endpoints
	mux.Handle("/api/v1/auth/forgot-password", createAPIHandler(authController.ForgotPassword))
	mux.Handle("/api/v1/auth/reset-password", createAPIHandler(authController.ResetPassword))
	mux.Handle("/api/v1/auth/verify-email", createAPIHandler(authController.VerifyEmail))

	// OAuth endpoints
	mux.Handle("/api/v1/auth/oauth/login", createAPIHandler(authController.OAuthLogin))

	// ===============================
	// AUTHENTICATED AUTH ENDPOINTS (Auth required)
	// ===============================

	// Session management endpoints
	mux.Handle("/api/v1/auth/logout", createAuthenticatedAPIHandler(authController.Logout, authMiddleware))
	mux.Handle("/api/v1/auth/logout-all", createAuthenticatedAPIHandler(authController.LogoutAllDevices, authMiddleware))
	mux.Handle("/api/v1/auth/sessions", createAuthenticatedAPIHandler(authController.GetSessions, authMiddleware))

	// Password change endpoint
	mux.Handle("/api/v1/auth/change-password", createAuthenticatedAPIHandler(authController.ChangePassword, authMiddleware))

	// Email verification endpoints
	mux.Handle("/api/v1/auth/send-verification", createAuthenticatedAPIHandler(authController.SendVerificationEmail, authMiddleware))

	// ===============================
	// USER API ENDPOINTS (MT-11)
	// ===============================

	// PUBLIC USER ENDPOINTS (No auth required)
	mux.Handle("/api/v1/users/leaderboard", createAPIHandler(userController.GetLeaderboard))
	mux.Handle("/api/v1/users/online", createAPIHandler(userController.GetOnlineUsers))

	// AUTHENTICATED USER PROFILE ENDPOINTS (Auth required)
	mux.Handle("/api/v1/users/profile", createAuthenticatedAPIHandler(userController.GetProfile, authMiddleware))
	mux.Handle("/api/v1/users/profile/update", createAuthenticatedAPIHandler(userController.UpdateProfile, authMiddleware))
	mux.Handle("/api/v1/users/profile/image", createAuthenticatedAPIHandler(userController.UploadProfileImage, authMiddleware))
	mux.Handle("/api/v1/users/profile/cv", createAuthenticatedAPIHandler(userController.UploadCV, authMiddleware))
	mux.Handle("/api/v1/users/profile/deactivate", createAuthenticatedAPIHandler(userController.DeactivateAccount, authMiddleware))

	// USER LISTING AND SEARCH ENDPOINTS (Auth required)
	mux.Handle("/api/v1/users", createAuthenticatedAPIHandler(userController.ListUsers, authMiddleware))
	mux.Handle("/api/v1/users/search", createAuthenticatedAPIHandler(userController.SearchUsers, authMiddleware))

	// USER STATUS ENDPOINTS (Auth required)
	mux.Handle("/api/v1/users/status/online", createAuthenticatedAPIHandler(userController.UpdateOnlineStatus, authMiddleware))

	// ===============================
	// 🛡️ ENHANCED POST API ENDPOINTS (Role-based Security)
	// ===============================

	// PUBLIC POST ENDPOINTS (No auth required)
	mux.Handle("/api/v1/posts/trending", createAPIHandler(postController.GetTrendingPosts))
	mux.Handle("/api/v1/posts/featured", createAPIHandler(postController.GetFeaturedPosts))

	// AUTHENTICATED POST CRUD ENDPOINTS (Enhanced with role-based access)
	mux.Handle("/api/v1/posts", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			postController.ListPosts(w, r)
		case http.MethodPost:
			// ✅ Any authenticated user can create posts
			postController.CreatePost(w, r)
		default:
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// POST SEARCH ENDPOINT (Auth required)
	mux.Handle("/api/v1/posts/search", createAuthenticatedAPIHandler(postController.SearchPosts, authMiddleware))

	// POST ANALYTICS ENDPOINT (Auth required)
	mux.Handle("/api/v1/posts/analytics", createAuthenticatedAPIHandler(postController.GetPostAnalytics, authMiddleware))

	// POST CATEGORY ENDPOINTS (Auth required)
	mux.Handle("/api/v1/posts/category/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			postController.GetPostsByCategory(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// POST USER ENDPOINTS (Auth required)
	mux.Handle("/api/v1/posts/user/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			postController.GetPostsByUser(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// ===============================
	// 🛡️ ENHANCED COMMENT API ENDPOINTS (Role-based Security) - MT-13
	// ===============================

	// AUTHENTICATED COMMENT CRUD ENDPOINTS (Enhanced with role-based access)
	mux.Handle("/api/v1/comments", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// GET /api/v1/comments - List comments with filters (not implemented in controller)
			response.QuickStatusResponse(w, r, http.StatusNotImplemented, "General comment listing not implemented")
		case http.MethodPost:
			// POST /api/v1/comments - Any authenticated user can create comments
			commentController.CreateComment(w, r)
		default:
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// COMMENT SEARCH ENDPOINT (Auth required)
	mux.Handle("/api/v1/comments/search", createAuthenticatedAPIHandler(commentController.SearchComments, authMiddleware))

	// COMMENT ANALYTICS ENDPOINT (Auth required)
	mux.Handle("/api/v1/comments/analytics", createAuthenticatedAPIHandler(commentController.GetCommentAnalytics, authMiddleware))

	// COMMENT CONTENT ENDPOINTS (Auth required)
	mux.Handle("/api/v1/comments/post/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			commentController.GetCommentsByPost(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	mux.Handle("/api/v1/comments/question/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			commentController.GetCommentsByQuestion(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	mux.Handle("/api/v1/comments/document/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			commentController.GetCommentsByDocument(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	mux.Handle("/api/v1/comments/user/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			commentController.GetCommentsByUser(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// PUBLIC COMMENT ENDPOINTS (No auth required) - 🆕 ADD THESE
	mux.Handle("/api/v1/comments/trending", createAPIHandler(commentController.GetTrendingComments))
	mux.Handle("/api/v1/comments/recent", createAPIHandler(commentController.GetRecentComments))

	// ===============================
	// 🛡️ ROLE-BASED MODERATION ENDPOINTS (Admin/Moderator Only)
	// ===============================

	// COMMENT MODERATION QUEUE (Admin/Moderator only) - 🆕 ADD THIS
	mux.Handle("/api/v1/comments/moderation/queue", createModeratorAPIHandler(commentController.GetModerationQueue, authMiddleware))

	// POST MODERATION ENDPOINT (Admin/Moderator only)
	mux.Handle("/api/v1/posts/moderate/", createModeratorAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postController.ModeratePost(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}, authMiddleware))

	// BULK MODERATION ENDPOINTS (Admin/Moderator only)
	mux.Handle("/api/v1/admin/posts/bulk-moderate", createAdminAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		// This would handle bulk moderation operations
		response.QuickStatusResponse(w, r, http.StatusNotImplemented, "Bulk moderation not yet implemented")
	}, authMiddleware))

	// ===============================
	// DYNAMIC USER ROUTES (Auth required) - MT-11
	// ===============================

	// Handle user-specific routes with dynamic IDs
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		// Handle different user route patterns
		if len(pathParts) >= 4 {
			switch {
			// GET /api/v1/users/{id}
			case len(pathParts) == 4 && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(userController.GetUserByID, authMiddleware)
				handler.ServeHTTP(w, r)

			// GET /api/v1/users/{id}/stats
			case len(pathParts) == 5 && pathParts[4] == "stats" && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(userController.GetUserStats, authMiddleware)
				handler.ServeHTTP(w, r)

			// GET /api/v1/users/{id}/activity
			case len(pathParts) == 5 && pathParts[4] == "activity" && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(userController.GetUserActivity, authMiddleware)
				handler.ServeHTTP(w, r)

			// GET /api/v1/users/username/{username}
			case len(pathParts) == 5 && pathParts[3] == "username" && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(userController.GetUserByUsername, authMiddleware)
				handler.ServeHTTP(w, r)

			default:
				response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
			}
		} else {
			response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
		}
	})

	// ===============================
	// 🛡️ ENHANCED DYNAMIC POST ROUTES (Role-based Security)
	// ===============================

	// Handle post-specific routes with enhanced role-based access control
	mux.HandleFunc("/api/v1/posts/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		// Handle different post route patterns with role-based security
		if len(pathParts) >= 4 {
			switch {
			// GET /api/v1/posts/{id} - Any authenticated user
			case len(pathParts) == 4 && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(postController.GetPost, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ PUT /api/v1/posts/{id} - Owner, Moderator, or Admin (handled in controller)
			case len(pathParts) == 4 && r.Method == http.MethodPut:
				handler := createAuthenticatedAPIHandler(postController.UpdatePost, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ DELETE /api/v1/posts/{id} - Owner, Moderator, or Admin (handled in controller)
			case len(pathParts) == 4 && r.Method == http.MethodDelete:
				handler := createAuthenticatedAPIHandler(postController.DeletePost, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/posts/{id}/react - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "react" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(postController.ReactToPost, authMiddleware)
				handler.ServeHTTP(w, r)

			// DELETE /api/v1/posts/{id}/react - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "react" && r.Method == http.MethodDelete:
				handler := createAuthenticatedAPIHandler(postController.RemoveReaction, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/posts/{id}/bookmark - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "bookmark" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(postController.BookmarkPost, authMiddleware)
				handler.ServeHTTP(w, r)

			// DELETE /api/v1/posts/{id}/bookmark - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "bookmark" && r.Method == http.MethodDelete:
				handler := createAuthenticatedAPIHandler(postController.UnbookmarkPost, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/posts/{id}/share - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "share" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(postController.SharePost, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/posts/{id}/report - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "report" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(postController.ReportPost, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ POST /api/v1/posts/{id}/moderate - Admin/Moderator only (enhanced security)
			case len(pathParts) == 5 && pathParts[4] == "moderate" && r.Method == http.MethodPost:
				handler := createModeratorAPIHandler(postController.ModeratePost, authMiddleware)
				handler.ServeHTTP(w, r)

			// GET /api/v1/posts/{id}/stats - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "stats" && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(postController.GetPostStats, authMiddleware)
				handler.ServeHTTP(w, r)

			// Handle category and user routes that weren't caught above
			case len(pathParts) >= 5 && pathParts[3] == "category":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(postController.GetPostsByCategory, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			case len(pathParts) >= 5 && pathParts[3] == "user":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(postController.GetPostsByUser, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			default:
				response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
			}
		} else {
			response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
		}
	})

	// ===============================
	// 🛡️ ENHANCED DYNAMIC COMMENT ROUTES (Role-based Security) - MT-13
	// ===============================

	// Handle comment-specific routes with enhanced role-based access control
	mux.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		// Handle different comment route patterns with role-based security
		if len(pathParts) >= 4 {
			switch {
			// GET /api/v1/comments/{id} - Any authenticated user
			case len(pathParts) == 4 && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(commentController.GetComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ PUT /api/v1/comments/{id} - Owner, Moderator, or Admin (handled in controller)
			case len(pathParts) == 4 && r.Method == http.MethodPut:
				handler := createAuthenticatedAPIHandler(commentController.UpdateComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ DELETE /api/v1/comments/{id} - Owner, Moderator, or Admin (handled in controller)
			case len(pathParts) == 4 && r.Method == http.MethodDelete:
				handler := createAuthenticatedAPIHandler(commentController.DeleteComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/comments/{id}/react - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "react" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(commentController.ReactToComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// DELETE /api/v1/comments/{id}/react - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "react" && r.Method == http.MethodDelete:
				handler := createAuthenticatedAPIHandler(commentController.RemoveCommentReaction, authMiddleware)
				handler.ServeHTTP(w, r)

			// POST /api/v1/comments/{id}/report - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "report" && r.Method == http.MethodPost:
				handler := createAuthenticatedAPIHandler(commentController.ReportComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// 🛡️ POST /api/v1/comments/{id}/moderate - Admin/Moderator only (enhanced security)
			case len(pathParts) == 5 && pathParts[4] == "moderate" && r.Method == http.MethodPost:
				handler := createModeratorAPIHandler(commentController.ModerateComment, authMiddleware)
				handler.ServeHTTP(w, r)

			// GET /api/v1/comments/{id}/stats - Any authenticated user
			case len(pathParts) == 5 && pathParts[4] == "stats" && r.Method == http.MethodGet:
				handler := createAuthenticatedAPIHandler(commentController.GetCommentStats, authMiddleware)
				handler.ServeHTTP(w, r)

			// Handle content type routes that weren't caught above
			case len(pathParts) >= 5 && pathParts[3] == "post":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(commentController.GetCommentsByPost, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			case len(pathParts) >= 5 && pathParts[3] == "question":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(commentController.GetCommentsByQuestion, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			case len(pathParts) >= 5 && pathParts[3] == "document":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(commentController.GetCommentsByDocument, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			case len(pathParts) >= 5 && pathParts[3] == "user":
				if r.Method == http.MethodGet {
					handler := createAuthenticatedAPIHandler(commentController.GetCommentsByUser, authMiddleware)
					handler.ServeHTTP(w, r)
				} else {
					response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				}

			default:
				response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
			}
		} else {
			response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
		}
	})

	// ===============================
	// DYNAMIC SESSION REVOCATION ROUTE (MT-10)
	// ===============================

	// Handle session revocation with dynamic session ID
	mux.HandleFunc("/api/v1/auth/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/auth/sessions/") {
			// Apply auth middleware and call controller
			handler := createAuthenticatedAPIHandler(authController.RevokeSession, authMiddleware)
			handler.ServeHTTP(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	})


// ===============================
// JOB API ENDPOINTS
// ===============================

// PUBLIC JOB ENDPOINTS (No auth required)
mux.Handle("/api/v1/jobs/featured", createAPIHandler(jobController.GetFeaturedJobs))
mux.Handle("/api/v1/jobs/search", createAPIHandler(jobController.SearchJobs))

// AUTHENTICATED JOB ENDPOINTS (Auth required)
mux.Handle("/api/v1/jobs", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobController.ListJobs(w, r)
	case http.MethodPost:
		// Any authenticated user can create jobs
		jobController.CreateJob(w, r)
	default:
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}, authMiddleware))

// JOB ANALYTICS ENDPOINT (Auth required)
mux.Handle("/api/v1/jobs/stats", createAuthenticatedAPIHandler(jobController.GetJobStats, authMiddleware))

// JOB EMPLOYER ENDPOINTS (Auth required)
mux.Handle("/api/v1/jobs/employer/", createAuthenticatedAPIHandler(func(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jobController.GetJobsByEmployer(w, r)
	} else {
		response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}, authMiddleware))

// USER APPLICATIONS ENDPOINT (Auth required)
mux.Handle("/api/v1/jobs/my-applications", createAuthenticatedAPIHandler(jobController.GetUserApplications, authMiddleware))

// ===============================
// DYNAMIC JOB ROUTES (Auth required)
// ===============================

// Handle job-specific routes with enhanced access control
mux.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	// Handle different job route patterns
	if len(pathParts) >= 4 {
		switch {
		// GET /api/v1/jobs/{id} - Any authenticated user
		case len(pathParts) == 4 && r.Method == http.MethodGet:
			handler := createAuthenticatedAPIHandler(jobController.GetJob, authMiddleware)
			handler.ServeHTTP(w, r)

		// PUT /api/v1/jobs/{id} - Owner only (handled in controller)
		case len(pathParts) == 4 && r.Method == http.MethodPut:
			handler := createAuthenticatedAPIHandler(jobController.UpdateJob, authMiddleware)
			handler.ServeHTTP(w, r)

		// DELETE /api/v1/jobs/{id} - Owner only (handled in controller)
		case len(pathParts) == 4 && r.Method == http.MethodDelete:
			handler := createAuthenticatedAPIHandler(jobController.DeleteJob, authMiddleware)
			handler.ServeHTTP(w, r)

		// POST /api/v1/jobs/{id}/apply - Any authenticated user
		case len(pathParts) == 5 && pathParts[4] == "apply" && r.Method == http.MethodPost:
			handler := createAuthenticatedAPIHandler(jobController.ApplyForJob, authMiddleware)
			handler.ServeHTTP(w, r)

		// GET /api/v1/jobs/{id}/applications - Job owner only (handled in controller)
		case len(pathParts) == 5 && pathParts[4] == "applications" && r.Method == http.MethodGet:
			handler := createAuthenticatedAPIHandler(jobController.GetJobApplications, authMiddleware)
			handler.ServeHTTP(w, r)

		// POST /api/v1/jobs/{id}/applications/{applicationId}/review - Job owner only
		case len(pathParts) == 7 && pathParts[4] == "applications" && pathParts[6] == "review" && r.Method == http.MethodPost:
			handler := createAuthenticatedAPIHandler(jobController.ReviewApplication, authMiddleware)
			handler.ServeHTTP(w, r)

		// Handle employer routes that weren't caught above
		case len(pathParts) >= 5 && pathParts[3] == "employer":
			if r.Method == http.MethodGet {
				handler := createAuthenticatedAPIHandler(jobController.GetJobsByEmployer, authMiddleware)
				handler.ServeHTTP(w, r)
			} else {
				response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			}

		default:
			response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
		}
	} else {
		response.QuickError(w, r, services.NewNotFoundError("endpoint not found"))
	}
})

	// ===============================
	// API INFO AND HEALTH ENDPOINTS
	// ===============================

	// API information endpoint
	mux.Handle("/api/v1/info", createAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		info := map[string]interface{}{
			"version":     "1.0.0",
			"name":        "EvalHub API",
			"description": "Enterprise API for EvalHub platform with role-based security",
			"security": map[string]interface{}{
				"authentication": "JWT + Session-based",
				"authorization":  "Role-based access control (RBAC)",
				"roles": map[string]interface{}{
					"admin":     "Full access to all operations",
					"moderator": "Content moderation and management",
					"reviewer":  "Content review and approval",
					"user":      "Standard user operations",
				},
				"content_security": []string{
					"XSS Protection",
					"SQL Injection Prevention",
					"Content Validation",
					"Spam Detection",
					"File Upload Security",
				},
			},
			"endpoints": map[string]interface{}{
				"auth": map[string]interface{}{
					"register":          "POST /api/v1/auth/register",
					"login":             "POST /api/v1/auth/login",
					"logout":            "POST /api/v1/auth/logout",
					"refresh":           "POST /api/v1/auth/refresh",
					"forgot_password":   "POST /api/v1/auth/forgot-password",
					"reset_password":    "POST /api/v1/auth/reset-password",
					"change_password":   "POST /api/v1/auth/change-password",
					"verify_email":      "POST /api/v1/auth/verify-email",
					"send_verification": "POST /api/v1/auth/send-verification",
					"oauth_login":       "POST /api/v1/auth/oauth/login",
					"sessions":          "GET /api/v1/auth/sessions",
					"revoke_session":    "DELETE /api/v1/auth/sessions/{id}",
				},
				"users": map[string]interface{}{
					"profile":         "GET /api/v1/users/profile",
					"update_profile":  "PUT /api/v1/users/profile/update",
					"upload_image":    "POST /api/v1/users/profile/image",
					"upload_cv":       "POST /api/v1/users/profile/cv",
					"deactivate":      "DELETE /api/v1/users/profile/deactivate",
					"list_users":      "GET /api/v1/users",
					"search_users":    "GET /api/v1/users/search",
					"get_user":        "GET /api/v1/users/{id}",
					"get_by_username": "GET /api/v1/users/username/{username}",
					"user_stats":      "GET /api/v1/users/{id}/stats",
					"user_activity":   "GET /api/v1/users/{id}/activity",
					"online_users":    "GET /api/v1/users/online",
					"leaderboard":     "GET /api/v1/users/leaderboard",
					"update_status":   "POST /api/v1/users/status/online",
				},
				"posts": map[string]interface{}{
					"create_post":       "POST /api/v1/posts",
					"list_posts":        "GET /api/v1/posts",
					"get_post":          "GET /api/v1/posts/{id}",
					"update_post":       "PUT /api/v1/posts/{id} (Owner/Moderator/Admin)",
					"delete_post":       "DELETE /api/v1/posts/{id} (Owner/Moderator/Admin)",
					"posts_by_user":     "GET /api/v1/posts/user/{userId}",
					"posts_by_category": "GET /api/v1/posts/category/{category}",
					"trending_posts":    "GET /api/v1/posts/trending",
					"featured_posts":    "GET /api/v1/posts/featured",
					"search_posts":      "GET /api/v1/posts/search",
					"react_to_post":     "POST /api/v1/posts/{id}/react",
					"remove_reaction":   "DELETE /api/v1/posts/{id}/react",
					"bookmark_post":     "POST /api/v1/posts/{id}/bookmark",
					"unbookmark_post":   "DELETE /api/v1/posts/{id}/bookmark",
					"share_post":        "POST /api/v1/posts/{id}/share",
					"report_post":       "POST /api/v1/posts/{id}/report",
					"moderate_post":     "POST /api/v1/posts/{id}/moderate (Moderator/Admin only)",
					"post_stats":        "GET /api/v1/posts/{id}/stats",
					"post_analytics":    "GET /api/v1/posts/analytics",
				},
				"comments": map[string]interface{}{
					"create_comment":       "POST /api/v1/comments",
					"get_comment":          "GET /api/v1/comments/{id}",
					"update_comment":       "PUT /api/v1/comments/{id} (Owner/Moderator/Admin)",
					"delete_comment":       "DELETE /api/v1/comments/{id} (Owner/Moderator/Admin)",
					"comments_by_post":     "GET /api/v1/comments/post/{postId}",
					"comments_by_question": "GET /api/v1/comments/question/{questionId}",
					"comments_by_document": "GET /api/v1/comments/document/{docId}",
					"comments_by_user":     "GET /api/v1/comments/user/{userId}",
					"trending_comments":    "GET /api/v1/comments/trending",
					"recent_comments":      "GET /api/v1/comments/recent",
					"search_comments":      "GET /api/v1/comments/search",
					"react_to_comment":     "POST /api/v1/comments/{id}/react",
					"remove_reaction":      "DELETE /api/v1/comments/{id}/react",
					"report_comment":       "POST /api/v1/comments/{id}/report",
					"moderate_comment":     "POST /api/v1/comments/{id}/moderate (Moderator/Admin only)",
					"comment_stats":        "GET /api/v1/comments/{id}/stats",
					"comment_analytics":    "GET /api/v1/comments/analytics",
					"moderation_queue":     "GET /api/v1/comments/moderation/queue (Moderator/Admin only)",
				},
			},
			"jobs": map[string]interface{}{
				"create_job":         "POST /api/v1/jobs",
				"list_jobs":          "GET /api/v1/jobs",
				"get_job":            "GET /api/v1/jobs/{id}",
				"update_job":         "PUT /api/v1/jobs/{id} (Owner only)",
				"delete_job":         "DELETE /api/v1/jobs/{id} (Owner only)",
				"jobs_by_employer":   "GET /api/v1/jobs/employer/{employerId}",
				"featured_jobs":      "GET /api/v1/jobs/featured",
				"search_jobs":        "GET /api/v1/jobs/search",
				"apply_for_job":      "POST /api/v1/jobs/{id}/apply",
				"get_applications":   "GET /api/v1/jobs/{id}/applications (Owner only)",
				"my_applications":    "GET /api/v1/jobs/my-applications",
				"review_application": "POST /api/v1/jobs/{id}/applications/{appId}/review (Owner only)",
				"job_stats":          "GET /api/v1/jobs/stats",
			},
			"features": []string{
				"JWT Authentication",
				"OAuth Integration",
				"Role-based Access Control (RBAC)",
				"Content Security & Validation",
				"XSS & SQL Injection Protection",
				"Spam Detection & Content Filtering",
				"File Upload Security",
				"Admin/Moderator Permissions",
				"Rate Limiting",
				"Request Validation",
				"User Management",
				"Profile Management",
				"File Upload Support",
				"User Search & Analytics",
				"Post Management",
				"Content Engagement",
				"Full-text Search",
				"Content Moderation",
				"Post Analytics",
				"Comment Management",
				"Comment Reactions",
				"Comment Moderation",
				"Comment Analytics",
				"Multi-parent Comments",
				"Media Upload Support",
				"Comprehensive Monitoring",
				"Security Headers",
				"Error Handling",
			},
		}

		response.QuickSuccess(w, r, info)
	}))

	// Health check endpoint for API
	mux.Handle("/api/v1/health", createAPIHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		// Get health from service collection
		ctx := r.Context()
		health, err := serviceCollection.HealthCheck(ctx)

		statusCode := http.StatusOK
		if err != nil || health.Status != "healthy" {
			statusCode = http.StatusServiceUnavailable
		}

		healthData := map[string]interface{}{
			"status":    health.Status,
			"version":   "1.0.0",
			"timestamp": health.Timestamp,
			"services":  health.Services,
			"uptime":    health.Uptime,
			"security": map[string]interface{}{
				"rbac_enabled":             true,
				"content_security":         true,
				"file_upload_security":     true,
				"xss_protection":           true,
				"sql_injection_protection": true,
			},
		}

		if statusCode == http.StatusOK {
			response.QuickSuccess(w, r, healthData)
		} else {
			response.QuickError(w, r, fmt.Errorf("API unhealthy"))
		}
	}))

	// 🆕 UPDATED LOGGER WITH COMMENT ENDPOINTS
	logger.Info("Enhanced API v1 routes added successfully",
		zap.Int("auth_endpoints", 15),
		zap.Int("user_endpoints", 13),
		zap.Int("post_endpoints", 18),
		zap.Int("comment_endpoints", 18),
		zap.Int("job_endpoints", 12),
		zap.Bool("rbac_enabled", true),
		zap.Bool("content_security_enabled", true),
		zap.String("base_path", "/api/v1"),
	)
}

// ===============================
// 🛡️ ENHANCED HELPER FUNCTIONS (Role-based Security)
// ===============================

// createAPIHandler creates a basic API handler with CORS
func createAPIHandler(handlerFunc http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for API endpoints
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Content-Type", "application/json")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handlerFunc(w, r)
	})
}

// createAuthenticatedAPIHandler creates an API handler that requires authentication
func createAuthenticatedAPIHandler(handlerFunc http.HandlerFunc, authMiddleware *middleware.AuthMiddleware) http.Handler {
	// First apply CORS and content type
	handler := createAPIHandler(handlerFunc)

	// Then apply authentication middleware
	return authMiddleware.RequireAuth()(handler)
}

// 🛡️ createModeratorAPIHandler creates an API handler that requires moderator or admin role
func createModeratorAPIHandler(handlerFunc http.HandlerFunc, authMiddleware *middleware.AuthMiddleware) http.Handler {
	// First apply CORS and content type
	handler := createAPIHandler(handlerFunc)
 
	// Apply authentication middleware
	handler = authMiddleware.RequireAuth()(handler)

	// Then apply role-based authorization (Moderator or Admin)
	return authMiddleware.RequireRole("admin", "moderator")(handler)
}

// 🛡️ createAdminAPIHandler creates an API handler that requires admin role only
func createAdminAPIHandler(handlerFunc http.HandlerFunc, authMiddleware *middleware.AuthMiddleware) http.Handler {
	// First apply CORS and content type
	handler := createAPIHandler(handlerFunc)

	// Apply authentication middleware
	handler = authMiddleware.RequireAuth()(handler)

	// Then apply role-based authorization (Admin only)
	return authMiddleware.RequireRole("admin")(handler)
}

// 🛡️ createOwnershipAPIHandler creates an API handler that requires resource ownership
func createOwnershipAPIHandler(handlerFunc http.HandlerFunc, authMiddleware *middleware.AuthMiddleware, resourceType string) http.Handler {
	// First apply CORS and content type
	handler := createAPIHandler(handlerFunc)

	// Apply authentication middleware
	handler = authMiddleware.RequireAuth()(handler)

	// Then apply ownership-based authorization
	return authMiddleware.RequireOwnership(resourceType)(handler)
}

// 🛡️ createMethodHandler creates a handler that routes different HTTP methods to different handlers
func createMethodHandler(methodHandlers map[string]http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler, exists := methodHandlers[r.Method]; exists {
			handler.ServeHTTP(w, r)
		} else {
			response.QuickStatusResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}
