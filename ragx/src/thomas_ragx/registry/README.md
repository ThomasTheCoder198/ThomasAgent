# Model registry

## Purpose
Ragx owns its model registry: providers with API keys sealed under `RAG_VAULT_MASTER_KEY`, models with capabilities, and one model per role. Core's registry is separate; ragx never asks core for a model.

## Entry points
- `RegistryService` — provider, model, and role operations for the admin UI and KB service.
- `seed_dev_registry` — idempotent offline embedding model for development.
- `LISTING_ROUTES` — provider kind to model-listing route (`ListingMethod`), and whether listing needs a key.
- `KIND_RULES` — per kind: is the API key required at creation, and is the base URL hidden, optional or required.
- `ModelListing` / `ListedModel` — a list returns names plus optional facts (capabilities, context window, prices).

## Dependencies
- Uses: LiteLLM 1.101.2 for supported provider listing, model facts, the bundled catalog and test calls; httpx for OpenRouter public lists; openai 2.54.0 for OpenAI-compatible listing; cryptography AES-256-GCM for key storage; ragx Postgres.
- Used by: the KB service for the embedding role and the admin UI.

## Run & test
```bash
cd ragx
uv run pytest -q tests/unit/test_vault.py tests/unit/test_listing.py tests/unit/test_openrouter_listing.py tests/unit/test_catalog_listing.py tests/unit/test_model_info.py tests/integration/test_registry_service.py
```

## Conventions
Keys are write-only. `Provider` exposes only `has_api_key`; `ProviderAccess` holds a `SecretStr` for one call. LiteLLM is imported lazily after `apply_litellm_policy`, using its bundled model map without downloading one. The typesafe kind arrives with the decision role in M6.

Listing routes (`LISTING_ROUTES` is data; the lister behind each method returns `ListedModel` rows):

| Kinds | Method | Network | Key needed to list |
| --- | --- | --- | --- |
| openai, anthropic, google, ollama | `litellm` (`get_models`) | provider | yes (not ollama) |
| openai_compatible | `openai_sdk` | server | no |
| openrouter | `openrouter` (`OpenRouterLister`) | public `GET {base}/models` and `{base}/embeddings/models` | no; a saved key is sent as a bearer token, never the request id |
| cohere, voyage, jina | `catalog` (`CatalogLister`) | none: LiteLLM bundled map | no |
| azure | `manual` | none | `LLM_PROVIDER_SYNC_UNSUPPORTED` |

LiteLLM `get_valid_models` is avoided because it returns an empty list for a bad key. The OpenRouter lister maps `output_modalities` (`embeddings` is embedding, `text` is chat), `supported_parameters` (`tools`, `reasoning`; `include_reasoning` alone is not reasoning), `input_modalities` (`image` is vision), `context_length` and per-token prices, the same capability rules as core `openrouter_models.go`. Two differences are deliberate: ragx also reads `/embeddings/models`, and an embedding row carries only the embedding capability (core would add tools or vision from its parameters). A list that carries facts is the authority, so Sync skips the per-row LiteLLM lookup; a bare name still goes through `describe_model`. The catalog takes LiteLLM entries whose `litellm_provider` is `cohere`, `voyage` or `jina_ai` and whose mode is embedding, rerank or chat (Cohere `command*` completion and OCR are skipped) and stores the name without the provider prefix, because `litellm_model_name` adds it back. `model_listing_timeout_seconds` bounds the OpenRouter calls; each logs `model listing call completed` with `duration_ms` and no headers.

Provider rules (`KIND_RULES`, enforced by `create_provider`; the field errors are `VALIDATION_FAILED` with `details.fields`):
- `apiKey REQUIRED` at creation for every kind except `ollama` and `openai_compatible`. Update keeps an existing key and does not demand one, so a keyless row from before the rule can still be edited.
- `baseUrl REQUIRED` for `openai_compatible` and `azure` (create and update). `ollama` shows an optional base URL; built-in kinds hide it in the UI but still accept an override.
- Address policy (`baseUrl INVALID` otherwise): the URL is parsed with `urllib.parse`; userinfo (`user:pass@`), an empty host, a bad port and whitespace are rejected. Built-in kinds (openrouter, openai, anthropic, google, cohere, voyage, jina, azure) accept only `https://` overrides. Self-hosted kinds (`ollama`, `openai_compatible`) are operator-trusted: `http` and private hosts such as `host.docker.internal` or `10.x` are allowed. There is no DNS or IP resolution check, so the operator account is trusted with where a self-hosted server lives.
- A key belongs to the address it was entered for. When an edit changes the stored base URL and submits no new key, `update_provider` clears the stored key in the same transaction (a key-requiring kind then needs a new key before Sync or a probe); a new key in the same edit replaces it. The Edit form says so.

