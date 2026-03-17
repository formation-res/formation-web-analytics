package queue

import (
	"testing"

	"github.com/formation-res/formation-web-analytics/internal/events"
)

func TestQueueEnqueueAndDrain(t *testing.T) {
	q := New(2)
	if !q.Enqueue([]events.Event{{Type: "page_view", SiteID: "site"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	if q.Len() != 1 {
		t.Fatalf("unexpected length: %d", q.Len())
	}
	drained := q.Drain(1)
	if len(drained) != 1 {
		t.Fatalf("expected 1 drained event")
	}
}

func TestQueueDropNewest(t *testing.T) {
	q := New(1)
	dropped := q.DropNewest([]events.Event{
		{Type: "page_view", SiteID: "site"},
		{Type: "click", SiteID: "site"},
	})
	if dropped != 1 {
		t.Fatalf("expected one event to be dropped, got %d", dropped)
	}
}
