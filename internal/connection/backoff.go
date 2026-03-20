package connection

import (
	"math/rand"
	"time"
)

// backoffState tracks exponential backoff state with full jitter.
// The algorithm is the AWS "Full Jitter" approach:
// sleep = random_between(0, min(cap, base * 2^attempt))
type backoffState struct {
	current time.Duration
	min     time.Duration
	max     time.Duration
}

// newBackoff returns a backoffState starting at min, capping at max.
func newBackoff(min, max time.Duration) *backoffState {
	return &backoffState{
		current: min,
		min:     min,
		max:     max,
	}
}

// Next returns a jittered delay in [0, current] and doubles current (up to max).
func (b *backoffState) Next() time.Duration {
	// Full jitter: random value in [0, current].
	jittered := time.Duration(rand.Int63n(int64(b.current) + 1))
	// Double current for next call, capped at max.
	b.current = b.current * 2
	if b.current > b.max {
		b.current = b.max
	}
	return jittered
}

// Reset sets current back to min.
func (b *backoffState) Reset() {
	b.current = b.min
}