Sync preserves manual rows and operator-edited capabilities. It fills `embedding_dims` only when empty, so a measured value survives a re-sync. Test runs per model: embedding uses `aembedding`, rerank uses `arerank`, chat uses `acompletion` with `max_tokens=1`.

Embedding size is measured, not typed. The embedder passes no `dimensions`, so the native vector length is what a KB collection needs. `assign_role(EMBEDDING, model)` on a model with no dims runs one probe embedding (`ModelChecker.measure_embedding_dims`, outside the DB transaction), stores the length with `set_measured_dims`, then assigns. A failed probe raises `LLM_PROVIDER_REJECTED` and assigns nothing. `set_measured_dims` is a conditional update (`embedding_dims IS NULL` and no `knowledge_bases.embedding_model_id` row), so it never changes a model a KB uses and is not an operator edit (`capabilities_edited` stays). Sync likewise never touches the dims of a model a KB uses. Test on an embedding model with no dims records the dims from its one call. `add_manual_model` still accepts `embedding_dims` and `context_window` (the dev seed passes 1536 offline); the UI sends none. `update_model(model_id, capabilities)` has no dims parameter: dims are written only by the measured path, `add_manual_model` and a sync into an empty column.

`assign_role` judges the model again after the probe, inside the transaction, under a row lock (`SELECT … FOR UPDATE`): the role fit is re-checked, the provider base URL must be unchanged since the probe (`LLM_PROVIDER_REJECTED` otherwise), and stored dims that differ from the probed length raise `LLM_MODEL_CAPABILITY_MISMATCH`. The KB boundary raises the same code for a role model with no dims instead of a bare `ValueError`. Lock order is provider row, then model rows, on every path (`update_provider`, `delete_provider`, `sync_models`, Test and `assign_role` all take `SELECT … FOR UPDATE` on the provider first), so an address edit waits for an in-flight role commit or sync instead of racing or deadlocking it. `update_provider` compares the new address with the locked row, so concurrent edits never leave an address with another edit's key; a sync, Test or role probe that finds the address changed since its provider call raises `LLM_PROVIDER_REJECTED`. Each probe, check or listing attempt logs one event (`outcome`, `error_type`, `duration_ms`); the OpenRouter per-request lines are measurements, and the failure is a 4xx code, so the UI does not log it again. HTTP calls get `request_id` from the middleware; `ragx seed-dev` binds a generated `job_` id. The worker and other CLI commands call the registry only to read model handles (no provider call), so they add no id here.

`ragx seed-dev` assigns an offline model to the embedding role. Every KB copies its embedding model at creation, so KBs created before assigning a real embedding model stay bound to the offline model and cannot be embedded in M1.3. Assign the real embedding role before creating KBs you keep.

## Common failures
- `INTERNAL_ERROR` on Sync or Test with `vault decrypt failed` in logs → `RAG_VAULT_MASTER_KEY` changed; restore it or re-enter the provider key.
- `LLM_PROVIDER_REJECTED` → check the provider key or base URL. On the Embedding role it means the probe call failed: the model was not assigned and no dims were stored.
- `VALIDATION_FAILED` with `apiKey REQUIRED` on Sync → the provider is a keyed kind with no saved key (Edit adds one). OpenRouter and the catalog kinds list without a key.

## Model handles
`LLMProviderService.model_handle(model_id)` resolves the KB-selected model and decrypts provider access for one call. Missing IDs raise `LLM_MODEL_NOT_FOUND`. `role_handle(role)` resolves the current role or raises `LLM_ROLE_NOT_ASSIGNED`. `ModelHandle.litellm_model` uses the shared provider-name mapping. Handles contain `SecretStr` credentials and must never be logged.
