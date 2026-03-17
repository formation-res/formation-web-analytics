# Privacy And Compliance Notes

This project is infrastructure, not legal advice. Running the collector on your own infrastructure can reduce third-country transfer exposure and improve control over analytics data, but it does not by itself make a deployment compliant with GDPR, the ePrivacy Directive, TTDSG, or any other local privacy law.

## What This Collector Processes

Depending on how you configure the server and client, the collector can process:

- online identifiers such as `session_id`, `anonymous_id`, and optional `user_id`
- page and navigation metadata such as `url`, `path`, `referrer`, `origin`, and `title`
- request metadata such as `user_agent`, parsed browser or device fields, parsed `accept_language` fields, and optional client timezone fields
- approximate geolocation derived from IP lookup
- optional IP-related metadata if `CAPTURE_CLIENT_IP=true` or `STORE_IP_METADATA=true`
- arbitrary custom event properties in `payload`

Even when raw IP storage is disabled, this can still involve personal data under EU law because online identifiers and device-related request metadata may be attributable to a natural person.

## Default Privacy Posture

The default configuration aims for data minimization, not legal sufficiency:

- `CAPTURE_CLIENT_IP=false`
- `STORE_IP_METADATA=false`
- `SANITIZE_URLS=true`
- `REQUIRE_ORIGIN=true`
- `REQUIRE_URL_HOST_MATCH=true`
- `RETENTION_DAYS=90`

These defaults reduce exposure, but they do not remove the need for a legal assessment of your own deployment.

## Controller Responsibilities

If you deploy this collector, you are responsible for:

- choosing a lawful basis for analytics processing
- assessing whether consent is required for your client-side implementation
- providing a privacy notice that accurately describes the data collected, purpose, recipients, retention, and user rights
- configuring retention and deletion periods appropriately
- limiting access to analytics data and managing credentials securely
- determining whether a DPIA, records of processing, or additional documentation are required
- putting appropriate processor and infrastructure agreements in place for hosting and storage providers

## Consent And Client-Side Storage

This repository contains the server. Whether consent is required will depend heavily on how the client library works in practice.

In particular, review whether your client-side integration:

- stores or reads identifiers from cookies, local storage, session storage, or similar device storage
- creates persistent identifiers across sessions
- combines analytics data with other datasets or user identities
- tracks users across domains or properties

Self-hosting analytics does not automatically remove consent requirements under ePrivacy or TTDSG.

## Data Minimization Guidance

For a lower-risk deployment:

- avoid sending `user_id` unless there is a documented need and legal basis
- keep custom `payload` fields free of names, emails, phone numbers, account numbers, or other directly identifying values
- avoid including query parameters or fragments that may contain personal data
- keep retention periods as short as practical
- use site-specific origin allowlists and strict domain matching
- review whether geolocation is actually needed for your use case

## GeoIP And MaxMind

This stack is designed so that deployers use their own MaxMind credentials and fetch GeoLite databases at runtime into a mounted volume. Do not bundle the GeoLite database into public container images unless you have confirmed that your license permits that distribution model.

Attribution required by MaxMind:

`This product includes GeoLite Data created by MaxMind, available from https://www.maxmind.com.`

## Recommended Deployment Review

Before going live, review at least:

- your client-side tracking behavior
- your cookie banner or consent mechanism, if any
- your privacy notice text
- your retention settings
- your Elasticsearch access controls and backups
- your cross-border data flows
- your incident response process

## References

- GDPR text: https://eur-lex.europa.eu/eli/reg/2016/679/oj
- ePrivacy Directive: https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX%3A32002L0058
- MaxMind GeoLite EULA: https://www.maxmind.com/en/geolite2/eula
