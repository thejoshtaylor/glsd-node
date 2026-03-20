package security

import (
	"sync"
	"testing"
	"time"
)

// TestRateLimiterAllow verifies that a limiter with burst=5 allows the first 5
// requests and rejects the 6th.
func TestRateLimiterAllow(t *testing.T) {
	// 5 requests per 60 seconds — effectively 5-token burst, slow refill
	crl := NewChannelRateLimiter(5, 60)
	channelID := int64(1)

	for i := 0; i < 5; i++ {
		allowed, delay := crl.Allow(channelID)
		if !allowed {
			t.Fatalf("request %d: expected allowed=true, got allowed=false (delay=%v)", i+1, delay)
		}
		if delay != 0 {
			t.Fatalf("request %d: expected delay=0, got delay=%v", i+1, delay)
		}
	}

	// 6th request must be rejected
	allowed, delay := crl.Allow(channelID)
	if allowed {
		t.Fatal("6th request: expected allowed=false after burst exhausted, got allowed=true")
	}
	if delay <= 0 {
		t.Fatalf("6th request: expected delay>0, got delay=%v", delay)
	}
}

// TestRateLimiterPerChannel verifies that exhausting channel 1 does not affect channel 2.
func TestRateLimiterPerChannel(t *testing.T) {
	crl := NewChannelRateLimiter(3, 60)
	ch1 := int64(1)
	ch2 := int64(2)

	// Exhaust channel 1
	for i := 0; i < 3; i++ {
		allowed, _ := crl.Allow(ch1)
		if !allowed {
			t.Fatalf("ch1 request %d should be allowed", i+1)
		}
	}
	allowed, _ := crl.Allow(ch1)
	if allowed {
		t.Fatal("ch1 4th request should be rejected")
	}

	// Channel 2 must still work
	allowed, delay := crl.Allow(ch2)
	if !allowed {
		t.Fatalf("ch2 first request should be allowed even after ch1 is exhausted (delay=%v)", delay)
	}
}

// TestRateLimiterConcurrent verifies that concurrent Allow calls do not cause
// data races or panics. Run with go test -race.
func TestRateLimiterConcurrent(t *testing.T) {
	crl := NewChannelRateLimiter(100, 1) // high burst so goroutines don't all block
	channelID := int64(42)

	var wg sync.WaitGroup
	start := make(chan struct{})

	const numGoroutines = 10
	results := make([]bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			<-start // synchronize start
			allowed, _ := crl.Allow(channelID)
			results[idx] = allowed
		}()
	}

	// Release all goroutines simultaneously
	close(start)
	wg.Wait()

	// Verify no panic occurred (reaching here means no panic)
	// At least some requests should have been allowed (burst=100)
	allowedCount := 0
	for _, r := range results {
		if r {
			allowedCount++
		}
	}
	if allowedCount == 0 {
		t.Fatal("expected at least one allowed request in concurrent test")
	}

	// Verify timer duration is sensible when blocked
	// Create a tight limiter to verify delay is > 0
	tight := NewChannelRateLimiter(1, 60)
	tight.Allow(channelID) // consume the one token
	ok, delay := tight.Allow(channelID)
	if ok {
		t.Fatal("tight limiter: second request should be rejected")
	}
	if delay <= 0 {
		t.Fatalf("tight limiter: expected positive delay, got %v", delay)
	}
	if delay > 2*time.Minute {
		t.Fatalf("tight limiter: delay %v seems unreasonably large", delay)
	}
}
