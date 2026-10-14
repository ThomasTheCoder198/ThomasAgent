from collections.abc import Callable, Sequence
from time import perf_counter
from typing import Any, Final
from uuid import UUID

from thomas_ragx.db.pool import Pool
from thomas_ragx.eval.golden import Expected, GoldenItem, normalize
from thomas_ragx.eval.report import (
    P50,
    P95,
    SECONDS_PER_MINUTE,
    EvalReport,
    IngestThroughput,
    ItemOutcome,
    percentile,
)
from thomas_ragx.kb.model import KnowledgeBase
from thomas_ragx.retrieval.search_service import (
    SEARCHABLE_STATUSES,
    SearchAnswer,
    SearchFilters,
    SearchService,
)

NO_EVIDENCE_CATEGORIES: Final = frozenset({"A1", "A4", "A5"})
MILLISECONDS_PER_SECOND: Final = 1000.0
DEFAULT_KS: Final = (5, 8)


class EvalRunner:
    """Metrics of ruling C11; quotes resolve to blocks at run time, so parser changes cannot hide."""

    def __init__(
        self,
        pool: Pool,
        search: SearchService,
        *,
        ks: Sequence[int] = DEFAULT_KS,
        clock: Callable[[], float] = perf_counter,
    ) -> None:
        self._pool = pool
        self._search = search
        self._ks = tuple(ks)
        self._clock = clock

    async def _blocks_by_file(self, org_id: str, kb_id: UUID) -> dict[str, list[dict[str, Any]]]:
        async with self._pool.connection() as conn:
            rows = await (
                await conn.execute(
                    b"SELECT f.name, b.file_id, b.block_id, b.text, b.locator FROM doc_blocks b "
                    b"JOIN kb_files f ON f.id = b.file_id "
                    b"WHERE b.org_id = %s AND b.kb_id = %s AND f.deleted_at IS NULL AND f.status = ANY(%s)",
                    (org_id, kb_id, SEARCHABLE_STATUSES),
                )
            ).fetchall()
        found: dict[str, list[dict[str, Any]]] = {}
        for row in rows:
            found.setdefault(row["name"], []).append(dict(row))
        return found

    async def _throughput(self, org_id: str, kb_id: UUID) -> IngestThroughput:
        async with self._pool.connection() as conn:
            row = await (
                await conn.execute(
                    b"SELECT count(*) AS files, coalesce(sum(page_count), 0) AS pages, "
                    b"coalesce(sum(extract(epoch FROM ingested_at - created_at)), 0) AS seconds "
                    b"FROM kb_files "
                    b"WHERE org_id = %s AND kb_id = %s AND ingested_at IS NOT NULL AND deleted_at IS NULL",
                    (org_id, kb_id),
                )
            ).fetchone()
        assert row is not None  # noqa: S101 - an aggregate always returns one row
        minutes = float(row["seconds"]) / SECONDS_PER_MINUTE or 1.0
        return IngestThroughput(
            int(row["files"]), int(row["pages"]), int(row["files"]) / minutes, int(row["pages"]) / minutes
        )

    @staticmethod
    def _hit(answer: SearchAnswer, expected: Sequence[Expected]) -> int | None:
        for result in answer.results:
            text = normalize(" ".join(block.text for block in result.blocks))
            if any(e.file == result.file_name and normalize(e.quote) in text for e in expected):
                return result.source
        return None

    @staticmethod
    def _locator_matches(
        answer: SearchAnswer, expected: Sequence[Expected], blocks: dict[str, list[dict[str, Any]]]
    ) -> tuple[bool, ...]:
        matches = []
        for e in expected:
            truth = [
                (b["block_id"], b["locator"])
                for b in blocks.get(e.file, [])
                if normalize(e.quote) in normalize(b["text"])
            ]
            cited = [
                (block.block_id, block.locator)
                for r in answer.results
                if r.file_name == e.file
                for block in r.blocks
                if normalize(e.quote) in normalize(block.text)
            ]
            if cited:
                matches.append(any(c in truth for c in cited))
        return tuple(matches)

    async def run(self, org_id: str, kb: KnowledgeBase, items: Sequence[GoldenItem]) -> EvalReport:
        blocks = await self._blocks_by_file(org_id, kb.id)
        unresolved = [
            item.id
            for item in items
            for e in item.expected
            if not any(normalize(e.quote) in normalize(b["text"]) for b in blocks.get(e.file, []))
        ]
        outcomes = []
        for item in items:
            started = self._clock()
            answer = await self._search.search(org_id, kb, item.question, SearchFilters())
            latency = (self._clock() - started) * MILLISECONDS_PER_SECOND
            outcomes.append(
                ItemOutcome(
                    item.id,
                    item.category,
                    item.lang,
                    self._hit(answer, item.expected) if item.expected else None,
                    self._locator_matches(answer, item.expected, blocks),
                    latency,
                    answer.insufficient_evidence,
                )
            )
        return self._report(items, outcomes, unresolved, await self._throughput(org_id, kb.id))

    def _report(
        self,
        items: Sequence[GoldenItem],
        outcomes: list[ItemOutcome],
        unresolved: list[str],
        ingest: IngestThroughput,
    ) -> EvalReport:
        scored = [o for o, i in zip(outcomes, items, strict=True) if i.expected]
        recall = {
            k: sum(1 for o in scored if o.hit_rank is not None and o.hit_rank <= k) / max(len(scored), 1)
            for k in self._ks
        }
        mrr = sum(1 / o.hit_rank for o in scored if o.hit_rank) / max(len(scored), 1)
        matches = [m for o in scored for m in o.locator_matches]
        no_evidence = [o for o in outcomes if o.category in NO_EVIDENCE_CATEGORIES]
        latencies = [o.latency_ms for o in outcomes] or [0.0]
        return EvalReport(
            items=len(items),
            scored=len(scored),
            recall=recall,
            mrr=mrr,
            locator_accuracy=sum(matches) / len(matches) if matches else None,
            no_evidence_rate=(
                sum(o.insufficient_evidence for o in no_evidence) / len(no_evidence) if no_evidence else None
            ),
            unresolved=sorted(set(unresolved)),
            latency_ms_p50=percentile(latencies, P50),
            latency_ms_p95=percentile(latencies, P95),
            ingest=ingest,
            outcomes=outcomes,
        )
