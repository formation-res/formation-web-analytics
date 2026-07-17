package batcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/elastic"
	"github.com/formation-res/formation-web-analytics/internal/events"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
	"github.com/formation-res/formation-web-analytics/internal/queue"
)

type Batcher struct {
	cfg     config.Config
	queue   *queue.Queue
	sender  elastic.BulkSender
	metrics *metrics.Registry
	logger  *slog.Logger
	ready   atomic.Bool
	done    chan struct{}
}

type pendingRetryError struct {
	pending []events.Event
	err     error
}

func (e *pendingRetryError) Error() string { return e.err.Error() }
func (e *pendingRetryError) Unwrap() error { return e.err }

func New(cfg config.Config, q *queue.Queue, sender elastic.BulkSender, registry *metrics.Registry, logger *slog.Logger) *Batcher {
	return &Batcher{
		cfg:     cfg,
		queue:   q,
		sender:  sender,
		metrics: registry,
		logger:  logger,
		done:    make(chan struct{}),
	}
}

func (b *Batcher) Run(ctx context.Context) {
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()
	defer close(b.done)
	defer b.ready.Store(false)
	b.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.flushAll(ctx); err != nil {
				b.logFlushError(err)
			}
		case <-b.queue.Notify():
			if b.queue.Len() >= b.cfg.MaxBatchSize {
				if err := b.flushFullBatches(ctx); err != nil {
					b.logFlushError(err)
				}
			}
		}
		b.metrics.SetQueueDepth(b.queue.Len())
	}
}

func (b *Batcher) Done() <-chan struct{} {
	return b.done
}

func (b *Batcher) Drain(ctx context.Context) error {
	return b.flushAll(ctx)
}

func (b *Batcher) Ready() bool {
	return b.ready.Load()
}

func (b *Batcher) flushFullBatches(ctx context.Context) error {
	for b.queue.Len() >= b.cfg.MaxBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.flush(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *Batcher) flushAll(ctx context.Context) error {
	for b.queue.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.flush(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *Batcher) flush(ctx context.Context) error {
	remaining := b.queue.Len()
	if remaining == 0 {
		return nil
	}
	batch := b.queue.Drain(b.cfg.MaxBatchSize)
	if len(batch) == 0 {
		return nil
	}
	started := time.Now()
	b.metrics.IncFlush()
	b.metrics.ObserveBatchSize(len(batch))
	if err := b.sendWithRetry(ctx, batch); err != nil {
		b.metrics.IncFlushFailure()
		if ctx.Err() != nil {
			restore := batch
			var pendingErr *pendingRetryError
			if errors.As(err, &pendingErr) {
				restore = pendingErr.pending
			}
			b.queue.Prepend(restore)
		}
		b.metrics.SetQueueDepth(b.queue.Len())
		b.metrics.ObserveFlushLatency(time.Since(started))
		return err
	}
	b.metrics.SetQueueDepth(b.queue.Len())
	b.metrics.ObserveFlushLatency(time.Since(started))
	return nil
}

func (b *Batcher) logFlushError(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	b.logger.Error("bulk flush failed", "error", err)
}

func (b *Batcher) sendWithRetry(ctx context.Context, batch []events.Event) error {
	pending := batch
	for attempt := 0; attempt <= b.cfg.MaxRetries; attempt++ {
		result, err := b.sender.Send(ctx, pending)
		b.metrics.AddBulkIndexed(result.Indexed)
		b.metrics.AddBulkFailed(result.Failed)
		if len(result.Retry) == 0 {
			if err != nil && ctx.Err() != nil {
				return &pendingRetryError{pending: pending, err: err}
			}
			return err
		}
		if err == nil {
			err = errors.New("bulk response contained retryable item failures")
		}
		if attempt == b.cfg.MaxRetries {
			if ctx.Err() != nil {
				return &pendingRetryError{pending: result.Retry, err: ctx.Err()}
			}
			b.metrics.AddBulkFailed(len(result.Retry))
			return fmt.Errorf("bulk retries exhausted with %d events pending: %w", len(result.Retry), err)
		}
		pending = result.Retry
		b.metrics.IncRetryAttempt()
		delay := elastic.Backoff(b.cfg.RetryMinBackoff, b.cfg.RetryMaxBackoff, attempt)
		b.logger.Warn("retrying bulk flush", "attempt", attempt+1, "delay", delay, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &pendingRetryError{pending: pending, err: ctx.Err()}
		case <-timer.C:
		}
	}
	return errors.New("bulk retry loop exited unexpectedly")
}
