// File: internal/handlers/web/dashboard_handlers.go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"evalhub/internal/monitoring"

	"go.uber.org/zap"
)

// ComprehensiveDashboardHandler handles comprehensive dashboard requests (preserves original CreateDashboardHandler functionality)
func ComprehensiveDashboardHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// Use the exact same method as the original CreateDashboardHandler
		dashboardData := dashboard.GetDashboardData(ctx)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(dashboardData); err != nil {
			dashboard.GetLogger().Error("Failed to encode dashboard response", zap.Error(err))
		}
	}
}

// MonitoringDashboardHandler provides monitoring-focused dashboard
func MonitoringDashboardHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)
		metrics := dashboard.GetComprehensiveMetrics()

		// Create monitoring-focused view with all original data
		monitoringData := map[string]interface{}{
			"overview": map[string]interface{}{
				"status":      health.Status,
				"uptime":      health.Uptime,
				"version":     health.Version,
				"environment": health.Environment,
				"timestamp":   time.Now(),
			},
			"summary": health.Summary,
			"performance": map[string]interface{}{
				"requests_per_second": health.Performance.RequestsPerSecond,
				"average_latency":     health.Performance.AverageLatency,
				"error_rate":         health.Performance.ErrorRate,
				"availability":       health.Performance.Availability,
				"active_requests":    health.Performance.ActiveRequests,
				"peak_active_requests": health.Performance.PeakActiveRequests,
				"slow_requests_percent": health.Performance.SlowRequestsPercent,
			},
			"components": func() map[string]interface{} {
				components := make(map[string]interface{})
				for name, component := range health.Components {
					components[name] = map[string]interface{}{
						"status":        component.Status,
						"response_time": component.ResponseTime,
						"last_check":    component.LastCheck,
						"details":       component.Details,
						"error":         component.Error,
					}
				}
				return components
			}(),
			"resources": health.Resources,
			"dependencies": health.Dependencies,
			"alerts": health.Alerts,
			"issues": health.Issues,
			"metrics": map[string]interface{}{
				"api":      metrics["api"],
				"database": metrics["database"],
				"system":   metrics["system"],
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(monitoringData); err != nil {
			dashboard.GetLogger().Error("Failed to encode monitoring dashboard response", zap.Error(err))
		}
	}
}

// AlertsDashboardHandler provides alerts-focused dashboard
func AlertsDashboardHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)

		// Create alerts-focused view with full alert information
		alertsData := map[string]interface{}{
			"summary": map[string]interface{}{
				"total_alerts":    len(health.Alerts),
				"critical_issues": health.Summary.CriticalIssues,
				"active_alerts":   health.Summary.ActiveAlerts,
				"timestamp":       time.Now(),
				"overall_status":  health.Status,
			},
			"alerts": health.Alerts,
			"issues": health.Issues,
			"component_status": func() map[string]interface{} {
				status := make(map[string]interface{})
				for name, component := range health.Components {
					status[name] = map[string]interface{}{
						"status": component.Status,
						"error":  component.Error,
						"last_check": component.LastCheck,
					}
				}
				return status
			}(),
			"performance_alerts": func() []interface{} {
				var perfAlerts []interface{}
				for _, alert := range health.Alerts {
					if alert.Type == "performance" || alert.Component == "api" {
						perfAlerts = append(perfAlerts, alert)
					}
				}
				return perfAlerts
			}(),
			"critical_issues": func() []interface{} {
				var criticalIssues []interface{}
				for _, issue := range health.Issues {
					if issue.Severity == "critical" {
						criticalIssues = append(criticalIssues, issue)
					}
				}
				return criticalIssues
			}(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(alertsData); err != nil {
			dashboard.GetLogger().Error("Failed to encode alerts dashboard response", zap.Error(err))
		}
	}
}

