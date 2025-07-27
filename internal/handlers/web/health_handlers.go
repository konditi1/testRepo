// File: internal/handlers/web/health_handlers.go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"evalhub/internal/monitoring"

	"go.uber.org/zap"
)

// HealthHandler handles basic health check requests (preserves original CreateHealthHandler functionality)
func HealthHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Use the original dashboard method exactly as it was
		health := dashboard.GetSystemHealth(ctx)

		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code based on health status (original logic preserved)
		switch health.Status {
		case "healthy":
			w.WriteHeader(http.StatusOK)
		case "degraded":
			w.WriteHeader(http.StatusOK) // Still OK, but with warnings
		case "unhealthy":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}

		if err := json.NewEncoder(w).Encode(health); err != nil {
			dashboard.GetLogger().Error("Failed to encode health response", zap.Error(err))
		}
	}
}

// DetailedHealthHandler handles detailed health check requests (preserves original functionality)
func DetailedHealthHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes in production
		if dashboard.GetEnvironment() == "production" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// Use the exact same logic as the original CreateHealthHandler
		health := dashboard.GetSystemHealth(ctx)

		w.Header().Set("Content-Type", "application/json")

		// Same status code logic as original
		switch health.Status {
		case "healthy":
			w.WriteHeader(http.StatusOK)
		case "degraded":
			w.WriteHeader(http.StatusOK)
		case "unhealthy":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}

		if err := json.NewEncoder(w).Encode(health); err != nil {
			dashboard.GetLogger().Error("Failed to encode detailed health response", zap.Error(err))
		}
	}
}

// SimpleHealthHandler provides a simple health check (new functionality for k8s probes)
func SimpleHealthHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"status":      "operational",
			"timestamp":   time.Now(),
			"uptime":      time.Since(dashboard.GetStartTime()).String(),
			"version":     dashboard.GetVersion(),
			"environment": dashboard.GetEnvironment(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode simple health response", zap.Error(err))
		}
	}
}

// LivenessHandler provides Kubernetes-style liveness probe
func LivenessHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Simple liveness check - just return OK if server is running
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"status":    "alive",
			"timestamp": time.Now(),
		}

		json.NewEncoder(w).Encode(response)
	}
}

// ReadinessHandler provides Kubernetes-style readiness probe
func ReadinessHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)

		w.Header().Set("Content-Type", "application/json")

		// Readiness is more strict than liveness
		if health.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		response := map[string]interface{}{
			"status":    health.Status,
			"ready":     health.Status == "healthy",
			"timestamp": time.Now(),
			"components": func() map[string]bool {
				components := make(map[string]bool)
				for name, component := range health.Components {
					components[name] = component.Status == "healthy"
				}
				return components
			}(),
		}

		json.NewEncoder(w).Encode(response)
	}
}

// StatusHandler provides application status information (preserves original simple status)
func StatusHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"application": "EvalHub",
			"version":     dashboard.GetVersion(),
			"environment": dashboard.GetEnvironment(),
			"uptime":      time.Since(dashboard.GetStartTime()).String(),
			"status":      "operational",
			"timestamp":   time.Now(),
			"features": map[string]interface{}{
				"enhanced_error_handling": true,
				"panic_recovery":          true,
				"error_metrics":           true,
				"graceful_degradation":    true,
				"enhanced_security":       true,
				"security_monitoring":     true,
				"performance_monitoring":  true,
				"health_checks":           true,
			},
		}

		json.NewEncoder(w).Encode(response)
	}
}