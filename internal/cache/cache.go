// internal/cache/cache.go
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ===============================
// CACHE INTERFACE
// ===============================

// Cache defines the caching interface
type Cache interface {
	// Basic operations
	Get(ctx context.Context, key string) (interface{}, bool)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) bool

	// Advanced operations
	GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error)
	SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error
	DeleteMultiple(ctx context.Context, keys []string) error
	DeletePattern(ctx context.Context, pattern string) error

	// TTL operations
	SetTTL(ctx context.Context, key string, ttl time.Duration) error
	GetTTL(ctx context.Context, key string) (time.Duration, error)

	// Atomic operations
	Increment(ctx context.Context, key string, delta int64) (int64, error)
	Decrement(ctx context.Context, key string, delta int64) (int64, error)

	// Cache management
	Clear(ctx context.Context) error
	Stats(ctx context.Context) (*CacheStats, error)
	Health(ctx context.Context) error
	Close() error
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits           int64         `json:"hits"`
	Misses         int64         `json:"misses"`
	Sets           int64         `json:"sets"`
	Deletes        int64         `json:"deletes"`
	Keys           int64         `json:"keys"`
	Memory         int64         `json:"memory_bytes"`
	HitRatio       float64       `json:"hit_ratio"`
	Uptime         time.Duration `json:"uptime"`
	UsedMemory     int64         `json:"used_memory"`
	MaxMemory      int64         `json:"max_memory"`
	ConnectedClients int64       `json:"connected_clients"`
	TotalCommands   int64       `json:"total_commands"`
	ExpiredKeys    int64         `json:"expired_keys"`
	EvictedKeys    int64         `json:"evicted_keys"`
}

// ===============================
// CACHE CONFIGURATION
// ===============================

// Config holds cache configuration
type Config struct {
	Provider        string        `json:"provider" yaml:"provider"`                 // "memory", "redis"
	TTL             time.Duration `json:"ttl" yaml:"ttl"`                           // Default TTL
	MaxKeys         int           `json:"max_keys" yaml:"max_keys"`                 // Max keys in memory cache
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"` // Cleanup interval for memory cache

	// Redis configuration
	RedisURL      string `json:"redis_url" yaml:"redis_url"`
	RedisDB       int    `json:"redis_db" yaml:"redis_db"`
	RedisPassword string `json:"redis_password" yaml:"redis_password"`
	PoolSize      int    `json:"pool_size" yaml:"pool_size"`

	// Performance tuning
	Serialization string `json:"serialization" yaml:"serialization"` // "json", "gob", "msgpack"
	Compression   bool   `json:"compression" yaml:"compression"`

	// Monitoring
	EnableMetrics bool   `json:"enable_metrics" yaml:"enable_metrics"`
	MetricsPrefix string `json:"metrics_prefix" yaml:"metrics_prefix"`
}

// DefaultConfig returns a default cache configuration
func DefaultConfig() *Config {
	return &Config{
		Provider:        "memory",
		TTL:             15 * time.Minute,
		MaxKeys:         10000,
		CleanupInterval: 5 * time.Minute,
		PoolSize:        10,
		Serialization:   "json",
		Compression:     false,
		EnableMetrics:   true,
		MetricsPrefix:   "cache",
	}
}

// ===============================
// MEMORY CACHE IMPLEMENTATION
// ===============================

// memoryCache implements Cache using in-memory storage
type memoryCache struct {
	mu              sync.RWMutex
	items           map[string]*cacheItem
	maxKeys         int
	cleanupInterval time.Duration
	logger          *zap.Logger
	stats           *CacheStats
	startTime       time.Time
	stopCh          chan struct{}
}

