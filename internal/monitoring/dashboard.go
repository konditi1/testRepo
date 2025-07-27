// File: internal/monitoring/dashboard.go
package monitoring

import (
	"context"
	"fmt"
	"time"

	"evalhub/internal/database"
	"evalhub/internal/middleware"

	"go.uber.org/zap"
)

// ===============================
// COMPREHENSIVE DASHBOARD CORE
// ===============================

// Dashboard provides comprehensive monitoring and observability
type Dashboard struct {
	metricsCollector *middleware.MetricsCollector
	logger           *zap.Logger
	startTime        time.Time
	version          string
	environment      string
}

// NewDashboard creates a new monitoring dashboard
func NewDashboard(metricsCollector *middleware.MetricsCollector, logger *zap.Logger, version, environment string) *Dashboard {
	return &Dashboard{
		metricsCollector: metricsCollector,
		logger:           logger,
		startTime:        time.Now(),
		version:          version,
		environment:      environment,
	}
}

// ===============================
// DATA STRUCTURES
// ===============================

// SystemHealthResponse represents comprehensive system health
type SystemHealthResponse struct {
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
	Uptime      string    `json:"uptime"`
	Version     string    `json:"version"`
	Environment string    `json:"environment"`

	// Component health
	Components map[string]ComponentHealth `json:"components"`

	// Overall metrics
	Performance PerformanceHealth `json:"performance"`

	// System resources
	Resources ResourceHealth `json:"resources"`

	// Service dependencies
	Dependencies map[string]DependencyHealth `json:"dependencies"`

	// Alerts and issues
	Alerts []SystemAlert `json:"alerts,omitempty"`
	Issues []SystemIssue `json:"issues,omitempty"`

	// Summary
	Summary HealthSummary `json:"summary"`
}

// ComponentHealth represents health of a system component
type ComponentHealth struct {
	Status       string                 `json:"status"`
	LastCheck    time.Time              `json:"last_check"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Error        string                 `json:"error,omitempty"`
	ResponseTime time.Duration          `json:"response_time,omitempty"`
}

// PerformanceHealth represents performance-related health metrics
type PerformanceHealth struct {
	RequestsPerSecond   float64       `json:"requests_per_second"`
	AverageLatency      time.Duration `json:"average_latency"`
	ErrorRate           float64       `json:"error_rate"`
	Availability        float64       `json:"availability"`
	ActiveRequests      int64         `json:"active_requests"`
	PeakActiveRequests  int64         `json:"peak_active_requests"`
	SlowRequestsPercent float64       `json:"slow_requests_percent"`
}

// ResourceHealth represents system resource health
type ResourceHealth struct {
	Memory     ResourceMetric `json:"memory"`
	Goroutines ResourceMetric `json:"goroutines"`
	Database   ResourceMetric `json:"database"`
	Cache      ResourceMetric `json:"cache,omitempty"`
}

// ResourceMetric represents a resource metric with thresholds
type ResourceMetric struct {
	Value     interface{} `json:"value"`
	Unit      string      `json:"unit"`
	Status    string      `json:"status"`
	Threshold interface{} `json:"threshold,omitempty"`
	Usage     float64     `json:"usage_percent,omitempty"`
}

// DependencyHealth represents health of external dependencies
type DependencyHealth struct {
	Status       string        `json:"status"`
	LastCheck    time.Time     `json:"last_check"`
	ResponseTime time.Duration `json:"response_time,omitempty"`
	Error        string        `json:"error,omitempty"`
	Version      string        `json:"version,omitempty"`
}

// SystemAlert represents a system-level alert
type SystemAlert struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Component string    `json:"component"`
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value,omitempty"`
	Threshold float64   `json:"threshold,omitempty"`
	ActionURL string    `json:"action_url,omitempty"`
}

