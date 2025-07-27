package database

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Metrics collects and tracks database performance metrics
type Metrics struct {
	db     *sql.DB
	logger *zap.Logger
	
	// Query metrics
	queryCount     int64
	queryDuration  int64 // nanoseconds
	errorCount     int64
	slowQueryCount int64
	
	// Query type counters
	execCount     int64
	queryRowCount int64
	selectCount   int64
	insertCount   int64
	updateCount   int64
	deleteCount   int64
	
	// Performance tracking
	slowQueryThreshold time.Duration
	
	// Historical data
	mu             sync.RWMutex
	hourlyStats    []HourlyMetrics
	dailyStats     []DailyMetrics
	
	stopCh chan struct{}
}

// HourlyMetrics represents metrics for one hour
type HourlyMetrics struct {
	Hour         time.Time
	QueryCount   int64
	ErrorCount   int64
	AvgDuration  time.Duration
	SlowQueries  int64
}

// DailyMetrics represents metrics for one day
type DailyMetrics struct {
	Date         time.Time
	QueryCount   int64
	ErrorCount   int64
	AvgDuration  time.Duration
	SlowQueries  int64
	PeakHour     time.Time
}

// MetricsSnapshot provides a point-in-time view of metrics
type MetricsSnapshot struct {
	QueryCount       int64             `json:"query_count"`
	ErrorCount       int64             `json:"error_count"`
	SlowQueryCount   int64             `json:"slow_query_count"`
	AvgQueryDuration time.Duration     `json:"avg_query_duration"`
	DBStats          sql.DBStats       `json:"db_stats"`
	CurrentHour      *HourlyMetrics    `json:"current_hour,omitempty"`
	Last24Hours      []HourlyMetrics   `json:"last_24_hours"`
	Timestamp        time.Time         `json:"timestamp"`
}

// NewMetrics creates a new metrics collector
func NewMetrics(db *sql.DB, logger *zap.Logger) *Metrics {
	m := &Metrics{
		db:                 db,
		logger:             logger,
		slowQueryThreshold: 100 * time.Millisecond,
		hourlyStats:        make([]HourlyMetrics, 0, 24), // Keep 24 hours
		dailyStats:         make([]DailyMetrics, 0, 30),  // Keep 30 days
		stopCh:             make(chan struct{}),
	}
	
	// Start background metric collection
	go m.collectPeriodicMetrics()
	
	return m
}

// RecordQuery records metrics for a database query
func (m *Metrics) RecordQuery(queryType string, duration time.Duration, err error) {
	atomic.AddInt64(&m.queryCount, 1)
	atomic.AddInt64(&m.queryDuration, int64(duration))
	
	if err != nil {
		atomic.AddInt64(&m.errorCount, 1)
	}
	
	if duration > m.slowQueryThreshold {
		atomic.AddInt64(&m.slowQueryCount, 1)
	}
	
	// Track query type
	switch queryType {
	case "exec":
		atomic.AddInt64(&m.execCount, 1)
	case "query":
		atomic.AddInt64(&m.selectCount, 1)
	case "query_row":
		atomic.AddInt64(&m.queryRowCount, 1)
	}
}

// Snapshot returns current metrics snapshot
func (m *Metrics) Snapshot() *MetricsSnapshot {
	queryCount := atomic.LoadInt64(&m.queryCount)
	errorCount := atomic.LoadInt64(&m.errorCount)
	slowQueryCount := atomic.LoadInt64(&m.slowQueryCount)
	totalDuration := atomic.LoadInt64(&m.queryDuration)
	
	var avgDuration time.Duration
	if queryCount > 0 {
		avgDuration = time.Duration(totalDuration / queryCount)
	}
	
	m.mu.RLock()
	last24Hours := make([]HourlyMetrics, len(m.hourlyStats))
	copy(last24Hours, m.hourlyStats)
	
	var currentHour *HourlyMetrics
	if len(m.hourlyStats) > 0 {
		latest := m.hourlyStats[len(m.hourlyStats)-1]
		currentHour = &latest
	}
	m.mu.RUnlock()
	
	return &MetricsSnapshot{
		QueryCount:       queryCount,
		ErrorCount:       errorCount,
		SlowQueryCount:   slowQueryCount,
		AvgQueryDuration: avgDuration,
		DBStats:          m.db.Stats(),
		CurrentHour:      currentHour,
		Last24Hours:      last24Hours,
		Timestamp:        time.Now(),
	}
}

// collectPeriodicMetrics runs background metric collection
func (m *Metrics) collectPeriodicMetrics() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.aggregateHourlyMetrics()
		case <-m.stopCh:
			return
		}
	}
}

