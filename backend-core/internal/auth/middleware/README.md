# Auth middleware

## Purpose
Checks session cookies, CSRF tokens and trusted tenant scope at the HTTP boundary.

## Entry points
`RequireSession`, `UserFrom`, cookie/header constants.

## Dependencies
Uses the auth service's `SessionAuthenticator` interface, auth model, shared HTTP response and named errors.

## Run & test
Covered by `go test ./internal/auth/...`.

## Conventions
Successful authentication sets user and tenant in the request context. Client values do not set tenant scope.

## Common failures
An absent session returns UNAUTHENTICATED; a mismatched CSRF token returns AUTH_CSRF_INVALID.
