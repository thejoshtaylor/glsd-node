package security

import (
	"testing"
	"time"
)

// TestChannelAuthCacheMiss verifies that Lookup on an empty cache returns (false, false).
func TestChannelAuthCacheMiss(t *testing.T) {
	cache := NewChannelAuthCache(1 * time.Minute)
	authorized, hit := cache.Lookup(12345)
	if hit {
		t.Fatal("expected cache miss on empty cache, got hit=true")
	}
	if authorized {
		t.Fatal("expected authorized=false on cache miss")
	}
}

// TestChannelAuthCacheHit verifies that Store(channelID, true) followed by Lookup
// returns (true, true).
func TestChannelAuthCacheHit(t *testing.T) {
	cache := NewChannelAuthCache(1 * time.Minute)
	cache.Store(12345, true)
	authorized, hit := cache.Lookup(12345)
	if !hit {
		t.Fatal("expected cache hit after Store, got hit=false")
	}
	if !authorized {
		t.Fatal("expected authorized=true after Store(true)")
	}
}

// TestChannelAuthCacheHitUnauthorized verifies that Store(channelID, false) followed
// by Lookup returns (false, true) — hit is true, but authorized is false.
func TestChannelAuthCacheHitUnauthorized(t *testing.T) {
	cache := NewChannelAuthCache(1 * time.Minute)
	cache.Store(12345, false)
	authorized, hit := cache.Lookup(12345)
	if !hit {
		t.Fatal("expected cache hit after Store(false), got hit=false")
	}
	if authorized {
		t.Fatal("expected authorized=false after Store(false)")
	}
}

// TestChannelAuthCacheExpiry verifies that after the TTL expires, Lookup returns
// (false, false) — treating the expired entry as a cache miss.
func TestChannelAuthCacheExpiry(t *testing.T) {
	cache := NewChannelAuthCache(50 * time.Millisecond)
	cache.Store(12345, true)

	// Verify it's a hit before expiry.
	_, hit := cache.Lookup(12345)
	if !hit {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	authorized, hit := cache.Lookup(12345)
	if hit {
		t.Fatal("expected cache miss after TTL expiry, got hit=true")
	}
	if authorized {
		t.Fatal("expected authorized=false after TTL expiry")
	}
}

// TestChannelAuthCacheDifferentChannels verifies that different channel IDs
// have independent cache entries.
func TestChannelAuthCacheDifferentChannels(t *testing.T) {
	cache := NewChannelAuthCache(1 * time.Minute)
	cache.Store(111, true)
	cache.Store(222, false)

	auth1, hit1 := cache.Lookup(111)
	if !hit1 {
		t.Fatal("expected hit for channel 111")
	}
	if !auth1 {
		t.Fatal("expected authorized=true for channel 111")
	}

	auth2, hit2 := cache.Lookup(222)
	if !hit2 {
		t.Fatal("expected hit for channel 222")
	}
	if auth2 {
		t.Fatal("expected authorized=false for channel 222")
	}

	// A third channel that was never stored should be a miss.
	_, hit3 := cache.Lookup(333)
	if hit3 {
		t.Fatal("expected miss for channel 333 (never stored)")
	}
}
