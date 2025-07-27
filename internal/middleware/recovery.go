// File: internal/middleware/recovery.go
package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"evalhub/internal/contextutils"
	"evalhub/internal/responseutil"
	"evalhub/internal/services"
	"evalhub/internal/utils/appinfo"

	"go.uber.org/zap"
)

// ===============================
// RECOVERY CONFIGURATION
// ===============================

// RecoveryConfig holds configuration for panic recovery middleware
type RecoveryConfig struct {
	// Stack trace settings
	EnableStackTrace     bool `json:"enable_stack_trace"`
	StackTraceInResponse bool `json:"stack_trace_in_response"`
	MaxStackFrames       int  `json:"max_stack_frames"`

	// Error handling
	EnableDetailedErrors bool `json:"enable_detailed_errors"`
	MaskInternalErrors   bool `json:"mask_internal_errors"`

	// Monitoring and alerting
	EnablePanicAlerts bool          `json:"enable_panic_alerts"`
	AlertThreshold    int           `json:"alert_threshold"` // Panics per minute
	AlertCooldown     time.Duration `json:"alert_cooldown"`

	// Graceful degradation
	EnableGracefulDegradation bool   `json:"enable_graceful_degradation"`
	ServiceDegradationMode    string `json:"service_degradation_mode"` // "readonly", "maintenance", "limited"

	// Recovery behavior
	EnableAutoRestart bool          `json:"enable_auto_restart"`
	RestartThreshold  int           `json:"restart_threshold"`
	RestartCooldown   time.Duration `json:"restart_cooldown"`

	// Custom error pages
	CustomErrorTemplate string `json:"custom_error_template"`
	PanicErrorTemplate  string `json:"panic_error_template"`
}

// DefaultRecoveryConfig returns production-ready recovery configuration
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		EnableStackTrace:          true,
		StackTraceInResponse:      false, // Don't expose stack traces in production
		MaxStackFrames:            20,
		EnableDetailedErrors:      true,
		MaskInternalErrors:        true,
		EnablePanicAlerts:         true,
		AlertThreshold:            5, // 5 panics per minute triggers alert
		AlertCooldown:             5 * time.Minute,
		EnableGracefulDegradation: true,
		ServiceDegradationMode:    "limited",
		EnableAutoRestart:         false, // Requires careful consideration
		RestartThreshold:          10,
		RestartCooldown:           10 * time.Minute,
		CustomErrorTemplate:       "error",
		PanicErrorTemplate:        "panic_error",
	}
}

// ===============================
// PANIC INFORMATION
// ===============================

// PanicInfo contains information about a panic
type PanicInfo struct {
	Timestamp   time.Time              `json:"timestamp"`
	RequestID   string                 `json:"request_id"`
	Error       interface{}            `json:"error"`
	StackTrace  []StackFrame           `json:"stack_trace,omitempty"`
	Request     *RequestInfo           `json:"request"`
	UserID      *int64                 `json:"user_id,omitempty"`
	Environment string                 `json:"environment"`
	Version     string                 `json:"version,omitempty"`
	MemoryStats *MemoryStats           `json:"memory_stats,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
}

// StackFrame represents a single stack frame
type StackFrame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Package  string `json:"package,omitempty"`
}

// RequestInfo contains information about the request that caused the panic
type RequestInfo struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	UserAgent   string            `json:"user_agent"`
	RemoteAddr  string            `json:"remote_addr"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
}

// MemoryStats contains memory usage statistics
type MemoryStats struct {
	Alloc      uint64 `json:"alloc"`
	TotalAlloc uint64 `json:"total_alloc"`
	Sys        uint64 `json:"sys"`
	Mallocs    uint64 `json:"mallocs"`
	Frees      uint64 `json:"frees"`
	HeapAlloc  uint64 `json:"heap_alloc"`
	HeapSys    uint64 `json:"heap_sys"`
}

// ===============================
// ENHANCED RECOVERY MIDDLEWARE
// ===============================

