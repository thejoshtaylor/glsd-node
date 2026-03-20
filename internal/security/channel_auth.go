package security

import (
	"sync"
	"time"
)

type cacheEntry struct {
	authorized bool
	expiresAt  time.Time
}

// ChannelAuthCache caches channel authorization results with a TTL.
// Safe for concurrent use from multiple middleware goroutines.
type ChannelAuthCache struct {
	m   sync.Map
	ttl time.Duration
}

// NewChannelAuthCache creates a cache with the given TTL.
func NewChannelAuthCache(ttl time.Duration) *ChannelAuthCache {
	return &ChannelAuthCache{ttl: ttl}
}

// Lookup returns the cached authorization result for channelID.
// Returns (authorized, hit). If hit is false, the caller must perform
// a fresh admin lookup via GetChatAdministrators.
func (c *ChannelAuthCache) Lookup(channelID int64) (authorized bool, hit bool) {
	v, ok := c.m.Load(channelID)
	if !ok {
		return false, false
	}
	entry := v.(cacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.m.Delete(channelID)
		return false, false
	}
	return entry.authorized, true
}

// Store caches the authorization result for channelID with the configured TTL.
func (c *ChannelAuthCache) Store(channelID int64, authorized bool) {
	c.m.Store(channelID, cacheEntry{
		authorized: authorized,
		expiresAt:  time.Now().Add(c.ttl),
	})
}
