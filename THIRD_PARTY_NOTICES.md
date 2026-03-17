# Third-Party Notices

This repository includes or depends on third-party software and services. The list below is intended as a practical notice file for source, binary, and container redistribution. It is not a substitute for reviewing the upstream license texts yourself.

## Project License

- Formation Web Analytics Collector: MIT

## Runtime And Build Dependencies

- Go standard library
  License: BSD-style license
  Source: https://go.dev/LICENSE

- `github.com/maxmind/mmdbwriter`
  License: Apache-2.0
  Source: https://github.com/maxmind/mmdbwriter
  License text: https://raw.githubusercontent.com/maxmind/mmdbwriter/main/LICENSE

- `github.com/oschwald/maxminddb-golang/v2`
  License: Apache-2.0
  Source: https://github.com/oschwald/maxminddb-golang
  License text: https://raw.githubusercontent.com/oschwald/maxminddb-golang/main/LICENSE

- `go4.org/netipx`
  License: BSD-3-Clause
  Source: https://github.com/go4org/netipx
  License text: https://raw.githubusercontent.com/go4org/netipx/master/LICENSE

- `golang.org/x/sys`
  License: BSD-3-Clause
  Source: https://cs.opensource.google/go/x/sys
  License text: https://raw.githubusercontent.com/golang/sys/master/LICENSE

## Container And Infrastructure Components

- Caddy
  License: Apache-2.0
  Source: https://github.com/caddyserver/caddy
  License text: https://raw.githubusercontent.com/caddyserver/caddy/master/LICENSE

- xcaddy
  License: Apache-2.0
  Source: https://github.com/caddyserver/xcaddy
  License text: https://raw.githubusercontent.com/caddyserver/xcaddy/master/LICENSE

- `github.com/mholt/caddy-ratelimit`
  License: MIT
  Source: https://github.com/mholt/caddy-ratelimit
  License text: https://raw.githubusercontent.com/mholt/caddy-ratelimit/master/LICENSE

- MaxMind `geoipupdate` container and GeoLite data
  Product terms and license depend on the specific image and database used.
  Sources:
  https://github.com/maxmind/geoipupdate
  https://www.maxmind.com/en/geolite2/eula

Attribution required for GeoLite data:

`This product includes GeoLite Data created by MaxMind, available from https://www.maxmind.com.`

## Test And Development Dependencies

- `jsdom`
  License: MIT
  Source: https://github.com/jsdom/jsdom
  License text: https://raw.githubusercontent.com/jsdom/jsdom/main/LICENSE.txt

## Notes

- Elasticsearch is referenced in local development and deployment examples. Review Elastic's licensing terms separately for any distribution, hosting, or managed-service use case involving Elasticsearch or Elastic container images.
- If you publish binaries or container images, include this file or an equivalent notice in your release artifacts and public distribution metadata.
