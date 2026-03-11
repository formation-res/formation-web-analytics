package batcher

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/config"
	"github.com/jillesvangurp/formation-web-analytics/internal/elastic"
	"github.com/jillesvangurp/formation-web-analytics/internal/events"
	"github.com/jillesvangurp/formation-web-analytics/internal/metrics"
	"github.com/jillesvangurp/formation-web-analytics/internal/queue"
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
			return elastic.BulkResult{Retryable: true}, context.DeadlineExceeded
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
