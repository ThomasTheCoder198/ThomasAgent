# Conversation controller

## Purpose
HTTP handlers only: read the request, call one service, write a DTO.

## Entry points
- `ConversationController` / `NewConversationController` (`conversation_controller.go`): `HandleListConversations`, `HandleGetConversation`, `HandleRenameConversation`, `HandleDeleteConversation` for `/api/v1/orgs/{orgId}/conversations[/{conversationId}]`, mounted inside `RequireOrg`. UI wired in 2c-x.

## Dependencies
- Uses: `conversation/service`, `conversation/dto`, `platform/config` (paging), `platform/httpx`, named catalog errors
- Used by: `cmd/core`

## Run & test
```bash
cd backend-core
go build ./internal/conversation/controller
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestConversationHTTP_|TestTenantIsolation_" -p 1 -count=1
```

## Conventions
A malformed conversation id answers `CONVERSATION_NOT_FOUND`, like a missing one. The list parses `limit` with `service.ParseLimit` and answers through `httpx.RespondPage`; rename and delete answer 204. Bodies are decoded with `httpx.DecodeJSON` (unknown fields refused, size capped).
