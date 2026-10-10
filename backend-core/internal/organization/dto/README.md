# Organization DTO

## Purpose
JSON request and response bodies of the organization endpoints and their conversions.

## Entry points
- `CreateOrganizationRequest`, `UpdateOrganizationRequest` (`organization.go`)
- `OrganizationResponse`, `NewOrganizationResponse`, `NewOrganizationResponses`

## Dependencies
- Uses: `organization/model`
- Used by: `organization/controller`

## Run & test
```bash
cd backend-core
go build ./internal/organization/dto
```

## Conventions
`ragxErrorCode` is `null` when the stored code is empty; a list is never `null`.
