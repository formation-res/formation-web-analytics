package batcher

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/config"
	"github.com/jillesvangurp/formation-web-analytics/internal/elastic"
	"github.com/jillesvangurp/formation-web-analytics/internal/events"
	"github.com/jillesvangurp/formation-web-analytics/internal/metrics"
	"github.com/jillesvangurp/formation-web-analytics/internal/queue"
)

type Batcher struct {
	cfg     config.Config
	queue   *queue.Queue
	sender  elastic.BulkSender
	metrics *metrics.Registry
	logger  *slog.Logger
	ready   atomic.Bool
}

func New(cfg config.Config, q *queue.Queue, sender elastic.BulkSender, registry *metrics.Registry, logger *slog.Logger) *Batcher {
	return &Batcher{
		cfg:     cfg,
		queue:   q,
		sender:  sender,
		metrics: registry,
		logger:  logger,
	}
}

func (b *Batcher) Run(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()
	b.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			b.flush(ctx)
			return
		case <-ticker.C:
			b.flush(ctx)
		case <-b.queue.Notify():
			if b.queue.Len() >= b.cfg.MaxBatchSize {
				b.flush(ctx)
			}
		}
		b.metrics.SetQueueDepth(b.queue.Len())
	}
}

func (b *Batcher) Ready() bool {
	return b.ready.Load()
}

func (b *Batcher) flush(ctx context.Context) {
	remaining := b.queue.Len()
	if remaining == 0 {
		return
	}
	batch := b.queue.Drain(b.cfg.MaxBatchSize)
	if len(batch) == 0 {
		return
	}
	started := time.Now()
	b.metrics.IncFlush()
	b.metrics.ObserveBatchSize(len(batch))
	if err := b.sendWithRetry(ctx, batch); err != nil {
		b.logger.Error("bulk flush failed", "error", err, "batch_size", len(batch))
		b.metrics.IncFlushFailure()
	}
	b.metrics.SetQueueDepth(b.queue.Len())
	b.metrics.ObserveFlushLatency(time.Since(started))
}

func (b *Batcher) sendWithRetry(ctx context.Context, batch []events.Event) error {
	var lastErr error
	for attempt := 0; attempt <= b.cfg.MaxRetries; attempt++ {
		result, err := b.sender.Send(ctx, batch)
		if err == nil && !result.Retryable {
			b.metrics.AddBulkIndexed(result.Indexed)
			b.metrics.AddBulkFailed(result.Failed)
			return nil
		}
		if err == nil && result.Retryable && result.Failed == 0 {
			err = context.DeadlineExceeded
		}
		lastErr = err
		b.metrics.AddBulkIndexed(result.Indexed)
		b.metrics.AddBulkFailed(result.Failed)
		if !result.Retryable || attempt == b.cfg.MaxRetries {
			return lastErr
		}
		b.metrics.IncRetryAttempt()
		delay := elastic.Backoff(b.cfg.RetryMinBackoff, b.cfg.RetryMaxBackoff, attempt)
		b.logger.Warn("retrying bulk flush", "attempt", attempt+1, "delay", delay, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}
