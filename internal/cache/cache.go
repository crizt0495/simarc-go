package cache

import (
	"sync"
	"time"
)

// Cache is a thread-safe in-memory cache with TTL and size cap.
// It is intentionally simple (no external dependency) so it remains
// portable to serverless and container deployments.
//
// For very large datasets, swap this with Redis/Memcached by
// implementing the same Get/Set/Delete interface.
type Cache struct {
	mu         sync.RWMutex
	items      map[string]entry
	defaultTTL time.Duration
	maxItems   int
	stopCh     chan struct{}
	stats      Stats
}

type entry struct {
	value     any
	expiresAt time.Time
}

type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Sets      int64
}

// Default cache instance used by the application.
var Default = New(5 * time.Minute, 5000)

// New creates a new cache with the given default TTL and max item count.
func New(defaultTTL time.Duration, maxItems int) *Cache {
	c := &Cache{
		items:      make(map[string]entry),
		defaultTTL: defaultTTL,
		maxItems:   maxItems,
		stopCh:     make(chan struct{}),
	}
	// Background sweeper: removes expired entries every minute to
	// prevent unbounded growth even when maxItems is not reached.
	go c.sweepLoop()
	return c
}

// Get retrieves a value. Returns (nil, false) on miss or expiry.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}
	c.mu.Lock()
	c.stats.Hits++
	c.mu.Unlock()
	return e.value, true
}

// Set stores a value with the cache's default TTL.
func (c *Cache) Set(key string, value any) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value with a custom TTL.
// A TTL of 0 or negative means "never expire" (use sparingly).
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Enforce maxItems by evicting the oldest entry if we're full.
	if _, exists := c.items[key]; !exists && len(c.items) >= c.maxItems {
		c.evictOldest()
	}
	c.items[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.stats.Sets++
}

// Delete removes a key.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// DeletePrefix removes all keys starting with the given prefix.
// Used to invalidate related entries (e.g. "dashboard:*").
func (c *Cache) DeletePrefix(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k := range c.items {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.items, k)
			n++
		}
	}
	return n
}

// Clear empties the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]entry)
	c.mu.Unlock()
}

// GetStats returns cache hit/miss counters for the monitoring page.
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Len returns the current number of entries (including non-expired).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stop terminates the background sweeper.
func (c *Cache) Stop() {
	close(c.stopCh)
}

func (c *Cache) sweepLoop() {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case now := <-t.C:
			c.mu.Lock()
			for k, e := range c.items {
				if now.After(e.expiresAt) {
					delete(c.items, k)
					c.stats.Evictions++
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *Cache) evictOldest() {
	// Find the entry with the earliest expiresAt and remove it.
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, e := range c.items {
		if first || e.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.expiresAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
		c.stats.Evictions++
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// GetOrSet returns the cached value for `key` or, on miss, calls `loader`
// to compute it, stores the result with the default TTL, and returns it.
//
// Loader errors are returned to the caller; nothing is cached on error so
// the next request can retry cleanly.
func GetOrSet(key string, loader func() (any, error)) (any, error) {
	if v, ok := Default.Get(key); ok {
		return v, nil
	}
	v, err := loader()
	if err != nil {
		return nil, err
	}
	Default.Set(key, v)
	return v, nil
}

// Invalidate removes a single key from the default cache.
func Invalidate(key string) {
	Default.Delete(key)
}

// InvalidatePrefix removes all keys with the given prefix.
func InvalidatePrefix(prefix string) int {
	return Default.DeletePrefix(prefix)
}
