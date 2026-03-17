#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: create-data-stream-and-templates.sh [options]

Options:
  --es-url URL                    Elasticsearch base URL (default: http://localhost:19920)
  --data-stream-name NAME         Data stream name (default: web-analytics)
  --configure-ilm true|false      Create/update ILM policy (default: true)
  --hot-rollover-gb N             Hot rollover primary shard size in GB (default: 20)
  --hot-max-age VALUE             Hot rollover max age (default: 7d)
  --delete-min-age VALUE          Delete phase min age (default: 90d)
  --number-of-shards N            index.number_of_shards (default: 1)
  --number-of-replicas N          index.number_of_replicas (default: 0)
  -h, --help                      Show this help
USAGE
}

truthy() {
  local value
  value="$(printf "%s" "$1" | tr '[:upper:]' '[:lower:]')"
  case "$value" in
    true|1|yes|y|on) return 0 ;;
    *) return 1 ;;
  esac
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

ES_URL="${ES_URL:-http://localhost:19920}"
DATA_STREAM_NAME="${DATA_STREAM_NAME:-web-analytics}"
CONFIGURE_ILM="${CONFIGURE_ILM:-true}"
HOT_ROLLOVER_GB="${HOT_ROLLOVER_GB:-20}"
HOT_MAX_AGE="${HOT_MAX_AGE:-7d}"
DELETE_MIN_AGE="${DELETE_MIN_AGE:-90d}"
NUMBER_OF_SHARDS="${NUMBER_OF_SHARDS:-1}"
NUMBER_OF_REPLICAS="${NUMBER_OF_REPLICAS:-0}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --es-url) ES_URL="$2"; shift 2 ;;
    --data-stream-name) DATA_STREAM_NAME="$2"; shift 2 ;;
    --configure-ilm) CONFIGURE_ILM="$2"; shift 2 ;;
    --hot-rollover-gb) HOT_ROLLOVER_GB="$2"; shift 2 ;;
    --hot-max-age) HOT_MAX_AGE="$2"; shift 2 ;;
    --delete-min-age) DELETE_MIN_AGE="$2"; shift 2 ;;
    --number-of-shards) NUMBER_OF_SHARDS="$2"; shift 2 ;;
    --number-of-replicas) NUMBER_OF_REPLICAS="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

require_cmd curl
require_cmd jq

PREFIX="$DATA_STREAM_NAME"
ILM_POLICY="${PREFIX}-ilm-policy"
TEMPLATE_SETTINGS="${PREFIX}-template-settings"
TEMPLATE_MAPPINGS="${PREFIX}-template-mappings"
INDEX_TEMPLATE="${PREFIX}-template"

curl_put() {
  local url="$1"
  local payload="$2"
  local status
  status="$(curl -s -o /tmp/formation-analytics-curl.out -w "%{http_code}" \
    -X PUT "$url" \
    -H "Content-Type: application/json" \
    -d "$payload")"
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    echo "Request failed: PUT $url -> $status" >&2
    cat /tmp/formation-analytics-curl.out >&2
    exit 1
  fi
}

curl_create() {
  local url="$1"
  local payload="${2:-}"
  local status
  if [[ -n "$payload" ]]; then
    status="$(curl -s -o /tmp/formation-analytics-curl.out -w "%{http_code}" \
      -X PUT "$url" \
      -H "Content-Type: application/json" \
      -d "$payload")"
  else
    status="$(curl -s -o /tmp/formation-analytics-curl.out -w "%{http_code}" \
      -X PUT "$url")"
  fi
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    if [[ "$status" == "400" || "$status" == "409" ]] && grep -q "resource_already_exists_exception" /tmp/formation-analytics-curl.out; then
      return 0
    fi
    echo "Request failed: PUT $url -> $status" >&2
    cat /tmp/formation-analytics-curl.out >&2
    exit 1
  fi
}

echo "Target Elasticsearch: $ES_URL"
echo "Data stream: $PREFIX"

if truthy "$CONFIGURE_ILM"; then
  echo "Creating ILM policy $ILM_POLICY ..."
  curl_put "$ES_URL/_ilm/policy/$ILM_POLICY" "{
    \"policy\": {
      \"phases\": {
        \"hot\": {
          \"actions\": {
            \"rollover\": {
              \"max_primary_shard_size\": \"${HOT_ROLLOVER_GB}gb\",
              \"max_age\": \"$HOT_MAX_AGE\"
            }
          }
        },
        \"delete\": {
          \"min_age\": \"$DELETE_MIN_AGE\",
          \"actions\": {
            \"delete\": {}
          }
        }
      }
    }
  }"
fi

echo "Creating component template $TEMPLATE_SETTINGS ..."
SETTINGS_JSON="\"number_of_shards\": $NUMBER_OF_SHARDS,
      \"number_of_replicas\": $NUMBER_OF_REPLICAS"
if truthy "$CONFIGURE_ILM"; then
  SETTINGS_JSON="$SETTINGS_JSON,
      \"index.lifecycle.name\": \"$ILM_POLICY\""
