// file: internal/middleware/rate_limiter.go
package middleware

import (
	"context"
	"encoding/json"
	"evalhub/internal/cache"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RateLimiterConfig holds rate limiting configuration
type RateLimiterConfig struct {
	// Global settings
	Enabled           bool          `json:"enabled"`
	FailureMode       string        `json:"failure_mode"`       // "allow", "deny"
	HeadersEnabled    bool          `json:"headers_enabled"`
	TrustForwardedFor bool          `json:"trust_forwarded_for"`
	
	// Default limits
	DefaultIPLimit       int           `json:"default_ip_limit"`        // requests per window
	DefaultUserLimit     int           `json:"default_user_limit"`      // requests per window for authenticated users
	DefaultEndpointLimit int           `json:"default_endpoint_limit"`  // requests per window per endpoint
	DefaultWindow        time.Duration `json:"default_window"`          // time window
	
	// Advanced settings
	BurstAllowance    int           `json:"burst_allowance"`     // allow burst above limit
	SlidingWindow     bool          `json:"sliding_window"`      // use sliding window vs fixed window
	Algorithm         string        `json:"algorithm"`           // "token_bucket", "sliding_window", "fixed_window"
	
	// Endpoint-specific limits
	EndpointLimits    map[string]*EndpointLimit `json:"endpoint_limits"`
	
	// User tier limits
	UserTierLimits    map[string]*UserTierLimit `json:"user_tier_limits"`
	
	// Whitelist/Blacklist
	WhitelistedIPs    []string      `json:"whitelisted_ips"`
	BlacklistedIPs    []string      `json:"blacklisted_ips"`
	WhitelistedUsers  []int64       `json:"whitelisted_users"`
	
	// DDoS protection
	DDoSThreshold     int           `json:"ddos_threshold"`      // triggers enhanced protection
	DDoSWindow        time.Duration `json:"ddos_window"`
	DDoSBlockDuration time.Duration `json:"ddos_block_duration"`
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	Path       string        `json:"path"`
	Method     string        `json:"method"`
	Limit      int           `json:"limit"`
	Window     time.Duration `json:"window"`
	BurstLimit int           `json:"burst_limit"`
	UserLimit  int           `json:"user_limit"`  // authenticated user limit
}

// UserTierLimit defines rate limits based on user tiers
type UserTierLimit struct {
	Tier   string        `json:"tier"`
	Limit  int           `json:"limit"`
	Window time.Duration `json:"window"`
	Burst  int           `json:"burst"`
}

// RateLimitResult represents the result of rate limit check
type RateLimitResult struct {
	Allowed      bool          `json:"allowed"`
	Limit        int           `json:"limit"`
	Remaining    int           `json:"remaining"`
	ResetTime    time.Time     `json:"reset_time"`
	RetryAfter   time.Duration `json:"retry_after"`
	LimitType    string        `json:"limit_type"`    // "ip", "user", "endpoint"
	LimitKey     string        `json:"limit_key"`
}

// DefaultRateLimiterConfig returns production-ready rate limiting configuration
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		Enabled:              true,
		FailureMode:          "allow", // Allow on cache failures
		HeadersEnabled:       true,
		TrustForwardedFor:    true,
		DefaultIPLimit:       1000,  // 1000 requests per hour per IP
		DefaultUserLimit:     5000,  // 5000 requests per hour per user
		DefaultEndpointLimit: 100,   // 100 requests per hour per endpoint
		DefaultWindow:        1 * time.Hour,
		BurstAllowance:       10,    // Allow 10 extra requests for bursts
		SlidingWindow:        true,
		Algorithm:            "sliding_window",
		EndpointLimits: map[string]*EndpointLimit{
			// Authentication endpoints - more restrictive
			"/api/auth/login": {
				Path:       "/api/auth/login",
				Method:     "POST",
				Limit:      10,
				Window:     15 * time.Minute,
				BurstLimit: 2,
				UserLimit:  20,
			},
			"/api/auth/register": {
				Path:       "/api/auth/register", 
				Method:     "POST",
				Limit:      5,
				Window:     15 * time.Minute,
				BurstLimit: 1,
				UserLimit:  10,
			},
			// API endpoints
			"/api/posts": {
				Path:       "/api/posts",
				Method:     "POST",
				Limit:      100,
				Window:     1 * time.Hour,
				BurstLimit: 10,
				UserLimit:  200,
			},
			"/api/comments": {
				Path:       "/api/comments",
				Method:     "POST", 
				Limit:      200,
				Window:     1 * time.Hour,
				BurstLimit: 20,
				UserLimit:  500,
			},
		},
		UserTierLimits: map[string]*UserTierLimit{
			"free": {
				Tier:   "free",
				Limit:  1000,
				Window: 1 * time.Hour,
				Burst:  50,
			},
			"premium": {
				Tier:   "premium",
				Limit:  10000,
				Window: 1 * time.Hour,
				Burst:  500,
			},
			"admin": {
				Tier:   "admin",
				Limit:  100000,
				Window: 1 * time.Hour,
				Burst:  5000,
			},
		},
		WhitelistedIPs:    []string{"127.0.0.1", "::1"},
		BlacklistedIPs:    []string{},
		WhitelistedUsers:  []int64{},
		DDoSThreshold:     10000,  // 10k requests in window triggers DDoS protection
		DDoSWindow:        5 * time.Minute,
		DDoSBlockDuration: 1 * time.Hour,
	}
}