// EnhancedRecovery creates an enhanced panic recovery middleware
func EnhancedRecovery(config *RecoveryConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	// Initialize panic monitor
	monitor := NewPanicMonitor(config, logger)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Capture panic information
					panicInfo := capturePanicInfo(err, r, config)

					// Log the panic with structured logging
					logPanic(logger, panicInfo, config)

					// Monitor and alert
					monitor.RecordPanic(panicInfo)

					// Handle graceful degradation
					if config.EnableGracefulDegradation {
						handleGracefulDegradation(w, r, panicInfo, config, logger)
					}

					// Send error response
					sendPanicResponse(w, r, panicInfo, config, logger)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// ===============================
// PANIC CAPTURE AND PROCESSING
// ===============================

// capturePanicInfo captures comprehensive information about a panic
func capturePanicInfo(err interface{}, r *http.Request, config *RecoveryConfig) *PanicInfo {
	info := &PanicInfo{
		Timestamp:   time.Now(),
		Error:       err,
		Request:     captureRequestInfo(r),
		Environment: getEnvironment(),
		Version:     getVersion(),
	}

	// Capture stack trace if enabled
	if config.EnableStackTrace {
		info.StackTrace = captureStackTrace(config.MaxStackFrames)
	}

	// Capture memory stats if detailed errors are enabled
	if config.EnableDetailedErrors {
		info.MemoryStats = captureMemoryStats()
	}

	// Get request ID and user ID from context if available
	if r != nil && r.Context() != nil {
		ctx := r.Context()
		info.RequestID = contextutils.GetRequestID(ctx)

		// Get user ID from context if available
		if userID := contextutils.GetUserID(ctx); userID != 0 {
			info.UserID = &userID
		}
	}

	// Initialize metrics map if needed
	if info.Metrics == nil {
		info.Metrics = make(map[string]interface{})
	}

	info.Metrics["panic_type"] = fmt.Sprintf("%T", err)
	info.Metrics["goroutines"] = runtime.NumGoroutine()
	info.Metrics["cgocalls"] = runtime.NumCgoCall()

	return info
}

// captureRequestInfo captures relevant request information
func captureRequestInfo(r *http.Request) *RequestInfo {
	headers := make(map[string]string)

	// Capture important headers (sanitized)
	importantHeaders := []string{
		"Content-Type", "Accept", "User-Agent", "Authorization",
		"X-Forwarded-For", "X-Real-IP", "X-Request-ID",
	}

	for _, header := range importantHeaders {
		if value := r.Header.Get(header); value != "" {
			// Sanitize sensitive headers
			if header == "Authorization" && len(value) > 10 {
				headers[header] = value[:10] + "***"
			} else {
				headers[header] = value
			}
		}
	}

	return &RequestInfo{
		Method:      r.Method,
		URL:         r.URL.String(),
		Headers:     headers,
		UserAgent:   r.UserAgent(),
		RemoteAddr:  getClientIP(r),
		ContentType: r.Header.Get("Content-Type"),
		Size:        r.ContentLength,
	}
}

// captureStackTrace captures the stack trace
func captureStackTrace(maxFrames int) []StackFrame {
	var frames []StackFrame

	// Skip the first few frames (this function, recover, etc.)
	pcs := make([]uintptr, maxFrames+3)
	n := runtime.Callers(3, pcs)

	if n > 0 {
		callersFrames := runtime.CallersFrames(pcs[:n])

		for i := 0; i < maxFrames; i++ {
			frame, more := callersFrames.Next()
			if !more {
				break
			}

			// Skip runtime frames
			if strings.HasPrefix(frame.Function, "runtime.") {
				continue
			}

			stackFrame := StackFrame{
				Function: frame.Function,
				File:     frame.File,
				Line:     frame.Line,
				Package:  extractPackage(frame.Function),
			}

			frames = append(frames, stackFrame)
		}
	}

	return frames
}

// captureMemoryStats captures current memory statistics
func captureMemoryStats() *MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &MemoryStats{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		Mallocs:    m.Mallocs,
		Frees:      m.Frees,
		HeapAlloc:  m.HeapAlloc,
		HeapSys:    m.HeapSys,
	}
}

