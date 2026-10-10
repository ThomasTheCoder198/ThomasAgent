# Organization model

## Purpose
Domain types of organizations and the display-name rule. No I/O.

## Entry points
- `Organization`, `Settings`, `RagxStatus` and its three values (`organization.go`)
- `OrganizationInput`, `OrganizationPatch`
- `NormalizeName`, `NameMaxLength`

## Dependencies
- Uses: `internal/errors`, `github.com/google/uuid`
- Used by: every other organization layer

## Run & test
```bash
cd backend-core
go test ./internal/organization/model -count=1
```

## Conventions
`Settings` is what core sends to ragx; the repository compares it with the row before recording a sync result. The 200-character limit counts characters, as ragx's does.
