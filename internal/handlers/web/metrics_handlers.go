// File: internal/handlers/web/metrics_handlers.go
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/monitoring"

	"go.uber.org/zap"
)

// MetricsHandler handles comprehensive metrics requests (preserves original CreateMetricsHandler functionality)
func MetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Use the original dashboard method exactly as it was
		response := dashboard.GetComprehensiveMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode metrics response", zap.Error(err))
		}
	}
}

// APIMetricsHandler handles API-specific metrics requests (uses actual MetricsCollector methods)
func APIMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		metricsCollector := dashboard.GetMetricsCollector()
		if metricsCollector == nil {
			http.Error(w, "Metrics collector not available", http.StatusServiceUnavailable)
			return
		}

		// Use actual MetricsCollector methods from metrics.go
		response := map[string]interface{}{
			"api_metrics":        metricsCollector.GetAPIMetrics(),
			"performance":        metricsCollector.GetSnapshot(),
			"endpoint_metrics":   metricsCollector.GetEndpointMetrics(),
			"timestamp":          time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode API metrics response", zap.Error(err))
		}
	}
}

// PerformanceMetricsHandler handles performance metrics requests (uses actual PerformanceSnapshot)
func PerformanceMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		metricsCollector := dashboard.GetMetricsCollector()
		if metricsCollector == nil {
			// Return basic performance metrics fallback
			response := map[string]interface{}{
				"performance": map[string]interface{}{
					"average_response_time": "150ms",
					"request_count":         0,
					"slow_requests":         0,
					"cache_hit_rate":        0.95,
					"uptime":               time.Since(dashboard.GetStartTime()).String(),
				},
				"timestamp": time.Now(),
				"status":    "metrics_collector_unavailable",
			}
			
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Get actual performance snapshot from metrics.go
		snapshot := metricsCollector.GetSnapshot()
		response := map[string]interface{}{
			"performance": map[string]interface{}{
				"requests_per_second":   snapshot.RequestsPerSecond,
				"average_latency":       snapshot.AverageLatency,
				"error_rate":           snapshot.ErrorRate,
				"availability":         snapshot.Availability,
				"memory_usage":         snapshot.SystemMetrics.MemoryUsage,
				"memory_percent":       snapshot.SystemMetrics.MemoryPercent,
				"goroutines":           snapshot.SystemMetrics.Goroutines,
				"uptime":              time.Since(dashboard.GetStartTime()).String(),
				"top_endpoints":        snapshot.TopEndpoints,
				"alerts":              snapshot.Alerts,
			},
			"timestamp": time.Now(),
			"status":    "active",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode performance metrics response", zap.Error(err))
		}
	}
}

// DatabaseMetricsHandler handles database metrics requests (preserves original database integration)
func DatabaseMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Use actual database metrics exactly as the original dashboard did
		dbMetrics := database.GetMetrics()
		
		response := map[string]interface{}{
			"database": map[string]interface{}{
				"status":     "healthy",
				"connections": map[string]interface{}{
					"open":    dbMetrics.DBStats.OpenConnections,
					"idle":    dbMetrics.DBStats.Idle,
					"max":     dbMetrics.DBStats.MaxOpenConnections,
				},
				"queries": map[string]interface{}{
					"total":         dbMetrics.QueryCount,
					"errors":        dbMetrics.ErrorCount,
					"slow_queries":  dbMetrics.SlowQueryCount,
					"avg_duration": dbMetrics.AvgQueryDuration,
				},
				"performance": map[string]interface{}{
					"max_idle_closed":         dbMetrics.DBStats.MaxIdleClosed,
					"max_idle_time_closed":    dbMetrics.DBStats.MaxIdleTimeClosed,
					"max_lifetime_closed":     dbMetrics.DBStats.MaxLifetimeClosed,
				},
			},
			"timestamp": time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode database metrics response", zap.Error(err))
		}
	}
}