// RateLimiter provides advanced rate limiting functionality
type RateLimiter struct {
	cache  cache.Cache
	config *RateLimiterConfig
	logger *zap.Logger
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cache cache.Cache, config *RateLimiterConfig, logger *zap.Logger) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}
	
	return &RateLimiter{
		cache:  cache,
		config: config,
		logger: logger,
	}
}

// RateLimit creates rate limiting middleware
func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if rate limiting is disabled
			if !limiter.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			requestLogger := GetRequestLogger(ctx)
			clientIP := getClientIP(r)
			
			// Check blacklist first
			if limiter.isBlacklisted(clientIP) {
				limiter.logger.Warn("Request from blacklisted IP",
					zap.String("ip", clientIP),
					zap.String("path", r.URL.Path),
				)
				limiter.writeRateLimitError(w, "IP blacklisted", http.StatusForbidden)
				return
			}

			// Check whitelist
			if limiter.isWhitelisted(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Check DDoS protection
			if ddosResult := limiter.checkDDoSProtection(ctx, clientIP); !ddosResult.Allowed {
				limiter.logger.Warn("DDoS protection triggered",
					zap.String("ip", clientIP),
					zap.String("path", r.URL.Path),
				)
				limiter.writeRateLimitHeaders(w, ddosResult)
				limiter.writeRateLimitError(w, "Rate limit exceeded - DDoS protection", http.StatusTooManyRequests)
				return
			}

			// Perform multi-tier rate limiting checks
			results := limiter.checkAllLimits(ctx, r)
			
			// Find the most restrictive limit that was exceeded
			for _, result := range results {
				if !result.Allowed {
					// Log rate limit violation
					requestLogger.Warn("Rate limit exceeded",
						zap.String("limit_type", result.LimitType),
						zap.String("limit_key", result.LimitKey),
						zap.Int("limit", result.Limit),
						zap.Int("remaining", result.Remaining),
						zap.Duration("retry_after", result.RetryAfter),
					)

					// Add rate limit headers
					limiter.writeRateLimitHeaders(w, result)
					
					// Return rate limit error
					limiter.writeRateLimitError(w, "Rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}

			// All checks passed - add headers with most restrictive limit info
			if len(results) > 0 {
				mostRestrictive := limiter.getMostRestrictiveResult(results)
				limiter.writeRateLimitHeaders(w, mostRestrictive)
			}

			// Continue to next middleware
			next.ServeHTTP(w, r)
		})
	}
}

// ===============================
// RATE LIMITING ALGORITHMS
// ===============================

