package router

import (
	"encoding/json"
	"evalhub/internal/handlers/web"
	"evalhub/internal/middleware"
	"evalhub/internal/monitoring"
	"evalhub/internal/response"
	"evalhub/internal/services"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	_ "evalhub/docs" // Import generated docs

	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"
)

// SetupRouter configures all HTTP routes and returns the main handler
func SetupRouter(serviceCollection *services.ServiceCollection, authMiddleware *middleware.AuthMiddleware, responseBuilder *response.Builder, logger *zap.Logger) http.Handler {
	// Create a new ServeMux
	mux := http.NewServeMux()

	// Serve static files (CSS, JS)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Handle uploaded files
	mux.HandleFunc("/uploads/", ConfigureUpload(logger))

	// 🔧 FIX: Properly configure Swagger UI with custom config
	mux.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect /swagger to /swagger/
		if r.URL.Path == "/swagger" {
			http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
			return
		}

		// Configure Swagger with proper doc URL
		handler := httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"), // Point to the correct JSON endpoint
		)
		handler.ServeHTTP(w, r)
	})

	// 🔧 FIX: Add explicit Swagger JSON endpoint
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "./docs/swagger.json")
	})

	// 🔧 FIX: Add Swagger YAML endpoint
	mux.HandleFunc("/swagger/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		http.ServeFile(w, r, "./docs/swagger.yaml")
	})

	// Public routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			web.NotFound(w, r)
			return
		}
		web.GuestMiddleware(http.HandlerFunc(web.HomeHandler)).ServeHTTP(w, r)
	})

	// Add JSON API endpoint for documents
	mux.Handle("/documents", web.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "json" {
			web.GetDocumentsJSONHandler(w, r)
		} else {
			web.GetDocumentsHandler(w, r)
		}
	})))

	mux.HandleFunc("/login", web.Login)
	mux.HandleFunc("/signup", web.SignUp)
	mux.HandleFunc("/logout", web.Logout)

	// Authenticated routes
	mux.Handle("/dashboard", web.AuthMiddleware(http.HandlerFunc(web.DashboardHandler)))
	mux.Handle("/search", web.AuthMiddleware(http.HandlerFunc(web.SearchPostsHandler)))
	mux.Handle("/api/search-suggestions", web.AuthMiddleware(http.HandlerFunc(web.SearchSuggestions)))
	mux.Handle("/create-post", web.AuthMiddleware(http.HandlerFunc(web.CreatePostHandler)))
	mux.Handle("/create-question", web.AuthMiddleware(http.HandlerFunc(web.CreateQuestionHandler)))
	mux.Handle("/posts", web.AuthMiddleware(http.HandlerFunc(web.ListPostsHandler)))
	mux.Handle("/view-post", web.AuthMiddleware(http.HandlerFunc(web.ViewPostHandler)))
	mux.Handle("/edit-post", web.AuthMiddleware(
		web.OwnershipMiddleware(http.HandlerFunc(web.EditPostHandler))))
	mux.Handle("/delete-post", web.AuthMiddleware(
		web.OwnershipMiddleware(http.HandlerFunc(web.DeletePostHandler))))
	mux.Handle("/like-post", web.AuthMiddleware(http.HandlerFunc(web.LikePostHandler)))
	mux.Handle("/dislike-post", web.AuthMiddleware(http.HandlerFunc(web.DislikePostHandler)))
	mux.Handle("/add-comment", web.AuthMiddleware(http.HandlerFunc(web.CreateCommentHandler)))
	mux.Handle("/edit-comment", web.AuthMiddleware(http.HandlerFunc(web.EditCommentHandler)))
	mux.Handle("/delete-comment", web.AuthMiddleware(http.HandlerFunc(web.DeleteCommentHandler)))
	mux.Handle("/like-comment", web.AuthMiddleware(http.HandlerFunc(web.LikeCommentHandler)))
	mux.Handle("/dislike-comment", web.AuthMiddleware(http.HandlerFunc(web.DislikeCommentHandler)))
	mux.Handle("/posts-by-category", web.AuthMiddleware(http.HandlerFunc(web.PostsByCategoryHandler)))
	mux.Handle("/user-posts", web.AuthMiddleware(http.HandlerFunc(web.UserPostHandler)))
	mux.Handle("/liked-posts", web.AuthMiddleware(http.HandlerFunc(web.FilterLikesHandler)))
	mux.Handle("/profile", web.AuthMiddleware(http.HandlerFunc(web.ProfileHandler)))
	mux.Handle("/view-profile", web.AuthMiddleware(http.HandlerFunc(web.ViewProfileHandler)))
	mux.Handle("/download-cv", web.AuthMiddleware(http.HandlerFunc(web.DownloadCVHandler)))
	mux.Handle("/chat", web.AuthMiddleware(http.HandlerFunc(web.ChatHandler)))
	mux.Handle("/ws", web.AuthMiddleware(http.HandlerFunc(web.WebSocketHandler)))
	mux.Handle("/view-question", web.AuthMiddleware(http.HandlerFunc(web.ViewQuestionHandler)))
	mux.Handle("/like-question", web.AuthMiddleware(http.HandlerFunc(web.LikeQuestionHandler)))
	mux.Handle("/dislike-question", web.AuthMiddleware(http.HandlerFunc(web.DislikeQuestionHandler)))

	// Initialize job handlers with required services
	jobHandlers := web.NewJobHandlers(serviceCollection.JobService)
	jobHandlers.RegisterRoutes(mux, web.AuthMiddleware)

	// Notification routes
	mux.Handle("/notifications", web.AuthMiddleware(http.HandlerFunc(web.NotificationsHandler)))
	mux.Handle("/notification-preferences", web.AuthMiddleware(http.HandlerFunc(web.NotificationPreferencesHandler)))

	// Notification API routes
	mux.Handle("/api/notifications", web.AuthMiddleware(http.HandlerFunc(web.GetNotificationsAPIHandler)))
	mux.Handle("/api/notification-summary", web.AuthMiddleware(http.HandlerFunc(web.GetNotificationSummaryAPIHandler)))
	mux.Handle("/api/notifications/mark-read", web.AuthMiddleware(http.HandlerFunc(web.MarkNotificationsReadHandler)))

	// Community API routes
	mux.Handle("/api/community-stats", web.AuthMiddleware(http.HandlerFunc(web.GetCommunityStatsHandler)))
	mux.Handle("/api/user-stats", web.AuthMiddleware(http.HandlerFunc(web.GetUserStatsHandler)))

	// Document-related routes
	mux.Handle("/upload-document", web.AuthMiddleware(http.HandlerFunc(web.UploadDocumentHandler)))
	mux.Handle("/view-document", web.AuthMiddleware(http.HandlerFunc(web.ViewDocumentHandler)))
	mux.Handle("/delete-document", web.AuthMiddleware(http.HandlerFunc(web.DeleteDocumentHandler)))
	mux.Handle("/like-document", web.AuthMiddleware(http.HandlerFunc(web.LikeDocumentHandler)))
	mux.Handle("/dislike-document", web.AuthMiddleware(http.HandlerFunc(web.DislikeDocumentHandler)))
	mux.Handle("/document-comment", web.AuthMiddleware(http.HandlerFunc(web.CreateDocumentCommentHandler)))

	// Social Integration API routes
	mux.Handle("/api/share-content", web.AuthMiddleware(http.HandlerFunc(web.ShareContentHandler)))
	mux.HandleFunc("/api/generate-share-url", web.ShareContentHandler)

	// OAuth routes
	mux.HandleFunc("/auth/google/login", web.GoogleLoginHandler)
	mux.HandleFunc("/auth/google/callback", web.GoogleCallbackHandler)
	mux.HandleFunc("/auth/github/login", web.GitHubLogin)
	mux.HandleFunc("/auth/github/callback", web.GitHubCallback)

	// Error handling routes
	mux.HandleFunc("/404", web.NotFound)
	mux.HandleFunc("/test500", func(w http.ResponseWriter, r *http.Request) {
		web.RenderErrorPage(w, http.StatusInternalServerError, fmt.Errorf("test 500 error"))
	})

	// 🔧 FIX: Add API v1 routes BEFORE returning
	AddAPIv1Routes(mux, serviceCollection, authMiddleware, responseBuilder, logger)

	logger.Info("Router setup completed with Swagger integration",
		zap.String("swagger_ui", "http://localhost:9000/swagger/"),
		zap.String("swagger_json", "http://localhost:9000/swagger/doc.json"),
	)

	return mux
}

