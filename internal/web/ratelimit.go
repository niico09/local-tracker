package web

import (
	"sync"
	"time"
)

// rateLimiter is an in-memory fixed-window counter. It needs no external store
// because the app is a single process.
type rateLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	now     func() time.Time
	entries map[string]*attempts
}

type attempts struct {
	count int
	reset time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, now: time.Now, entries: map[string]*attempts{}}
}

// Allow reports whether the key has budget left in the current window.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || l.now().After(e.reset) {
		return true
	}
	return e.count < l.max
}

// Fail records one failed attempt, starting a new window when needed.
func (l *rateLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || l.now().After(e.reset) {
		e = &attempts{reset: l.now().Add(l.window)}
		l.entries[key] = e
	}
	e.count++
}

// Reset clears the key after a successful login.
func (l *rateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
