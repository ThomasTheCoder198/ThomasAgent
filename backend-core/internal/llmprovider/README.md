# LLM provider

## Purpose

Owns the platform LLM providers, their models, write-only vaulted provider keys, capability-checked role assignments and model resolution. Every operation requires a tenant from trusted context; absent context fails closed. Public input never chooses a tenant.

## Entry points

The domain follows the same layers as every core domain; a request flows `controller → service → repository / client`, and `model` and `dto` are plain types.

| Folder | Holds | Example |
|---|---|---|
| `controller/` | HTTP handlers only: read the request, call one service, write a DTO | `ProviderController.HandleCreateProvider` |
| `dto/` | JSON request and response types and their conversions | `ProviderRequest`, `ProviderResponse`, `DeleteResponse` |
| `service/` | Business rules and transactions; one service per file (`XService` interface + `XServiceImpl`) | `ProviderService`, `LLMModelService`, `ModelSyncService`, `RoleService`, `ResolveService` |
| `repository/` | SQL only, one table per file (`XRepo` interface + implementation) | `ProviderRepo`, `LLMModelRepo`, `RoleRepo` |
| `client/` | Calls to the outside world: a provider's `GET /models` with retry, breakers and a size cap | `ModelCatalog`, `HTTPModelCatalog` |
| `model/` | Domain types and their rules | `Provider`, `LLMModel`, `Role`, `LLMModel.FitsRole` |

`cmd/core/container.go` builds the repositories, client, services and controllers; `cmd/core/routes.go` registers the paths.

## Dependencies

- Uses: `platform/vault`, `platform/audit`, `platform/postgres` (DBTX, `WithTransaction`), `platform/httpx`, `platform/retry`, `platform/outbound`, `platform/config`, named catalog errors.
- Used by: the public `/api/v1/providers`, `/api/v1/models`, `/api/v1/model-roles` routes. `ResolveService` has no HTTP route: the M2 agent runtime will call it inside core.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/llmprovider/... ./tests/integration -count=1
```

Unit tests sit next to the code they cover (`service/*_test.go` against a real migrated Postgres, `client/*_test.go` against local HTTP fixtures). The HTTP flow with a real login, session and CSRF lives in `tests/integration/llmprovider_http_test.go`.

## Conventions

LLM calls that generate content arrive in M2 through Firebase Genkit; `client/` only lists models today. Calls to any outside service go through `client/`, never from a controller or a service directly.

Each catalog attempt records `outbound_requests_total` and `outbound_request_duration_seconds` with `target` equal to the provider kind, through the optional `HTTPModelCatalog.Metrics` recorder, and emits one safe structured log with `success` and `duration_ms`. Logs and errors never contain raw URLs, response bodies or keys. The approved outbound client enforces destination/DNS rules, rejects redirects and ignores proxy environment variables. Complete JSON responses are bounded by CORE_PROVIDER_MAX_RESPONSE_BYTES, including chunked responses. Retry-After applies to retryable statuses; other 4xx and malformed/oversized JSON are rejected without retry. Unknown OpenRouter prices stay null.

API keys are write-only: absent/null preserves a key unless the base URL changes (then the stored key is deleted), empty removes it, and another value replaces it. Provider responses expose `hasApiKey` only. Provider updates lock the row so parallel updates cannot orphan secrets. Models in use cannot be deleted, including through provider cascade deletion. Mutations and audit records share one transaction (`postgres.WithTransaction`).

`ResolveService` returns the decrypted key; it is never exposed over HTTP (the old `/internal/models/resolve` route was removed because ragx no longer calls core).

Kinds: openrouter, openai, anthropic, google, azure, ollama, openai_compatible, cohere, voyage, jina, typesafe. Capabilities: chat, tools, vision, reasoning, embedding, rerank, decision. Catalog sync supports OpenRouter only.

| Role         | Required capability                          |
| ------------ | -------------------------------------------- |
| chat.default | chat + tools                                 |
| chat.fast    | chat                                         |
| vision       | vision                                       |
| embedding    | embedding                                    |
| rerank       | rerank or decision                           |
| decision     | decision; unassigned falls back to chat.fast |

Sync rejects malformed remote entries atomically, caps catalog size (CORE_PROVIDER_MAX_REMOTE_MODELS, default 1000), updates only existing sync rows, and removes stale sync rows unless any tenant role still references them. Manual models are never overwritten by sync; collisions count as skipped. Pruning runs only for complete nonempty snapshots.

Each provider ID owns its circuit breaker. CORE_PROVIDER_MAX_BREAKERS bounds cached circuits (default 256); cache pressure discards only inactive closed circuits, least recently used first. If every cached circuit is protected, new provider calls return PROVIDER_UNAVAILABLE.

## Common failures

- Provider rejected: invalid credential/endpoint, blocked destination, or malformed/oversized catalog response; correct settings before retrying.
- Provider unavailable: transient service failure, disabled provider, timeout or open circuit; find the request by its request id.
- Capability mismatch: the selected model lacks the role capabilities; choose a suitable model.
- Model in use: a role still references the model; reassign the role before deletion.