// PerformanceDashboardHandler provides performance-focused dashboard
func PerformanceDashboardHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production"{
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)
		metrics := dashboard.GetComprehensiveMetrics()

		// Get additional performance data from metrics collector
		var endpointMetrics interface{}
		var topEndpoints interface{}
		
		metricsCollector := dashboard.GetMetricsCollector()
		if metricsCollector != nil {
			endpointMetrics = metricsCollector.GetEndpointMetrics()
			snapshot := metricsCollector.GetSnapshot()
			topEndpoints = snapshot.TopEndpoints
		} else {
			endpointMetrics = map[string]interface{}{}
			topEndpoints = []interface{}{}
		}

		// Create performance-focused view with comprehensive performance data
		performanceData := map[string]interface{}{
			"overview": map[string]interface{}{
				"requests_per_second":   health.Performance.RequestsPerSecond,
				"average_latency":       health.Performance.AverageLatency,
				"error_rate":           health.Performance.ErrorRate,
				"availability":         health.Performance.Availability,
				"active_requests":      health.Performance.ActiveRequests,
				"peak_active_requests": health.Performance.PeakActiveRequests,
				"slow_requests_percent": health.Performance.SlowRequestsPercent,
				"timestamp":            time.Now(),
			},
			"scores": map[string]interface{}{
				"performance_score": health.Summary.PerformanceScore,
				"reliability_score": health.Summary.ReliabilityScore,
				"operational_score": health.Summary.OperationalScore,
			},
			"resources": health.Resources,
			"api_metrics": metrics["api"],
			"endpoint_metrics": endpointMetrics,
			"top_endpoints": topEndpoints,
			"performance_issues": func() []interface{} {
				var perfIssues []interface{}
				for _, issue := range health.Issues {
					if issue.Component == "api" || issue.Type == "performance" {
						perfIssues = append(perfIssues, issue)
					}
				}
				return perfIssues
			}(),
			"system_performance": func() map[string]interface{} {
				return map[string]interface{}{
					"memory_usage":    health.Resources.Memory.Usage,
					"memory_status":   health.Resources.Memory.Status,
					"goroutines":      health.Resources.Goroutines.Value,
					"database_usage":  health.Resources.Database.Usage,
					"database_status": health.Resources.Database.Status,
				}
			}(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(performanceData); err != nil {
			dashboard.GetLogger().Error("Failed to encode performance dashboard response", zap.Error(err))
		}
	}
}

// SystemDashboardHandler provides system-focused dashboard
func SystemDashboardHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization for internal routes
		if dashboard.GetEnvironment() == "production" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)
		metrics := dashboard.GetComprehensiveMetrics()

		// Create system-focused view with all system information
		systemData := map[string]interface{}{
			"info": map[string]interface{}{
				"version":     health.Version,
				"environment": health.Environment,
				"uptime":      health.Uptime,
				"status":      health.Status,
				"timestamp":   time.Now(),
			},
			"components": health.Components,
			"dependencies": health.Dependencies,
			"resources": health.Resources,
			"system_metrics": metrics["system"],
			"database_metrics": metrics["database"],
			"configuration": map[string]interface{}{
				"environment":     dashboard.GetEnvironment(),
				"debug_mode":      dashboard.GetEnvironment() != "production",
				"features_enabled": map[string]bool{
					"enhanced_error_handling": true,
					"panic_recovery":          true,
					"error_metrics":           true,
					"security_monitoring":     true,
					"performance_monitoring":  true,
					"health_checks":           true,
					"real_time_metrics":       true,
					"endpoint_tracking":       true,
					"user_metrics":           true,
				},
			},
			"health_summary": health.Summary,
			"system_health": map[string]interface{}{
				"healthy_components": health.Summary.HealthyComponents,
				"total_components":   health.Summary.TotalComponents,
				"component_health_percentage": func() float64 {
					if health.Summary.TotalComponents > 0 {
						return float64(health.Summary.HealthyComponents) / float64(health.Summary.TotalComponents) * 100
					}
					return 0
				}(),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(systemData); err != nil {
			dashboard.GetLogger().Error("Failed to encode system dashboard response", zap.Error(err))
		}
	}
}

// QuickStatsHandler provides quick stats for lightweight monitoring (preserves quick access pattern)
func QuickStatsHandler(dashboard *monitoring.Dashboard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		health := dashboard.GetSystemHealth(ctx)

		// Create quick stats view with essential information only
		quickStats := map[string]interface{}{
			"status":           health.Status,
			"uptime":          health.Uptime,
			"healthy_components": health.Summary.HealthyComponents,
			"total_components":   health.Summary.TotalComponents,
			"active_alerts":      health.Summary.ActiveAlerts,
			"critical_issues":    health.Summary.CriticalIssues,
			"performance_score":  health.Summary.PerformanceScore,
			"reliability_score":  health.Summary.ReliabilityScore,
			"operational_score":  health.Summary.OperationalScore,
			"error_rate":        health.Performance.ErrorRate,
			"requests_per_second": health.Performance.RequestsPerSecond,
			"average_latency":    health.Performance.AverageLatency,
			"availability":      health.Performance.Availability,
			"timestamp":         time.Now(),
			"environment":       health.Environment,
			"version":          health.Version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(quickStats); err != nil {
			dashboard.GetLogger().Error("Failed to encode quick stats response", zap.Error(err))
		}
	}
}