fi
curl_put "$ES_URL/_component_template/$TEMPLATE_SETTINGS" "{
  \"template\": {
    \"settings\": {
      $SETTINGS_JSON
    }
  },
  \"_meta\": {
    \"created_by\": \"formation-web-analytics\",
    \"created_at\": \"$(date -Iseconds)\"
  }
}"

echo "Creating component template $TEMPLATE_MAPPINGS ..."
curl_put "$ES_URL/_component_template/$TEMPLATE_MAPPINGS" "{
  \"template\": {
    \"mappings\": {
      \"dynamic\": false,
      \"properties\": {
        \"@timestamp\": {\"type\": \"date\"},
        \"received_at\": {\"type\": \"date\"},
        \"type\": {\"type\": \"keyword\"},
        \"site_id\": {\"type\": \"keyword\"},
        \"session_id\": {\"type\": \"keyword\"},
        \"anonymous_id\": {\"type\": \"keyword\"},
        \"user_id\": {\"type\": \"keyword\"},
        \"path\": {\"type\": \"keyword\"},
        \"url\": {\"type\": \"wildcard\"},
        \"referrer\": {\"type\": \"wildcard\"},
        \"title\": {
          \"type\": \"text\",
          \"fields\": {
            \"keyword\": {\"type\": \"keyword\", \"ignore_above\": 256}
          }
        },
        \"request_domain\": {\"type\": \"keyword\"},
        \"request_host\": {\"type\": \"keyword\"},
        \"client_ip\": {\"type\": \"ip\", \"ignore_malformed\": true},
        \"geo_country_iso_code\": {\"type\": \"keyword\"},
        \"geo_country_name\": {\"type\": \"keyword\"},
        \"geo_city_name\": {\"type\": \"keyword\"},
        \"geo_location\": {\"type\": \"geo_point\", \"ignore_malformed\": true},
        \"forwarded_for\": {\"type\": \"wildcard\"},
        \"user_agent\": {\"type\": \"wildcard\"},
        \"accept_language\": {\"type\": \"keyword\"},
        \"origin\": {\"type\": \"wildcard\"},
        \"referer_header\": {\"type\": \"wildcard\"},
        \"scheme\": {\"type\": \"keyword\"},
        \"remote_addr\": {\"type\": \"keyword\"},
        \"collector_version\": {\"type\": \"keyword\"},
        \"traffic_quality\": {\"type\": \"keyword\"},
        \"is_suspect\": {\"type\": \"boolean\"},
        \"suspicion_reasons\": {\"type\": \"keyword\"},
        \"payload\": {\"type\": \"flattened\"}
      }
    }
  },
  \"_meta\": {
    \"created_by\": \"formation-web-analytics\",
    \"created_at\": \"$(date -Iseconds)\"
  }
}"

echo "Creating index template $INDEX_TEMPLATE ..."
curl_put "$ES_URL/_index_template/$INDEX_TEMPLATE" "{
  \"index_patterns\": [\"${PREFIX}*\"],
  \"data_stream\": {},
  \"priority\": 500,
  \"composed_of\": [\"$TEMPLATE_SETTINGS\", \"$TEMPLATE_MAPPINGS\"]
}"

echo "Creating data stream $PREFIX ..."
curl_create "$ES_URL/_data_stream/$PREFIX"

echo
echo "=== VERIFYING DEPLOYED RESOURCES ==="
if truthy "$CONFIGURE_ILM"; then
  echo "--- ILM Policy ($ILM_POLICY) ---"
  curl -s "$ES_URL/_ilm/policy/$ILM_POLICY?pretty" | jq .
fi
echo "--- Component Template: Settings ($TEMPLATE_SETTINGS) ---"
curl -s "$ES_URL/_component_template/$TEMPLATE_SETTINGS?pretty" | jq .
echo "--- Component Template: Mappings ($TEMPLATE_MAPPINGS) ---"
curl -s "$ES_URL/_component_template/$TEMPLATE_MAPPINGS?pretty" | jq .
echo "--- Index Template ($INDEX_TEMPLATE) ---"
curl -s "$ES_URL/_index_template/$INDEX_TEMPLATE?pretty" | jq .
echo "--- Data Stream ($PREFIX) ---"
curl -s "$ES_URL/_data_stream/$PREFIX?pretty" | jq .

echo
echo "=== SUMMARY ==="
echo "Created data stream: $PREFIX"
echo "Created index template: $INDEX_TEMPLATE"
echo "Created component template (settings): $TEMPLATE_SETTINGS"
echo "Created component template (mappings): $TEMPLATE_MAPPINGS"
if truthy "$CONFIGURE_ILM"; then
  echo "Created ILM policy: $ILM_POLICY"
else
  echo "ILM policy: skipped (--configure-ilm false)"
fi
echo "Settings: shards=$NUMBER_OF_SHARDS replicas=$NUMBER_OF_REPLICAS hot_max_age=$HOT_MAX_AGE hot_rollover_gb=${HOT_ROLLOVER_GB} delete_min_age=$DELETE_MIN_AGE"
echo
echo "Recommended next step: trigger a rollover once to initialize backing index generation."
echo "curl -X POST \"$ES_URL/$PREFIX/_rollover?pretty\" -H \"Content-Type: application/json\" -d '{}'"
