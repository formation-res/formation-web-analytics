package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/batcher"
	"github.com/jillesvangurp/formation-web-analytics/internal/config"
	"github.com/jillesvangurp/formation-web-analytics/internal/elastic"
	"github.com/jillesvangurp/formation-web-analytics/internal/geo"
	"github.com/jillesvangurp/formation-web-analytics/internal/httpapi"
	"github.com/jillesvangurp/formation-web-analytics/internal/metrics"
	"github.com/jillesvangurp/formation-web-analytics/internal/queue"
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
	go b.Run(ctx)

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

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		if metricsServer != nil {
			_ = metricsServer.Shutdown(shutdownCtx)
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

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
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
