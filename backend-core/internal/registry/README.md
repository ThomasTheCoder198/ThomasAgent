# Registry

## Purpose

Owns the platform provider/model catalog, write-only vaulted provider keys, capability-checked role assignments and model resolution. M0.2 operates on the trusted platform tenant `default`; organization context and organization-to-platform role fallback are introduced in M1. Public input does not choose a tenant.

## Entry points

- `NewService` injects PostgreSQL, the vault and a `Catalog`.
- Provider create/update/delete/list/test methods keep keys out of returned records and commit mutations with audit events.
- Model create/delete/list/sync methods validate metadata and atomically import OpenRouter models.
- `AssignRole`, `ListRoles`, `Resolve` enforce capability requirements; an unassigned decision role falls back to chat.fast. Disabled providers cannot resolve.
- `NewHTTPCatalogWithClient` injects the guarded outbound client, response limit, retry policy, circuit breaker and logger. The compatibility constructor creates a guarded public-only client.

## Dependencies

- Uses: vault, audit, PostgreSQL DBTX, catalog errors, retry/breaker, platform outbound/config and OpenTelemetry.
- Used by: public Registry HTTP handlers and the service-token protected internal resolver.

## Run & test

```bash
cd backend-core
go test ./internal/registry/... -count=1
```

Database tests require Docker Desktop and create isolated PostgreSQL testcontainers; catalog tests use local HTTP fixtures through an explicitly injected test client.

## Conventions

API keys are accepted only as write inputs: absent/null preserves a key, empty removes it, and another value replaces it. Provider updates lock the row so parallel updates cannot orphan secrets. Reads and mutations explicitly constrain platform data. Models in use cannot be deleted, including through provider cascade deletion. Mutations and audit records share a transaction.

Catalog calls create a client span per attempt and emit safe structured outcome logs with trace_id. They never expose raw URLs, response bodies or keys in errors/logs. The approved outbound client enforces destination/DNS rules, rejects redirects and ignores proxy environment variables. Unknown/dynamic OpenRouter prices (negative or nonfinite sentinels) remain nullable instead of being stored as invalid costs. Complete JSON responses are bounded by CORE_PROVIDER_MAX_RESPONSE_BYTES, including chunked responses. Retry-After applies to retryable statuses; other 4xx and malformed/oversized JSON are rejected without retry.

## Common failures

- Provider rejected: invalid credential/endpoint, blocked destination, or malformed/oversized catalog response; correct settings before retrying.
- Provider unavailable: transient service failure, disabled provider, timeout or open circuit; inspect safe trace correlation at the HTTP boundary.
- Capability mismatch: the selected model lacks the role capabilities; choose a suitable model.
- Model in use: a role still references the model; reassign the role before deletion.

## HTTP interface (Task 7)

`NewHandler(service, maxBodyBytes).Mount(router)` adds provider CRUD/test/sync, model create/list/delete and model-role list/assignment under `/api/v1`. The caller mounts public handlers behind `auth.RequireSession`; unsafe methods require the session CSRF header. JSON body decoding enforces the configured byte cap and field-only errors. Output uses the shared `data/meta` or `error` response and camelCase fields. Provider responses expose `hasApiKey` only; absent/null `apiKey` preserves the existing key, while an empty string removes it.

`MountInternal(router, service, serviceToken)` exposes `GET /internal/models/resolve?role=chat.fast` with exact `Authorization: Bearer <token>` authentication. The configured token must be nonempty, the Bearer prefix must be present and the token is compared in constant time. The trusted internal `data` response includes `role`, `providerKind`, `baseUrl`, `apiKey`, `modelRef`, `capabilities`, optional `embeddingDims` and `fallbackFrom`; provider keys appear only here, never in the public API. The caller supplies the shared tracing/access-log router; HTTP handlers report errors once through `httpx.WriteError` and never log bodies or credentials.

Kinds: openrouter, openai, anthropic, google, azure, ollama, openai_compatible, cohere, voyage, jina, typesafe. Capabilities: chat, tools, vision, reasoning, embedding, rerank, decision. M0.2 catalog sync supports OpenRouter only; the Genkit ModelResolver arrives in M2.

| Role         | Required capability                          |
| ------------ | -------------------------------------------- |
| chat.default | chat + tools                                 |
| chat.fast    | chat                                         |
| vision       | vision                                       |
| embedding    | embedding                                    |
| rerank       | rerank or decision                           |
| decision     | decision; unassigned falls back to chat.fast |

Registry tables are scoped to the [Platform tenant](../../../docs/glossary.md). M1 business `tenant_id` must come from trusted context, never client/model parameters; M0.2 exposes no tenant selector.

HTTP integration tests use isolated PostgreSQL and Redis containers for actual owner login/session/CSRF and use a fake catalog for provider calls. Run Docker suites serially with `go test ./internal/registry -count=1 -p 2` on Windows.
