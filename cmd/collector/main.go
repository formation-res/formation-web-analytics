package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/formation-res/formation-web-analytics/internal/batcher"
	"github.com/formation-res/formation-web-analytics/internal/config"
	"github.com/formation-res/formation-web-analytics/internal/elastic"
	"github.com/formation-res/formation-web-analytics/internal/geo"
	"github.com/formation-res/formation-web-analytics/internal/httpapi"
	"github.com/formation-res/formation-web-analytics/internal/metrics"
	"github.com/formation-res/formation-web-analytics/internal/queue"
)

var version = "dev"

func main() {
	cfg, err := config.Load(version)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	logger.Info("starting collector",
		"listen_addr", cfg.ListenAddr,
		"metrics_enabled", cfg.MetricsEnabled,
		"metrics_listen_addr", cfg.MetricsListenAddr,
		"allowed_domains", cfg.AllowedDomains,
		"data_stream", cfg.DataStream,
		"flush_interval", cfg.FlushInterval,
		"max_batch_size", cfg.MaxBatchSize,
		"max_queue_size", cfg.MaxQueueSize,
		"drop_policy", cfg.DropPolicy,
		"rate_limit_per_minute", cfg.RateLimitPerMinute,
		"rate_limit_max_clients", cfg.RateLimitMaxClients,
		"capture_client_ip", cfg.CaptureClientIP,
		"trust_proxy_headers", cfg.TrustProxyHeaders,
		"read_timeout", cfg.ReadTimeout,
		"write_timeout", cfg.WriteTimeout,
		"idle_timeout", cfg.IdleTimeout,
		"collector_version", cfg.CollectorVersion,
	)

	registry := metrics.New()
	q := queue.New(cfg.MaxQueueSize)
	sender := elastic.New(cfg, registry)
	geoResolver, err := geo.New(cfg.GeoIPDBPath)
	if err != nil {
		logger.Error("failed to open geoip database", "path", cfg.GeoIPDBPath, "error", err)
		os.Exit(1)
	}
	defer geoResolver.Close()
	b := batcher.New(cfg, q, sender, registry, logger)
	server := httpapi.New(cfg, q, b, sender, geoResolver, registry, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	geoWatchDone := make(chan struct{})
	go func() {
		defer close(geoWatchDone)
		watchGeoIP(ctx, geoResolver, logger)
	}()
	batchCtx, stopBatcher := context.WithCancel(context.Background())
	go b.Run(batchCtx)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	var metricsServer *http.Server
	if cfg.MetricsEnabled {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("GET /metrics", server.MetricsHandler())
		metricsServer = &http.Server{
			Addr:              cfg.MetricsListenAddr,
			Handler:           metricsMux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		}
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("collector server shutdown failed", "error", err)
		}
		if metricsServer != nil {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				logger.Error("metrics server shutdown failed", "error", err)
			}
		}

		stopBatcher()
		select {
		case <-b.Done():
			if err := b.Drain(shutdownCtx); err != nil {
				logger.Error("queued event drain failed", "error", err, "queue_depth", q.Len())
			}
		case <-shutdownCtx.Done():
			logger.Error("timed out stopping batcher", "error", shutdownCtx.Err(), "queue_depth", q.Len())
		}
	}()

	if metricsServer != nil {
		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server failed", "error", err)
				stop()
			}
		}()
	}

	serveErr := httpServer.ListenAndServe()
	stop()
	if serveErr != nil && serveErr != http.ErrServerClosed {
		logger.Error("server failed", "error", serveErr)
	}
	<-shutdownDone
	<-geoWatchDone
	if serveErr != nil && serveErr != http.ErrServerClosed {
		os.Exit(1)
	}
}

func watchGeoIP(ctx context.Context, resolver geo.Resolver, logger *slog.Logger) {
	for {
		timer := time.NewTimer(durationUntilNextGeoIPReload(time.Now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			reloaded, err := resolver.ReloadIfChanged()
			if err != nil {
				logger.Warn("failed to reload geoip database", "error", err)
				continue
			}
			if reloaded {
				logger.Info("reloaded geoip database")
			}
		}
	}
}

func durationUntilNextGeoIPReload(now time.Time) time.Duration {
	now = now.UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return nextMidnight.Sub(now)
}

func parseLevel(raw string) slog.Level {
	switch raw {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
