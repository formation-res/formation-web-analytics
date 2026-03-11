#!/usr/bin/env bash
set -euo pipefail

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd jq
require_cmd go
require_cmd docker

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ES_URL="${ES_URL:-http://localhost:9990}"
DEFAULT_PORT=$((18080 + ($$ % 1000)))
LISTEN_ADDR="${LISTEN_ADDR:-127.0.0.1:${DEFAULT_PORT}}"
COLLECT_URL="http://${LISTEN_ADDR}/collect"
LOG_FILE="$(mktemp)"
GEOIP_DB_FILE="$(mktemp -t formation-geoip.XXXXXX).mmdb"
SITE_ID="smoke-test-$(date +%s)-$$"

cleanup() {
  if [[ -n "${COLLECTOR_PID:-}" ]]; then
    kill "$COLLECTOR_PID" >/dev/null 2>&1 || true
    wait "$COLLECTOR_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE"
  rm -f "$GEOIP_DB_FILE"
}
trap cleanup EXIT

cd "$ROOT"

docker compose -f docker-compose.elasticsearch.yml up -d >/dev/null
until curl -fsS "${ES_URL}/_cluster/health" >/dev/null; do
  sleep 2
done

./scripts/create-data-stream-and-templates.sh --es-url "$ES_URL" >/dev/null
go run ./tools/generate-test-geoip-db "$GEOIP_DB_FILE" >/dev/null

ALLOWED_DOMAINS=example.com \
ELASTICSEARCH_URL="$ES_URL" \
ELASTICSEARCH_DATA_STREAM=analytics-events \
ELASTICSEARCH_API_KEY=dummy \
GEOIP_DB_PATH="$GEOIP_DB_FILE" \
LISTEN_ADDR="$LISTEN_ADDR" \
FLUSH_INTERVAL=100ms \
MAX_BATCH_SIZE=1 \
MAX_QUEUE_SIZE=100 \
MAX_PAYLOAD_BYTES=4096 \
MAX_EVENTS_PER_REQUEST=10 \
MAX_FIELD_LENGTH=512 \
MAX_PAYLOAD_ENTRIES=32 \
MAX_PAYLOAD_DEPTH=4 \
go run ./cmd/collector >"$LOG_FILE" 2>&1 &
COLLECTOR_PID=$!

until curl -fsS "http://${LISTEN_ADDR}/readyz" >/dev/null; do
  sleep 1
done

curl -fsS -X POST "$COLLECT_URL" \
  -H 'Content-Type: application/json' \
  -H 'Origin: https://example.com' \
  -H 'Host: example.com' \
  -H 'X-Real-IP: 1.2.3.4' \
  -d "{\"type\":\"page_view\",\"site_id\":\"${SITE_ID}\",\"path\":\"/pricing\",\"url\":\"https://example.com/pricing\",\"payload\":{\"utm_source\":\"smoke\"}}" >/dev/null

for _ in $(seq 1 20); do
  response="$(curl -fsS "${ES_URL}/analytics-events/_search" -H 'Content-Type: application/json' -d "{\"size\":1,\"query\":{\"term\":{\"site_id\":\"${SITE_ID}\"}}}")"
  hits="$(printf '%s' "$response" | jq '.hits.total.value')"
  if [[ "$hits" -ge 1 ]]; then
    geo_code="$(printf '%s' "$response" | jq -r '.hits.hits[0]._source.geo_country_iso_code')"
    if [[ "$geo_code" != "EX" ]]; then
      echo "Smoke test failed: expected geo_country_iso_code=EX, got $geo_code" >&2
      exit 1
    fi
    echo "Smoke test passed: event indexed into analytics-events with geolocation"
    exit 0
  fi
  sleep 1
done

echo "Smoke test failed: event was not indexed" >&2
echo "--- collector log ---" >&2
cat "$LOG_FILE" >&2
exit 1
