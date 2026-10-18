# Conversation dto

## Purpose
All JSON request and response types of the Conversations API, and their conversion from `conversation/model` types.

## Entry points
- `RenameConversationRequest` (`conversation.go`)
- `ConversationSummaryResponse`, `NewConversationSummaryResponses`, `NewPageInfo`: list rows and `meta.page`. An empty list is `[]`, never `null`.
- `ChatScopeResponse`, `NewChatScopeResponse`: the index's `ChatScope`.
- `MessageResponse`, `ConversationDetailResponse`, `NewConversationDetailResponse`: messages are AI SDK `UIMessage`s whose `parts` and `metadata` are passed through as stored.

## Dependencies
- Uses: `conversation/model`, `platform/httpx` (`PageInfo`), `github.com/google/uuid`
- Used by: `conversation/controller`

## Run & test
```bash
cd backend-core
go build ./internal/conversation/dto
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run TestConversationHTTP_ -p 1 -count=1
```

## Conventions
DTOs carry no tenant id. Several responses are struct conversions of the `model` types (`ConversationSummaryResponse(c)`), so a field added to one side must be added to the other or the build fails. Services take `model` types only.