// cacheItem represents a cached item
type cacheItem struct {
	Value       interface{}
	ExpiresAt   time.Time
	CreatedAt   time.Time
	AccessedAt  time.Time
	AccessCount int64
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(config *Config, logger *zap.Logger) Cache {
	cache := &memoryCache{
		items:           make(map[string]*cacheItem),
		maxKeys:         config.MaxKeys,
		cleanupInterval: config.CleanupInterval,
		logger:          logger,
		stats:           &CacheStats{},
		startTime:       time.Now(),
		stopCh:          make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (c *memoryCache) Get(ctx context.Context, key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		c.stats.Misses++
		return nil, false
	}

	// Check expiration
	if time.Now().After(item.ExpiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		c.mu.RLock()
		c.stats.Misses++
		return nil, false
	}

	// Update access statistics
	item.AccessedAt = time.Now()
	item.AccessCount++
	c.stats.Hits++

	return item.Value, true
}

// Set stores a value in the cache
func (c *memoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict items
	if len(c.items) >= c.maxKeys {
		c.evictLRU()
	}

	now := time.Now()
	c.items[key] = &cacheItem{
		Value:       value,
		ExpiresAt:   now.Add(ttl),
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 0,
	}

	c.stats.Sets++
	c.stats.Keys = int64(len(c.items))

	return nil
}

// Delete removes a value from the cache
func (c *memoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; exists {
		delete(c.items, key)
		c.stats.Deletes++
		c.stats.Keys = int64(len(c.items))
	}

	return nil
}

// Exists checks if a key exists in the cache
func (c *memoryCache) Exists(ctx context.Context, key string) bool {
	_, found := c.Get(ctx, key)
	return found
}

// GetMultiple retrieves multiple values from the cache
func (c *memoryCache) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, key := range keys {
		if value, found := c.Get(ctx, key); found {
			result[key] = value
		}
	}

	return result, nil
}

// SetMultiple stores multiple values in the cache
func (c *memoryCache) SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	for key, value := range items {
		if err := c.Set(ctx, key, value, ttl); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMultiple removes multiple values from the cache
func (c *memoryCache) DeleteMultiple(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := c.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// DeletePattern removes all keys matching a pattern
func (c *memoryCache) DeletePattern(ctx context.Context, pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToDelete []string

	// Simple pattern matching with wildcards
	for key := range c.items {
		if matchPattern(key, pattern) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.items, key)
		c.stats.Deletes++
	}

	c.stats.Keys = int64(len(c.items))

	return nil
}

// SetTTL updates the TTL for a key
func (c *memoryCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		item.ExpiresAt = time.Now().Add(ttl)
	}

	return nil
}

// GetTTL returns the TTL for a key
func (c *memoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if item, exists := c.items[key]; exists {
		remaining := time.Until(item.ExpiresAt)
		if remaining > 0 {
			return remaining, nil
		}
	}

	return 0, fmt.Errorf("key not found or expired")
}

// Increment atomically increments a numeric value
func (c *memoryCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		// Create new item with delta value
		now := time.Now()
		c.items[key] = &cacheItem{
			Value:       delta,
			ExpiresAt:   now.Add(24 * time.Hour), // Default TTL for counters
			CreatedAt:   now,
			AccessedAt:  now,
			AccessCount: 0,
		}
		return delta, nil
	}

	// Check if value is numeric
	switch v := item.Value.(type) {
	case int64:
		newValue := v + delta
		item.Value = newValue
		item.AccessedAt = time.Now()
		item.AccessCount++
		return newValue, nil
	case int:
		newValue := int64(v) + delta
		item.Value = newValue
		item.AccessedAt = time.Now()
		item.AccessCount++
		return newValue, nil
	default:
		return 0, fmt.Errorf("value is not numeric")
	}
}

// Decrement atomically decrements a numeric value
func (c *memoryCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return c.Increment(ctx, key, -delta)
}

// Clear removes all items from the cache
func (c *memoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.stats.Keys = 0

	return nil
}

// Stats returns cache statistics
func (c *memoryCache) Stats(ctx context.Context) (*CacheStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := *c.stats // Copy stats
	stats.Keys = int64(len(c.items))
	stats.Uptime = time.Since(c.startTime)

	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRatio = float64(stats.Hits) / float64(total)
	}

	// Calculate memory usage (approximate)
	stats.Memory = int64(len(c.items)) * 100 // Rough estimate

	return &stats, nil
}

