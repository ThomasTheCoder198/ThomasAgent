# Conversation repository

## Purpose
SQL for the `conversations`, `messages`, `runs` and `run_steps` tables. Every method takes the pool or a service transaction and scopes conversation rows by the tenant and authenticated user in ctx. Conversation insertion derives its owner from ctx and refuses a conflicting supplied owner.
The system-only `PurgeInactiveAcrossTenants` deletes a bounded batch of inactive conversations while skipping rows with a running run. Child rows cascade with each deletion.

## Entry points
- `ConversationRepo` (`conversation_repository.go`): `ListPage` returns up to `limit+1` rows so the caller can tell a next page exists.
- `MessageRepo` (`message_repository.go`): `ListLatest`, `ListAll` and `ListRange` return oldest first.
- `RunRepo` (`run_repository.go`): `ExpireStale` fails a `running` row after its deadline plus the configured finish window (`RUN_DEADLINE_EXCEEDED`), or earlier when its lease lapses (`RUN_INTERRUPTED`); a live lease remains valid through the finish window. `PeakInputTokens` is the largest `input_tokens` of a `model` step in the conversation's latest finished run (0 when none), the input of M2b's compaction trigger; `LatestRun` is the newest run of a conversation (for `lastRun`).
- Lease-fenced run writes (amendment W5), all compared with the database `now()`: `ClaimRun` (fresh token per attempt, tenant and conversation owner checked), `ClaimState` (explains a refused claim), `RenewLease`, `InsertFencedStep` (holds the run row `FOR UPDATE` for the statement), `FinishFencedRun` (also clears the lease) and `FailRun` (the interrupted / queued-past-deadline transition; `onlyIfExpired` also needs a lapsed lease). They return `ok=false` when no row matched; the service turns that into `RUN_LEASE_LOST`.

## Dependencies
- Uses: `conversation/model`, `platform/postgres`, `platform/tenant`, `auth/middleware`
- Used by: the conversation service and chat store

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -run "TestChatRepositories|TestChatMigration|TestRunLease_" -p 1 -count=1
```

## Conventions
Every insert selects its parent row under the tenant in ctx, so a child can never attach to another org's parent: `InsertConversation` answers `AGENT_NOT_FOUND` for a foreign agent, `InsertMessage` and `InsertRun` answer `CONVERSATION_NOT_FOUND` for a foreign conversation, and `InsertStep` answers `RUN_NOT_ACTIVE` for a foreign run. The composite foreign keys `(parent_id, tenant_id)` of the migration make the database refuse the same writes. A missing or foreign conversation is `CONVERSATION_NOT_FOUND`; a missing or foreign run is `RUN_NOT_ACTIVE`. A repeated message id in one conversation is `CONFLICT` on `id`. A second `running` run is `RUN_ALREADY_ACTIVE`, recognised by the constraint name `uq_runs_one_active`.
