#!/usr/bin/env sh
set -eu

geoip_path="${GEOIP_DB_PATH:-/data/GeoLite2-City.mmdb}"
timeout_seconds="${GEOIP_WAIT_TIMEOUT:-300}"
elapsed=0

while [ ! -s "$geoip_path" ]; do
  if [ "$elapsed" -ge "$timeout_seconds" ]; then
    echo "Timed out waiting for GeoIP database at $geoip_path" >&2
    exit 1
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done
