package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"evalhub/internal/cache"
	"go.uber.org/zap"
)

// cacheService implements advanced caching with multiple backends
type cacheService struct {
	primary  cache.Cache // Redis for production
	fallback cache.Cache // In-memory for fallback
	logger   *zap.Logger
	config   *CacheConfig
	metrics  *CacheMetrics
	mu       sync.RWMutex
}

// CacheConfig holds cache service configuration
type CacheConfig struct {
	DefaultTTL       time.Duration `json:"default_ttl"`
	MaxKeySize       int           `json:"max_key_size"`
	MaxValueSize     int           `json:"max_value_size"`
	EnableFallback   bool          `json:"enable_fallback"`
	EnableMetrics    bool          `json:"enable_metrics"`
	KeyPrefix        string        `json:"key_prefix"`
	CompressionSize  int           `json:"compression_size"` // Compress values larger than this
	EnableEncryption bool          `json:"enable_encryption"`
}

// CacheMetrics tracks cache performance
type CacheMetrics struct {
	Hits             int64         `json:"hits"`
	Misses           int64         `json:"misses"`
	Sets             int64         `json:"sets"`
	Deletes          int64         `json:"deletes"`
	Keys             int64         `json:"keys"`
	Memory           int64         `json:"memory_bytes"`
	HitRatio         float64       `json:"hit_ratio"`
	Uptime           time.Duration `json:"uptime"`
	UsedMemory       int64         `json:"used_memory"`
	MaxMemory        int64         `json:"max_memory"`
	ConnectedClients int64         `json:"connected_clients"`
	TotalCommands    int64         `json:"total_commands"`
	ExpiredKeys      int64         `json:"expired_keys"`
	EvictedKeys      int64         `json:"evicted_keys"`
	LastResetTime    time.Time     `json:"last_reset_time"`
	mu               sync.RWMutex
}

// NewCacheService creates a new enterprise cache service
func NewCacheService(
	primary cache.Cache,
	fallback cache.Cache,
	logger *zap.Logger,
	config *CacheConfig,
) CacheService {
	if config == nil {
		config = DefaultCacheConfig()
	}

	service := &cacheService{
		primary:  primary,
		fallback: fallback,
		logger:   logger,
		config:   config,
		metrics:  &CacheMetrics{LastResetTime: time.Now()},
	}

	// Start background cleanup if configured
	if config.EnableMetrics {
		go service.startMetricsCollection()
	}

	return service
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		DefaultTTL:       15 * time.Minute,
		MaxKeySize:       250,              // Redis max key size
		MaxValueSize:     10 * 1024 * 1024, // 10MB
		EnableFallback:   true,
		EnableMetrics:    true,
		KeyPrefix:        "evalhub:",
		CompressionSize:  1024, // 1KB
		EnableEncryption: false,
	}
}

// ===============================
// BASIC CACHE OPERATIONS
// ===============================

// Get retrieves a value from cache with fallback support
func (c *cacheService) Get(ctx context.Context, key string) (interface{}, bool) {
	// Validate key
	if err := c.validateKey(key); err != nil {
		c.logger.Warn("Invalid cache key", zap.String("key", key), zap.Error(err))
		c.incrementMetric("errors")
		return nil, false
	}

	// Add prefix to key
	fullKey := c.buildKey(key)

	// Try primary cache first
	if value, found := c.getFromBackend(ctx, c.primary, fullKey); found {
		c.incrementMetric("hits")
		return value, true
	}

	// Try fallback if enabled
	if c.config.EnableFallback && c.fallback != nil {
		if value, found := c.getFromBackend(ctx, c.fallback, fullKey); found {
			c.incrementMetric("hits")
			// Restore to primary cache
			go c.restoreToPrimary(ctx, fullKey, value)
			return value, true
		}
	}

	c.incrementMetric("misses")
	return nil, false
}

