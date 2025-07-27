// File: internal/middleware/metrics.go
package middleware

import (
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ===============================
// METRICS CONFIGURATION
// ===============================

// MetricsConfig holds configuration for API metrics middleware
type MetricsConfig struct {
	// Collection settings
	EnableMetrics         bool `json:"enable_metrics"`
	EnableDetailedMetrics bool `json:"enable_detailed_metrics"`
	EnableEndpointMetrics bool `json:"enable_endpoint_metrics"`
	EnableUserMetrics     bool `json:"enable_user_metrics"`

	// Performance tracking
	SlowRequestThreshold    time.Duration `json:"slow_request_threshold"`
	VerySlowThreshold       time.Duration `json:"very_slow_threshold"`
	EnablePerformanceAlerts bool          `json:"enable_performance_alerts"`

	// Sampling and retention
	SampleRate            float64 `json:"sample_rate"`
	MetricsRetentionHours int     `json:"metrics_retention_hours"`
	MaxEndpointsTracked   int     `json:"max_endpoints_tracked"`

	// Aggregation settings
	AggregationInterval   time.Duration `json:"aggregation_interval"`
	EnableRealTimeMetrics bool          `json:"enable_real_time_metrics"`

	// Export settings
	EnablePrometheusExport bool   `json:"enable_prometheus_export"`
	MetricsEndpoint        string `json:"metrics_endpoint"`

	// Alerting
	AlertThresholds AlertThresholds `json:"alert_thresholds"`
}

// AlertThresholds defines thresholds for various alerts
type AlertThresholds struct {
	ErrorRatePercent       float64 `json:"error_rate_percent"`       // 5% error rate
	SlowRequestsPercent    float64 `json:"slow_requests_percent"`    // 10% slow requests
	HighLatencyMs          int64   `json:"high_latency_ms"`          // 2000ms average
	HighThroughputRPS      int64   `json:"high_throughput_rps"`      // 1000 RPS
	LowAvailabilityPercent float64 `json:"low_availability_percent"` // 99% availability
}

// DefaultMetricsConfig returns production-ready metrics configuration
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		EnableMetrics:           true,
		EnableDetailedMetrics:   true,
		EnableEndpointMetrics:   true,
		EnableUserMetrics:       true,
		SlowRequestThreshold:    500 * time.Millisecond,
		VerySlowThreshold:       2 * time.Second,
		EnablePerformanceAlerts: true,
		SampleRate:              1.0,    // 100% sampling
		MetricsRetentionHours:   24 * 7, // 7 days
		MaxEndpointsTracked:     100,
		AggregationInterval:     1 * time.Minute,
		EnableRealTimeMetrics:   true,
		EnablePrometheusExport:  true,
		MetricsEndpoint:         "/internal/metrics",
		AlertThresholds: AlertThresholds{
			ErrorRatePercent:       5.0,
			SlowRequestsPercent:    10.0,
			HighLatencyMs:          2000,
			HighThroughputRPS:      1000,
			LowAvailabilityPercent: 99.0,
		},
	}
}

// ===============================
// METRICS DATA STRUCTURES
// ===============================

// APIMetrics contains comprehensive API metrics
type APIMetrics struct {
	// Request counters
	TotalRequests   int64 `json:"total_requests"`
	SuccessRequests int64 `json:"success_requests"`
	ErrorRequests   int64 `json:"error_requests"`

	// Response time metrics
	TotalResponseTime int64 `json:"total_response_time_ns"`
	MinResponseTime   int64 `json:"min_response_time_ns"`
	MaxResponseTime   int64 `json:"max_response_time_ns"`

	// Performance counters
	SlowRequests     int64 `json:"slow_requests"`
	VerySlowRequests int64 `json:"very_slow_requests"`

	// Status code counters
	Status2xx int64 `json:"status_2xx"`
	Status3xx int64 `json:"status_3xx"`
	Status4xx int64 `json:"status_4xx"`
	Status5xx int64 `json:"status_5xx"`

	// Method counters
	GetRequests    int64 `json:"get_requests"`
	PostRequests   int64 `json:"post_requests"`
	PutRequests    int64 `json:"put_requests"`
	DeleteRequests int64 `json:"delete_requests"`
	OtherRequests  int64 `json:"other_requests"`

	// Size metrics
	TotalRequestBytes  int64 `json:"total_request_bytes"`
	TotalResponseBytes int64 `json:"total_response_bytes"`

	// Concurrent metrics
	ActiveRequests     int64 `json:"active_requests"`
	PeakActiveRequests int64 `json:"peak_active_requests"`
}