// aggregateHourlyMetrics aggregates metrics for the current hour
func (m *Metrics) aggregateHourlyMetrics() {
	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	
	queryCount := atomic.LoadInt64(&m.queryCount)
	errorCount := atomic.LoadInt64(&m.errorCount)
	slowQueryCount := atomic.LoadInt64(&m.slowQueryCount)
	totalDuration := atomic.LoadInt64(&m.queryDuration)
	
	var avgDuration time.Duration
	if queryCount > 0 {
		avgDuration = time.Duration(totalDuration / queryCount)
	}
	
	hourlyMetric := HourlyMetrics{
		Hour:        currentHour,
		QueryCount:  queryCount,
		ErrorCount:  errorCount,
		AvgDuration: avgDuration,
		SlowQueries: slowQueryCount,
	}
	
	m.mu.Lock()
	
	// Add to hourly stats
	m.hourlyStats = append(m.hourlyStats, hourlyMetric)
	
	// Keep only last 24 hours
	if len(m.hourlyStats) > 24 {
		m.hourlyStats = m.hourlyStats[len(m.hourlyStats)-24:]
	}
	
	// Aggregate daily stats if it's a new day
	if len(m.hourlyStats) >= 24 {
		m.aggregateDailyMetrics()
	}
	
	m.mu.Unlock()
	
	// Log hourly summary
	m.logger.Info("Hourly database metrics",
		zap.Time("hour", currentHour),
		zap.Int64("queries", queryCount),
		zap.Int64("errors", errorCount),
		zap.Int64("slow_queries", slowQueryCount),
		zap.Duration("avg_duration", avgDuration),
	)
}

// aggregateDailyMetrics aggregates hourly metrics into daily stats
func (m *Metrics) aggregateDailyMetrics() {
	if len(m.hourlyStats) == 0 {
		return
	}
	
	// Get the date from the last complete day
	lastHour := m.hourlyStats[len(m.hourlyStats)-1]
	date := time.Date(lastHour.Hour.Year(), lastHour.Hour.Month(), lastHour.Hour.Day(), 0, 0, 0, 0, lastHour.Hour.Location())
	
	var totalQueries, totalErrors, totalSlowQueries int64
	var totalDuration time.Duration
	var peakHour time.Time
	var peakQueries int64
	
	// Aggregate last 24 hours
	for _, hourStat := range m.hourlyStats {
		totalQueries += hourStat.QueryCount
		totalErrors += hourStat.ErrorCount
		totalSlowQueries += hourStat.SlowQueries
		totalDuration += hourStat.AvgDuration
		
		if hourStat.QueryCount > peakQueries {
			peakQueries = hourStat.QueryCount
			peakHour = hourStat.Hour
		}
	}
	
	avgDuration := totalDuration / time.Duration(len(m.hourlyStats))
	
	dailyMetric := DailyMetrics{
		Date:        date,
		QueryCount:  totalQueries,
		ErrorCount:  totalErrors,
		AvgDuration: avgDuration,
		SlowQueries: totalSlowQueries,
		PeakHour:    peakHour,
	}
	
	m.dailyStats = append(m.dailyStats, dailyMetric)
	
	// Keep only last 30 days
	if len(m.dailyStats) > 30 {
		m.dailyStats = m.dailyStats[len(m.dailyStats)-30:]
	}
}

// GetConnectionMetrics returns detailed connection pool metrics
func (m *Metrics) GetConnectionMetrics() map[string]interface{} {
	stats := m.db.Stats()
	
	return map[string]interface{}{
		"max_open_connections":     stats.MaxOpenConnections,
		"open_connections":         stats.OpenConnections,
		"in_use":                  stats.InUse,
		"idle":                    stats.Idle,
		"wait_count":              stats.WaitCount,
		"wait_duration":           stats.WaitDuration,
		"max_idle_closed":         stats.MaxIdleClosed,
		"max_idle_time_closed":    stats.MaxIdleTimeClosed,
		"max_lifetime_closed":     stats.MaxLifetimeClosed,
		"connection_efficiency":   float64(stats.InUse) / float64(stats.OpenConnections),
	}
}

// LogPerformanceSummary logs a comprehensive performance summary
func (m *Metrics) LogPerformanceSummary() {
	snapshot := m.Snapshot()
	connectionMetrics := m.GetConnectionMetrics()
	
	m.logger.Info("Database performance summary",
		zap.Int64("total_queries", snapshot.QueryCount),
		zap.Int64("errors", snapshot.ErrorCount),
		zap.Int64("slow_queries", snapshot.SlowQueryCount),
		zap.Duration("avg_query_duration", snapshot.AvgQueryDuration),
		zap.Int("open_connections", snapshot.DBStats.OpenConnections),
		zap.Int("idle_connections", snapshot.DBStats.Idle),
		zap.Float64("connection_efficiency", connectionMetrics["connection_efficiency"].(float64)),
	)
}

// Stop stops the metrics collection
func (m *Metrics) Stop() {
	close(m.stopCh)
}

// Reset resets all metrics (useful for testing)
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.queryCount, 0)
	atomic.StoreInt64(&m.queryDuration, 0)
	atomic.StoreInt64(&m.errorCount, 0)
	atomic.StoreInt64(&m.slowQueryCount, 0)
	atomic.StoreInt64(&m.execCount, 0)
	atomic.StoreInt64(&m.queryRowCount, 0)
	atomic.StoreInt64(&m.selectCount, 0)
	atomic.StoreInt64(&m.insertCount, 0)
	atomic.StoreInt64(&m.updateCount, 0)
	atomic.StoreInt64(&m.deleteCount, 0)
	
	m.mu.Lock()
	m.hourlyStats = m.hourlyStats[:0]
	m.dailyStats = m.dailyStats[:0]
	m.mu.Unlock()
}