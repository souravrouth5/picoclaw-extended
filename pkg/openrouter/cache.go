package openrouter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

const defaultCacheTTL = 5 * time.Minute

type cacheEntry struct {
	response  *providers.LLMResponse
	expiresAt time.Time
}

// ResponseCache is a simple in-memory TTL cache for LLM responses.
// Keyed by a hash of (modelID + serialised messages).
type ResponseCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// NewResponseCache creates a cache with the given TTL (0 = use default 5 min).
func NewResponseCache(ttl time.Duration) *ResponseCache {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	c := &ResponseCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
	go c.evictLoop()
	return c
}

// Get returns a cached response and true, or nil and false on miss/expiry.
func (c *ResponseCache) Get(modelID string, messages []providers.Message) (*providers.LLMResponse, bool) {
	key := cacheKey(modelID, messages)
	c.mu.RLock()
	e := c.entries[key]
	c.mu.RUnlock()
	if e == nil || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.response, true
}

// Set stores a response in the cache.
func (c *ResponseCache) Set(modelID string, messages []providers.Message, resp *providers.LLMResponse) {
	key := cacheKey(modelID, messages)
	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		response:  resp,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate removes all entries for a model (e.g. when it becomes paid).
func (c *ResponseCache) Invalidate(modelID string) {
	prefix := modelID + ":"
	c.mu.Lock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

func (c *ResponseCache) evictLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

func cacheKey(modelID string, messages []providers.Message) string {
	b, _ := json.Marshal(messages)
	h := sha256.Sum256(append([]byte(modelID+":"), b...))
	return fmt.Sprintf("%s:%x", modelID, h[:8])
}