// EndpointMetrics contains metrics for a specific endpoint
type EndpointMetrics struct {
	Path          string           `json:"path"`
	Method        string           `json:"method"`
	RequestCount  int64            `json:"request_count"`
	ErrorCount    int64            `json:"error_count"`
	TotalDuration int64            `json:"total_duration_ns"`
	MinDuration   int64            `json:"min_duration_ns"`
	MaxDuration   int64            `json:"max_duration_ns"`
	LastAccess    time.Time        `json:"last_access"`
	StatusCodes   map[int]int64    `json:"status_codes"`
	UserAgents    map[string]int64 `json:"user_agents,omitempty"`
	ResponseSizes map[string]int64 `json:"response_sizes,omitempty"`
}

// UserMetrics contains metrics for a specific user
type UserMetrics struct {
	UserID        int64            `json:"user_id"`
	RequestCount  int64            `json:"request_count"`
	ErrorCount    int64            `json:"error_count"`
	LastSeen      time.Time        `json:"last_seen"`
	Endpoints     map[string]int64 `json:"endpoints"`
	TotalDuration int64            `json:"total_duration_ns"`
}

// PerformanceSnapshot represents a point-in-time performance view
type PerformanceSnapshot struct {
	Timestamp         time.Time             `json:"timestamp"`
	RequestsPerSecond float64               `json:"requests_per_second"`
	AverageLatency    time.Duration         `json:"average_latency"`
	ErrorRate         float64               `json:"error_rate"`
	Availability      float64               `json:"availability"`
	TopEndpoints      []EndpointPerformance `json:"top_endpoints"`
	SystemMetrics     SystemMetrics         `json:"system_metrics"`
	Alerts            []PerformanceAlert    `json:"alerts,omitempty"`
}

// EndpointPerformance represents performance data for an endpoint
type EndpointPerformance struct {
	Endpoint       string        `json:"endpoint"`
	RequestCount   int64         `json:"request_count"`
	AverageLatency time.Duration `json:"average_latency"`
	ErrorRate      float64       `json:"error_rate"`
	Throughput     float64       `json:"throughput_rps"`
}

// SystemMetrics contains system-level performance metrics
type SystemMetrics struct {
	MemoryUsage   uint64  `json:"memory_usage_bytes"`
	MemoryPercent float64 `json:"memory_percent"`
	Goroutines    int     `json:"goroutines"`
	CGOCalls      int64   `json:"cgo_calls"`
	CPUUsage      float64 `json:"cpu_usage_percent,omitempty"`
	DiskUsage     float64 `json:"disk_usage_percent,omitempty"`
}

// PerformanceAlert represents a performance alert
type PerformanceAlert struct {
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
}

// ===============================
// METRICS COLLECTOR
// ===============================

// MetricsCollector collects and aggregates API metrics
type MetricsCollector struct {
	config *MetricsConfig
	logger *zap.Logger

	// Global metrics
	apiMetrics *APIMetrics

	// Endpoint-specific metrics
	mu              sync.RWMutex
	endpointMetrics map[string]*EndpointMetrics
	userMetrics     map[int64]*UserMetrics

	// Time-series data
	snapshots   []PerformanceSnapshot
	snapshotsMu sync.RWMutex

	// Alerts
	alerts         []PerformanceAlert
	alertsMu       sync.RWMutex
	lastAlertCheck time.Time

	// Background processing
	stopCh    chan struct{}
	startTime time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config *MetricsConfig, logger *zap.Logger) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	collector := &MetricsCollector{
		config:          config,
		logger:          logger,
		apiMetrics:      &APIMetrics{},
		endpointMetrics: make(map[string]*EndpointMetrics),
		userMetrics:     make(map[int64]*UserMetrics),
		snapshots:       make([]PerformanceSnapshot, 0),
		alerts:          make([]PerformanceAlert, 0),
		stopCh:          make(chan struct{}),
		startTime:       time.Now(),
	}

	// Start background processing
	if config.EnableRealTimeMetrics {
		go collector.startBackgroundProcessing()
	}

	return collector
}

