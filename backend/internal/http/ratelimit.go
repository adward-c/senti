package http

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	buckets map[string]rateBucket
	now     func() time.Time
}

type rateBucket struct {
	start time.Time
	count int
}

func newRateLimiter(window time.Duration, limit int) *rateLimiter {
	return &rateLimiter{
		window:  window,
		limit:   limit,
		buckets: make(map[string]rateBucket),
		now:     time.Now,
	}
}

func (r *rateLimiter) Allow(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	bucket := r.buckets[userID]
	if bucket.start.IsZero() || now.Sub(bucket.start) >= r.window {
		r.buckets[userID] = rateBucket{start: now, count: 1}
		return true
	}
	if bucket.count >= r.limit {
		return false
	}
	bucket.count++
	r.buckets[userID] = bucket
	return true
}