// checkAllLimits performs all configured rate limit checks
func (rl *RateLimiter) checkAllLimits(ctx context.Context, r *http.Request) []*RateLimitResult {
	var results []*RateLimitResult
	
	clientIP := getClientIP(r)
	userID := getUserIDFromContext(r.Context())
	path := r.URL.Path
	method := r.Method

	// 1. IP-based rate limiting
	if ipResult := rl.checkIPLimit(ctx, clientIP); ipResult != nil {
		results = append(results, ipResult)
	}

	// 2. User-based rate limiting (if authenticated)
	if userID > 0 {
		if userResult := rl.checkUserLimit(ctx, userID); userResult != nil {
			results = append(results, userResult)
		}
	}

	// 3. Endpoint-specific rate limiting
	if endpointResult := rl.checkEndpointLimit(ctx, path, method, clientIP, userID); endpointResult != nil {
		results = append(results, endpointResult)
	}

	// 4. Global endpoint rate limiting
	if globalResult := rl.checkGlobalEndpointLimit(ctx, path); globalResult != nil {
		results = append(results, globalResult)
	}

	return results
}

// checkIPLimit checks IP-based rate limits
func (rl *RateLimiter) checkIPLimit(ctx context.Context, ip string) *RateLimitResult {
	key := fmt.Sprintf("rate_limit:ip:%s", ip)
	limit := rl.config.DefaultIPLimit
	window := rl.config.DefaultWindow

	return rl.checkLimit(ctx, key, limit, window, "ip", ip)
}

// checkUserLimit checks user-based rate limits
func (rl *RateLimiter) checkUserLimit(ctx context.Context, userID int64) *RateLimitResult {
	// Get user tier from context or default to "free"
	userTier := getUserTierFromContext(context.TODO()) // You'd implement this
	if userTier == "" {
		userTier = "free"
	}

	// Get tier-specific limits
	tierLimit, exists := rl.config.UserTierLimits[userTier]
	if !exists {
		tierLimit = &UserTierLimit{
			Limit:  rl.config.DefaultUserLimit,
			Window: rl.config.DefaultWindow,
			Burst:  rl.config.BurstAllowance,
		}
	}

	key := fmt.Sprintf("rate_limit:user:%d", userID)
	return rl.checkLimit(ctx, key, tierLimit.Limit, tierLimit.Window, "user", fmt.Sprintf("user_%d", userID))
}

// checkEndpointLimit checks endpoint-specific rate limits
func (rl *RateLimiter) checkEndpointLimit(ctx context.Context, path, method, ip string, userID int64) *RateLimitResult {
	endpointKey := path
	if method != "" {
		endpointKey = method + ":" + path
	}

	// Check for exact endpoint configuration
	endpointLimit, exists := rl.config.EndpointLimits[endpointKey]
	if !exists {
		// Check for path-only configuration
		endpointLimit, exists = rl.config.EndpointLimits[path]
	}

	if !exists {
		return nil // No specific limit for this endpoint
	}

	// Choose appropriate limit based on authentication status
	limit := endpointLimit.Limit
	if userID > 0 && endpointLimit.UserLimit > 0 {
		limit = endpointLimit.UserLimit
	}

	// Create different keys for IP vs User limits
	var key string
	var limitType string
	var limitKey string

	if userID > 0 {
		key = fmt.Sprintf("rate_limit:endpoint:%s:user:%d", endpointKey, userID)
		limitType = "endpoint_user"
		limitKey = fmt.Sprintf("%s_user_%d", endpointKey, userID)
	} else {
		key = fmt.Sprintf("rate_limit:endpoint:%s:ip:%s", endpointKey, ip)
		limitType = "endpoint_ip"
		limitKey = fmt.Sprintf("%s_ip_%s", endpointKey, maskIP(ip))
	}

	return rl.checkLimit(ctx, key, limit, endpointLimit.Window, limitType, limitKey)
}

// checkGlobalEndpointLimit checks global per-endpoint limits
func (rl *RateLimiter) checkGlobalEndpointLimit(ctx context.Context, path string) *RateLimitResult {
	if rl.config.DefaultEndpointLimit <= 0 {
		return nil
	}

	key := fmt.Sprintf("rate_limit:global_endpoint:%s", path)
	return rl.checkLimit(ctx, key, rl.config.DefaultEndpointLimit, rl.config.DefaultWindow, "global_endpoint", path)
}

// checkLimit performs the actual rate limit check using the configured algorithm
func (rl *RateLimiter) checkLimit(ctx context.Context, key string, limit int, window time.Duration, limitType, limitKey string) *RateLimitResult {
	switch rl.config.Algorithm {
	case "sliding_window":
		return rl.checkSlidingWindow(ctx, key, limit, window, limitType, limitKey)
	case "token_bucket":
		return rl.checkTokenBucket(ctx, key, limit, window, limitType, limitKey)
	case "fixed_window":
		return rl.checkFixedWindow(ctx, key, limit, window, limitType, limitKey)
	default:
		return rl.checkSlidingWindow(ctx, key, limit, window, limitType, limitKey)
	}
}

