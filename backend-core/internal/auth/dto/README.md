# Auth DTO

## Purpose
JSON request and response bodies of the auth endpoints and their conversions from model types.

## Entry points
- `LoginRequest`, `LoginResponse`, `LogoutResponse` (`login.go`)
- `UserResponse`, `MeResponse` (`user.go`)

## Dependencies
- Uses: `auth/model`
- Used by: `auth/controller`

## Run & test
```bash
cd backend-core
go build ./internal/auth/dto
```

## Conventions
Never pass a DTO into the service layer; responses expose only id, email and display name.
