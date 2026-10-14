# Providers

## Purpose
Provider adapters resolve per-call access and return catalog errors for the ingest and retrieval pipelines.

## Entry points
- `embedding.Embedder` defines the async embedding boundary.
- `embedding.LiteLLMEmbedder` calls `litellm.aembedding` through its module attribute so OpenInference observes requests, and sorts vectors by response index.
- `litellm_errors.to_app_error` maps authentication, invalid requests, missing models and permission refusals to non-retryable `LLM_PROVIDER_REJECTED`; rate limits, timeouts, outages and other failures become retryable `PROVIDER_UNAVAILABLE` (B19).

## Dependencies
Uses registry model handles, LiteLLM and the injected platform retry factory. Used by the ingest embedding cache.

## Run & test
```bash
uv run pytest -q tests/unit/test_embedder.py tests/unit/test_litellm_errors.py
```

## Conventions
Keys and base URLs are passed on each call; never assign LiteLLM global credentials. Logs contain provider kind, outcome and duration only; request context supplies `request_id`. Outbound RED metrics use only bounded provider kind and outcome labels. Call through the LiteLLM module attribute after process startup installs its instrumentor and content policy.

## Common failures
- Provider rejects access: correct the registry credential or model configuration; rejected calls are not retried.
- Provider unavailable: bounded retries use the injected platform retry policy and honor provider Retry-After headers.

## Configured metrics and response validation
`LiteLLMEmbedder(retrying_factory, metrics=recorder)` requires an injected `EmbeddingRecorder`; worker wiring supplies its configured `WorkerMetrics`. No global meter is used. Each response must contain exactly one index for every input position; duplicate, missing or out-of-range indices raise non-retryable `INGEST_EMBEDDING_MISMATCH`.

## Optional section summaries
`chat.ChatModel` defines completion; `chat.LiteLLMChat(retrying_factory, metrics=recorder)` calls the module-level `litellm.acompletion` with per-call model access, maps provider errors and uses platform retry including Retry-After. Completion logs contain kind, outcome and duration; worker context supplies request_id. No prompts, responses or provider error messages are logged. Inject the configured `WorkerMetrics` recorder when wiring the summarizer in Task 3b-6. Run `uv run pytest -q tests/unit/test_chat.py tests/unit/test_summaries.py`.
