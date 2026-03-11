#!/bin/sh
set -eu

domains="${CADDY_DOMAINS:-}"
upstream="${UPSTREAM:-collector:8080}"

if [ -z "$domains" ]; then
  echo "CADDY_DOMAINS is required" >&2
  exit 1
fi

sites=""
origin_hosts=""
first=1
OLD_IFS=$IFS
IFS=','
for raw_domain in $domains; do
  domain=$(printf "%s" "$raw_domain" | tr '[:upper:]' '[:lower:]' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  if [ -z "$domain" ]; then
    continue
  fi
  if [ $first -eq 1 ]; then
    sites="$domain"
    origin_hosts=$(printf "%s" "$domain" | sed 's/\./\\./g')
    first=0
  else
    sites="$sites, $domain"
    origin_hosts="$origin_hosts|$(printf "%s" "$domain" | sed 's/\./\\./g')"
  fi
done
IFS=$OLD_IFS

if [ -z "$sites" ]; then
  echo "No valid domains found in CADDY_DOMAINS" >&2
  exit 1
fi

SITES="$sites" ORIGIN_HOSTS="$origin_hosts" UPSTREAM="$upstream" \
  exec envsubst < /etc/caddy/Caddyfile.template
