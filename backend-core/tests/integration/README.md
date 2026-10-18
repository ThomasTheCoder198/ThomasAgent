# Integration tests

## Purpose
HTTP tests that cross layers and domains: real login, session, CSRF, rate limiting, event streams, the provider API organization provisioning and the app proxy.

## Entry points
- `auth_*_test.go`: auth flows
- `llmprovider_*_test.go`: provider routes with a real login
- `org_roles_test.go`: per-org model roles (org row → platform row → decision→chat.fast), the org role routes, and clearing a platform role
- `model_capabilities_test.go`: `PATCH /api/v1/models/{modelId}`: an edit survives sync, input validation, and the guard on roles still pinned to the model
- `appproxy_http_test.go`: the generic ragx app proxy (scope, paths, headers, error table, session and CSRF) against the same fake
- `chat_repository_test.go`: agent, conversation, message and run repositories against Postgres, including tenant scope, cross-tenant parent inserts (repository and composite foreign keys) and the `ListRange` bounds; `chat_migration_test.go` runs the agents/chat migration down and up again on a throwaway container; `chat_fixture_test.go` holds the shared seed helpers (`seedReadyOrg`, `seedUser`, `orgContext`, `conversationContext`)
- `chat_store_test.go`: the `ChatStore` lifecycle (conversation, messages, runs and steps, history with `PeakInputTokens`, `ListMessagesRange`, summary, regenerate drop) stale-run expiry, concurrent Agent mismatch and same-org user ownership; `conversation_http_test.go`: the Conversations API (paging, get with scope, rename, delete and same-org user isolation); `tenant_isolation_test.go`: org B cannot list, read, change or claim org A's agents, conversations and runs (over HTTP and through every `ChatStore` writer), and the org-role, agent and conversation routes run only behind the real `RequireOrg`
- `organization_http_test.go`: organization routes against a fake ragx; `ragx_fake_test.go` is that fake (it verifies every App token with the shared vector key from `contracts/app-token.json`, like ragx, and is reused by the proxy tests)

## Dependencies
- Uses: every domain, Docker Postgres and Redis
- Used by: CI (`task test:core`)

## Run & test
```bash
cd backend-core
TESTCONTAINERS_RYUK_DISABLED=true go test ./tests/integration -count=1
```

## Conventions
Unit tests belong next to their code; only tests that need several layers wired together go here.
