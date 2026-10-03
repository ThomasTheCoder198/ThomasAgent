# retry

## Purpose
Provides shared synchronous retries with exponential backoff and full jitter,
and a circuit breaker for provider calls.

## Entry points
- `FromConfig` builds a policy from typed `config.RetryConfig`.
- `Do` executes an operation up to `MaxAttempts`, including its initial call.
- `Backoff` computes the capped jittered delay for a zero-based retry attempt.
- `IsRetryable` classifies catalog errors, network errors and `RetryAfterer`.
- `NewBreaker` and `Breaker.Execute` gate provider calls with gobreaker.

## Dependencies
- Uses: `config`, `apperr`, Go context/time/network packages, gobreaker v2.4.0.
- Used by: future LLM, OCR and MCP provider adapters.

## Run & test
```bash
cd backend-core
go test ./internal/platform/retry/... -count=1
golangci-lint run ./...
```

## Conventions
- Load and validate config before calling `FromConfig`; custom policies must
  supply valid attempts/delays and non-nil sleep/random functions.
- Positive `RetryAfterer` delays override backoff, including its maximum.
- Catalog retryability takes precedence over `RetryAfterer` wrappers; a delay
  hint cannot cause non-retryable catalog errors to be retried.
- Context cancellation is non-retryable; the default wait is cancellable.
- Failed waits preserve both operation and wait errors through `errors.Join`.
- Breaker settings are supplied by callers; use explicit positive thresholds,
  open timeouts and half-open call limits to avoid library defaults.
- Consecutive operation failures trip the breaker. Open and saturated half-open
  states return catalog `PROVIDER_UNAVAILABLE` with the library error as cause.
- Operations own outbound spans; handlers/workers own structured error logs.
- Operations with side effects must be safe to repeat or use idempotency keys.

## Common failures
- No retry: inspect catalog retryability and whether the error wraps cancellation.
- Provider unavailable before execution: the breaker is open or half-open capacity
  is exhausted; wait for the configured timeout before a probe.
- Unexpected long wait: a positive provider Retry-After takes precedence over
  the configured backoff cap.
