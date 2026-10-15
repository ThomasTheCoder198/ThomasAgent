# Eval harness

## Purpose
Scores `kb_search` retrieval against a labelled golden set on a live KB: recall@k, MRR, citation locator accuracy, no-evidence rate, retrieval latency and ingest throughput. It calls `SearchService` in-process (no HTTP, no App token). It does not judge answers; answer quality waits for M6.

## Entry points
- `ragx eval run --set PATH --org ORG --kb KB_ID [--out PATH]`: prints a summary log line and writes the JSON report to `--out`.
- `golden.py`: `load_golden`, `normalize`, `GoldenItem`, `Expected`.
- `runner.py`: `EvalRunner(pool, search).run(org_id, kb, items) -> EvalReport`.
- `report.py`: `EvalReport.to_json()`, `IngestThroughput`, `percentile` (nearest rank).
- Uses `retrieval.factory.create_search_service`, the same construction the API uses, including the exported metrics recorder.

## Golden-set format
JSONL, one UTF-8 object per line: `id`, `lang` (`vi`|`en`), `category` (`A1`..`A6`), `question`, `expected` (list of `{"file", "quote"}`). An empty `expected` is excluded from recall and MRR. Format and the committed synthetic set: [tests/eval/README.md](../../../tests/eval/README.md). The owner's real set under git-ignored `docs/` uses the same format (O4).

## Metrics (ruling C11)
Text is compared after NFC normalization, lowercasing and whitespace collapsing.
- **hit**: a returned result's labelled blocks, joined with a space, contain an expected quote, and the result's file name equals the expected file. A quote that spans two blocks of one result therefore counts for recall and MRR, but not for locator accuracy.
- **recall@k** (k = 5 and 8): hits whose result number is at most k, over items with expectations (`scored`).
- **MRR**: mean of 1 / (number of the first result holding a hit); 0 for a miss.
- **locator accuracy**: for every expected quote that a single returned block contains (a quote spanning blocks is skipped), whether that block's `(blockId, locator)` equals a `doc_blocks` row of the same file containing the quote. `null` when nothing was cited.
- **no-evidence rate**: share of A1, A4 and A5 items answered with `insufficientEvidence: true` (`null` without such items). A fake reranker scores everything above the floor, so only a real reranker gives this number meaning.
- **unresolved**: item ids whose expected quote is not in any live block of that file: the golden set and the corpus disagree, fix the set before reading other numbers.
- **latency p50/p95**: end to end inside ragx per question, including query embedding, Qdrant, Postgres liveness check and the rerank call. It excludes HTTP and core.
- **ingest throughput**: files and pages per minute over the KB's ingested, non-deleted files, as the sum of `ingested_at - created_at` (presign to indexed, so browser upload time is included). Files are counted per file, not overlapped, so a parallel ingest run looks slower than wall time. `pages` is 0 for local text formats.

## Dependencies
- Uses: `SearchService` (`retrieval/`), Postgres (`doc_blocks`, `kb_files`), Qdrant, the model registry (embedding and rerank roles) and the configured LLM providers, through `retrieval.factory.create_search_service`.
- Used by: the `ragx eval run` CLI and `task ragx:eval`; the M6 baseline comparison.

## Run & test
Owner on a real KB (stack from `task up:ragx`; the worker container holds the keys and Qdrant access):
```bash
task ragx:eval SET=docs/golden/set.jsonl ORG=<org id> KB=<kb uuid> [OUT=docs/superpowers/reports/m1-3-eval.json]
```
The task copies the set into `ragx-worker`, runs `ragx eval run` there, and copies the report back (default `docs/superpowers/reports/m1-3-eval.json`, git-ignored). Tests: `uv run pytest -q tests/unit/test_eval_report.py tests/integration/test_eval_harness.py` from `ragx/`.

## Conventions
Results are M6 baselines, not gates yet. The integration test floors recall@5 at 0.7 with fake models only to catch breakage; do not tune thresholds to it. The CLI binds one `job_` request id and the org for the whole run and logs `eval finished` with scored count, recall@5, MRR and `duration_ms`, never questions or document text. A failure with a catalog code (`KB_NOT_FOUND`, `LLM_ROLE_NOT_ASSIGNED`) logs `eval failed` with `error_code` and exits 1 without a traceback. Any other failure (a malformed `--kb`, an unreachable dependency) logs one `eval failed` with `error_type` only, no message, and also exits 1. Metrics and tracing providers are shut down even when Postgres or Qdrant is unreachable. Only live files (`ready` or `degraded`, not deleted) count as ground truth, the same set search can return. `ragx eval run` also starts the metrics provider, so provider calls it makes export the same RED metrics as the API, flushed once on exit.

## Common failures
- `LLM_ROLE_NOT_ASSIGNED` → no `rerank` role assigned (C1) → assign one in the admin UI.
- `KB_NOT_FOUND` → `--org` and `--kb` do not match → check both ids.
- `unresolved` not empty → expected quote or file name is not in the KB → fix the set or re-ingest.
