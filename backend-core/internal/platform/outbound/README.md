# Outbound HTTP

## Purpose

Creates the provider HTTP client with destination validation and DNS pinning. LLM provider service owns catalog response byte limits and its outbound logging boundary.

## Entry points

- `NewClient(timeout, privateAllowlist)` returns a guarded `*http.Client`.
- `Instrument(next, target, recorder, propagateRequestID)` wraps a transport to record `outbound_*` metrics; the ragx transport (`TargetRagx`) enables request-id propagation. M1 builds the ragx client with it.

`Instrument` clones requests, removes every case spelling of supplied `X-Request-Id` headers, and adds a validated context ID only when propagation is enabled for the `ragx` target. HTTP 4xx/5xx responses and transport failures are recorded as errors.
- `IsSupportedScheme` shares the named HTTP/HTTPS scheme policy with LLM provider service validation.
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

Redirects are rejected and ambient HTTP proxies are disabled. No package state is mutable globally. Dial/DNS/TLS/body errors discard raw causes; operational policy errors use nonretryable `LLM_PROVIDER_REJECTED` and network failures use retryable catalog errors. This layer does not log: LLM provider service logs once, with the request id.

Go `http.Client.Do` adds the request URL to its `url.Error` even when the transport cause is safe. Catalog callers must map errors to safe catalog codes without retaining that raw wrapper, and must never log request URLs, headers or bodies.

This module has no tables. It connects providers owned by the [Platform tenant](../../../../docs/glossary.md); M1 business `tenant_id` comes from trusted context. Private exceptions grant network access only, never tenant access.

## Common failures

- Rejected destination: use public HTTPS or explicitly configure the exact private hostname and port.
- Mixed public/private DNS answers: fix provider DNS; any denied answer rejects the destination.
- Invalid allowlist: use comma-separated `hostname:port` values, with brackets around IPv6 literals.
- Upstream failure: LLM provider service applies the configured timeout/retry/breaker policy and returns a sanitized error.
