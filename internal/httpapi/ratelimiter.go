package httpapi

import (
	"sync"
	"time"
)

type rateLimiter struct {
	limit       int
	maxClients  int
	mu          sync.Mutex
	clients     map[string]rateLimitState
	windowStart time.Time
}

type rateLimitState struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(limit, maxClients int) *rateLimiter {
	if maxClients <= 0 {
		maxClients = 100000
	}
	return &rateLimiter{
		limit:      limit,
		maxClients: maxClients,
		clients:    map[string]rateLimitState{},
	}
}

func (r *rateLimiter) Allow(clientID string, now time.Time) bool {
	if r == nil || r.limit == 0 || clientID == "" {
		return true
	}
	windowStart := now.UTC().Truncate(time.Minute)

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.windowStart.Equal(windowStart) {
		r.clients = make(map[string]rateLimitState, min(r.maxClients, 1024))
		r.windowStart = windowStart
	}
	state, exists := r.clients[clientID]
	if !state.windowStart.Equal(windowStart) {
		state = rateLimitState{windowStart: windowStart}
	}
	if !exists && len(r.clients) >= r.maxClients {
		return false
	}
	if state.count >= r.limit {
		r.clients[clientID] = state
		return false
	}
	state.count++
	r.clients[clientID] = state
	return true
}
