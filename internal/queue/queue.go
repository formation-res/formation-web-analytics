package queue

import (
	"sync"

	"github.com/formation-res/formation-web-analytics/internal/events"
)

type Queue struct {
	mu       sync.Mutex
	items    []events.Event
	head     int
	size     int
	capacity int
	notifyCh chan struct{}
}

func New(capacity int) *Queue {
	return &Queue{
		items:    make([]events.Event, capacity),
		capacity: capacity,
		notifyCh: make(chan struct{}, 1),
	}
}

func (q *Queue) Enqueue(batch []events.Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size+len(batch) > q.capacity {
		return false
	}
	q.append(batch)
	q.notify()
	return true
}

func (q *Queue) DropNewest(batch []events.Event) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	available := q.capacity - q.size
	if available <= 0 {
		return len(batch)
	}
	if len(batch) <= available {
		q.append(batch)
		q.notify()
		return 0
	}
	q.append(batch[:available])
	q.notify()
	return len(batch) - available
}

func (q *Queue) Prepend(batch []events.Event) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size+len(batch) > q.capacity {
		q.grow(q.size + len(batch))
	}
	for i := len(batch) - 1; i >= 0; i-- {
		q.head = (q.head - 1 + q.capacity) % q.capacity
		q.items[q.head] = batch[i]
		q.size++
	}
	q.notify()
	return true
}

func (q *Queue) grow(capacity int) {
	items := make([]events.Event, capacity)
	for i := range q.size {
		items[i] = q.items[(q.head+i)%q.capacity]
	}
	q.items = items
	q.head = 0
	q.capacity = capacity
}

func (q *Queue) Drain(max int) []events.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size == 0 {
		return nil
	}
	if max <= 0 || max > q.size {
		max = q.size
	}
	drained := make([]events.Event, max)
	for i := range max {
		index := (q.head + i) % q.capacity
		drained[i] = q.items[index]
		q.items[index] = events.Event{}
	}
	q.head = (q.head + max) % q.capacity
	q.size -= max
	if q.size == 0 {
		q.head = 0
	}
	return drained
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

func (q *Queue) Notify() <-chan struct{} {
	return q.notifyCh
}

func (q *Queue) append(batch []events.Event) {
	for i := range batch {
		index := (q.head + q.size) % q.capacity
		q.items[index] = batch[i]
		q.size++
	}
}

func (q *Queue) notify() {
	select {
	case q.notifyCh <- struct{}{}:
	default:
	}
}