// ===============================
// PANIC MONITORING AND ALERTING
// ===============================

// PanicMonitor monitors panic frequency and triggers alerts
type PanicMonitor struct {
	config      *RecoveryConfig
	logger      *zap.Logger
	panicCounts map[string]int
	lastAlert   time.Time
	lastRestart time.Time
}

// NewPanicMonitor creates a new panic monitor
func NewPanicMonitor(config *RecoveryConfig, logger *zap.Logger) *PanicMonitor {
	return &PanicMonitor{
		config:      config,
		logger:      logger,
		panicCounts: make(map[string]int),
	}
}

// RecordPanic records a panic and triggers alerts if necessary
func (pm *PanicMonitor) RecordPanic(info *PanicInfo) {
	// Record panic
	minute := time.Now().Truncate(time.Minute).Format("2006-01-02T15:04")
	pm.panicCounts[minute]++

	// Clean old entries
	pm.cleanOldEntries()

	// Check if we should trigger an alert
	if pm.config.EnablePanicAlerts && pm.shouldTriggerAlert() {
		pm.triggerPanicAlert(info)
	}

	// Check if we should restart (very dangerous - use with caution)
	if pm.config.EnableAutoRestart && pm.shouldRestart() {
		pm.triggerAutoRestart(info)
	}
}

// shouldTriggerAlert determines if an alert should be triggered
func (pm *PanicMonitor) shouldTriggerAlert() bool {
	if time.Since(pm.lastAlert) < pm.config.AlertCooldown {
		return false
	}

	totalPanics := 0
	for _, count := range pm.panicCounts {
		totalPanics += count
	}

	return totalPanics >= pm.config.AlertThreshold
}

// shouldRestart determines if auto-restart should be triggered
func (pm *PanicMonitor) shouldRestart() bool {
	if time.Since(pm.lastRestart) < pm.config.RestartCooldown {
		return false
	}

	totalPanics := 0
	for _, count := range pm.panicCounts {
		totalPanics += count
	}

	return totalPanics >= pm.config.RestartThreshold
}

// triggerPanicAlert triggers a panic alert
func (pm *PanicMonitor) triggerPanicAlert(info *PanicInfo) {
	pm.lastAlert = time.Now()

	pm.logger.Error("PANIC ALERT TRIGGERED",
		zap.String("alert_type", "high_panic_rate"),
		zap.String("request_id", info.RequestID),
		zap.Int("threshold", pm.config.AlertThreshold),
		zap.Any("panic_counts", pm.panicCounts),
		zap.Time("timestamp", info.Timestamp),
	)

	// Here you would integrate with your alerting system
	// - Send to Slack/Discord
	// - Send to PagerDuty
	// - Send email alerts
	// - Send to monitoring systems (DataDog, New Relic, etc.)

	// Placeholder for alert integration
	go pm.sendAlert(info)
}

// triggerAutoRestart triggers an automatic restart (use with extreme caution)
func (pm *PanicMonitor) triggerAutoRestart(info *PanicInfo) {
	pm.lastRestart = time.Now()

	pm.logger.Fatal("AUTO-RESTART TRIGGERED",
		zap.String("reason", "excessive_panics"),
		zap.String("request_id", info.RequestID),
		zap.Int("threshold", pm.config.RestartThreshold),
		zap.Any("panic_counts", pm.panicCounts),
	)

	// This will cause the application to exit
	// Your process manager (systemd, Docker, etc.) should restart it
}

// cleanOldEntries removes old panic count entries
func (pm *PanicMonitor) cleanOldEntries() {
	cutoff := time.Now().Add(-5 * time.Minute).Truncate(time.Minute)

	for minute := range pm.panicCounts {
		if t, err := time.Parse("2006-01-02T15:04", minute); err == nil {
			if t.Before(cutoff) {
				delete(pm.panicCounts, minute)
			}
		}
	}
}

