package httpapi

import (
	"sync"
	"time"
)

type rateLimiter struct {
	limit   int
	mu      sync.Mutex
	clients map[string]rateLimitState
}

type rateLimitState struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		clients: map[string]rateLimitState{},
	}
}

func (r *rateLimiter) Allow(clientID string, now time.Time) bool {
	if r == nil || r.limit == 0 || clientID == "" {
		return true
	}
	windowStart := now.UTC().Truncate(time.Minute)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.prune(windowStart)
	state := r.clients[clientID]
	if !state.windowStart.Equal(windowStart) {
		state = rateLimitState{windowStart: windowStart}
	}
	if state.count >= r.limit {
		r.clients[clientID] = state
		return false
	}
	state.count++
	r.clients[clientID] = state
	return true
}

func (r *rateLimiter) prune(activeWindow time.Time) {
	cutoff := activeWindow.Add(-2 * time.Minute)
	for clientID, state := range r.clients {
		if state.windowStart.Before(cutoff) {
			delete(r.clients, clientID)
		}
	}
}
