# LLM provider model

## Purpose
Domain types of providers, models, capabilities and roles, plus the rules that belong to them. No I/O.

## Entry points
- `Provider`, `ProviderInput`, `StoredProvider`, `ProviderAccess` (`provider.go`)
- `LLMModel` (`CapabilitiesEdited`: the operator's edit that sync keeps), `LLMModelInput`, `FitsRole` (`llm_model.go`)
- `Capability` (`capability.go`); `Role`, `RequirementsForRole`, `CoreChatRoles`, `ResolvedModel`, `OrgRoleView`, `ResolvedSource*` (`role.go`)
- `RemoteModel` (`remote_model.go`), `SyncResult` (`sync_result.go`)

## Dependencies
- Uses: `github.com/google/uuid`
- Used by: every other llmprovider layer

## Run & test
```bash
cd backend-core
go build ./internal/llmprovider/model
```

## Conventions
`StoredProvider` carries the vault reference and never leaves the service layer.