// ===============================
// SLIDING WINDOW ALGORITHM
// ===============================

// checkSlidingWindow implements sliding window rate limiting
func (rl *RateLimiter) checkSlidingWindow(ctx context.Context, key string, limit int, window time.Duration, limitType, limitKey string) *RateLimitResult {
	now := time.Now()
	windowStart := now.Add(-window)
	
	// Keys for current and previous windows
	currentWindow := now.Truncate(window).Unix()
	previousWindow := windowStart.Truncate(window).Unix()
	
	currentKey := fmt.Sprintf("%s:window:%d", key, currentWindow)
	previousKey := fmt.Sprintf("%s:window:%d", key, previousWindow)

	// Get counts for both windows
	currentCount := rl.getCount(ctx, currentKey)
	previousCount := rl.getCount(ctx, previousKey)
	
	// Calculate sliding window count
	windowProgress := float64(now.Sub(windowStart)) / float64(window)
	slidingCount := int(float64(previousCount)*(1-windowProgress) + float64(currentCount))
	
	// Check if limit exceeded
	allowed := slidingCount < limit
	remaining := limit - slidingCount
	if remaining < 0 {
		remaining = 0
	}

	// Increment current window if allowed
	if allowed {
		rl.incrementCount(ctx, currentKey, window)
	}

	resetTime := time.Unix(currentWindow, 0).Add(window)
	retryAfter := time.Until(resetTime)

	return &RateLimitResult{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: retryAfter,
		LimitType:  limitType,
		LimitKey:   limitKey,
	}
}

// ===============================
// TOKEN BUCKET ALGORITHM
// ===============================

// checkTokenBucket implements token bucket rate limiting
func (rl *RateLimiter) checkTokenBucket(ctx context.Context, key string, limit int, window time.Duration, limitType, limitKey string) *RateLimitResult {
	bucketKey := fmt.Sprintf("%s:bucket", key)
	timestampKey := fmt.Sprintf("%s:timestamp", key)
	
	now := time.Now()
	refillRate := float64(limit) / window.Seconds() // tokens per second
	
	// Get current bucket state
	tokens := rl.getTokens(ctx, bucketKey, limit)
	lastRefill := rl.getTimestamp(ctx, timestampKey, now)
	
	// Calculate tokens to add based on elapsed time
	elapsed := now.Sub(lastRefill).Seconds()
	tokensToAdd := elapsed * refillRate
	tokens = math.Min(float64(limit), tokens+tokensToAdd)
	
	// Check if request is allowed
	allowed := tokens >= 1.0
	if allowed {
		tokens -= 1.0
	}
	
	// Update bucket state
	rl.setTokens(ctx, bucketKey, tokens, window)
	rl.setTimestamp(ctx, timestampKey, now, window)
	
	remaining := int(tokens)
	nextRefill := time.Duration((1.0-tokens)/refillRate) * time.Second
	
	return &RateLimitResult{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		ResetTime:  now.Add(nextRefill),
		RetryAfter: nextRefill,
		LimitType:  limitType,
		LimitKey:   limitKey,
	}
}

// ===============================
// FIXED WINDOW ALGORITHM
// ===============================

// checkFixedWindow implements fixed window rate limiting
func (rl *RateLimiter) checkFixedWindow(ctx context.Context, key string, limit int, window time.Duration, limitType, limitKey string) *RateLimitResult {
	now := time.Now()
	windowStart := now.Truncate(window)
	windowKey := fmt.Sprintf("%s:window:%d", key, windowStart.Unix())
	
	// Get current count
	count := rl.getCount(ctx, windowKey)
	
	// Check if limit exceeded
	allowed := count < limit
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	
	// Increment count if allowed
	if allowed {
		rl.incrementCount(ctx, windowKey, window)
	}
	
	resetTime := windowStart.Add(window)
	retryAfter := time.Until(resetTime)
	
	return &RateLimitResult{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: retryAfter,
		LimitType:  limitType,
		LimitKey:   limitKey,
	}
}

