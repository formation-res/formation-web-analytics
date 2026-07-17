# Collector API

The collector accepts browser analytics events over HTTP. The canonical OpenAPI 3.1 contract is [api/openapi.json](../api/openapi.json). A running collector also serves the same document from `GET /openapi.json`.

For browser applications, use the [Formation Web Analytics Client](https://github.com/formation-res/formation-web-analytics-client). Call the HTTP API directly for custom clients, server-side checks, or integration testing.

## Base URL and content type

Local development uses `http://localhost:8080`. Replace it with the public collector URL in deployed environments.

Send ingestion requests as JSON:

```http
Content-Type: application/json
Origin: https://www.example.com
User-Agent: your-client-user-agent
```

The default configuration requires `Origin`. It also rejects empty and configured automated user agents. Browser requests normally add both headers. Custom clients need to send them explicitly and must be allowed by the deployment configuration.

## Submit an event

`POST /collect` accepts one event or a batch wrapper. `POST /batch` is an alias intended for batch delivery and currently accepts the same two shapes.

```bash
curl https://analytics.example.com/collect \
  --header 'Content-Type: application/json' \
  --header 'Origin: https://www.example.com' \
  --header 'User-Agent: ExampleAnalyticsClient/1.0' \
  --data '{
    "type": "page_view",
    "site_id": "marketing",
    "timestamp": "2026-07-17T09:30:00Z",
    "session_id": "session-01",
    "anonymous_id": "visitor-01",
    "url": "https://www.example.com/pricing",
    "path": "/pricing",
    "title": "Pricing",
    "payload": {"campaign": "summer"}
  }'
```

A batch wraps events in an `events` array:

```json
{
  "events": [
    {"type": "page_view", "site_id": "marketing", "path": "/"},
    {"type": "signup_started", "site_id": "marketing", "path": "/signup"}
  ]
}
```

### Event fields

| Field | Required | Description |
| --- | --- | --- |
| `type` | yes | Event name. Use 1 to 128 ASCII letters, numbers, `_`, `.`, `:`, or `-`. |
| `site_id` | yes | Stable website or property ID, using the same character rules as `type`. |
| `timestamp` | no | RFC 3339 client event time. Missing or invalid values become the server receive time. |
| `session_id` | no | Client-generated browsing-session ID. |
| `anonymous_id` | no | Client-generated visitor ID. |
| `user_id` | no | Application user ID or `null`. Review whether collecting it is necessary. |
| `path` | no | Page path. Query and fragment data are removed by default. |
| `url` | no | HTTP or HTTPS page URL. Its host must match `Origin` by default. |
| `referrer` | no | Previous page URL. Query, fragment, and userinfo data are removed by default. |
| `title` | no | Page title. |
| `payload` | no | Event-specific JSON object. Default limits are 128 entries, depth 4, and 10,240 bytes per string. |
| `timezone` | no | IANA timezone such as `Europe/Berlin`. |
| `timezone_offset_minutes` | no | UTC offset from `-840` through `840`. |

The collector rejects unknown top-level fields. It enriches accepted events with request host, origin, browser, operating system, device, language, collector version, traffic quality, and GeoIP data before queueing them. Raw IP storage is off by default.

`MAX_FIELD_LENGTH`, `MAX_PAYLOAD_ENTRIES`, `MAX_PAYLOAD_DEPTH`, `MAX_PAYLOAD_BYTES`, and `MAX_EVENTS_PER_REQUEST` control deployment limits. Oversized strings in supported fields are truncated before validation. The default maximum is 100 events and 1 MiB per request.

## Admission and delivery

Success returns HTTP `202`:

```json
{"ok": true}
```

This confirms admission, not Elasticsearch persistence. The collector queues and flushes events asynchronously. With `DROP_POLICY=reject`, a full queue returns `503 queue_full`. With `DROP_POLICY=drop_newest`, the collector can return `202` while dropping some or all submitted events under queue pressure.

Requests can also be rejected by:

- the request-domain allowlist in `ALLOWED_DOMAINS`
- the per-site origin binding in `SITE_ORIGIN_MAP`
- the default URL-host-to-Origin check
- the required-Origin and user-agent policies
- the per-client-IP rate limit
- JSON, request-size, batch-size, and event validation

Errors have a stable JSON shape:

```json
{"ok": false, "error": "invalid_event", "detail": "invalid timezone"}
```

`detail` is only present for `invalid_event`. The OpenAPI document lists every current status and error code.

## Health and readiness

- `GET /healthz` returns `200 {"ok":true}` when the HTTP process is running. It does not check Elasticsearch.
- `GET /readyz` checks batcher readiness. With `REQUIRE_ELASTIC_READY=true`, it also checks Elasticsearch and returns `503` with `batcher_not_ready` or `elasticsearch_not_ready` when unavailable.

## CORS

`OPTIONS /collect` and `OPTIONS /batch` return `204`. Allowed origins receive these headers:

```http
Access-Control-Allow-Origin: <request Origin>
Access-Control-Allow-Methods: POST, OPTIONS
Access-Control-Allow-Headers: Content-Type
Access-Control-Max-Age: 600
```

The CORS origin check uses `ALLOWED_DOMAINS`. Per-site origin checks happen after the request body is decoded.

## Metrics

When `METRICS_ENABLED=true`, Prometheus metrics are available from `GET /metrics` on the separate `METRICS_LISTEN_ADDR` listener, which defaults to `:9090`. The main collector listener does not expose this route, and the production Caddy configuration does not publish it.
