# Call log

## Purpose
Writes one structured event per model call (`llm.call`) and per tool call (`tool.call`), and records the LLM
metrics. It does not call models or tools; M1 (ragx tools), M2 (agent loop) and M3a (Apps) call it.

## Entry points
- `New(log, recorder, cfg)` — built once at startup with the shared logger, the metrics registry and `config.LLMLogConfig`.
- `LLM(ctx, LLMCall)` — after every model call, including failed attempts (`WillRetry` marks a retry).
- `Tool(ctx, ToolCall)` — after every tool call; pass the `X-Ragx-Trace-Id` response header as `RagxTraceID`.

## Dependencies
- Uses: slog (through `logging.NewLogger`), `internal/platform/metrics`, `internal/platform/config`, `internal/platform/outbound` (`IsRagxTraceID`).
- Used by: future model and tool callers.

## Run & test
```bash
cd backend-core
go test ./internal/platform/calllog -count=1
```

## Conventions
Metadata is always logged; prompt/response/tool content only when `CORE_LLM_LOG_CONTENT=true`, each field redacted with the shared logger's policy before being cut to
`CORE_LLM_LOG_CONTENT_MAX_BYTES` on a UTF-8 boundary (`truncated=true`). The shared logger also redacts secrets and adds
`request_id`. Levels: success info, retry warn, failure error (tool failures warn). `llm_usage` in Postgres stays
the source of truth for cost. A ragx trace id is logged only in its 32-hex form (`outbound.IsRagxTraceID`, the same check `internal/platform/ragx` uses); Grafana links it to Phoenix.

Complete JSON content is decoded with number precision retained, recursively redacted using the shared key/value policy, then serialized and truncated. Plain text uses the shared value policy. This applies to prompt, response, tool arguments and tool results when content logging is enabled.

Structured JSON uses the shared camelCase/acronym secret policy, including `accessToken`, `refreshToken`, `clientSecret`, `APIKey` and `privateKey`; token usage counters retain their numeric values.

## Common failures
- Prompts missing from logs → the content switch is off (default) → set `CORE_LLM_LOG_CONTENT=true` while debugging.