// ===============================
// METRICS MIDDLEWARE
// ===============================

// APIMetricsMiddleware creates comprehensive API metrics middleware
func APIMetricsMiddleware(collector *MetricsCollector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !collector.config.EnableMetrics {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			requestLogger := GetRequestLogger(r.Context())
			requestID := GetRequestID(r.Context())

			// Increment active requests
			atomic.AddInt64(&collector.apiMetrics.ActiveRequests, 1)
			defer atomic.AddInt64(&collector.apiMetrics.ActiveRequests, -1)

			// Update peak active requests
			active := atomic.LoadInt64(&collector.apiMetrics.ActiveRequests)
			for {
				peak := atomic.LoadInt64(&collector.apiMetrics.PeakActiveRequests)
				if active <= peak || atomic.CompareAndSwapInt64(&collector.apiMetrics.PeakActiveRequests, peak, active) {
					break
				}
			}

			// Create metrics-aware response writer
			writer := &MetricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				bytesWritten:   0,
			}

			// Process request
			next.ServeHTTP(writer, r)

			// Calculate duration
			duration := time.Since(start)

			// Record metrics
			collector.recordRequest(r, writer, duration, requestID)

			// Log performance if slow
			if duration > collector.config.SlowRequestThreshold {
				requestLogger.Warn("Slow request detected",
					zap.String("request_id", requestID),
					zap.Duration("duration", duration),
					zap.String("endpoint", r.Method+" "+r.URL.Path),
					zap.Int("status", writer.statusCode),
				)
			}
		})
	}
}

// ===============================
// METRICS RESPONSE WRITER
// ===============================

// MetricsResponseWriter wraps http.ResponseWriter to capture metrics
type MetricsResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	headersSent  bool
}