// 🆕 ENHANCED MONITORING ROUTES SETUP
func SetupMonitoringRoutes(mux *http.ServeMux, dashboard *monitoring.Dashboard, logger *zap.Logger) {
	// ===============================
	// PUBLIC HEALTH ENDPOINTS
	// ===============================

	// Basic health check (public)
	mux.HandleFunc("/health", web.HealthHandler(dashboard))
	mux.HandleFunc("/status", web.StatusHandler(dashboard))

	// Kubernetes-style probes (public)
	mux.HandleFunc("/healthz", web.LivenessHandler(dashboard))
	mux.HandleFunc("/readyz", web.ReadinessHandler(dashboard))

	// ===============================
	// INTERNAL MONITORING ENDPOINTS
	// ===============================

	// Comprehensive health checks (internal)
	mux.HandleFunc("/internal/health", web.DetailedHealthHandler(dashboard))
	mux.HandleFunc("/internal/health/detailed", web.DetailedHealthHandler(dashboard))

	// Metrics endpoints (internal)
	mux.HandleFunc("/internal/metrics", web.MetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/api", web.APIMetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/performance", web.PerformanceMetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/database", web.DatabaseMetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/system", web.SystemMetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/endpoints", web.EndpointMetricsHandler(dashboard))
	mux.HandleFunc("/internal/metrics/prometheus", web.PrometheusMetricsHandler(dashboard))

	// Dashboard endpoints (internal)
	mux.HandleFunc("/internal/dashboard", web.ComprehensiveDashboardHandler(dashboard))
	mux.HandleFunc("/internal/dashboard/monitoring", web.MonitoringDashboardHandler(dashboard))
	mux.HandleFunc("/internal/dashboard/alerts", web.AlertsDashboardHandler(dashboard))
	mux.HandleFunc("/internal/dashboard/performance", web.PerformanceDashboardHandler(dashboard))
	mux.HandleFunc("/internal/dashboard/system", web.SystemDashboardHandler(dashboard))
	mux.HandleFunc("/internal/dashboard/quick", web.QuickStatsHandler(dashboard))
	// ===============================
	// SECURITY MONITORING ENDPOINTS
	// ===============================

	// Security metrics and health
	mux.HandleFunc("/internal/metrics/security", web.SecurityMetricsHandler())
	mux.HandleFunc("/internal/security/health", web.SecurityHealthHandler())
	mux.HandleFunc("/internal/security/config", web.SecurityConfigHandler())

	// Security reporting endpoints
	mux.HandleFunc("/api/security/csp-report", web.CSPReportHandler(logger))
	mux.HandleFunc("/api/security/violations", web.SecurityViolationsHandler(logger))
	mux.HandleFunc("/api/security/hsts-report", web.HSTSReportHandler(logger))
	mux.HandleFunc("/api/security/expect-ct-report", web.ExpectCTReportHandler(logger))

	// ===============================
	// LEGACY COMPATIBILITY ENDPOINTS
	// ===============================

	// Maintain backward compatibility with existing monitoring
	setupLegacyCompatibilityRoutes(mux, dashboard)
}

// 🆕 ENHANCED ERROR MONITORING SETUP
func SetupErrorMonitoring(mux *http.ServeMux, tracker *middleware.ErrorTracker) {
	if mux == nil || tracker == nil {
		return
	}

	// Enhanced error metrics endpoint
	mux.HandleFunc("/internal/metrics/errors", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if !web.IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Use the actual error tracker
		middleware.GetErrorMetricsHandler(tracker)(w, r)
	})

	// Error analytics endpoint
	mux.HandleFunc("/internal/analytics/errors", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if !web.IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Enhanced error analytics
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"error_analytics": map[string]interface{}{
				"trends":           "error_trends_data",
				"top_errors":       "top_errors_data",
				"error_patterns":   "error_patterns_data",
				"resolution_times": "resolution_times_data",
			},
			"timestamp": time.Now(),
			"status":    "active",
		}

		json.NewEncoder(w).Encode(response)
	})
}

