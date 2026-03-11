package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type bucketCounter struct {
	upper float64
	count atomic.Uint64
}

type histogram struct {
	sum   atomic.Uint64
	count atomic.Uint64
	items []bucketCounter
}

func newHistogram(bounds []float64) histogram {
	items := make([]bucketCounter, 0, len(bounds))
	for _, bound := range bounds {
		items = append(items, bucketCounter{upper: bound})
	}
	return histogram{items: items}
}

func (h *histogram) observe(value float64) {
	h.count.Add(1)
	h.sum.Add(uint64(value * 1000))
	for i := range h.items {
		if value <= h.items[i].upper {
			h.items[i].count.Add(1)
		}
	}
}

type Registry struct {
	startedAt           time.Time
	acceptedEvents      atomic.Uint64
	rejectedEvents      atomic.Uint64
	droppedEvents       atomic.Uint64
	flushes             atomic.Uint64
	flushFailures       atomic.Uint64
	retryAttempts       atomic.Uint64
	bulkIndexed         atomic.Uint64
	bulkFailed          atomic.Uint64
	queueFull           atomic.Uint64
	queuedCurrent       atomic.Int64
	mutex               sync.Mutex
	batchSizeHistogram  histogram
	flushLatencySeconds histogram
}

func New() *Registry {
	return &Registry{
		startedAt:           time.Now(),
		batchSizeHistogram:  newHistogram([]float64{1, 10, 50, 100, 250, 500, 1000, 5000}),
		flushLatencySeconds: newHistogram([]float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}),
	}
}

func (r *Registry) IncAccepted(n int)      { r.acceptedEvents.Add(uint64(n)) }
func (r *Registry) IncRejected(n int)      { r.rejectedEvents.Add(uint64(n)) }
func (r *Registry) IncDropped(n int)       { r.droppedEvents.Add(uint64(n)) }
func (r *Registry) IncFlush()              { r.flushes.Add(1) }
func (r *Registry) IncFlushFailure()       { r.flushFailures.Add(1) }
func (r *Registry) IncRetryAttempt()       { r.retryAttempts.Add(1) }
func (r *Registry) AddBulkIndexed(n int)   { r.bulkIndexed.Add(uint64(n)) }
func (r *Registry) AddBulkFailed(n int)    { r.bulkFailed.Add(uint64(n)) }
func (r *Registry) IncQueueFull()          { r.queueFull.Add(1) }
func (r *Registry) SetQueueDepth(n int)    { r.queuedCurrent.Store(int64(n)) }
func (r *Registry) ObserveBatchSize(n int) { r.batchSizeHistogram.observe(float64(n)) }
func (r *Registry) ObserveFlushLatency(d time.Duration) {
	r.flushLatencySeconds.observe(d.Seconds())
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(r.render()))
	})
}

func (r *Registry) render() string {
	var b strings.Builder
	writeCounter := func(name string, value uint64) {
		fmt.Fprintf(&b, "# TYPE %s counter\n%s %d\n", name, name, value)
	}
	writeGauge := func(name string, value int64) {
		fmt.Fprintf(&b, "# TYPE %s gauge\n%s %d\n", name, name, value)
	}
	writeCounter("analytics_accepted_events_total", r.acceptedEvents.Load())
	writeCounter("analytics_rejected_events_total", r.rejectedEvents.Load())
	writeCounter("analytics_dropped_events_total", r.droppedEvents.Load())
	writeGauge("analytics_queued_events_current", r.queuedCurrent.Load())
	writeCounter("analytics_flushes_total", r.flushes.Load())
	writeCounter("analytics_flush_failures_total", r.flushFailures.Load())
	writeCounter("analytics_retry_attempts_total", r.retryAttempts.Load())
	writeCounter("analytics_bulk_items_indexed_total", r.bulkIndexed.Load())
	writeCounter("analytics_bulk_items_failed_total", r.bulkFailed.Load())
	writeCounter("analytics_queue_full_occurrences_total", r.queueFull.Load())
	r.writeHistogram(&b, "analytics_batch_size", &r.batchSizeHistogram)
	r.writeHistogram(&b, "analytics_flush_latency_seconds", &r.flushLatencySeconds)
	fmt.Fprintf(&b, "# TYPE analytics_process_uptime_seconds gauge\nanalytics_process_uptime_seconds %.3f\n", time.Since(r.startedAt).Seconds())
	return b.String()
}

func (r *Registry) writeHistogram(b *strings.Builder, name string, h *histogram) {
	fmt.Fprintf(b, "# TYPE %s histogram\n", name)
	var running uint64
	for _, bucket := range h.items {
		running = bucket.count.Load()
		fmt.Fprintf(b, "%s_bucket{le=\"%.3f\"} %d\n", name, bucket.upper, running)
	}
	fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %d\n", name, h.count.Load())
	fmt.Fprintf(b, "%s_sum %.3f\n", name, float64(h.sum.Load())/1000)
	fmt.Fprintf(b, "%s_count %d\n", name, h.count.Load())
}