// Set stores a value in cache with TTL
func (c *cacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Validate key and value
	if err := c.validateKey(key); err != nil {
		c.incrementMetric("errors")
		return NewValidationError("invalid cache key", err)
	}

	if err := c.validateValue(value); err != nil {
		c.incrementMetric("errors")
		return NewValidationError("invalid cache value", err)
	}

	// Use default TTL if not specified
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	// Add prefix to key
	fullKey := c.buildKey(key)

	// Set in primary cache
	if err := c.setInBackend(ctx, c.primary, fullKey, value, ttl); err != nil {
		c.logger.Error("Failed to set value in primary cache",
			zap.String("key", key),
			zap.Error(err),
		)

		// Try fallback if primary fails
		if c.config.EnableFallback && c.fallback != nil {
			if fallbackErr := c.setInBackend(ctx, c.fallback, fullKey, value, ttl); fallbackErr != nil {
				c.incrementMetric("errors")
				return NewInternalError("failed to set value in any cache backend")
			}
		} else {
			c.incrementMetric("errors")
			return NewInternalError("failed to set value in cache")
		}
	}

	// Set in fallback for redundancy
	if c.config.EnableFallback && c.fallback != nil {
		go c.setInBackend(ctx, c.fallback, fullKey, value, ttl)
	}

	c.incrementMetric("sets")
	c.incrementMetric("total_keys")
	return nil
}

// Delete removes a value from cache
func (c *cacheService) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate key
	if err := c.validateKey(key); err != nil {
		c.incrementMetric("errors")
		return fmt.Errorf("invalid key: %w", err)
	}

	// Delete from primary cache
	err := c.deleteFromBackend(ctx, c.primary, key)
	if err != nil {
		c.incrementMetric("errors")
		c.logger.Error("Failed to delete from primary cache",
			zap.String("key", key),
			zap.Error(err))
	}

	// If fallback is enabled, delete from fallback as well
	if c.config.EnableFallback && c.fallback != nil {
		if err := c.deleteFromBackend(ctx, c.fallback, key); err != nil {
			c.logger.Warn("Failed to delete from fallback cache",
				zap.String("key", key),
				zap.Error(err))
		}
	}

	c.incrementMetric("deletes")
	return err
}

// DeleteMultiple removes multiple values from cache
func (c *cacheService) DeleteMultiple(ctx context.Context, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate keys
	for _, key := range keys {
		if err := c.validateKey(key); err != nil {
			c.incrementMetric("errors")
			return fmt.Errorf("invalid key %q: %w", key, err)
		}
	}

	// Delete from primary cache
	err := c.primary.DeleteMultiple(ctx, keys)
	if err != nil {
		c.incrementMetric("errors")
		c.logger.Error("Failed to delete multiple keys from primary cache",
			zap.Strings("keys", keys),
			zap.Error(err))
	}

	// If fallback is enabled, delete from fallback as well
	if c.config.EnableFallback && c.fallback != nil {
		if err := c.fallback.DeleteMultiple(ctx, keys); err != nil {
			c.logger.Warn("Failed to delete multiple keys from fallback cache",
				zap.Strings("keys", keys),
				zap.Error(err))
		}
	}

	c.incrementMetric("deletes")
	return err
}

// ===============================
// ADVANCED CACHE OPERATIONS
// ===============================

// GetMultiple retrieves multiple values efficiently
func (c *cacheService) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]interface{})
	var missingKeys []string

	// Try to get from primary cache first
	primaryResults, err := c.primary.GetMultiple(ctx, keys)
	if err != nil {
		c.incrementMetric("errors")
		c.logger.Error("Failed to get multiple from primary cache",
			zap.Strings("keys", keys),
			zap.Error(err))
		primaryResults = make(map[string]interface{})
	}

	// Check for missing keys
	for _, key := range keys {
		if val, ok := primaryResults[key]; ok {
			result[key] = val
		} else {
			missingKeys = append(missingKeys, key)
		}
	}

	// If all keys found, return
	if len(missingKeys) == 0 {
		return result, nil
	}

	// Try to get missing keys from fallback if enabled
	if c.config.EnableFallback && c.fallback != nil {
		fallbackResults, err := c.fallback.GetMultiple(ctx, missingKeys)
		if err != nil {
			c.logger.Warn("Failed to get multiple from fallback cache",
				zap.Strings("keys", missingKeys),
				zap.Error(err))
			fallbackResults = make(map[string]interface{})
		}

		// Update results with fallback values and restore to primary
		for key, value := range fallbackResults {
			result[key] = value

			// Restore to primary cache with TTL if possible
			if ttl, err := c.fallback.GetTTL(ctx, key); err == nil {
				if err := c.primary.Set(ctx, key, value, ttl); err != nil {
					c.logger.Warn("Failed to restore to primary cache",
						zap.String("key", key),
						zap.Error(err))
				}
			} else {
				if err := c.primary.Set(ctx, key, value, c.config.DefaultTTL); err != nil {
					c.logger.Warn("Failed to restore to primary cache",
						zap.String("key", key),
						zap.Error(err))
				}
			}

			c.incrementMetric("hits")
		}

		// Update metrics for any still missing keys
		for _, key := range missingKeys {
			if _, found := fallbackResults[key]; !found {
				c.incrementMetric("misses")
			}
		}
	} else {
		// If no fallback, increment misses for all missing keys
		for range missingKeys {
			c.incrementMetric("misses")
		}
	}

	return result, nil
}