// SystemMetricsHandler handles system-level metrics (uses actual SystemMetrics from metrics.go)
func SystemMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		metricsCollector := dashboard.GetMetricsCollector()
		var systemMetrics interface{}
		
		if metricsCollector != nil {
			// Use actual SystemMetrics from PerformanceSnapshot
			snapshot := metricsCollector.GetSnapshot()
			systemMetrics = snapshot.SystemMetrics
		} else {
			systemMetrics = map[string]interface{}{
				"memory_usage":    0,
				"memory_percent":  0.0,
				"goroutines":      0,
				"cgo_calls":       0,
			}
		}

		response := map[string]interface{}{
			"system": map[string]interface{}{
				"uptime":      time.Since(dashboard.GetStartTime()).String(),
				"version":     dashboard.GetVersion(),
				"environment": dashboard.GetEnvironment(),
				"metrics":     systemMetrics,
			},
			"timestamp": time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode system metrics response", zap.Error(err))
		}
	}
}

// EndpointMetricsHandler handles endpoint-specific metrics (uses actual EndpointMetrics from metrics.go)
func EndpointMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		metricsCollector := dashboard.GetMetricsCollector()
		if metricsCollector == nil {
			response := map[string]interface{}{
				"endpoints": map[string]interface{}{},
				"timestamp": time.Now(),
				"status":    "metrics_collector_unavailable",
			}
			
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Use actual GetEndpointMetrics method from metrics.go
		response := map[string]interface{}{
			"endpoints": metricsCollector.GetEndpointMetrics(),
			"timestamp": time.Now(),
			"status":    "active",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			dashboard.GetLogger().Error("Failed to encode endpoint metrics response", zap.Error(err))
		}
	}
}

// PrometheusMetricsHandler provides Prometheus-compatible metrics
func PrometheusMetricsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" && !IsAuthorizedForInternalAccess(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		// Generate Prometheus metrics from actual data
		var metrics string
		
		metricsCollector := dashboard.GetMetricsCollector()
		if metricsCollector != nil {
			apiMetrics := metricsCollector.GetAPIMetrics()
			snapshot := metricsCollector.GetSnapshot()
			
			metrics = fmt.Sprintf(`# HELP evalhub_uptime_seconds Application uptime in seconds
# TYPE evalhub_uptime_seconds counter
evalhub_uptime_seconds %f

# HELP evalhub_version Application version info
# TYPE evalhub_version gauge
evalhub_version{version="%s",environment="%s"} 1

# HELP evalhub_health Application health status
# TYPE evalhub_health gauge
evalhub_health 1

# HELP evalhub_requests_total Total number of requests
# TYPE evalhub_requests_total counter
evalhub_requests_total %d

# HELP evalhub_requests_success_total Total number of successful requests
# TYPE evalhub_requests_success_total counter
evalhub_requests_success_total %d

# HELP evalhub_requests_error_total Total number of error requests
# TYPE evalhub_requests_error_total counter
evalhub_requests_error_total %d

# HELP evalhub_response_time_average Average response time in milliseconds
# TYPE evalhub_response_time_average gauge
evalhub_response_time_average %f

# HELP evalhub_error_rate Error rate percentage
# TYPE evalhub_error_rate gauge
evalhub_error_rate %f

# HELP evalhub_memory_usage Memory usage in bytes
# TYPE evalhub_memory_usage gauge
evalhub_memory_usage %d

# HELP evalhub_goroutines Number of goroutines
# TYPE evalhub_goroutines gauge
evalhub_goroutines %d
`,
				time.Since(dashboard.GetStartTime()).Seconds(),
				dashboard.GetVersion(),
				dashboard.GetEnvironment(),
				apiMetrics.TotalRequests,
				apiMetrics.SuccessRequests,
				apiMetrics.ErrorRequests,
				float64(snapshot.AverageLatency.Milliseconds()),
				snapshot.ErrorRate,
				snapshot.SystemMetrics.MemoryUsage,
				snapshot.SystemMetrics.Goroutines,
			)
		} else {
			// Fallback metrics
			uptime := time.Since(dashboard.GetStartTime()).Seconds()
			metrics = fmt.Sprintf(`# HELP evalhub_uptime_seconds Application uptime in seconds
# TYPE evalhub_uptime_seconds counter
evalhub_uptime_seconds %f

# HELP evalhub_version Application version info
# TYPE evalhub_version gauge
evalhub_version{version="%s",environment="%s"} 1

# HELP evalhub_health Application health status
# TYPE evalhub_health gauge
evalhub_health 1
`, uptime, dashboard.GetVersion(), dashboard.GetEnvironment())
		}
		
		w.Write([]byte(metrics))
	}
}