// Health checks cache health
func (c *memoryCache) Health(ctx context.Context) error {
	// Simple health check - try to set and get a value
	testKey := "__health_check__"
	testValue := time.Now().Unix()

	if err := c.Set(ctx, testKey, testValue, time.Minute); err != nil {
		return fmt.Errorf("cache health check failed: unable to set value: %w", err)
	}

	if value, found := c.Get(ctx, testKey); !found {
		return fmt.Errorf("cache health check failed: unable to get value")
	} else if value != testValue {
		return fmt.Errorf("cache health check failed: value mismatch")
	}

	// Clean up test key
	c.Delete(ctx, testKey)

	return nil
}

// Close closes the cache and cleanup resources
func (c *memoryCache) Close() error {
	close(c.stopCh)
	return nil
}

// cleanup runs periodic cleanup of expired items
func (c *memoryCache) cleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopCh:
			return
		}
	}
}

// cleanupExpired removes expired items
func (c *memoryCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, item := range c.items {
		if now.After(item.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(c.items, key)
	}

	if len(expiredKeys) > 0 {
		c.logger.Debug("Cleaned up expired cache items",
			zap.Int("expired_count", len(expiredKeys)),
			zap.Int("remaining_count", len(c.items)),
		)
	}

	c.stats.Keys = int64(len(c.items))
}

// evictLRU evicts the least recently used item
func (c *memoryCache) evictLRU() {
	if len(c.items) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.AccessedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.AccessedAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// ===============================
// UTILITY FUNCTIONS
// ===============================

// matchPattern performs simple wildcard pattern matching
func matchPattern(str, pattern string) bool {
	// Handle simple wildcard pattern (*)
	if pattern == "*" {
		return true
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(str, prefix)
	}

	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(str, suffix)
	}

	// Exact match
	return str == pattern
}

// ===============================
// FACTORY FUNCTION
// ===============================

// NewCache creates a new cache instance based on configuration
func NewCache(config *Config, logger *zap.Logger) (Cache, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	switch strings.ToLower(config.Provider) {
	case "redis":
		return NewRedisCache(config, logger)
	case "memory", "":
		logger.Info("Using in-memory cache")
		return NewMemoryCache(config, logger), nil
	default:
		return nil, fmt.Errorf("unsupported cache provider: %s", config.Provider)
	}
}

// ===============================
// REDIS CACHE IMPLEMENTATION
// ===============================

type redisCache struct {
	client *redis.Client
	logger *zap.Logger
	config *Config
}

// NewRedisCache creates a new Redis-based cache
func NewRedisCache(config *Config, logger *zap.Logger) (Cache, error) {
	if config == nil {
		return nil, fmt.Errorf("cache config cannot be nil")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Parse Redis URL if provided
	var options *redis.Options
	if config.RedisURL != "" {
		var err error
		options, err = redis.ParseURL(config.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
		}
	} else {
		options = &redis.Options{
			Addr:     "localhost:6379",
			Password: config.RedisPassword,
			DB:       config.RedisDB,
		}
	}

	// Set pool size if configured
	if config.PoolSize > 0 {
		options.PoolSize = config.PoolSize
	}

	client := redis.NewClient(options)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	cache := &redisCache{
		client: client,
		logger: logger,
		config: config,
	}

	logger.Info("Redis cache initialized",
		zap.String("addr", options.Addr),
		zap.Int("db", options.DB),
	)

	return cache, nil
}

func (r *redisCache) Get(ctx context.Context, key string) (interface{}, bool) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, false
	} else if err != nil {
		r.logger.Error("Failed to get from Redis",
			zap.String("key", key),
			zap.Error(err))
		return nil, false
	}

	// Try to unmarshal JSON first (for complex types)
	var result interface{}
	if err := json.Unmarshal([]byte(val), &result); err == nil {
		return result, true
	}

	// If not JSON, return as string
	return val, true
}

func (r *redisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	var val string
	switch v := value.(type) {
	case string:
		val = v
	case []byte:
		val = string(v)
	default:
		// Try to marshal to JSON for complex types
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
		val = string(data)
	}

	if ttl <= 0 {
		ttl = r.config.TTL
	}

	return r.client.Set(ctx, key, val, ttl).Err()
}

func (r *redisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *redisCache) Exists(ctx context.Context, key string) bool {
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		r.logger.Error("Failed to check key existence",
			zap.String("key", key),
			zap.Error(err))
		return false
	}
	return exists > 0
}

