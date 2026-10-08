# Auth model

## Purpose
Domain types of login: users, stored sessions and freshly issued sessions, plus the rules that belong to them (email normalization, token hashing). No I/O.

## Entry points
- `User`, `NormalizeEmail` (`user.go`)
- `Session`, `IssuedSession`, `HashSessionToken` (`session.go`)

## Dependencies
- Uses: the standard library only
- Used by: every other auth layer

## Run & test
```bash
cd backend-core
go build ./internal/auth/model
```

## Conventions
Only the SHA-256 of a session token is ever stored or looked up.