// sendAlert sends alert to external systems (placeholder)
func (pm *PanicMonitor) sendAlert(info *PanicInfo) {
	// Implement your alerting logic here
	// Examples:
	// - Slack webhook
	// - Email service
	// - PagerDuty API
	// - Custom webhook

	pm.logger.Info("Alert sent",
		zap.String("alert_type", "panic_threshold_exceeded"),
		zap.String("request_id", info.RequestID),
	)
}

// ===============================
// GRACEFUL DEGRADATION
// ===============================

// handleGracefulDegradation implements graceful degradation strategies
func handleGracefulDegradation(w http.ResponseWriter, r *http.Request, info *PanicInfo, config *RecoveryConfig, logger *zap.Logger) {
	switch config.ServiceDegradationMode {
	case "readonly":
		// Switch to read-only mode
		setReadOnlyMode(w, r, logger)
	case "maintenance":
		// Enter maintenance mode
		setMaintenanceMode(w, r, logger)
	case "limited":
		// Provide limited functionality
		setLimitedMode(w, r, logger)
	default:
		// Normal error handling
	}
}

// setReadOnlyMode sets the service to read-only mode
func setReadOnlyMode(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
	w.Header().Set("X-Service-Mode", "readonly")

	logger.Warn("Service degraded to read-only mode",
		zap.String("reason", "panic_recovery"),
		zap.String("request_id", GetRequestID(r.Context())),
	)
}

// setMaintenanceMode sets the service to maintenance mode
func setMaintenanceMode(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
	w.Header().Set("X-Service-Mode", "maintenance")
	w.Header().Set("Retry-After", "300") // 5 minutes

	logger.Warn("Service degraded to maintenance mode",
		zap.String("reason", "panic_recovery"),
		zap.String("request_id", GetRequestID(r.Context())),
	)
}

// setLimitedMode sets the service to limited functionality mode
func setLimitedMode(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
	w.Header().Set("X-Service-Mode", "limited")

	logger.Warn("Service degraded to limited mode",
		zap.String("reason", "panic_recovery"),
		zap.String("request_id", GetRequestID(r.Context())),
	)
}

// ===============================
// RESPONSE HANDLING
// ===============================

// sendPanicResponse sends an appropriate response for a panic
func sendPanicResponse(w http.ResponseWriter, r *http.Request, info *PanicInfo, config *RecoveryConfig, logger *zap.Logger) {
	// Try to get response builder from context
	if builder := responseutil.GetBuilder(r.Context()); builder != nil {
		if rb, ok := builder.(responseutil.ResponseBuilder); ok {
			// Create an error from the panic info
			err := fmt.Errorf("panic: %v", info.Error)

			// Add stack trace if enabled and in development
			if config.StackTraceInResponse && !config.MaskInternalErrors {
				if serviceErr, ok := err.(*services.ServiceError); ok {
					if serviceErr.Details == nil {
						serviceErr.Details = make(map[string]interface{})
					}
					serviceErr.Details["stack_trace"] = info.StackTrace
					serviceErr.Details["panic_type"] = fmt.Sprintf("%T", info.Error)
				}
			}

			rb.WriteError(w, r, err)
			return
		}
	}
	sendFallbackPanicResponse(w, r, info, config)
}

// sendFallbackPanicResponse sends a fallback response when response builder is not available
func sendFallbackPanicResponse(w http.ResponseWriter, r *http.Request, info *PanicInfo, config *RecoveryConfig) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusInternalServerError)

	errorMessage := "Internal server error"
	if config.EnableDetailedErrors && !config.MaskInternalErrors {
		errorMessage = fmt.Sprintf("Internal server error: %v", info.Error)
	}

	response := map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"type":    "INTERNAL_ERROR",
			"message": errorMessage,
		},
		"request_id": info.RequestID,
		"timestamp":  info.Timestamp.Unix(),
	}

	// Add stack trace in development
	if config.StackTraceInResponse && !config.MaskInternalErrors {
		response["stack_trace"] = info.StackTrace
	}

	// Write JSON response (ignore encoding errors at this point)
	fmt.Fprintf(w, `{"success":false,"error":{"type":"INTERNAL_ERROR","message":"%s"},"request_id":"%s","timestamp":%d}`,
		errorMessage, info.RequestID, info.Timestamp.Unix())
}

