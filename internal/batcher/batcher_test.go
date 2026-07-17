package batcher

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/elastic"
	"github.com/formation-res/formation-web-analytics/internal/events"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
	"github.com/formation-res/formation-web-analytics/internal/queue"
)

type stubSender struct {
	send func(context.Context, []events.Event) (elastic.BulkResult, error)
}

func (s stubSender) Send(ctx context.Context, batch []events.Event) (elastic.BulkResult, error) {
	return s.send(ctx, batch)
}

func (s stubSender) Ping(context.Context) error { return nil }

func TestBatcherRetriesRetryableFailures(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{
		FlushInterval:   time.Millisecond,
		MaxBatchSize:    1,
		MaxRetries:      1,
		RetryMinBackoff: time.Millisecond,
		RetryMaxBackoff: time.Millisecond,
	}
	attempts := 0
	retried := make(chan struct{}, 1)
	sender := stubSender{send: func(_ context.Context, batch []events.Event) (elastic.BulkResult, error) {
		attempts++
		if attempts == 1 {
			return elastic.BulkResult{Retry: append([]events.Event(nil), batch...)}, context.DeadlineExceeded
		}
		select {
		case retried <- struct{}{}:
		default:
		}
		return elastic.BulkResult{Indexed: len(batch)}, nil
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if !q.Enqueue([]events.Event{{Type: "page_view", SiteID: "site"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	go b.Run(ctx)

	select {
	case <-retried:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for batch retry: %v", ctx.Err())
	}

	if attempts < 2 {
		t.Fatalf("expected retry, got %d attempts", attempts)
	}
}

func TestBatcherRetriesOnlyRetryableItems(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{
		MaxRetries:      1,
		RetryMinBackoff: time.Nanosecond * 4,
		RetryMaxBackoff: time.Nanosecond * 4,
	}
	attempts := 0
	sender := stubSender{send: func(_ context.Context, batch []events.Event) (elastic.BulkResult, error) {
		attempts++
		if attempts == 1 {
			if len(batch) != 2 {
				t.Fatalf("expected initial batch of 2, got %d", len(batch))
			}
			return elastic.BulkResult{Indexed: 1, Retry: []events.Event{batch[1]}}, nil
		}
		if len(batch) != 1 || batch[0].Type != "click" {
			t.Fatalf("expected only retryable event, got %#v", batch)
		}
		return elastic.BulkResult{Indexed: 1}, nil
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	err := b.sendWithRetry(context.Background(), []events.Event{
		{Type: "page_view", SiteID: "site"},
		{Type: "click", SiteID: "site"},
	})
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestBatcherReportsRetryExhaustion(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{MaxRetries: 0}
	sender := stubSender{send: func(_ context.Context, batch []events.Event) (elastic.BulkResult, error) {
		return elastic.BulkResult{Retry: append([]events.Event(nil), batch...)}, nil
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	if err := b.sendWithRetry(context.Background(), []events.Event{{Type: "page_view", SiteID: "site"}}); err == nil {
		t.Fatal("expected retry exhaustion to return an error")
	}
}

func TestBatcherDrainsAllFullBatchesFromSingleNotification(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{
		FlushInterval:   time.Hour,
		MaxBatchSize:    2,
		MaxRetries:      0,
		RetryMinBackoff: time.Millisecond,
		RetryMaxBackoff: time.Millisecond,
	}
	sent := make(chan int, 3)
	sender := stubSender{send: func(_ context.Context, batch []events.Event) (elastic.BulkResult, error) {
		sent <- len(batch)
		return elastic.BulkResult{Indexed: len(batch)}, nil
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if !q.Enqueue([]events.Event{{Type: "a"}, {Type: "b"}, {Type: "c"}, {Type: "d"}, {Type: "e"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go b.Run(ctx)
	defer func() {
		cancel()
		<-b.Done()
	}()

	for range 2 {
		select {
		case size := <-sent:
			if size != 2 {
				t.Fatalf("expected full batch of 2, got %d", size)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for backlog drain")
		}
	}
	if q.Len() != 1 {
		t.Fatalf("expected only the partial batch to remain, got %d", q.Len())
	}
}

func TestBatcherDrainFlushesPartialBatch(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{MaxBatchSize: 10, MaxRetries: 0}
	sender := stubSender{send: func(_ context.Context, batch []events.Event) (elastic.BulkResult, error) {
		return elastic.BulkResult{Indexed: len(batch)}, nil
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if !q.Enqueue([]events.Event{{Type: "page_view"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	if err := b.Drain(context.Background()); err != nil {
		t.Fatalf("expected drain to succeed: %v", err)
	}
	if q.Len() != 0 {
		t.Fatalf("expected empty queue after drain, got %d", q.Len())
	}
}

func TestBatcherRestoresInterruptedBatch(t *testing.T) {
	registry := metrics.New()
	q := queue.New(10)
	cfg := config.Config{MaxBatchSize: 10, MaxRetries: 0}
	sender := stubSender{send: func(ctx context.Context, batch []events.Event) (elastic.BulkResult, error) {
		return elastic.BulkResult{Indexed: 1, Retry: []events.Event{batch[1]}}, ctx.Err()
	}}
	b := New(cfg, q, sender, registry, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if !q.Enqueue([]events.Event{{Type: "page_view"}, {Type: "click"}}) {
		t.Fatal("expected enqueue to succeed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.flush(ctx); err == nil {
		t.Fatal("expected interrupted flush to fail")
	}
	if q.Len() != 1 {
		t.Fatalf("expected only the pending event to be restored, got %d events", q.Len())
	}
	if restored := q.Drain(1); len(restored) != 1 || restored[0].Type != "click" {
		t.Fatalf("unexpected restored batch: %#v", restored)
	}
}
