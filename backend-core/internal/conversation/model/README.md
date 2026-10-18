# Conversation model

## Purpose
Domain types of conversations, messages, runs and run steps, plus the paging types of the conversation list. No I/O.

## Entry points
- `Conversation`, `Message`, `History`, `ConversationSummary`, `ConversationDetail`, `Scope`, `Page`, `PageMeta` (`conversation.go`)
- `Run`, `RunStep`, `RunStatus*`, `StepKind*`, `StepOutcome*` (`run.go`)

## Dependencies
- Uses: `github.com/google/uuid`
- Used by: `conversation/repository` and the later conversation layers

## Run & test
```bash
cd backend-core
go build ./internal/conversation/...
```

## Conventions
`Message.Parts` and `Metadata` are raw JSON, stored and returned verbatim. The status, kind and outcome values mirror the CHECK constraints of migration `20261017000001`.
