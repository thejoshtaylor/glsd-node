package security

import (
	"sync"

	"golang.org/x/time/rate"
)

// ProjectRateLimiter implements a per-project token bucket rate limiter.
// Each project gets its own rate.Limiter keyed on a string project name;
// concurrent access is guarded by a mutex.
type ProjectRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	burst    int
}

// NewProjectRateLimiter creates a ProjectRateLimiter that allows requestsPerWindow
// requests per windowSeconds seconds per project.
func NewProjectRateLimiter(requestsPerWindow, windowSeconds int) *ProjectRateLimiter {
	r := rate.Limit(float64(requestsPerWindow) / float64(windowSeconds))
	return &ProjectRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    r,
		burst:    requestsPerWindow,
	}
}

// Allow reports whether the given project is allowed to make a request right now.
// Returns true if the request is allowed, false if the burst is exhausted.
func (p *ProjectRateLimiter) Allow(project string) bool {
	p.mu.Lock()
	l, ok := p.limiters[project]
	if !ok {
		l = rate.NewLimiter(p.limit, p.burst)
		p.limiters[project] = l
	}
	p.mu.Unlock()
	return l.Allow()
}