func (w *MetricsResponseWriter) WriteHeader(code int) {
	if !w.headersSent {
		w.statusCode = code
		w.headersSent = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *MetricsResponseWriter) Write(data []byte) (int, error) {
	if !w.headersSent {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytesWritten += int64(written)
	return written, err
}

// ===============================
// METRICS RECORDING
// ===============================

// recordRequest records metrics for a completed request
func (c *MetricsCollector) recordRequest(r *http.Request, w *MetricsResponseWriter, duration time.Duration, requestID string) {
	// Sample requests if configured
	if c.config.SampleRate < 1.0 && !c.shouldSample(r.URL.Path) {
		return
	}

	// Record global metrics
	c.recordGlobalMetrics(r, w, duration)

	// Record endpoint metrics if enabled
	if c.config.EnableEndpointMetrics {
		c.recordEndpointMetrics(r, w, duration)
	}

	// Record user metrics if enabled
	if c.config.EnableUserMetrics {
		if userID := getUserIDFromContext(r.Context()); userID > 0 {
			c.recordUserMetrics(userID, r, w, duration)
		}
	}
}

// recordGlobalMetrics records API-wide metrics
func (c *MetricsCollector) recordGlobalMetrics(r *http.Request, w *MetricsResponseWriter, duration time.Duration) {
	// Increment total requests
	atomic.AddInt64(&c.apiMetrics.TotalRequests, 1)

	// Record response time
	durationNs := duration.Nanoseconds()
	atomic.AddInt64(&c.apiMetrics.TotalResponseTime, durationNs)

	// Update min/max response times
	c.updateMinMaxResponseTime(durationNs)

	// Record success/error
	if w.statusCode >= 200 && w.statusCode < 400 {
		atomic.AddInt64(&c.apiMetrics.SuccessRequests, 1)
	} else {
		atomic.AddInt64(&c.apiMetrics.ErrorRequests, 1)
	}

	// Record performance metrics
	if duration > c.config.SlowRequestThreshold {
		atomic.AddInt64(&c.apiMetrics.SlowRequests, 1)
	}
	if duration > c.config.VerySlowThreshold {
		atomic.AddInt64(&c.apiMetrics.VerySlowRequests, 1)
	}

	// Record status code categories
	switch {
	case w.statusCode >= 200 && w.statusCode < 300:
		atomic.AddInt64(&c.apiMetrics.Status2xx, 1)
	case w.statusCode >= 300 && w.statusCode < 400:
		atomic.AddInt64(&c.apiMetrics.Status3xx, 1)
	case w.statusCode >= 400 && w.statusCode < 500:
		atomic.AddInt64(&c.apiMetrics.Status4xx, 1)
	case w.statusCode >= 500:
		atomic.AddInt64(&c.apiMetrics.Status5xx, 1)
	}

	// Record HTTP methods
	switch r.Method {
	case "GET":
		atomic.AddInt64(&c.apiMetrics.GetRequests, 1)
	case "POST":
		atomic.AddInt64(&c.apiMetrics.PostRequests, 1)
	case "PUT":
		atomic.AddInt64(&c.apiMetrics.PutRequests, 1)
	case "DELETE":
		atomic.AddInt64(&c.apiMetrics.DeleteRequests, 1)
	default:
		atomic.AddInt64(&c.apiMetrics.OtherRequests, 1)
	}

	// Record bytes
	atomic.AddInt64(&c.apiMetrics.TotalRequestBytes, r.ContentLength)
	atomic.AddInt64(&c.apiMetrics.TotalResponseBytes, w.bytesWritten)
}

// recordEndpointMetrics records metrics for specific endpoints
func (c *MetricsCollector) recordEndpointMetrics(r *http.Request, w *MetricsResponseWriter, duration time.Duration) {
	endpoint := c.normalizeEndpoint(r.Method, r.URL.Path)

	c.mu.Lock()
	defer c.mu.Unlock()

	metrics, exists := c.endpointMetrics[endpoint]
	if !exists {
		// Check if we're at the limit
		if len(c.endpointMetrics) >= c.config.MaxEndpointsTracked {
			// Skip tracking new endpoints
			return
		}

		metrics = &EndpointMetrics{
			Path:          r.URL.Path,
			Method:        r.Method,
			StatusCodes:   make(map[int]int64),
			UserAgents:    make(map[string]int64),
			ResponseSizes: make(map[string]int64),
		}
		c.endpointMetrics[endpoint] = metrics
	}

	// Update metrics
	metrics.RequestCount++
	metrics.TotalDuration += duration.Nanoseconds()
	metrics.LastAccess = time.Now()

	// Update min/max duration
	durationNs := duration.Nanoseconds()
	if metrics.MinDuration == 0 || durationNs < metrics.MinDuration {
		metrics.MinDuration = durationNs
	}
	if durationNs > metrics.MaxDuration {
		metrics.MaxDuration = durationNs
	}

	// Record errors
	if w.statusCode >= 400 {
		metrics.ErrorCount++
	}

	// Record status codes
	metrics.StatusCodes[w.statusCode]++

	// Record user agents (if detailed metrics enabled)
	if c.config.EnableDetailedMetrics {
		userAgent := r.UserAgent()
		if userAgent != "" {
			metrics.UserAgents[userAgent]++
		}

		// Record response size categories
		sizeCategory := c.getResponseSizeCategory(w.bytesWritten)
		metrics.ResponseSizes[sizeCategory]++
	}
}

// recordUserMetrics records metrics for specific users
func (c *MetricsCollector) recordUserMetrics(userID int64, r *http.Request, w *MetricsResponseWriter, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics, exists := c.userMetrics[userID]
	if !exists {
		metrics = &UserMetrics{
			UserID:    userID,
			Endpoints: make(map[string]int64),
		}
		c.userMetrics[userID] = metrics
	}

	// Update metrics
	metrics.RequestCount++
	metrics.TotalDuration += duration.Nanoseconds()
	metrics.LastSeen = time.Now()

	// Record errors
	if w.statusCode >= 400 {
		metrics.ErrorCount++
	}

	// Record endpoint usage
	endpoint := c.normalizeEndpoint(r.Method, r.URL.Path)
	metrics.Endpoints[endpoint]++
}

// ===============================
// HELPER METHODS
// ===============================

// updateMinMaxResponseTime atomically updates min/max response times
func (c *MetricsCollector) updateMinMaxResponseTime(durationNs int64) {
	// Update minimum
	for {
		current := atomic.LoadInt64(&c.apiMetrics.MinResponseTime)
		if current != 0 && current <= durationNs {
			break
		}
		if atomic.CompareAndSwapInt64(&c.apiMetrics.MinResponseTime, current, durationNs) {
			break
		}
	}

	// Update maximum
	for {
		current := atomic.LoadInt64(&c.apiMetrics.MaxResponseTime)
		if current >= durationNs {
			break
		}
		if atomic.CompareAndSwapInt64(&c.apiMetrics.MaxResponseTime, current, durationNs) {
			break
		}
	}
}

// normalizeEndpoint normalizes endpoint paths for consistent tracking
func (c *MetricsCollector) normalizeEndpoint(method, path string) string {
	// Basic normalization - you can enhance this with more sophisticated patterns
	// Replace IDs with placeholders
	normalizedPath := path

	// Common ID patterns
	patterns := []struct {
		pattern     string
		replacement string
	}{
		{"/view-post?id=", "/view-post/{id}"},
		{"/edit-post?id=", "/edit-post/{id}"},
		{"/delete-post?id=", "/delete-post/{id}"},
		{"/user/", "/user/{id}"},
		{"/posts/", "/posts/{id}"},
		{"/api/users/", "/api/users/{id}"},
	}

	for _, p := range patterns {
		if strings.Contains(normalizedPath, p.pattern) {
			normalizedPath = p.replacement
			break
		}
	}

	return method + " " + normalizedPath
}

// shouldSample determines if a request should be sampled
func (c *MetricsCollector) shouldSample(path string) bool {
	// Always sample critical endpoints
	criticalPaths := []string{
		"/api/", "/auth/", "/login", "/signup",
	}

	for _, critical := range criticalPaths {
		if strings.HasPrefix(path, critical) {
			return true
		}
	}

	// Simple hash-based sampling
	if c.config.SampleRate >= 1.0 {
		return true
	}

	hash := 0
	for _, c := range path {
		hash = hash*31 + int(c)
	}
	return float64(hash%100)/100.0 < c.config.SampleRate
}

// getResponseSizeCategory categorizes response sizes
func (c *MetricsCollector) getResponseSizeCategory(bytes int64) string {
	switch {
	case bytes < 1024: // < 1KB
		return "small"
	case bytes < 10*1024: // < 10KB
		return "medium"
	case bytes < 100*1024: // < 100KB
		return "large"
	case bytes < 1024*1024: // < 1MB
		return "xlarge"
	default:
		return "huge"
	}
}

// ===============================
// BACKGROUND PROCESSING
// ===============================

// startBackgroundProcessing starts background metric processing
func (c *MetricsCollector) startBackgroundProcessing() {
	ticker := time.NewTicker(c.config.AggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.createPerformanceSnapshot()
			c.checkAlerts()
			c.cleanupOldData()
		case <-c.stopCh:
			return
		}
	}
}

// createPerformanceSnapshot creates a point-in-time performance snapshot
func (c *MetricsCollector) createPerformanceSnapshot() {
	snapshot := c.calculatePerformanceSnapshot()

	c.snapshotsMu.Lock()
	c.snapshots = append(c.snapshots, snapshot)

	// Keep only recent snapshots
	maxSnapshots := c.config.MetricsRetentionHours * 60 / int(c.config.AggregationInterval.Minutes())
	if len(c.snapshots) > maxSnapshots {
		c.snapshots = c.snapshots[len(c.snapshots)-maxSnapshots:]
	}
	c.snapshotsMu.Unlock()

	c.logger.Debug("Performance snapshot created",
		zap.Float64("rps", snapshot.RequestsPerSecond),
		zap.Duration("avg_latency", snapshot.AverageLatency),
		zap.Float64("error_rate", snapshot.ErrorRate),
		zap.Float64("availability", snapshot.Availability),
	)
}

// calculatePerformanceSnapshot calculates current performance metrics
func (c *MetricsCollector) calculatePerformanceSnapshot() PerformanceSnapshot {
	now := time.Now()

	// Get current metrics
	totalRequests := atomic.LoadInt64(&c.apiMetrics.TotalRequests)
	successRequests := atomic.LoadInt64(&c.apiMetrics.SuccessRequests)
	errorRequests := atomic.LoadInt64(&c.apiMetrics.ErrorRequests)
	totalResponseTime := atomic.LoadInt64(&c.apiMetrics.TotalResponseTime)

	// Calculate rates and averages
	uptime := now.Sub(c.startTime)
	var rps float64
	if uptime.Seconds() > 0 {
		rps = float64(totalRequests) / uptime.Seconds()
	}

	var avgLatency time.Duration
	if totalRequests > 0 {
		avgLatency = time.Duration(totalResponseTime / totalRequests)
	}

	var errorRate float64
	if totalRequests > 0 {
		errorRate = float64(errorRequests) / float64(totalRequests) * 100
	}

	var availability float64
	if totalRequests > 0 {
		availability = float64(successRequests) / float64(totalRequests) * 100
	}

	// Get top endpoints
	topEndpoints := c.getTopEndpoints(5)

	// Get system metrics
	systemMetrics := c.getSystemMetrics()

	return PerformanceSnapshot{
		Timestamp:         now,
		RequestsPerSecond: rps,
		AverageLatency:    avgLatency,
		ErrorRate:         errorRate,
		Availability:      availability,
		TopEndpoints:      topEndpoints,
		SystemMetrics:     systemMetrics,
	}
}

// getTopEndpoints returns the top N endpoints by request count
func (c *MetricsCollector) getTopEndpoints(n int) []EndpointPerformance {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type endpointSort struct {
		endpoint string
		metrics  *EndpointMetrics
	}

	var endpoints []endpointSort
	for endpoint, metrics := range c.endpointMetrics {
		endpoints = append(endpoints, endpointSort{endpoint, metrics})
	}

	// Simple bubble sort for top N (good enough for small N)
	for i := 0; i < len(endpoints)-1; i++ {
		for j := 0; j < len(endpoints)-i-1; j++ {
			if endpoints[j].metrics.RequestCount < endpoints[j+1].metrics.RequestCount {
				endpoints[j], endpoints[j+1] = endpoints[j+1], endpoints[j]
			}
		}
	}

	// Take top N
	if n > len(endpoints) {
		n = len(endpoints)
	}

	result := make([]EndpointPerformance, n)
	for i := 0; i < n; i++ {
		metrics := endpoints[i].metrics
		var avgLatency time.Duration
		var errorRate float64

		if metrics.RequestCount > 0 {
			avgLatency = time.Duration(metrics.TotalDuration / metrics.RequestCount)
			errorRate = float64(metrics.ErrorCount) / float64(metrics.RequestCount) * 100
		}

		uptime := time.Since(c.startTime)
		throughput := float64(metrics.RequestCount) / uptime.Seconds()

		result[i] = EndpointPerformance{
			Endpoint:       endpoints[i].endpoint,
			RequestCount:   metrics.RequestCount,
			AverageLatency: avgLatency,
			ErrorRate:      errorRate,
			Throughput:     throughput,
		}
	}

	return result
}

// getSystemMetrics returns current system metrics
func (c *MetricsCollector) getSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemMetrics{
		MemoryUsage:   m.Alloc,
		MemoryPercent: float64(m.Alloc) / float64(m.Sys) * 100,
		Goroutines:    runtime.NumGoroutine(),
		CGOCalls:      runtime.NumCgoCall(),
	}
}