// 🆕 LEGACY COMPATIBILITY ROUTES
func setupLegacyCompatibilityRoutes(mux *http.ServeMux, dashboard *monitoring.Dashboard) {
	// Legacy routes for backward compatibility with existing monitoring tools

	// Map old routes to new handlers
	mux.HandleFunc("/metrics", web.MetricsHandler(dashboard))
	mux.HandleFunc("/ping", web.SimpleHealthHandler(dashboard))
	mux.HandleFunc("/version", web.StatusHandler(dashboard))

	// Legacy configuration endpoint
	mux.HandleFunc("/internal/config/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if !web.IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"configuration": map[string]interface{}{
				"environment":           dashboard.GetEnvironment(),
				"debug_mode":            dashboard.GetEnvironment() != "production",
				"logging_level":         getLoggingLevel(),
				"security_enabled":      true,
				"rate_limiting_enabled": true,
				"monitoring_enabled":    true,
			},
			"timestamp": time.Now(),
		}

		json.NewEncoder(w).Encode(response)
	})
}

// 🆕 UTILITY FUNCTIONS

// Helper function to get environment
func getEnvironment() string {
	env := os.Getenv("GO_ENV")
	if env == "" {
		return "development"
	}
	return env
}

// Helper function to get logging level
func getLoggingLevel() string {
	switch getEnvironment() {
	case "production":
		return "info"
	case "staging":
		return "debug"
	default:
		return "debug"
	}
}

// Helper function to get client IP
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// 🆕 ENHANCED UPLOAD HANDLER
func ConfigureUpload(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Enhanced error handling for upload
		defer func() {
			if err := recover(); err != nil {
				// Log the panic and return a proper error response
				logger.Error("Panic in upload handler",
					zap.Any("error", err),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
				)
				http.Error(w, "Upload failed due to server error", http.StatusInternalServerError)
			}
		}()

		// Basic security checks
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check content length
		if r.ContentLength > 100*1024*1024 { // 100MB limit
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
			return
		}

		// Your existing upload implementation should go here
		// This is just a placeholder - use your actual implementation
		http.Error(w, "Upload handler not implemented", http.StatusNotImplemented)
	}
}
