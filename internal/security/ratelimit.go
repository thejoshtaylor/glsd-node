package security

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ChannelRateLimiter implements a per-channel token bucket rate limiter.
// Each channel gets its own rate.Limiter; concurrent access is guarded by a mutex.
type ChannelRateLimiter struct {
	mu       sync.Mutex
	limiters map[int64]*rate.Limiter
	limit    rate.Limit
	burst    int
}

// NewChannelRateLimiter creates a ChannelRateLimiter that allows requestsPerWindow
// requests per windowSeconds seconds per channel.
func NewChannelRateLimiter(requestsPerWindow int, windowSeconds int) *ChannelRateLimiter {
	r := rate.Limit(float64(requestsPerWindow) / float64(windowSeconds))
	return &ChannelRateLimiter{
		limiters: make(map[int64]*rate.Limiter),
		limit:    r,
		burst:    requestsPerWindow,
	}
}

// Allow reports whether channelID is allowed to make a request right now.
// Returns (true, 0) if the request is allowed.
// Returns (false, delay) if the request should be delayed by at least delay.
func (crl *ChannelRateLimiter) Allow(channelID int64) (bool, time.Duration) {
	crl.mu.Lock()
	l, ok := crl.limiters[channelID]
	if !ok {
		l = rate.NewLimiter(crl.limit, crl.burst)
		crl.limiters[channelID] = l
	}
	crl.mu.Unlock()

	r := l.Reserve()
	if !r.OK() {
		return false, 0
	}
	delay := r.Delay()
	if delay > 0 {
		r.Cancel()
		return false, delay
	}
	return true, 0
}
