// Package ratelimit provides a fixed-window per-key counter.
//
// Fixed window rather than a token bucket: the thing being protected is a
// cheap endpoint where the goal is to stop one host opening thousands of
// sessions, not to shape a smooth rate. A burst of `limit` at a window
// boundary is acceptable for that.
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	count int
	start time.Time
}

type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	limit   int
	period  time.Duration
}

// New returns a limiter allowing limit events per key per period.
func New(limit int, period time.Duration) *Limiter {
	l := &Limiter{windows: make(map[string]*window), limit: limit, period: period}
	go l.reap()
	return l
}

// Allow records an event for key and reports whether it is within the limit.
// An empty key is always allowed: it means the caller could not determine a
// client identity, and refusing everyone in that case would be a worse
// failure than allowing them.
func (l *Limiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) >= l.period {
		l.windows[key] = &window{count: 1, start: now}
		return true
	}

	w.count++
	return w.count <= l.limit
}

func (l *Limiter) reap() {
	for range time.Tick(time.Minute) {
		cutoff := time.Now().Add(-l.period)
		l.mu.Lock()
		for k, w := range l.windows {
			if w.start.Before(cutoff) {
				delete(l.windows, k)
			}
		}
		l.mu.Unlock()
	}
}
