# Formation Web Analytics Collector

Minimal self-hosted analytics collector in Go with a separate TypeScript client library.

## Introduction

Formation Web Analytics Collector is currently an early access release. It is already useful for teams that want to own their analytics pipeline, but it should still be treated as infrastructure for technical adopters rather than a finished product. Expect rough edges, limited ergonomics, and breaking changes while the project matures.

The main way to send data to this collector is the [Formation Web Analytics Client](https://github.com/formation-res/formation-web-analytics-client/). That client library is the intended integration path for browser-based event collection and should be considered part of the recommended setup for this server. The client repository was renamed, but the published npm package is currently still `@tryformation/formation-web-analytics-client`.

If you decide to use this today, assess the operational and compliance risks carefully. There is no polished UI yet, so setup, validation, and analysis require hands-on work with your own infrastructure and Elasticsearch. You should review your deployment, retention settings, access controls, backups, incident handling, cookie and consent requirements, and privacy notices before collecting production traffic. The mitigation is straightforward: start with a limited rollout, keep your configuration conservative, test your data handling end to end, and make sure your legal and security review matches your specific use case.

The main reason to adopt this approach is control. This project is intended for self-hosting web analytics in the EU in a way that can reduce third-country transfer exposure and support a GDPR-aligned deployment when configured and operated correctly. Instead of sending visitor data to foreign advertising or tracking companies, you keep collection, storage, access, and governance under your own control. That reduces third-party exposure, simplifies data residency choices, and lets you decide exactly what is collected, retained, and shared.

There is currently no user interface yet. [formationxyz.com](https://formationxyz.com) is waiting to announce some agentic solutions around this, so stay tuned and check the site for updates and more information.

The published container setup is designed around compliant GeoIP distribution. The Docker image does not need to ship a MaxMind database. Instead, the Compose stack expects you to provide your own MaxMind account ID and license key, downloads `GeoLite2-City.mmdb` into a mounted volume at runtime, and periodically refreshes that database in place.

## Backend

The collector exposes:

- `POST /collect`
- `POST /batch`
- `GET /healthz`
- `GET /readyz`

Events are validated, enriched with request metadata and local GeoIP metadata, queued in memory, and flushed to an Elasticsearch data stream via the Bulk API.
Validation and abuse guard rails include request body limits, JSON-only ingest, bounded batch sizes, field-length limits, and payload depth/entry limits.
The default in-memory queue size is `10_000` events and the default maximum bulk batch size is `500` events.
Metrics are disabled by default; when enabled, `GET /metrics` is served on a separate listener configured with `METRICS_LISTEN_ADDR`.

## Local development

1. Copy `.env.example` to `.env` and set Elasticsearch credentials.
2. Get your own MaxMind account ID and license key, then set `MAXMIND_ACCOUNT_ID` and `MAXMIND_LICENSE_KEY`.
3. For non-Docker runs, download/update `GeoLite2-City.mmdb` locally with `geoipupdate` and point `GEOIP_DB_PATH` at it.
4. Start local Elasticsearch with `docker compose -f docker-compose.elasticsearch.yml up -d`.
5. Create the default `web-analytics` data stream, ILM policy, and templates with `./scripts/create-data-stream-and-templates.sh`.
   Use `--data-stream-name your-name` if you want a different data stream.
6. Run `make test-backend`.
7. Run `go run ./cmd/collector`.
8. Or start the deployment stack with `docker compose up --build`.
9. Run `make smoke-test` for a local collector-to-Elasticsearch verification.
10. Run `make smoke-test-browser-client` for an end-to-end check using the Formation Web Analytics Client in a Docker-managed test container.

## GeoIP updates

Docker Compose now includes a `geoipupdate` service based on MaxMind's official container image. It downloads `GeoLite2-City.mmdb` into a shared Docker volume, and the collector waits for that database before starting.

Use your own MaxMind credentials here. The values in `.env.example` are placeholders only, and you should never commit real `MAXMIND_LICENSE_KEY` values or any populated local env files to your repository.

This is also the recommended distribution model for public container releases: publish the collector image without bundling the MaxMind database, mount a persistent volume for GeoIP data, and let `geoipupdate` fetch and periodically refresh the database on the user's deployment using their own credentials.

Attribution: This product includes GeoLite Data created by MaxMind, available from [maxmind.com](https://www.maxmind.com).

Relevant variables:

- `ALLOWED_DOMAINS` should list your collector hostnames
- `SITE_ORIGIN_MAP` optional per-site origin allowlist in the form `marketing:tryformation.com|www.tryformation.com;docs:docs.tryformation.com`
- `CADDY_DOMAINS` default example `analytics.tryformation.com`
- `CADDY_RATE_LIMIT_EVENTS` default `120`
- `CADDY_RATE_LIMIT_WINDOW` default `1m`
- `MAXMIND_ACCOUNT_ID`
- `MAXMIND_LICENSE_KEY`
- `GEOIPUPDATE_EDITION_IDS` default `GeoLite2-City`
- `GEOIPUPDATE_FREQUENCY` in hours; `0` means run once and exit
- `GEOIP_DB_PATH` default `/data/GeoLite2-City.mmdb`
- `GEOIP_WAIT_TIMEOUT` collector startup wait timeout in seconds
- `STORE_IP_METADATA` default `false`
- `SANITIZE_URLS` default `true`
- `REQUIRE_ORIGIN` default `true`
- `REQUIRE_URL_HOST_MATCH` default `true`
- `RATE_LIMIT_PER_MINUTE` default `300`
- `BLOCKED_USER_AGENTS` default `bot,crawler,spider,curl,wget,python-requests,go-http-client`
- `SUSPECT_USER_AGENTS` default `headless,playwright,puppeteer,selenium,phantomjs`

If your environment uses egress controls, allow HTTPS redirects to:

- `mm-prod-geoip-databases.a2649acb697e2c09b632799562c076f2.r2.cloudflarestorage.com`

## Test Elasticsearch

Start a local Elasticsearch 9 node with:

```bash
docker compose -f docker-compose.elasticsearch.yml up -d
```

The local test cluster is exposed on `http://localhost:19920`.

Provision the default `web-analytics` data stream, ILM policy, and templates with:

```bash
./scripts/create-data-stream-and-templates.sh
```

Or specify a different data stream name:

```bash
./scripts/create-data-stream-and-templates.sh --data-stream-name your-data-stream
```

The script creates:

- data stream `<name>` where the default is `web-analytics`
- ILM policy `<name>-ilm-policy`
- component templates `<name>-template-settings` and `<name>-template-mappings`
- index template `<name>-template`

The mappings are tuned for this collector's analytics event shape: fixed top-level dimensions as keywords/dates/IP or wildcard fields, and a `payload` field stored as `flattened` for arbitrary event properties without unbounded mapping growth.

## Guard rails

- Contributor workflow and test/run expectations: [CONTRIBUTING.md](CONTRIBUTING.md)
- Done checklist and current reassessment: [docs/definition-of-done.md](docs/definition-of-done.md)
- Privacy, consent, and compliance notes for deployers: [docs/privacy-and-compliance.md](docs/privacy-and-compliance.md)
- Third-party licenses and notices: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)

## Validation limits

- `Content-Type` must be `application/json` when present.
- Requests larger than `MAX_PAYLOAD_BYTES` are rejected.
- Batches larger than `MAX_EVENTS_PER_REQUEST` are rejected.
- Core string fields are bounded by `MAX_FIELD_LENGTH`.
- `payload` is bounded by `MAX_PAYLOAD_ENTRIES` and `MAX_PAYLOAD_DEPTH`.
- Unknown top-level JSON fields are rejected.
- `GEOIP_DB_PATH` is required; ingest startup fails without a local database.

## Abuse controls

- Requests without an `Origin` header are rejected by default.
- Requests with obviously automated user agents are rejected by default.
- Requests with browser-automation style user agents are accepted but marked as suspect.
- Requests are rate limited per client IP in-memory with `RATE_LIMIT_PER_MINUTE`.
- Caddy applies edge rate limiting before the request reaches the collector.
- `SITE_ORIGIN_MAP` can bind each `site_id` to an explicit set of allowed origins.
- Event `url` hosts must match the request `Origin` host by default.
- Query strings and fragments are stripped from `url`, `referrer`, `referer_header`, and `path` by default.

## Production Example

For your production setup, keep `analytics.tryformation.com` as the collector host and bind each site to a canonical `site_id`, for example:

```env
ALLOWED_DOMAINS=analytics.tryformation.com,open-rtls.com,formation-xyz.com,tryformation.com
SITE_ORIGIN_MAP=open-rtls:open-rtls.com;formation-xyz:formation-xyz.com;tryformation:tryformation.com
```

This keeps `site_id` stable per property and prevents one allowed origin from writing traffic into another site's bucket.

## Notes

- The collector is intentionally lossy under pressure or prolonged Elasticsearch outages.
- CORS is enforced in both Caddy and the backend.
- `/metrics` is intentionally not exposed through Caddy.
- Raw IP storage is disabled unless `CAPTURE_CLIENT_IP=true`.
- GeoIP enrichment still works without storing raw IP metadata because the lookup happens before indexing.