func (r *redisCache) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return map[string]interface{}{}, nil
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{}, len(keys))
	for i, key := range keys {
		if vals[i] != nil {
			// Try to unmarshal JSON first
			var val interface{}
			if str, ok := vals[i].(string); ok {
				if err := json.Unmarshal([]byte(str), &val); err != nil {
					val = str // Use as string if not JSON
				}
			} else {
				val = vals[i]
			}
			result[key] = val
		}
	}

	return result, nil
}

func (r *redisCache) SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for key, value := range items {
		var val string
		switch v := value.(type) {
		case string:
			val = v
		case []byte:
			val = string(v)
		default:
			data, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
			}
			val = string(data)
		}

		if ttl <= 0 {
			ttl = r.config.TTL
		}

		pipe.Set(ctx, key, val, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisCache) DeleteMultiple(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

func (r *redisCache) DeletePattern(ctx context.Context, pattern string) error {
	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		// Delete in batches to avoid blocking Redis for too long
		if len(keys) >= 1000 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}

	if err := iter.Err(); err != nil {
		return err
	}

	// Delete any remaining keys
	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}

	return nil
}

func (r *redisCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = r.config.TTL
	}
	return r.client.Expire(ctx, key, ttl).Err()
}

func (r *redisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	dur, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return dur, nil
}

func (r *redisCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	return r.client.IncrBy(ctx, key, delta).Result()
}

func (r *redisCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return r.client.DecrBy(ctx, key, delta).Result()
}

func (r *redisCache) Clear(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

func (r *redisCache) Stats(ctx context.Context) (*CacheStats, error) {
	stats := &CacheStats{}

	// Get Redis info for various sections
	infoSections := []string{"stats", "memory", "clients", "commandstats"}
	
	for _, section := range infoSections {
		info, err := r.client.Info(ctx, section).Result()
		if err != nil {
			r.logger.Warn("Failed to get Redis info", 
				zap.String("section", section), 
				zap.Error(err))
			continue
		}

		lines := strings.Split(info, "\r\n")
		for _, line := range lines {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			// Memory stats
			case "used_memory":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.UsedMemory = v
				}
			case "maxmemory":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.MaxMemory = v
				}

			// Client stats
			case "connected_clients":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.ConnectedClients = v
				}

			// Stats
			case "total_commands_processed":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.TotalCommands = v
				}
			case "expired_keys":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.ExpiredKeys = v
				}
			case "evicted_keys":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.EvictedKeys = v
				}
			case "keyspace_hits":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.Hits = v
				}
			case "keyspace_misses":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.Misses = v
				}
			}
		}
	}

	// Get total keys
	keys, err := r.client.DBSize(ctx).Result()
	if err == nil {
		stats.Keys = keys
	}

	// Calculate hit ratio if we have hits and/or misses
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRatio = float64(stats.Hits) / float64(total)
	}

	return stats, nil
}

func (r *redisCache) Health(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	return err
}

func (r *redisCache) Close() error {
	return r.client.Close()
}

// ===============================
// CACHE MIDDLEWARE
// ===============================

// CacheMiddleware provides caching middleware for services
type CacheMiddleware struct {
	cache  Cache
	logger *zap.Logger
}

// NewCacheMiddleware creates cache middleware
func NewCacheMiddleware(cache Cache, logger *zap.Logger) *CacheMiddleware {
	return &CacheMiddleware{
		cache:  cache,
		logger: logger,
	}
}

// CacheResult caches the result of a function call
func (cm *CacheMiddleware) CacheResult(ctx context.Context, key string, ttl time.Duration, fn func() (interface{}, error)) (interface{}, error) {
	// Try cache first
	if value, found := cm.cache.Get(ctx, key); found {
		cm.logger.Debug("Cache hit", zap.String("key", key))
		return value, nil
	}

	// Execute function
	result, err := fn()
	if err != nil {
		return nil, err
	}

	// Cache the result
	if cacheErr := cm.cache.Set(ctx, key, result, ttl); cacheErr != nil {
		cm.logger.Warn("Failed to cache result",
			zap.String("key", key),
			zap.Error(cacheErr),
		)
	} else {
		cm.logger.Debug("Cache set", zap.String("key", key))
	}

	return result, nil
}