// ===============================
// DDOS PROTECTION
// ===============================

// checkDDoSProtection implements DDoS protection
func (rl *RateLimiter) checkDDoSProtection(ctx context.Context, ip string) *RateLimitResult {
	if rl.config.DDoSThreshold <= 0 {
		return &RateLimitResult{Allowed: true}
	}

	ddosKey := fmt.Sprintf("ddos_protection:ip:%s", ip)
	blockKey := fmt.Sprintf("ddos_block:ip:%s", ddosKey)
	
	// Check if IP is currently blocked
	if blocked := rl.cache.Exists(ctx, blockKey); blocked {
		ttl, _ := rl.cache.GetTTL(ctx, blockKey)
		return &RateLimitResult{
			Allowed:    false,
			Limit:      0,
			Remaining:  0,
			ResetTime:  time.Now().Add(ttl),
			RetryAfter: ttl,
			LimitType:  "ddos",
			LimitKey:   ip,
		}
	}

	// Check request rate
	result := rl.checkFixedWindow(ctx, ddosKey, rl.config.DDoSThreshold, rl.config.DDoSWindow, "ddos", ip)
	
	// If threshold exceeded, block the IP
	if !result.Allowed {
		rl.cache.Set(ctx, blockKey, true, rl.config.DDoSBlockDuration)
		rl.logger.Warn("IP blocked due to DDoS protection",
			zap.String("ip", ip),
			zap.Int("threshold", rl.config.DDoSThreshold),
			zap.Duration("block_duration", rl.config.DDoSBlockDuration),
		)
	}

	return result
}

// ===============================
// HELPER METHODS
// ===============================

// isWhitelisted checks if request should bypass rate limiting
func (rl *RateLimiter) isWhitelisted(r *http.Request) bool {
	clientIP := getClientIP(r)
	
	// Check IP whitelist
	for _, whitelistedIP := range rl.config.WhitelistedIPs {
		if clientIP == whitelistedIP {
			return true
		}
	}
	
	// Check user whitelist
	userID := getUserIDFromContext(r.Context())
	if userID > 0 {
		for _, whitelistedUser := range rl.config.WhitelistedUsers {
			if userID == whitelistedUser {
				return true
			}
		}
	}
	
	return false
}

// isBlacklisted checks if IP is blacklisted
func (rl *RateLimiter) isBlacklisted(ip string) bool {
	for _, blacklistedIP := range rl.config.BlacklistedIPs {
		if ip == blacklistedIP {
			return true
		}
	}
	return false
}

// getMostRestrictiveResult finds the most restrictive (lowest remaining) result
func (rl *RateLimiter) getMostRestrictiveResult(results []*RateLimitResult) *RateLimitResult {
	if len(results) == 0 {
		return nil
	}
	
	mostRestrictive := results[0]
	for _, result := range results[1:] {
		if result.Remaining < mostRestrictive.Remaining {
			mostRestrictive = result
		}
	}
	
	return mostRestrictive
}

// writeRateLimitHeaders adds rate limit headers to response
func (rl *RateLimiter) writeRateLimitHeaders(w http.ResponseWriter, result *RateLimitResult) {
	if !rl.config.HeadersEnabled {
		return
	}
	
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))
	w.Header().Set("X-RateLimit-Type", result.LimitType)
	
	if !result.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
	}
}

// writeRateLimitError writes rate limit error response
func (rl *RateLimiter) writeRateLimitError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "RATE_LIMIT_EXCEEDED",
			"message": message,
		},
		"timestamp": time.Now().Unix(),
	}
	
	// Encode the errorResponse map to JSON and write it
	json.NewEncoder(w).Encode(errorResponse)
}

// ===============================
// CACHE OPERATIONS
// ===============================

// getCount gets count from cache
func (rl *RateLimiter) getCount(ctx context.Context, key string) int {
	if value, found := rl.cache.Get(ctx, key); found {
		if count, ok := value.(int64); ok {
			return int(count)
		}
		if count, ok := value.(int); ok {
			return count
		}
	}
	return 0
}