// checkAlerts checks for performance alerts
func (c *MetricsCollector) checkAlerts() {
	if !c.config.EnablePerformanceAlerts {
		return
	}

	now := time.Now()
	if now.Sub(c.lastAlertCheck) < c.config.AggregationInterval {
		return
	}
	c.lastAlertCheck = now

	snapshot := c.calculatePerformanceSnapshot()
	var newAlerts []PerformanceAlert

	// Check error rate
	if snapshot.ErrorRate > c.config.AlertThresholds.ErrorRatePercent {
		newAlerts = append(newAlerts, PerformanceAlert{
			Type:      "high_error_rate",
			Message:   "High error rate detected",
			Severity:  "warning",
			Timestamp: now,
			Value:     snapshot.ErrorRate,
			Threshold: c.config.AlertThresholds.ErrorRatePercent,
		})
	}

	// Check latency
	if snapshot.AverageLatency.Milliseconds() > c.config.AlertThresholds.HighLatencyMs {
		newAlerts = append(newAlerts, PerformanceAlert{
			Type:      "high_latency",
			Message:   "High average latency detected",
			Severity:  "warning",
			Timestamp: now,
			Value:     float64(snapshot.AverageLatency.Milliseconds()),
			Threshold: float64(c.config.AlertThresholds.HighLatencyMs),
		})
	}

	// Check availability
	if snapshot.Availability < c.config.AlertThresholds.LowAvailabilityPercent {
		newAlerts = append(newAlerts, PerformanceAlert{
			Type:      "low_availability",
			Message:   "Low availability detected",
			Severity:  "critical",
			Timestamp: now,
			Value:     snapshot.Availability,
			Threshold: c.config.AlertThresholds.LowAvailabilityPercent,
		})
	}

	// Add new alerts
	if len(newAlerts) > 0 {
		c.alertsMu.Lock()
		c.alerts = append(c.alerts, newAlerts...)

		// Keep only recent alerts (last 24 hours)
		cutoff := now.Add(-24 * time.Hour)
		filtered := c.alerts[:0]
		for _, alert := range c.alerts {
			if alert.Timestamp.After(cutoff) {
				filtered = append(filtered, alert)
			}
		}
		c.alerts = filtered
		c.alertsMu.Unlock()

		// Log alerts
		for _, alert := range newAlerts {
			c.logger.Warn("Performance alert",
				zap.String("type", alert.Type),
				zap.String("message", alert.Message),
				zap.String("severity", alert.Severity),
				zap.Float64("value", alert.Value),
				zap.Float64("threshold", alert.Threshold),
			)
		}
	}
}

