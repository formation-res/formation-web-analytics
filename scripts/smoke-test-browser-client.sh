#!/usr/bin/env bash
set -euo pipefail

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd docker
require_cmd go

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.browser-client-smoke.yml}"
ES_PORT="${ES_PORT:-19920}"
ES_URL="${ES_URL:-http://localhost:${ES_PORT}}"
DATA_STREAM_NAME="${DATA_STREAM_NAME:-web-analytics}"
SITE_ID="${SITE_ID:-browser-smoke-$(date +%s)-$$}"
PROJECT_NAME="formation-browser-smoke-$$"
TMP_DIR="${ROOT}/.tmp"
mkdir -p "$TMP_DIR"
GEOIP_DB_FILE="$(mktemp "${TMP_DIR}/formation-browser-geoip.XXXXXX.mmdb")"

cleanup() {
  cd "$ROOT"
  docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
  rm -f "$GEOIP_DB_FILE"
}
trap cleanup EXIT

cd "$ROOT"
go run ./tools/generate-test-geoip-db "$GEOIP_DB_FILE" >/dev/null

export ES_PORT ES_URL DATA_STREAM_NAME SITE_ID GEOIP_DB_FILE

docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" up -d analytics-elasticsearch >/dev/null
until curl -fsS "${ES_URL}/_cluster/health" >/dev/null; do
  sleep 2
done

./scripts/create-data-stream-and-templates.sh --es-url "$ES_URL" --data-stream-name "$DATA_STREAM_NAME" >/dev/null

set +e
docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" up --build --abort-on-container-exit --exit-code-from browser-client-smoke browser-client-smoke
status=$?
set -e

if [[ "$status" -ne 0 ]]; then
  echo "--- collector logs ---" >&2
  docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" logs collector >&2 || true
  echo "--- browser-client-smoke logs ---" >&2
  docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" logs browser-client-smoke >&2 || true
  exit "$status"
fi
