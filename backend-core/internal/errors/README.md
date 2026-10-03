# errors

## Purpose
Owns the Go application's centralized error definitions and per-call errors.
Codes, HTTP statuses, retryability and VI/EN messages come from `contracts/errors.yaml`.
HTTP serialization and boundary logging belong to `platform/httpx` and callers.

## Entry points
- Generated `errors.go`: `ErrNotFound`, other named sentinels, `Code` constants and `LookupDefinition`.
- `ErrorDefinition`: explicit `HTTPStatus`, `Retryable`, `MessageVI` and `MessageEN` metadata.
- `ErrNotFound.WithCause(err)`, `.WithDetails(details)` and `.WithMessage(message)` construct fresh `*AppError` values.
- `From`: finds wrapped application errors and normalizes named sentinels; unknown causes become INTERNAL_ERROR.
- `Status`, `Retryable`, `LocalizedMessage` and `LangFromHeader`: policy and localization for boundaries.

## Dependencies
- Uses: generated contracts and the Go standard library.
- Used by: HTTP boundaries, retry, command startup and domain services.
- `platform/apperr` delegates its existing API here for compatibility.

## Run & test
```bash
task gen
cd backend-core
go test ./internal/errors ./internal/platform/apperr ./internal/platform/httpx ./internal/platform/retry -count=1
```

## Conventions
```go
import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"

return errors.ErrNotFound
// Preserve the cause for the caller's boundary log.
return errors.ErrProviderUnavailable.WithCause(err)
```
Named errors are typed constants, so a request cannot mutate a shared error.
`WithCause`, `WithMessage` and `WithDetails` return fresh values; details maps are copied shallowly.
Standard `errors.Is` matches application codes and still traverses causes.
Standard `errors.As` can normalize a sentinel into `*AppError`; `From` uses the same behavior.
Generated lookup uses an immutable switch rather than a mutable package-wide map.
Unknown codes retain their code but use INTERNAL_ERROR status, retryability and messages.
Never edit `errors.go` manually or log here; log once at the caller's boundary.

## Common failures
- Missing named error: add its definition to `contracts/errors.yaml` and run `task gen`.
- Import name conflict: alias the standard library package to `stderrors` when both are needed.
- A custom 5xx message must remain private: serialize via `httpx.WriteError`.
