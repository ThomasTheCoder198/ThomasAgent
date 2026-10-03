# apperr

## Purpose
Owns catalog-backed application errors, localization and cause wrapping.
HTTP serialization belongs to httpx.

## Entry points
- `New` and options construct errors with catalog codes.
- `From` finds wrapped application errors or wraps unknown causes as INTERNAL_ERROR.
- `Status`, `Retryable`, and `LocalizedMessage` consult the generated catalog.
- `LangFromHeader` selects English for an English prefix; Vietnamese is the default.

## Dependencies
- Uses: `contracts/errors.yaml` through generated `codes_gen.go`, Go standard library.
- Used by: HTTP boundaries and future domain services.

## Run & test
```bash
task gen
cd backend-core
go test ./internal/platform/apperr/... -count=1
```

## Conventions
- Register codes in contracts and regenerate; never edit codes_gen.go.
- Unknown codes use INTERNAL_ERROR catalog status and messages.
- Custom messages override localization; httpx sanitizes all server errors.
- Keep causes for boundary logging and errors.Is/errors.As.
- Do not log errors here; log once at the boundary.

## Common failures
- Undefined catalog code: register it in contracts/errors.yaml and run task gen.
- Unexpected language: only the leading English language prefix selects English.
