# Organization controller

## Purpose
HTTP handlers only: read the request, call the service, write a DTO.

## Entry points
- `OrganizationController` (`organization_controller.go`)

## Dependencies
- Uses: `organization/service`, `organization/dto`, `platform/httpx`
- Used by: `cmd/core`

## Run & test
```bash
cd backend-core
go build ./internal/organization/controller
```

## Conventions
The path id is passed to the service as a string: a malformed id must be `NOT_FOUND`, so `httpx.PathUUID` is not used. Covered over HTTP by `tests/integration/organization_http_test.go`.
