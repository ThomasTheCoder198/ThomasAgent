# Auth handler

## Purpose
Owns login, logout and current-user HTTP routes and their cookie responses.

## Entry points
`NewAuthHandler`, `NewAuthHandlerWithTrustedProxies`, and `Mount`.

## Dependencies
Consumes the auth service interface, login limiter, auth DTOs, middleware and shared HTTP response helpers.

## Run & test
Covered by `go test ./internal/auth/...` and the generated API collection.

## Conventions
The handler reads request DTOs, returns response DTOs and logs credential failures without revealing credentials.

## Common failures
An untrusted forwarding header cannot choose the client IP; CSRF failures use the shared error response.
