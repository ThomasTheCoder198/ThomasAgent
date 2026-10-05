# retry

## Purpose
Provides shared synchronous retries with exponential backoff and full jitter,
and a circuit breaker for provider calls.

## Entry points
- `NewPolicy` builds a policy embedding `config.RetryConfig`; `Sleep` and `Rand` remain injectable test seams.
- `NewBreaker(name, config.BreakerConfig)` creates the named breaker directly from typed config.
- `Do` executes an operation up to `MaxAttempts`, including its initial call, stops after context cancellation, and rejects nonpositive attempt counts.
- `Backoff` computes the capped jittered delay for a zero-based retry attempt.
- `IsRetryable` classifies catalog errors, network errors and `RetryAfterProvider`.
- `NewBreaker` and `Breaker.Execute` gate provider calls with gobreaker; CanDiscard reports whether an inactive closed circuit has no consecutive failures for bounded catalog cache eviction.

## Dependencies
- Uses: `config`, `internal/errors`, Go context/time/network packages, gobreaker v2.4.0.
- Used by: future LLM, OCR and MCP provider adapters.

## Run & test
```bash
cd backend-core
go test ./internal/platform/retry/... -count=1
golangci-lint fmt ./...
golangci-lint run ./...
```

## Conventions
See the [shared naming glossary](../../../../docs/glossary.md) for terms used across services.
- Load and validate config before calling `NewPolicy`; custom policies must
  supply valid attempts/delays and non-nil sleep/random functions.
- Positive `RetryAfterProvider` delays set a minimum wait; `Policy.MaxDelay`
  caps generated backoff only.
- Catalog retryability takes precedence over `RetryAfterProvider` wrappers; a delay
  hint cannot cause non-retryable catalog errors to be retried.
- Named errors and per-call `*AppError` values share the same retry policy;
  the breaker returns `ErrProviderUnavailable.WithCause(err)` when calls are blocked.
- Context cancellation is non-retryable; the default wait is cancellable. Cancellation before an operation, during a failed operation, and failed waits return catalog PROVIDER_UNAVAILABLE AppError values while retaining context causes for errors.Is. An operation returning `nil` completes successfully even if its caller cancels the context in the same call; cancellation must not discard completed provider data or prompt a duplicate retry.
- Failed waits preserve operation, wait, and cancellation causes inside the catalog AppError.
- Per-attempt HTTP/client timeouts, including wrapped `context.DeadlineExceeded`, retry through `IsRetryable` while the parent context is alive. Only `ctx.Err()` stops retries for cancellation/deadline; the post-operation check runs after accepting a successful result. Terminal context errors map to `PROVIDER_UNAVAILABLE` with the original cause, and parent cancellation retains both operation and parent causes.
- Load and validate config before calling `NewBreaker`; see [config defaults](../config/README.md). Thresholds and half-open call limits must be at least one.
- Only consecutive retryable operation failures trip the breaker. Client errors
  count as successful calls so invalid requests cannot block valid traffic.
  Open and saturated half-open
  states return catalog `PROVIDER_UNAVAILABLE` with the library error as cause.
- Do creates a parented core.retry.execute span for every execution. Operations own outbound spans; handlers/workers own structured error logs.
- Operations with side effects must be safe to repeat or use idempotency keys.

## Common failures
- No retry: inspect catalog retryability and whether the error wraps cancellation.
- Provider unavailable before execution: the breaker is open or half-open capacity
  is exhausted; wait for the configured timeout before a probe.
- Long wait: provider Retry-After may exceed the backoff cap; cancellation or the
  request deadline interrupts the wait.
  Provider `Retry-After` sets a minimum wait, even when it exceeds the local
  backoff cap; cancellation and the enclosing request deadline still interrupt
  that wait. Retryability remains controlled by the shared error catalog.
