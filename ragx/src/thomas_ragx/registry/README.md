# Model registry

## Purpose
Ragx owns its model registry: providers with API keys sealed under `RAG_VAULT_MASTER_KEY`, models with capabilities, and one model per role. Core's registry is separate; ragx never asks core for a model.

## Entry points
- `RegistryService` — provider, model, and role operations for the admin UI and KB service.
- `seed_dev_registry` — idempotent offline embedding model for development.
- `LISTING_ROUTES` — provider kind to model-listing SDK route.

## Dependencies
- Uses: LiteLLM 1.101.2 for supported provider listing, model facts, and test calls; openai 2.54.0 for OpenRouter and OpenAI-compatible listing; cryptography AES-256-GCM for key storage; ragx Postgres.
- Used by: the KB service for the embedding role and the admin UI.

## Run & test
```bash
cd ragx
uv run pytest -q tests/unit/test_vault.py tests/unit/test_listing.py tests/unit/test_model_info.py tests/integration/test_registry_service.py
```

## Conventions
Keys are write-only. `Provider` exposes only `has_api_key`; `ProviderAccess` holds a `SecretStr` for one call. `LISTING_ROUTES` is data: Azure, Cohere, Voyage and Jina are manual (`LLM_PROVIDER_SYNC_UNSUPPORTED`). LiteLLM's `get_valid_models` is avoided because it returns an empty list for a bad key. Sync preserves manual rows and operator-edited capabilities. Test runs per model: embedding uses `aembedding`, rerank uses `arerank`, chat uses `acompletion` with `max_tokens=1`. LiteLLM is imported lazily after `apply_litellm_policy`, using its bundled model map without downloading one. The typesafe kind arrives with the decision role in M6.

`ragx seed-dev` assigns an offline model to the embedding role. Every KB copies its embedding model at creation, so KBs created before assigning a real embedding model stay bound to the offline model and cannot be embedded in M1.3. Assign the real embedding role before creating KBs you keep.

## Common failures
- `INTERNAL_ERROR` on Sync or Test with `vault decrypt failed` in logs → `RAG_VAULT_MASTER_KEY` changed; restore it or re-enter the provider key.
- `LLM_PROVIDER_REJECTED` → check the provider key or base URL.
