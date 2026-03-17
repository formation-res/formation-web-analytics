package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/jillesvangurp/formation-web-analytics/internal/batcher"
	"github.com/jillesvangurp/formation-web-analytics/internal/config"
	"github.com/jillesvangurp/formation-web-analytics/internal/elastic"
	"github.com/jillesvangurp/formation-web-analytics/internal/events"
	"github.com/jillesvangurp/formation-web-analytics/internal/geo"
	"github.com/jillesvangurp/formation-web-analytics/internal/metrics"
	"github.com/jillesvangurp/formation-web-analytics/internal/queue"
)

type Server struct {
	cfg     config.Config
	queue   *queue.Queue
	batcher *batcher.Batcher
	elastic elastic.BulkSender
	geo     geo.Resolver
	metrics *metrics.Registry
	logger  *slog.Logger
}

func New(cfg config.Config, q *queue.Queue, b *batcher.Batcher, sender elastic.BulkSender, geoResolver geo.Resolver, registry *metrics.Registry, logger *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		queue:   q,
		batcher: b,
		elastic: sender,
		geo:     geoResolver,
		metrics: registry,
		logger:  logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /collect", s.handleCollect)
	mux.HandleFunc("POST /batch", s.handleCollect)
	mux.HandleFunc("OPTIONS /collect", s.handleOptions)
	mux.HandleFunc("OPTIONS /batch", s.handleOptions)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	return s.withCommonHeaders(mux)
}

func (s *Server) MetricsHandler() http.Handler {
	return s.metrics.Handler()
}

func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	s.setCORS(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCollect(w http.ResponseWriter, r *http.Request) {
	s.setCORS(w, r)
	if !isJSONRequest(r) {
		s.metrics.IncRejected(1)
		s.respondError(w, http.StatusUnsupportedMediaType, "content_type_must_be_application_json")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxPayloadBytes))
	if err != nil {
		s.metrics.IncRejected(1)
		s.respondError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		return
	}
	if len(body) == 0 {
		s.metrics.IncRejected(1)
		s.respondError(w, http.StatusBadRequest, "empty_body")
		return
	}
	eventBatch, err := events.DecodeBatch(body)
	if err != nil {
		s.metrics.IncRejected(1)
		if errors.Is(err, events.ErrEmptyBatch) {
			s.respondError(w, http.StatusBadRequest, "empty_batch")
			return
		}
		s.respondError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if len(eventBatch) > s.cfg.MaxEventsPerRequest {
		s.metrics.IncRejected(len(eventBatch))
		s.respondError(w, http.StatusRequestEntityTooLarge, "too_many_events")
		return
	}

	now := time.Now()
	enriched := make([]events.Event, 0, len(eventBatch))
	for i := range eventBatch {
		domain, resolvedClientIP := events.Enrich(r, s.cfg, &eventBatch[i], now)
		if !events.AllowedDomain(s.cfg, domain) {
			s.logger.Info("domain rejected", "domain", domain, "host", r.Host)
			s.metrics.IncRejected(1)
			s.respondError(w, http.StatusForbidden, "domain_not_allowed")
			return
		}
		if err := eventBatch[i].Validate(s.cfg); err != nil {
			s.metrics.IncRejected(1)
			s.respondError(w, http.StatusBadRequest, "invalid_event")
			return
		}
		if geoResult, ok := s.geo.Lookup(resolvedClientIP); ok {
			eventBatch[i].GeoCountryISO = geoResult.CountryISOCode
			eventBatch[i].GeoCountryName = geoResult.CountryName
			eventBatch[i].GeoCityName = geoResult.CityName
		}
		enriched = append(enriched, eventBatch[i])
	}

	if s.cfg.DropPolicy == config.DropPolicyReject {
		if ok := s.queue.Enqueue(enriched); !ok {
			s.metrics.IncRejected(len(enriched))
			s.metrics.IncQueueFull()
			s.respondError(w, http.StatusServiceUnavailable, "queue_full")
			return
		}
		s.metrics.IncAccepted(len(enriched))
		s.metrics.SetQueueDepth(s.queue.Len())
		s.respondOK(w)
		return
	}

	dropped := s.queue.DropNewest(enriched)
	if dropped > 0 {
		s.metrics.IncDropped(dropped)
		s.metrics.IncQueueFull()
	}
	s.metrics.IncAccepted(len(enriched) - dropped)
	s.metrics.SetQueueDepth(s.queue.Len())
	s.respondOK(w)
}

func isJSONRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.setCORS(w, r)
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.setCORS(w, r)
	if !s.batcher.Ready() {
		s.respondJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "batcher_not_ready"})
		return
	}
	if s.cfg.RequireElasticReady {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.elastic.Ping(ctx); err != nil {
			s.respondJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "elasticsearch_not_ready"})
			return
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	if events.AllowedDomain(s.cfg, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.Header().Set("Vary", "Origin")
	}
}

func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) respondOK(w http.ResponseWriter) {
	s.respondJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) respondError(w http.ResponseWriter, status int, code string) {
	s.respondJSON(w, status, map[string]any{"ok": false, "error": code})
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil && !errors.Is(err, io.EOF) {
		s.logger.Error("failed to write response", "error", err)
	}
}