// SetMultiple stores multiple values efficiently
func (c *cacheService) SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	// Validate all items first
	validItems := make(map[string]interface{})
	for key, value := range items {
		if err := c.validateKey(key); err != nil {
			c.logger.Warn("Skipping invalid key in batch set", zap.String("key", key))
			continue
		}
		if err := c.validateValue(value); err != nil {
			c.logger.Warn("Skipping invalid value in batch set", zap.String("key", key))
			continue
		}
		validItems[c.buildKey(key)] = value
	}

	// Set in primary cache
	if err := c.setMultipleInBackend(ctx, c.primary, validItems, ttl); err != nil {
		c.logger.Error("Failed to set multiple values in primary cache", zap.Error(err))
		c.incrementMetric("errors")
		return NewInternalError("failed to set multiple values")
	}

	// Set in fallback asynchronously
	if c.config.EnableFallback && c.fallback != nil {
		go c.setMultipleInBackend(ctx, c.fallback, validItems, ttl)
	}

	c.metrics.mu.Lock()
	c.metrics.Sets += int64(len(validItems))
	c.metrics.Keys += int64(len(validItems))
	c.metrics.mu.Unlock()

	return nil
}

// ===============================
// PATTERN-BASED OPERATIONS
// ===============================

// DeletePattern deletes all keys matching a pattern
func (c *cacheService) DeletePattern(ctx context.Context, pattern string) error {
	if pattern == "" {
		return NewValidationError("pattern cannot be empty", nil)
	}

	fullPattern := c.buildKey(pattern)

	// Delete from primary
	if err := c.deletePatternFromBackend(ctx, c.primary, fullPattern); err != nil {
		c.logger.Error("Failed to delete pattern from primary cache",
			zap.String("pattern", pattern),
			zap.Error(err),
		)
	}

	// Delete from fallback
	if c.config.EnableFallback && c.fallback != nil {
		if err := c.deletePatternFromBackend(ctx, c.fallback, fullPattern); err != nil {
			c.logger.Warn("Failed to delete pattern from fallback cache",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GetKeysWithPattern retrieves all keys matching a pattern
func (c *cacheService) GetKeysWithPattern(ctx context.Context, pattern string) ([]string, error) {
	if pattern == "" {
		return nil, NewValidationError("pattern cannot be empty", nil)
	}

	fullPattern := c.buildKey(pattern)

	// Get from primary cache
	keys, err := c.getKeysFromBackend(ctx, c.primary, fullPattern)
	if err != nil {
		return nil, NewInternalError("failed to get keys by pattern")
	}

	// Remove prefix from keys
	result := make([]string, 0, len(keys))
	prefix := c.config.KeyPrefix
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			result = append(result, key[len(prefix):])
		}
	}

	return result, nil
}

// ===============================
// ATOMIC OPERATIONS
// ===============================

// Increment atomically increments a numeric value
func (c *cacheService) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	if err := c.validateKey(key); err != nil {
		return 0, NewValidationError("invalid cache key", err)
	}

	fullKey := c.buildKey(key)

	// Try primary cache
	if value, err := c.incrementInBackend(ctx, c.primary, fullKey, delta); err == nil {
		return value, nil
	}

	// Try fallback
	if c.config.EnableFallback && c.fallback != nil {
		if value, err := c.incrementInBackend(ctx, c.fallback, fullKey, delta); err == nil {
			return value, nil
		}
	}

	c.incrementMetric("errors")
	return 0, NewInternalError("failed to increment value")
}

// SetTTL updates the TTL of an existing key
func (c *cacheService) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	if err := c.validateKey(key); err != nil {
		return NewValidationError("invalid cache key", err)
	}

	fullKey := c.buildKey(key)

	// Set TTL in primary
	if err := c.setTTLInBackend(ctx, c.primary, fullKey, ttl); err != nil {
		c.logger.Error("Failed to set TTL in primary cache",
			zap.String("key", key),
			zap.Error(err),
		)
	}

	// Set TTL in fallback
	if c.config.EnableFallback && c.fallback != nil {
		if err := c.setTTLInBackend(ctx, c.fallback, fullKey, ttl); err != nil {
			c.logger.Warn("Failed to set TTL in fallback cache",
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}

	return nil
}

// ===============================
// CACHE MANAGEMENT
// ===============================

// Clear removes all cached data
func (c *cacheService) Clear(ctx context.Context) error {
	// Clear primary
	if err := c.clearBackend(ctx, c.primary); err != nil {
		c.logger.Error("Failed to clear primary cache", zap.Error(err))
	}

	// Clear fallback
	if c.config.EnableFallback && c.fallback != nil {
		if err := c.clearBackend(ctx, c.fallback); err != nil {
			c.logger.Error("Failed to clear fallback cache", zap.Error(err))
		}
	}

	// Reset metrics
	c.resetMetrics()

	return nil
}

// GetStats returns cache statistics
func (c *cacheService) GetStats(ctx context.Context) *CacheStats {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	hitRate := 0.0
	totalRequests := c.metrics.Hits + c.metrics.Misses
	if totalRequests > 0 {
		hitRate = float64(c.metrics.Hits) / float64(totalRequests) * 100
	}

	return &CacheStats{
		HitCount:     c.metrics.Hits,
		MissCount:    c.metrics.Misses,
		HitRate:      hitRate,
		Size:         c.metrics.Keys,
		MaxSize:      c.metrics.MaxMemory,
		EvictedCount: c.metrics.EvictedKeys,
	}
}

// ===============================
// SPECIALIZED CACHE METHODS
// ===============================

// CacheUser caches user data with appropriate TTL
func (c *cacheService) CacheUser(ctx context.Context, userID int64, user interface{}) error {
	key := fmt.Sprintf("user:%d", userID)
	return c.Set(ctx, key, user, 15*time.Minute)
}

// GetCachedUser retrieves cached user data
func (c *cacheService) GetCachedUser(ctx context.Context, userID int64) (interface{}, bool) {
	key := fmt.Sprintf("user:%d", userID)
	return c.Get(ctx, key)
}

// CachePost caches post data
func (c *cacheService) CachePost(ctx context.Context, postID int64, post interface{}) error {
	key := fmt.Sprintf("post:%d", postID)
	return c.Set(ctx, key, post, 30*time.Minute)
}

// GetCachedPost retrieves cached post data
func (c *cacheService) GetCachedPost(ctx context.Context, postID int64) (interface{}, bool) {
	key := fmt.Sprintf("post:%d", postID)
	return c.Get(ctx, key)
}

// CacheSearchResults caches search results with shorter TTL
func (c *cacheService) CacheSearchResults(ctx context.Context, query string, results interface{}) error {
	key := fmt.Sprintf("search:%s", query)
	return c.Set(ctx, key, results, 5*time.Minute)
}

// InvalidateUserCache removes all user-related cache entries
func (c *cacheService) InvalidateUserCache(ctx context.Context, userID int64) error {
	patterns := []string{
		fmt.Sprintf("user:%d", userID),
		fmt.Sprintf("user:%d:*", userID),
		fmt.Sprintf("posts:user:%d", userID),
		fmt.Sprintf("stats:user:%d", userID),
	}

	for _, pattern := range patterns {
		if err := c.DeletePattern(ctx, pattern); err != nil {
			c.logger.Warn("Failed to invalidate cache pattern",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
		}
	}

	return nil
}

// ===============================
// HELPER METHODS
// ===============================

// validateKey validates cache key
func (c *cacheService) validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if len(key) > c.config.MaxKeySize-len(c.config.KeyPrefix) {
		return fmt.Errorf("key too long (max %d chars)", c.config.MaxKeySize-len(c.config.KeyPrefix))
	}
	return nil
}

// validateValue validates cache value
func (c *cacheService) validateValue(value interface{}) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	// Estimate serialized size
	if data, err := json.Marshal(value); err != nil {
		return fmt.Errorf("value is not serializable: %w", err)
	} else if len(data) > c.config.MaxValueSize {
		return fmt.Errorf("value too large (max %d bytes)", c.config.MaxValueSize)
	}

	return nil
}

// buildKey creates the full cache key with prefix
func (c *cacheService) buildKey(key string) string {
	return c.config.KeyPrefix + key
}

// incrementMetric safely increments a metric
func (c *cacheService) incrementMetric(metric string) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	switch metric {
	case "hits":
		c.metrics.Hits++
	case "misses":
		c.metrics.Misses++
	case "sets":
		c.metrics.Sets++
	case "deletes":
		c.metrics.Deletes++
	case "keys":
		c.metrics.Keys++
	case "evictions":
		c.metrics.EvictedKeys++
	}
}

