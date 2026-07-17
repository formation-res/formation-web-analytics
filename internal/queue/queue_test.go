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

func TestQueuePreservesOrderAcrossWraparound(t *testing.T) {
	q := New(4)
	if !q.Enqueue([]events.Event{{Type: "a"}, {Type: "b"}, {Type: "c"}}) {
		t.Fatal("expected initial enqueue to succeed")
	}
	if drained := q.Drain(2); len(drained) != 2 || drained[0].Type != "a" || drained[1].Type != "b" {
		t.Fatalf("unexpected initial drain: %#v", drained)
	}
	if !q.Enqueue([]events.Event{{Type: "d"}, {Type: "e"}, {Type: "f"}}) {
		t.Fatal("expected wrapped enqueue to succeed")
	}
	drained := q.Drain(0)
	want := []string{"c", "d", "e", "f"}
	for i := range want {
		if drained[i].Type != want[i] {
			t.Fatalf("unexpected wrapped order at %d: got %q want %q", i, drained[i].Type, want[i])
		}
	}
}

func TestQueuePrependRestoresBatchOrder(t *testing.T) {
	q := New(4)
	if !q.Enqueue([]events.Event{{Type: "c"}, {Type: "d"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	if !q.Prepend([]events.Event{{Type: "a"}, {Type: "b"}}) {
		t.Fatal("expected prepend to succeed")
	}
	drained := q.Drain(0)
	want := []string{"a", "b", "c", "d"}
	for i := range want {
		if drained[i].Type != want[i] {
			t.Fatalf("unexpected restored order at %d: got %q want %q", i, drained[i].Type, want[i])
		}
	}
}

func TestQueuePrependCanRestoreIntoAFullQueue(t *testing.T) {
	q := New(2)
	if !q.Enqueue([]events.Event{{Type: "new-a"}, {Type: "new-b"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	if !q.Prepend([]events.Event{{Type: "interrupted"}}) {
		t.Fatal("expected interrupted batch to be restored")
	}
	drained := q.Drain(0)
	if len(drained) != 3 || drained[0].Type != "interrupted" || drained[1].Type != "new-a" || drained[2].Type != "new-b" {
		t.Fatalf("unexpected restored queue: %#v", drained)
	}
}
