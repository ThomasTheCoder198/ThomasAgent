# LLM

## Purpose

Owns the platform LLM providers, their models, write-only vaulted provider keys, capability-checked role assignments and model resolution. Every operation requires a tenant from trusted context; absent context fails closed. The internal service-token boundary (`auth.RequireServiceToken`) selects platform tenant `default` after authentication. Public input does not choose a tenant.

## Entry points

| File | What it holds |
|---|---|
| `provider.go` | `Provider`, `ProviderInput`, `ProviderKind` and its default base URLs |
| `model.go` | `Model`, `ModelInput`, `Capability`, `RemoteModel`, `SyncResult` |
| `role.go` | `Role`, `RequirementsForRole`, `RoleAssignment`, `ResolvedModel` |
| `service.go` | `Service`, `NewService`, the `ModelCatalog` interface, the shared transaction helper |
| `provider_service.go` | Create/update/delete/list/test providers; API-key rules |
| `model_service.go` | Create/delete/list models; `SyncModels` from the remote catalog |
| `role_service.go` | `AssignRole`, `ListRoles`, `Resolve` |
| `repository.go`, `provider_repository.go`, `model_repository.go`, `role_repository.go` | SQL, one file per table |
| `model_catalog.go` | `HTTPModelCatalog`: the provider's `GET /models` with retry, per-provider circuit breakers and a size cap |
| `provider_controller.go`, `model_controller.go`, `role_controller.go`, `resolve_controller.go` | HTTP handlers with their JSON request/response types |
| `controller_shared.go` | `DeleteResponse`, path id parsing |

`cmd/core/server.go` builds the service and controllers; `cmd/core/routes.go` registers the paths.

## Dependencies

- Uses: `platform/vault`, `platform/audit`, PostgreSQL DBTX, named catalog errors, `platform/retry` (policy and breaker), `platform/outbound` and `platform/config`.
- Used by: the public `/api/v1/providers`, `/api/v1/models`, `/api/v1/model-roles` routes and the internal `/internal/models/resolve` route.

## Run & test

```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/llm -count=1
```

Database tests require Docker Desktop and create isolated PostgreSQL testcontainers; catalog tests use local HTTP fixtures through an explicitly injected test client. Test files mirror the source files; `controller_test.go` runs the HTTP routes with a real owner login, session and CSRF, and a fake catalog.

## Conventions

Each catalog attempt records `outbound_requests_total` and `outbound_request_duration_seconds` with `target` equal to the provider kind, through the optional `HTTPModelCatalog.Metrics` recorder.

API keys are accepted only as write inputs: absent/null preserves a key unless the base URL changes (then the stored key is deleted), empty removes it, and another value replaces it. Provider responses expose `hasApiKey` only. Provider updates lock the row so parallel updates cannot orphan secrets. Reads and mutations explicitly constrain tenant data from context. Models in use cannot be deleted, including through provider cascade deletion. Mutations and audit records share a transaction.

Each catalog attempt emits one safe structured outcome log with `success` and `duration_ms`; the logger adds the request id. They never expose raw URLs, response bodies or keys in errors/logs. The approved outbound client enforces destination/DNS rules, rejects redirects and ignores proxy environment variables. Unknown/dynamic OpenRouter prices (negative or nonfinite values) remain nullable instead of being stored as invalid costs. Complete JSON responses are bounded by CORE_PROVIDER_MAX_RESPONSE_BYTES, including chunked responses. Retry-After applies to retryable statuses; other 4xx and malformed/oversized JSON are rejected without retry.

The internal resolve response includes `role`, `providerKind`, `baseUrl`, `apiKey`, `modelRef`, `capabilities`, optional `embeddingDims` and `fallbackFrom`; provider keys appear only here. Responses use `Cache-Control: no-store`.

Kinds: openrouter, openai, anthropic, google, azure, ollama, openai_compatible, cohere, voyage, jina, typesafe. Capabilities: chat, tools, vision, reasoning, embedding, rerank, decision. Catalog sync supports OpenRouter only; the Genkit ModelResolver arrives in M2.

| Role         | Required capability                          |
| ------------ | -------------------------------------------- |
| chat.default | chat + tools                                 |
| chat.fast    | chat                                         |
| vision       | vision                                       |
| embedding    | embedding                                    |
| rerank       | rerank or decision                           |
| decision     | decision; unassigned falls back to chat.fast |

Sync rejects malformed remote entries atomically, caps catalog size, updates only existing sync rows, and removes stale sync rows unless any tenant role still references them. Manual models are never overwritten by sync; collisions count as skipped. CORE_PROVIDER_MAX_REMOTE_MODELS controls the sync cap (default 1000). Counts include manual collisions and entries omitted by the cap. Pruning runs only for complete nonempty snapshots: empty, failed, and truncated responses preserve existing rows. Malformed entries reject the entire import transaction, preserving previous rows and emitting no success audit. Catalog decoding retains an invalid placeholder so the service can reject the batch consistently.

Each provider ID owns its circuit breaker; failures of one catalog cannot open another provider's circuit. CORE_PROVIDER_MAX_BREAKERS bounds cached circuits (default 256). Cache pressure discards only inactive closed circuits with no consecutive failures, choosing the least recently used. Open, half-open, accumulating-failure, and in-flight circuits remain protected. If every cached circuit is protected, new provider calls return PROVIDER_UNAVAILABLE until capacity becomes safely reusable. Missing or nonpositive remote context lengths stay null. Duplicate manual creates return 409.

## Common failures

- Provider rejected: invalid credential/endpoint, blocked destination, or malformed/oversized catalog response; correct settings before retrying.
- Provider unavailable: transient service failure, disabled provider, timeout or open circuit; find the request by its request id.
- Capability mismatch: the selected model lacks the role capabilities; choose a suitable model.
- Model in use: a role still references the model; reassign the role before deletion.
