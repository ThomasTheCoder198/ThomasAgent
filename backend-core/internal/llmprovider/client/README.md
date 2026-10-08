# LLM provider client

## Purpose
Calls to provider APIs. Today only `GET /models`, used to test a provider and to sync OpenRouter models.

## Entry points
- `ModelCatalog`, `HTTPModelCatalog` (`model_catalog.go`)
- Per-provider circuit breakers (`provider_breakers.go`)
- OpenRouter response mapping (`openrouter_models.go`)
- Status and Retry-After handling (`http_errors.go`)

## Dependencies
- Uses: `platform/outbound`, `platform/retry`, `platform/metrics`, `llmprovider/model`
- Used by: `llmprovider/service`, `cmd/core`

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/llmprovider/client -count=1
```

## Conventions
Generation calls arrive in M2 through Firebase Genkit and will live here too. Errors and logs never carry URLs, bodies or keys.