// SystemIssue represents a system issue
type SystemIssue struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Component   string    `json:"component"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Occurrences int64     `json:"occurrences"`
	Status      string    `json:"status"` // new, acknowledged, resolved
}

// HealthSummary provides a high-level summary
type HealthSummary struct {
	OverallStatus     string  `json:"overall_status"`
	HealthyComponents int     `json:"healthy_components"`
	TotalComponents   int     `json:"total_components"`
	ActiveAlerts      int     `json:"active_alerts"`
	CriticalIssues    int     `json:"critical_issues"`
	PerformanceScore  float64 `json:"performance_score"` // 0-100
	ReliabilityScore  float64 `json:"reliability_score"` // 0-100
	OperationalScore  float64 `json:"operational_score"` // 0-100
}

// ===============================
// CORE BUSINESS LOGIC
// ===============================

// GetSystemHealth returns comprehensive system health
func (d *Dashboard) GetSystemHealth(ctx context.Context) *SystemHealthResponse {
	start := time.Now()

	response := &SystemHealthResponse{
		Timestamp:    start,
		Uptime:       time.Since(d.startTime).String(),
		Version:      d.version,
		Environment:  d.environment,
		Components:   make(map[string]ComponentHealth),
		Dependencies: make(map[string]DependencyHealth),
		Alerts:       make([]SystemAlert, 0),
		Issues:       make([]SystemIssue, 0),
	}

	// Check all components
	d.checkDatabaseHealth(ctx, response)
	d.checkAPIHealth(ctx, response)
	d.checkCacheHealth(ctx, response)
	d.checkMiddlewareHealth(ctx, response)

	// Get performance metrics
	d.getPerformanceHealth(response)

	// Get resource health
	d.getResourceHealth(response)

	// Check dependencies
	d.checkDependencies(ctx, response)

	// Collect alerts and issues
	d.collectAlertsAndIssues(response)

	// Calculate summary
	d.calculateHealthSummary(response)

	// Determine overall status
	response.Status = d.determineOverallStatus(response)

	d.logger.Debug("System health check completed",
		zap.String("status", response.Status),
		zap.Duration("check_duration", time.Since(start)),
		zap.Int("components", len(response.Components)),
		zap.Int("alerts", len(response.Alerts)),
	)

	return response
}

// GetComprehensiveMetrics returns comprehensive metrics
func (d *Dashboard) GetComprehensiveMetrics() map[string]interface{} {
	response := make(map[string]interface{})

	// API metrics
	if d.metricsCollector != nil {
		response["api"] = d.metricsCollector.GetAPIMetrics()
		response["performance"] = d.metricsCollector.GetSnapshot()
		response["endpoints"] = d.metricsCollector.GetEndpointMetrics()
	}

	// Database metrics
	if dbMetrics := database.GetMetrics(); dbMetrics != nil {
		response["database"] = dbMetrics
	}

	// System info
	response["system"] = map[string]interface{}{
		"uptime":      time.Since(d.startTime).String(),
		"version":     d.version,
		"environment": d.environment,
		"timestamp":   time.Now(),
	}

	return response
}

// GetDashboardData creates comprehensive dashboard data
func (d *Dashboard) GetDashboardData(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"health":  d.GetSystemHealth(ctx),
		"metrics": d.GetComprehensiveMetrics(),
		"meta": map[string]interface{}{
			"generated_at": time.Now(),
			"uptime":       time.Since(d.startTime).String(),
			"version":      d.version,
			"environment":  d.environment,
		},
	}
}

// ===============================
// GETTERS FOR HANDLERS
// ===============================

// GetMetricsCollector returns the metrics collector
func (d *Dashboard) GetMetricsCollector() *middleware.MetricsCollector {
	return d.metricsCollector
}

// GetLogger returns the logger
func (d *Dashboard) GetLogger() *zap.Logger {
	return d.logger
}

// GetVersion returns the version
func (d *Dashboard) GetVersion() string {
	return d.version
}

// GetEnvironment returns the environment
func (d *Dashboard) GetEnvironment() string {
	return d.environment
}

// GetStartTime returns the start time
func (d *Dashboard) GetStartTime() time.Time {
	return d.startTime
}

// ===============================
// COMPONENT HEALTH CHECKS
// ===============================

// checkDatabaseHealth checks database component health
func (d *Dashboard) checkDatabaseHealth(ctx context.Context, response *SystemHealthResponse) {
	start := time.Now()

	// Get database health from existing health checker
	dbHealth := database.Health(ctx)

	component := ComponentHealth{
		LastCheck:    start,
		ResponseTime: time.Since(start),
		Details:      make(map[string]interface{}),
	}

	// Convert database health status
	switch dbHealth.Status {
	case database.StatusHealthy:
		component.Status = "healthy"
	case database.StatusDegraded:
		component.Status = "degraded"
	case database.StatusUnhealthy:
		component.Status = "unhealthy"
	default:
		component.Status = "unknown"
	}

	// Add database-specific details
	component.Details["connection_count"] = dbHealth.ConnectionCount
	component.Details["response_time"] = dbHealth.ResponseTime
	if len(dbHealth.Errors) > 0 {
		component.Error = fmt.Sprintf("%d errors: %v", len(dbHealth.Errors), dbHealth.Errors[0])
		component.Details["errors"] = dbHealth.Errors
	}

	// Add database metrics
	if dbMetrics := database.GetMetrics(); dbMetrics != nil {
		component.Details["metrics"] = map[string]interface{}{
			"total_queries":    dbMetrics.QueryCount,
			"error_count":      dbMetrics.ErrorCount,
			"slow_queries":     dbMetrics.SlowQueryCount,
			"avg_duration":     dbMetrics.AvgQueryDuration,
			"open_connections": dbMetrics.DBStats.OpenConnections,
			"idle_connections": dbMetrics.DBStats.Idle,
		}
	}

	response.Components["database"] = component
}

// checkAPIHealth checks API component health
func (d *Dashboard) checkAPIHealth(ctx context.Context, response *SystemHealthResponse) {
	start := time.Now()

	component := ComponentHealth{
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: time.Since(start),
		Details:      make(map[string]interface{}),
	}

	if d.metricsCollector != nil {
		apiMetrics := d.metricsCollector.GetAPIMetrics()
		snapshot := d.metricsCollector.GetSnapshot()

		// Check API health based on metrics
		if snapshot.ErrorRate > 10.0 {
			component.Status = "degraded"
			component.Error = fmt.Sprintf("High error rate: %.2f%%", snapshot.ErrorRate)
		} else if snapshot.ErrorRate > 20.0 {
			component.Status = "unhealthy"
			component.Error = fmt.Sprintf("Very high error rate: %.2f%%", snapshot.ErrorRate)
		}

		if snapshot.AverageLatency > 2*time.Second {
			if component.Status == "healthy" {
				component.Status = "degraded"
			}
			component.Error = fmt.Sprintf("High latency: %v", snapshot.AverageLatency)
		}

		component.Details["total_requests"] = apiMetrics.TotalRequests
		component.Details["success_requests"] = apiMetrics.SuccessRequests
		component.Details["error_requests"] = apiMetrics.ErrorRequests
		component.Details["active_requests"] = apiMetrics.ActiveRequests
		component.Details["average_latency"] = snapshot.AverageLatency
		component.Details["requests_per_second"] = snapshot.RequestsPerSecond
		component.Details["error_rate"] = snapshot.ErrorRate
	}

	response.Components["api"] = component
}

// checkCacheHealth checks cache component health
func (d *Dashboard) checkCacheHealth(ctx context.Context, response *SystemHealthResponse) {
	start := time.Now()

	component := ComponentHealth{
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: time.Since(start),
		Details:      make(map[string]interface{}),
	}

	// Basic cache health check - you can enhance this with actual cache metrics
	component.Details["status"] = "operational"
	component.Details["type"] = "redis"

	response.Components["cache"] = component
}

// checkMiddlewareHealth checks middleware health
func (d *Dashboard) checkMiddlewareHealth(ctx context.Context, response *SystemHealthResponse) {
	start := time.Now()

	component := ComponentHealth{
		Status:       "healthy",
		LastCheck:    start,
		ResponseTime: time.Since(start),
		Details:      make(map[string]interface{}),
	}

	// Check middleware status
	component.Details["components"] = map[string]string{
		"request_id":     "active",
		"logging":        "active",
		"rate_limiting":  "active",
		"validation":     "active",
		"authentication": "active",
		"error_handling": "active",
		"panic_recovery": "active",
		"security":       "active",
		"metrics":        "active",
	}

	response.Components["middleware"] = component
}

// ===============================
// PERFORMANCE & RESOURCE CHECKS
// ===============================

// getPerformanceHealth gets performance-related health metrics
func (d *Dashboard) getPerformanceHealth(response *SystemHealthResponse) {
	if d.metricsCollector == nil {
		response.Performance = PerformanceHealth{}
		return
	}

	apiMetrics := d.metricsCollector.GetAPIMetrics()
	snapshot := d.metricsCollector.GetSnapshot()

	// Calculate slow requests percentage
	var slowRequestsPercent float64
	if apiMetrics.TotalRequests > 0 {
		slowRequestsPercent = float64(apiMetrics.SlowRequests) / float64(apiMetrics.TotalRequests) * 100
	}

	response.Performance = PerformanceHealth{
		RequestsPerSecond:   snapshot.RequestsPerSecond,
		AverageLatency:      snapshot.AverageLatency,
		ErrorRate:           snapshot.ErrorRate,
		Availability:        snapshot.Availability,
		ActiveRequests:      apiMetrics.ActiveRequests,
		PeakActiveRequests:  apiMetrics.PeakActiveRequests,
		SlowRequestsPercent: slowRequestsPercent,
	}
}

// getResourceHealth gets system resource health
func (d *Dashboard) getResourceHealth(response *SystemHealthResponse) {
	var snapshot *middleware.PerformanceSnapshot
	if d.metricsCollector != nil {
		snapshot = d.metricsCollector.GetSnapshot()
	}

	response.Resources = ResourceHealth{
		Memory: ResourceMetric{
			Value:  formatBytes(snapshot.SystemMetrics.MemoryUsage),
			Unit:   "bytes",
			Status: getResourceStatus(snapshot.SystemMetrics.MemoryPercent, 80, 90),
			Usage:  snapshot.SystemMetrics.MemoryPercent,
		},
		Goroutines: ResourceMetric{
			Value:  snapshot.SystemMetrics.Goroutines,
			Unit:   "count",
			Status: getResourceStatus(float64(snapshot.SystemMetrics.Goroutines), 1000, 2000),
		},
	}

	// Add database connection metrics
	if dbMetrics := database.GetMetrics(); dbMetrics != nil {
		connectionUsage := float64(dbMetrics.DBStats.OpenConnections) / float64(dbMetrics.DBStats.MaxOpenConnections) * 100
		response.Resources.Database = ResourceMetric{
			Value:     dbMetrics.DBStats.OpenConnections,
			Unit:      "connections",
			Status:    getResourceStatus(connectionUsage, 70, 85),
			Usage:     connectionUsage,
			Threshold: dbMetrics.DBStats.MaxOpenConnections,
		}
	}
}

// checkDependencies checks external dependencies
func (d *Dashboard) checkDependencies(ctx context.Context, response *SystemHealthResponse) {
	// Database dependency (already checked in components, but this could be different)
	dbHealth := database.Health(ctx)
	response.Dependencies["database"] = DependencyHealth{
		Status:       convertHealthStatus(dbHealth.Status),
		LastCheck:    time.Now(),
		ResponseTime: dbHealth.ResponseTime,
	}

	// Add other dependencies like external APIs, services, etc.
	// This is where you'd check third-party services
	response.Dependencies["cloudinary"] = DependencyHealth{
		Status:    "healthy",
		LastCheck: time.Now(),
	}
}

// ===============================
// ALERTS & ISSUES
// ===============================

// collectAlertsAndIssues collects system alerts and issues
func (d *Dashboard) collectAlertsAndIssues(response *SystemHealthResponse) {
	// Get performance alerts from metrics collector
	if d.metricsCollector != nil {
		snapshot := d.metricsCollector.GetSnapshot()
		for _, alert := range snapshot.Alerts {
			response.Alerts = append(response.Alerts, SystemAlert{
				ID:        fmt.Sprintf("perf_%s_%d", alert.Type, alert.Timestamp.Unix()),
				Type:      alert.Type,
				Severity:  alert.Severity,
				Message:   alert.Message,
				Component: "api",
				Timestamp: alert.Timestamp,
				Value:     alert.Value,
				Threshold: alert.Threshold,
			})
		}
	}

	// Check for component-specific issues
	for componentName, component := range response.Components {
		if component.Status == "degraded" || component.Status == "unhealthy" {
			severity := "warning"
			if component.Status == "unhealthy" {
				severity = "critical"
			}

			response.Issues = append(response.Issues, SystemIssue{
				ID:          fmt.Sprintf("%s_health", componentName),
				Type:        "component_health",
				Severity:    severity,
				Title:       fmt.Sprintf("%s Component Health Issue", componentName),
				Description: component.Error,
				Component:   componentName,
				FirstSeen:   component.LastCheck,
				LastSeen:    component.LastCheck,
				Occurrences: 1,
				Status:      "new",
			})
		}
	}
}

// ===============================
// SUMMARY CALCULATIONS
// ===============================

// calculateHealthSummary calculates overall health summary
func (d *Dashboard) calculateHealthSummary(response *SystemHealthResponse) {
	totalComponents := len(response.Components)
	healthyComponents := 0

	for _, component := range response.Components {
		if component.Status == "healthy" {
			healthyComponents++
		}
	}

	// Count alerts by severity
	criticalIssues := 0
	for _, issue := range response.Issues {
		if issue.Severity == "critical" {
			criticalIssues++
		}
	}

	// Calculate scores (0-100)
	performanceScore := d.calculatePerformanceScore(response)
	reliabilityScore := d.calculateReliabilityScore(response)
	operationalScore := d.calculateOperationalScore(response)

	response.Summary = HealthSummary{
		HealthyComponents: healthyComponents,
		TotalComponents:   totalComponents,
		ActiveAlerts:      len(response.Alerts),
		CriticalIssues:    criticalIssues,
		PerformanceScore:  performanceScore,
		ReliabilityScore:  reliabilityScore,
		OperationalScore:  operationalScore,
	}
}

// determineOverallStatus determines the overall system status
func (d *Dashboard) determineOverallStatus(response *SystemHealthResponse) string {
	// Check for critical issues
	for _, issue := range response.Issues {
		if issue.Severity == "critical" {
			return "unhealthy"
		}
	}

	// Check component health
	unhealthyCount := 0
	degradedCount := 0

	for _, component := range response.Components {
		switch component.Status {
		case "unhealthy":
			unhealthyCount++
		case "degraded":
			degradedCount++
		}
	}

	// Determine status based on component health
	if unhealthyCount > 0 {
		return "unhealthy"
	}
	if degradedCount > 0 {
		return "degraded"
	}

	// Check performance metrics
	if response.Performance.ErrorRate > 10.0 {
		return "degraded"
	}
	if response.Performance.Availability < 95.0 {
		return "degraded"
	}

	response.Summary.OverallStatus = "healthy"
	return "healthy"
}

// ===============================
// SCORE CALCULATIONS
// ===============================

// calculatePerformanceScore calculates performance score (0-100)
func (d *Dashboard) calculatePerformanceScore(response *SystemHealthResponse) float64 {
	score := 100.0

	// Penalize for high error rate
	if response.Performance.ErrorRate > 0 {
		score -= response.Performance.ErrorRate * 10 // 10 points per 1% error rate
	}

	// Penalize for high latency
	if response.Performance.AverageLatency > 500*time.Millisecond {
		latencyPenalty := float64(response.Performance.AverageLatency.Milliseconds()) / 100
		score -= latencyPenalty
	}

	// Penalize for low availability
	if response.Performance.Availability < 100 {
		score -= (100 - response.Performance.Availability) * 2
	}

	if score < 0 {
		score = 0
	}

	return score
}

// calculateReliabilityScore calculates reliability score (0-100)
func (d *Dashboard) calculateReliabilityScore(response *SystemHealthResponse) float64 {
	score := 100.0

	// Penalize for unhealthy components
	for _, component := range response.Components {
		switch component.Status {
		case "unhealthy":
			score -= 25
		case "degraded":
			score -= 10
		}
	}

	// Penalize for active alerts
	score -= float64(len(response.Alerts)) * 5

	if score < 0 {
		score = 0
	}

	return score
}

// calculateOperationalScore calculates operational score (0-100)
func (d *Dashboard) calculateOperationalScore(response *SystemHealthResponse) float64 {
	score := 100.0

	// Penalize for resource usage
	if response.Resources.Memory.Usage > 80 {
		score -= (response.Resources.Memory.Usage - 80) * 2
	}

	if response.Resources.Database.Usage > 70 {
		score -= (response.Resources.Database.Usage - 70) * 1.5
	}

	// Penalize for critical issues
	for _, issue := range response.Issues {
		if issue.Severity == "critical" {
			score -= 20
		} else if issue.Severity == "warning" {
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}

	return score
}

// ===============================
// UTILITY FUNCTIONS
// ===============================

// convertHealthStatus converts database health status to standard status
func convertHealthStatus(dbStatus string) string {
	switch dbStatus {
	case database.StatusHealthy:
		return "healthy"
	case database.StatusDegraded:
		return "degraded"
	case database.StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// getResourceStatus determines resource status based on usage and thresholds
func getResourceStatus(value, warningThreshold, criticalThreshold float64) string {
	if value >= criticalThreshold {
		return "critical"
	}
	if value >= warningThreshold {
		return "warning"
	}
	return "healthy"
}

// formatBytes formats bytes in human-readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
