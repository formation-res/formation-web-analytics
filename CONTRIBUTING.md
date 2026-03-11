# Contributing

## Guard Rails

- Run `make test-backend` before pushing. The backend suite is always run with `-timeout 10s`; tests that need longer should be fixed, not left open-ended.
- Run `make test-elasticsearch` when changing Elasticsearch mappings, ILM, templates, or bootstrap logic.
- Prefer deterministic tests over `time.Sleep`. Use channels, contexts, or explicit deadlines.
- Keep the stack reproducible. Local Elasticsearch for development is [docker-compose.elasticsearch.yml](/Users/jillesvangurp/git/formation/formation-web-analytics/docker-compose.elasticsearch.yml); production stack is [docker-compose.yml](/Users/jillesvangurp/git/formation/formation-web-analytics/docker-compose.yml).
- If behavior changes, update [README.md](/Users/jillesvangurp/git/formation/formation-web-analytics/README.md) or [docs/definition-of-done.md](/Users/jillesvangurp/git/formation/formation-web-analytics/docs/definition-of-done.md) in the same change.

## Best Practices For Tests

- Cover externally visible behavior first: config parsing, allowlist enforcement, enrichment, queue pressure, batching, retry, and bulk payload shape.
- Add or update abuse-path tests when validation rules change: malformed JSON, wrong content type, oversized batches, and invalid payloads.
- Use table-driven tests when multiple cases share the same behavior.
- Avoid hidden global state in tests. Configure the system under test explicitly.
- Bound all asynchronous tests with `context.WithTimeout` or `go test -timeout`.
- Keep test helpers close to the package unless they are reused broadly.

## Best Practices For Running The Stack

- Use `.env.example` as the source of truth for required environment variables.
- `GEOIP_DB_PATH` is mandatory in every real collector run; do not make geolocation best-effort at startup.
- In Docker Compose, treat `geoipupdate` as part of the stack, not an optional sidecar. The collector should only start once the shared MMDB exists.
- Start local Elasticsearch with `docker compose -f docker-compose.elasticsearch.yml up -d`.
- Provision the analytics data stream with `./scripts/create-data-stream-and-templates.sh`.
- Start the collector and Caddy stack with `docker compose up --build`.
- Run `make smoke-test` when you need a full local ingest check through collector into Elasticsearch.
- Tear down local dependencies with `docker compose down` or `docker compose -f docker-compose.elasticsearch.yml down` to avoid stale state between runs.