// cleanupOldData removes old metrics data
func (c *MetricsCollector) cleanupOldData() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(c.config.MetricsRetentionHours) * time.Hour)

	// Clean up endpoint metrics that haven't been accessed recently
	for endpoint, metrics := range c.endpointMetrics {
		if metrics.LastAccess.Before(cutoff) {
			delete(c.endpointMetrics, endpoint)
		}
	}

	// Clean up user metrics that haven't been seen recently
	for userID, metrics := range c.userMetrics {
		if metrics.LastSeen.Before(cutoff) {
			delete(c.userMetrics, userID)
		}
	}
}

// ===============================
// PUBLIC API
// ===============================

// GetSnapshot returns current metrics snapshot
func (c *MetricsCollector) GetSnapshot() *PerformanceSnapshot {
	snapshot := c.calculatePerformanceSnapshot()

	// Add current alerts
	c.alertsMu.RLock()
	if len(c.alerts) > 0 {
		snapshot.Alerts = make([]PerformanceAlert, len(c.alerts))
		copy(snapshot.Alerts, c.alerts)
	}
	c.alertsMu.RUnlock()

	return &snapshot
}

// GetAPIMetrics returns current API metrics
func (c *MetricsCollector) GetAPIMetrics() *APIMetrics {
	// Create a copy to avoid race conditions
	metrics := &APIMetrics{}
	*metrics = *c.apiMetrics

	// Load atomic values
	metrics.TotalRequests = atomic.LoadInt64(&c.apiMetrics.TotalRequests)
	metrics.SuccessRequests = atomic.LoadInt64(&c.apiMetrics.SuccessRequests)
	metrics.ErrorRequests = atomic.LoadInt64(&c.apiMetrics.ErrorRequests)
	metrics.TotalResponseTime = atomic.LoadInt64(&c.apiMetrics.TotalResponseTime)
	metrics.MinResponseTime = atomic.LoadInt64(&c.apiMetrics.MinResponseTime)
	metrics.MaxResponseTime = atomic.LoadInt64(&c.apiMetrics.MaxResponseTime)
	metrics.SlowRequests = atomic.LoadInt64(&c.apiMetrics.SlowRequests)
	metrics.VerySlowRequests = atomic.LoadInt64(&c.apiMetrics.VerySlowRequests)
	metrics.ActiveRequests = atomic.LoadInt64(&c.apiMetrics.ActiveRequests)
	metrics.PeakActiveRequests = atomic.LoadInt64(&c.apiMetrics.PeakActiveRequests)

	return metrics
}

// GetEndpointMetrics returns metrics for all endpoints
func (c *MetricsCollector) GetEndpointMetrics() map[string]*EndpointMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*EndpointMetrics)
	for endpoint, metrics := range c.endpointMetrics {
		// Create a copy
		metricsCopy := *metrics
		metricsCopy.StatusCodes = make(map[int]int64)
		for k, v := range metrics.StatusCodes {
			metricsCopy.StatusCodes[k] = v
		}
		result[endpoint] = &metricsCopy
	}

	return result
}

// Stop stops the metrics collector
func (c *MetricsCollector) Stop() {
	close(c.stopCh)
}
