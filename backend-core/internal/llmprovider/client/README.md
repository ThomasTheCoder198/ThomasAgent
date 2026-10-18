# LLM provider client

## Purpose
Calls to provider APIs. Today only the model list, used to test a provider and to sync models. Model facts come only from what each provider's API publishes (O8); there is no hand-maintained catalog.

## Entry points
- `ModelCatalog`, `HTTPModelCatalog` (`model_catalog.go`): one `ListModels` that picks the request and decoder by provider kind
- Per-provider circuit breakers (`provider_breakers.go`)
- OpenRouter response mapping and decision models (`openrouter_models.go`)
- Google native list mapping (`google_models.go`)
- Status and Retry-After handling (`http_errors.go`)

| Kind | List | Facts |
|---|---|---|
| openrouter | `GET /models` + `GET /models?output_modalities=decisions` | capabilities, context, prices; decision models get `[decision]` |
| google | native `GET /models?pageSize=1000` with `x-goog-api-key` | context; `generateContent` → chat, `embedContent` → embedding; no tools flag, no prices |
| others | `GET /models` (Bearer) | names only |

## Dependencies
- Uses: `platform/outbound`, `platform/retry`, `platform/metrics`, `llmprovider/model`
- Used by: `llmprovider/service`, `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/llmprovider/client -count=1
```

## Conventions
Generation calls arrive in M2 through Firebase Genkit and will live here too. Errors and logs never carry URLs, bodies or keys; the key travels in a header (`Authorization: Bearer`, or `x-goog-api-key` for Google), never in the URL.

- **Decision list (ruling A2-5).** OpenRouter's `output_modalities=decisions` list is alpha. It is fetched once, outside the circuit breaker and without retry; a failure only logs `provider.list_models` with `decisions=false` and the chat models still sync. A model in both lists keeps only the `decision` capability, with the decision list's context and prices. The chat list and the decision list each record one outbound observation.
- **Google (ruling A2-6).** One page of up to 1000 models; `nextPageToken` is ignored on purpose. A model whose methods are neither `generateContent` nor `embedContent` has no capability in the catalog result (sync drops such a model, see below).
- **Sync keeps only chat models from Google.** The catalog returns every Google model with the capabilities its methods give; `ModelSyncService` then drops models that do not generate content and any embedding capability (ruling F2a3-1), because core roles never use embeddings and the list has no dimensions.
- **Google has no tools flag.** Its list does not publish tool support, so a Google model gets `chat` without `tools` and does not fit `chat.default`. The operator adds `tools` by editing the model (`PATCH /api/v1/models/{modelId}`, the operator's edit survives later syncs); the Settings page shows the gap. This is O8 as decided.