// ===============================
// LOGGING
// ===============================

// logPanic logs the panic with comprehensive information
func logPanic(logger *zap.Logger, info *PanicInfo, config *RecoveryConfig) {
	fields := []zap.Field{
		zap.String("event", "panic_recovered"),
		zap.String("request_id", info.RequestID),
		zap.Any("panic_error", info.Error),
		zap.String("panic_type", fmt.Sprintf("%T", info.Error)),
		zap.Time("timestamp", info.Timestamp),
		zap.String("method", info.Request.Method),
		zap.String("url", info.Request.URL),
		zap.String("remote_addr", info.Request.RemoteAddr),
		zap.String("user_agent", info.Request.UserAgent),
		zap.Int("goroutines", runtime.NumGoroutine()),
	}

	// Add user ID if available
	if info.UserID != nil {
		fields = append(fields, zap.Int64("user_id", *info.UserID))
	}

	// Add memory stats
	if info.MemoryStats != nil {
		fields = append(fields,
			zap.Uint64("memory_alloc", info.MemoryStats.Alloc),
			zap.Uint64("memory_sys", info.MemoryStats.Sys),
			zap.Uint64("memory_heap_alloc", info.MemoryStats.HeapAlloc),
		)
	}

	// Add stack trace if enabled
	if config.EnableStackTrace && len(info.StackTrace) > 0 {
		stackStrings := make([]string, len(info.StackTrace))
		for i, frame := range info.StackTrace {
			stackStrings[i] = fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line)
		}
		fields = append(fields, zap.Strings("stack_trace", stackStrings))
	}

	// Add custom metrics
	if len(info.Metrics) > 0 {
		fields = append(fields, zap.Any("metrics", info.Metrics))
	}

	// Log with ERROR level
	logger.Error("Panic recovered", fields...)

	// Also log a warning about the panic for monitoring
	logger.Warn("Application panic occurred",
		zap.String("request_id", info.RequestID),
		zap.String("panic_summary", fmt.Sprintf("%v", info.Error)),
		zap.String("endpoint", info.Request.Method+" "+info.Request.URL),
	)
}

// ===============================
// UTILITY FUNCTIONS
// ===============================

// extractPackage extracts package name from function name
func extractPackage(functionName string) string {
	parts := strings.Split(functionName, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if dotIndex := strings.LastIndex(lastPart, "."); dotIndex != -1 {
			return lastPart[:dotIndex]
		}
	}
	return ""
}

// getEnvironment returns the current environment
func getEnvironment() string {
	return appinfo.GetEnvironment()
}

// getVersion returns the application version
func getVersion() string {
	return appinfo.GetVersion()
}

// Enhanced Hijacker support for the response writer
type PanicRecoveryWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *PanicRecoveryWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *PanicRecoveryWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *PanicRecoveryWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not support hijacking")
}

// ===============================
// INTEGRATION HELPERS
// ===============================

// CreateEnhancedRecoveryStack creates a complete recovery middleware stack
func CreateEnhancedRecoveryStack(config *RecoveryConfig, logger *zap.Logger) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	return func(next http.Handler) http.Handler {
		// Stack recovery middleware
		handler := next
		handler = EnhancedRecovery(config, logger)(handler)
		return handler
	}
}

// ReplaceLegacyRecovery replaces the old RecoverPanic middleware
// This should be used instead of middleware.RecoverPanic
func ReplaceLegacyRecovery(logger *zap.Logger) func(http.Handler) http.Handler {
	config := DefaultRecoveryConfig()

	// Adjust config based on environment
	env := getEnvironment()
	if env == "production" {
		config.StackTraceInResponse = false
		config.MaskInternalErrors = true
		config.EnableDetailedErrors = false
	} else {
		config.StackTraceInResponse = true
		config.MaskInternalErrors = false
		config.EnableDetailedErrors = true
	}

	return EnhancedRecovery(config, logger)
}