// resetMetrics resets all metrics
func (c *cacheService) resetMetrics() {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.Hits = 0
	c.metrics.Misses = 0
	c.metrics.Sets = 0
	c.metrics.Deletes = 0
	c.metrics.Keys = 0
	c.metrics.EvictedKeys = 0
	c.metrics.LastResetTime = time.Now()
}

// startMetricsCollection starts background metrics collection
func (c *cacheService) startMetricsCollection() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		stats := c.GetStats(context.Background())
		c.logger.Debug("Cache metrics",
			zap.Int64("hits", stats.HitCount),
			zap.Int64("misses", stats.MissCount),
			zap.Float64("hit_rate", stats.HitRate),
			zap.Int64("size", stats.Size),
		)
	}
}

// Backend operation methods
func (c *cacheService) getFromBackend(ctx context.Context, backend cache.Cache, key string) (interface{}, bool) {
	return backend.Get(ctx, key)
}

func (c *cacheService) setInBackend(ctx context.Context, backend cache.Cache, key string, value interface{}, ttl time.Duration) error {
	return backend.Set(ctx, key, value, ttl)
}

func (c *cacheService) deleteFromBackend(ctx context.Context, backend cache.Cache, key string) error {
	return backend.Delete(ctx, key)
}

func (c *cacheService) getMultipleFromBackend(ctx context.Context, backend cache.Cache, keys []string) map[string]interface{} {
	result, err := backend.GetMultiple(ctx, keys)
	if err != nil {
		c.logger.Error("Error getting multiple from backend", zap.Error(err))
		return make(map[string]interface{})
	}
	return result
}

