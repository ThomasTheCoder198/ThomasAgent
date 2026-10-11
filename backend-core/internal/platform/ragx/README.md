# ragx caller

## Purpose
The only way core calls ragx: the generated client (`client_gen.go`, oapi-codegen v2.8.0 from `contracts/openapi/ragx.v1.yaml`) wrapped by `Caller`, which signs an App token per call, sends `X-Request-Id`, maps errors and logs one "ragx call" line.

## Entry points
- `NewRagxCaller(cfg, signer, recorder, log)` builds the caller; `TokenSigner` is satisfied by `apptoken.Signer`.
- `Caller.ProvisionOrg` is the typed call, bounded by `CORE_RAGX_REQUEST_TIMEOUT`.
- Header names and the Idempotency-Key format come from `httpx`, not local copies.
- `Caller.Forward(ctx, ForwardRequest)` is the generic data plane behind the app proxy: it uses the route's deadline and caps the response at `CORE_RAGX_MAX_RESPONSE_BYTES`. It is never retried.
- The generated model `Org` is used as it is.

## Dependencies
- Uses: `apptoken`, `outbound.Instrument` (`outbound_*` metrics with `target="ragx"`), `config.RagxConfig`, `requestid`, `tenant`, the shared error catalog.
- Used by: `organization/service`, `appproxy/service`.

## Run & test
```bash
task gen:ragx-client
cd backend-core
go test ./internal/platform/ragx/... -count=1
```

## Conventions
- `oid` is `tenant.ID(ctx)`; a context without a trusted tenant fails with `ErrMissingTenant` before any request.
- `rid` equals the `X-Request-Id` sent: both come from one context value, and a context without a valid id gets a `job_` id first.
- ragx 4xx catalog codes pass through with their details, provided the code's catalog status matches the response status.
- Typed calls: 5xx, timeouts, unreachable ragx, 401/403 (a rejected App token or scope is core misconfiguration) and unknown codes or statuses become `RAGX_UNAVAILABLE`.
- `Forward`: 2xx and catalog 4xx pass through verbatim; 401 becomes `RAGX_UNAVAILABLE`; 403 becomes `NOT_FOUND` (the route needs a scope the UI never gets); a HEAD answer (which never has a body) with any other 4xx passes through as a bare status; everything else, including a body over the cap, becomes `RAGX_UNAVAILABLE`.
- The log line carries `jti` always, and `method` and `route` (contract brace spelling) for `Forward`; never the token or a body.
- Error bodies are read up to `CORE_RAGX_MAX_ERROR_BODY_BYTES`.
- Typed calls get `CORE_RAGX_REQUEST_TIMEOUT`; `Forward` uses the caller's context, which the HTTP route bounds.
- Redirects are never followed, and no ambient proxy is used.
- Never use `ClientWithResponses`: it buffers whole bodies.
- Generated names: `OrgId` is a string and `KbId`/`FileId` are UUIDs. The OpenAPI response components are `KbFileResponse` and `KnowledgeBaseResponse`, because `oapi-codegen.yaml` sets `resolve-type-name-collisions`.

## Common failures
- `client_gen.go` stale or missing → `task gen`.
- `RAGX_UNAVAILABLE` with cause "401 APP_TOKEN_INVALID" → `CORE_APP_SIGNING_KEY_ID` or the key does not match ragx's `RAG_CORE_PUBLIC_KEYS`.
- `RAGX_UNAVAILABLE` with cause "403" → the token scope does not cover the route; check the scope used by the method.