// incrementCount increments count in cache
func (rl *RateLimiter) incrementCount(ctx context.Context, key string, ttl time.Duration) {
	// Use atomic increment operation
	if newCount, err := rl.cache.Increment(ctx, key, 1); err != nil {
		// Fallback: set initial value
		rl.cache.Set(ctx, key, 1, ttl)
	} else if newCount == 1 {
		// Set TTL on first increment
		rl.cache.SetTTL(ctx, key, ttl)
	}
}

// getTokens gets token count from cache
func (rl *RateLimiter) getTokens(ctx context.Context, key string, maxTokens int) float64 {
	if value, found := rl.cache.Get(ctx, key); found {
		if tokens, ok := value.(float64); ok {
			return tokens
		}
		if tokens, ok := value.(int64); ok {
			return float64(tokens)
		}
	}
	return float64(maxTokens) // Start with full bucket
}

// setTokens sets token count in cache
func (rl *RateLimiter) setTokens(ctx context.Context, key string, tokens float64, ttl time.Duration) {
	rl.cache.Set(ctx, key, tokens, ttl)
}

// getTimestamp gets timestamp from cache
func (rl *RateLimiter) getTimestamp(ctx context.Context, key string, defaultTime time.Time) time.Time {
	if value, found := rl.cache.Get(ctx, key); found {
		if timestamp, ok := value.(time.Time); ok {
			return timestamp
		}
		if timestamp, ok := value.(int64); ok {
			return time.Unix(timestamp, 0)
		}
	}
	return defaultTime
}

// setTimestamp sets timestamp in cache
func (rl *RateLimiter) setTimestamp(ctx context.Context, key string, timestamp time.Time, ttl time.Duration) {
	rl.cache.Set(ctx, key, timestamp, ttl)
}

// ===============================
// UTILITY FUNCTIONS
// ===============================

// getUserTierFromContext gets user tier from context (placeholder)
func getUserTierFromContext(ctx context.Context) string {
	// This would integrate with your user service
	// For now, return default
	return "free"
}

// maskIP masks IP address for logging privacy
func maskIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.***.*", parts[0], parts[1])
	}
	return "***"
}

// ===============================
// ADVANCED FEATURES
// ===============================

// RateLimiterStats provides rate limiter statistics
type RateLimiterStats struct {
	TotalRequests    int64   `json:"total_requests"`
	AllowedRequests  int64   `json:"allowed_requests"`
	BlockedRequests  int64   `json:"blocked_requests"`
	BlockedByType    map[string]int64 `json:"blocked_by_type"`
	DDoSBlocks       int64   `json:"ddos_blocks"`
	TopLimitedIPs    []string `json:"top_limited_ips"`
	AverageBlockTime float64 `json:"average_block_time_seconds"`
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats(ctx context.Context) (*RateLimiterStats, error) {
	// This would collect statistics from cache
	// Implementation depends on your monitoring requirements
	return &RateLimiterStats{
		TotalRequests:   0,
		AllowedRequests: 0,
		BlockedRequests: 0,
		BlockedByType:   make(map[string]int64),
		DDoSBlocks:      0,
		TopLimitedIPs:   []string{},
		AverageBlockTime: 0,
	}, nil
}

// ClearIPLimits clears all rate limits for an IP (admin function)
func (rl *RateLimiter) ClearIPLimits(ctx context.Context, ip string) error {
	patterns := []string{
		fmt.Sprintf("rate_limit:ip:%s*", ip),
		fmt.Sprintf("ddos_protection:ip:%s*", ip),
		fmt.Sprintf("ddos_block:ip:%s*", ip),
	}
	
	for _, pattern := range patterns {
		if err := rl.cache.DeletePattern(ctx, pattern); err != nil {
			rl.logger.Warn("Failed to clear rate limit pattern",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
		}
	}
	
	rl.logger.Info("Cleared rate limits for IP", zap.String("ip", ip))
	return nil
}

// ClearUserLimits clears all rate limits for a user (admin function)
func (rl *RateLimiter) ClearUserLimits(ctx context.Context, userID int64) error {
	pattern := fmt.Sprintf("rate_limit:user:%d*", userID)
	
	if err := rl.cache.DeletePattern(ctx, pattern); err != nil {
		rl.logger.Warn("Failed to clear user rate limits",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return err
	}
	
	rl.logger.Info("Cleared rate limits for user", zap.Int64("user_id", userID))
	return nil
}