func (c *cacheService) setMultipleInBackend(ctx context.Context, backend cache.Cache, items map[string]interface{}, ttl time.Duration) error {
	return backend.SetMultiple(ctx, items, ttl)
}

func (c *cacheService) deletePatternFromBackend(ctx context.Context, backend cache.Cache, pattern string) error {
	return backend.DeletePattern(ctx, pattern)
}

func (c *cacheService) getKeysFromBackend(ctx context.Context, backend cache.Cache, pattern string) ([]string, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend is nil")
	}
	// Use DeletePattern with a no-op pattern to get all keys
	// Note: This is a workaround since the Cache interface doesn't have a direct way to list keys
	// In a real implementation, you might want to add a Keys() method to the Cache interface
	return []string{}, fmt.Errorf("key pattern matching not directly supported")
}

func (c *cacheService) incrementInBackend(ctx context.Context, backend cache.Cache, key string, delta int64) (int64, error) {
	if backend == nil {
		return 0, fmt.Errorf("backend is nil")
	}
	return backend.Increment(ctx, key, delta)
}

// setTTLInBackend updates the TTL for a key in the specified backend
func (c *cacheService) setTTLInBackend(ctx context.Context, backend cache.Cache, key string, ttl time.Duration) error {
	if backend == nil {
		return fmt.Errorf("backend is nil")
	}
	return backend.SetTTL(ctx, key, ttl)
}

func (c *cacheService) clearBackend(ctx context.Context, backend cache.Cache) error {
	if backend == nil {
		return fmt.Errorf("backend is nil")
	}
	return backend.Clear(ctx)
}

func (c *cacheService) getBackendStats(ctx context.Context) map[string]interface{} {
	stats := make(map[string]interface{})

	if c.primary != nil {
		if primaryStats, err := c.primary.Stats(ctx); err == nil {
			stats["primary"] = primaryStats
		} else {
			c.logger.Warn("Failed to get primary cache stats", zap.Error(err))
		}
	}

	if c.fallback != nil {
		if fallbackStats, err := c.fallback.Stats(ctx); err == nil {
			stats["fallback"] = fallbackStats
		} else {
			c.logger.Warn("Failed to get fallback cache stats", zap.Error(err))
		}
	}

	return stats
}

func (c *cacheService) restoreToPrimary(ctx context.Context, key string, value interface{}) {
	if err := c.setInBackend(ctx, c.primary, key, value, c.config.DefaultTTL); err != nil {
		c.logger.Warn("Failed to restore value to primary cache",
			zap.String("key", key),
			zap.Error(err),
		)
	}
}
