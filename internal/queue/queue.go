package queue

import (
	"sync"

	"github.com/formation-res/formation-web-analytics/internal/events"
)

type Queue struct {
	mu       sync.Mutex
	items    []events.Event
	capacity int
	notifyCh chan struct{}
}

func New(capacity int) *Queue {
	return &Queue{
		items:    make([]events.Event, 0, capacity),
		capacity: capacity,
		notifyCh: make(chan struct{}, 1),
	}
}

func (q *Queue) Enqueue(batch []events.Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items)+len(batch) > q.capacity {
		return false
	}
	q.items = append(q.items, batch...)
	select {
	case q.notifyCh <- struct{}{}:
	default:
	}
	return true
}

func (q *Queue) DropNewest(batch []events.Event) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	available := q.capacity - len(q.items)
	if available <= 0 {
		return len(batch)
	}
	if len(batch) <= available {
		q.items = append(q.items, batch...)
		select {
		case q.notifyCh <- struct{}{}:
		default:
		}
		return 0
	}
	q.items = append(q.items, batch[:available]...)
	select {
	case q.notifyCh <- struct{}{}:
	default:
	}
	return len(batch) - available
}

func (q *Queue) Drain(max int) []events.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	if max <= 0 || max > len(q.items) {
		max = len(q.items)
	}
	drained := make([]events.Event, max)
	copy(drained, q.items[:max])
	q.items = append(q.items[:0], q.items[max:]...)
	return drained
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *Queue) Notify() <-chan struct{} {
	return q.notifyCh
}
