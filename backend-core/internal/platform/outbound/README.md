# Outbound HTTP

## Purpose

Creates the provider HTTP client with destination validation and DNS pinning. Registry owns catalog response byte limits and its outbound tracing/logging boundary.

## Entry points

- `NewClient(timeout, privateAllowlist)` returns a guarded `*http.Client`.
- `ValidatePrivateAllowlist` validates exact `hostname:port` configuration without reading environment variables.

## Dependencies

- Uses Go HTTP, DNS and IP libraries and the shared error catalog.
- Used by core startup, provider catalog calls and config validation.
- Resolver/dialer ports belong to this consumer and support deterministic unit tests.

## Run & test

```bash
cd backend-core
go test ./internal/platform/outbound ./internal/platform/config -count=1
```

## Conventions

Public destinations require HTTPS. Requests with credentials, fragments or unsupported schemes are rejected before transport. Every DNS answer must be permitted before any dial; connections use checked IP literals while HTTP keeps the hostname for TLS verification. Reserved and special-use ranges, IPv4-mapped IPv6, unspecified addresses and multicast are rejected. Exact private allowlist entries permit HTTP plus private/loopback addresses; link-local metadata addresses remain blocked.

Redirects are rejected and ambient HTTP proxies are disabled. No package state is mutable globally. Dial/DNS/TLS/body errors discard raw causes; operational policy errors use nonretryable `REGISTRY_PROVIDER_REJECTED` and network failures use retryable catalog errors. This layer does not log: Registry logs once with its span and `trace_id`.

Go `http.Client.Do` adds the request URL to its `url.Error` even when the transport cause is safe. Catalog callers must map errors to safe catalog codes without retaining that raw wrapper, and must never log request URLs, headers or bodies.

This module has no tables. It connects providers owned by the [Platform tenant](../../../../docs/glossary.md); M1 business `tenant_id` comes from trusted context. Private exceptions grant network access only, never tenant access.

## Common failures

- Rejected destination: use public HTTPS or explicitly configure the exact private hostname and port.
- Mixed public/private DNS answers: fix provider DNS; any denied answer rejects the destination.
- Invalid allowlist: use comma-separated `hostname:port` values, with brackets around IPv6 literals.
- Upstream failure: Registry applies the configured timeout/retry/breaker policy and returns a sanitized error.
