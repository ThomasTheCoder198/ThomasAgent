# ragx caller

## Purpose
The only way core calls ragx: the generated client (`client_gen.go`, oapi-codegen v2.8.0 from `contracts/openapi/ragx.v1.yaml`) wrapped by `Caller`, which signs an App token per call, sends `X-Request-Id`, maps errors and logs one "ragx call" line.

## Entry points
- `NewRagxCaller(cfg, signer, recorder, log)` builds the caller; `TokenSigner` is satisfied by `apptoken.Signer`.
- `Caller` methods: `ProvisionOrg`, `ListKnowledgeBases`, `CreateKnowledgeBase`, `GetKnowledgeBase`, `RenameKnowledgeBase`, `DeleteKnowledgeBase`, `ListFiles`, `UploadFile`, `DeleteFile`, `RetryFile`, `OpenFileRaw`, `OpenFilePreview`.
- `FilePage` and `Upload` are the paged-list result and the streamed upload input. The generated models (`Org`, `KnowledgeBase`, `KbFile`) are used as they are.

## Dependencies
- Uses: `apptoken`, `outbound.Instrument` (`outbound_*` metrics with `target="ragx"`), `config.RagxConfig`, `requestid`, `tenant`, the shared error catalog.
- Used by: organization/job and knowledgebase/service.

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
- 5xx, timeouts, unreachable ragx, 401/403 (a rejected App token or scope is core misconfiguration) and unknown codes or statuses become `RAGX_UNAVAILABLE`.
- Error bodies are read up to `CORE_RAGX_MAX_ERROR_BODY_BYTES`.
- JSON calls get `CORE_RAGX_REQUEST_TIMEOUT`; uploads and `OpenFile*` use the caller's context, which the HTTP route bounds with the transfer timeout. The caller of `OpenFileRaw` and `OpenFilePreview` closes the body.
- Uploads stream with the caller's `Content-Length` (ragx rejects chunked bodies).
- Redirects are never followed, and no ambient proxy is used.
- Never use `ClientWithResponses`: it buffers whole bodies.
- Generated names: `OrgId` is a string and `KbId`/`FileId` are UUIDs. The OpenAPI response components are `KbFileResponse` and `KnowledgeBaseResponse`, because `oapi-codegen.yaml` sets `resolve-type-name-collisions`.

## Common failures
- `client_gen.go` stale or missing → `task gen`.
- `RAGX_UNAVAILABLE` with cause "401 APP_TOKEN_INVALID" → `CORE_APP_SIGNING_KEY_ID` or the key does not match ragx's `RAG_CORE_PUBLIC_KEYS`.
- `RAGX_UNAVAILABLE` with cause "403" → the token scope does not cover the route; check the scope used by the method.
