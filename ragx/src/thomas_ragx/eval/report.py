import math
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Any, Final

P50: Final = 0.5
P95: Final = 0.95
SECONDS_PER_MINUTE: Final = 60.0


def percentile(values: Sequence[float], fraction: float) -> float:
    ordered = sorted(values)
    return ordered[max(math.ceil(fraction * len(ordered)) - 1, 0)]


@dataclass(frozen=True)
class IngestThroughput:
    files: int
    pages: int
    files_per_minute: float
    pages_per_minute: float


@dataclass(frozen=True)
class ItemOutcome:
    id: str
    category: str
    lang: str
    hit_rank: int | None
    locator_matches: tuple[bool, ...]
    latency_ms: float
    insufficient_evidence: bool


@dataclass(frozen=True)
class EvalReport:
    items: int
    scored: int
    recall: dict[int, float]
    mrr: float
    locator_accuracy: float | None
    no_evidence_rate: float | None
    unresolved: list[str]
    latency_ms_p50: float
    latency_ms_p95: float
    ingest: IngestThroughput
    outcomes: list[ItemOutcome] = field(default_factory=list)

    def to_json(self) -> dict[str, Any]:
        return {
            "items": self.items,
            "scored": self.scored,
            "recall": {str(k): v for k, v in self.recall.items()},
            "mrr": self.mrr,
            "locatorAccuracy": self.locator_accuracy,
            "noEvidenceRate": self.no_evidence_rate,
            "unresolved": self.unresolved,
            "latencyMs": {"p50": self.latency_ms_p50, "p95": self.latency_ms_p95},
            "ingest": {
                "files": self.ingest.files,
                "pages": self.ingest.pages,
                "filesPerMinute": self.ingest.files_per_minute,
                "pagesPerMinute": self.ingest.pages_per_minute,
            },
            "outcomes": [
                {
                    "id": o.id,
                    "category": o.category,
                    "lang": o.lang,
                    "hitRank": o.hit_rank,
                    "locatorMatches": list(o.locator_matches),
                    "latencyMs": o.latency_ms,
                    "insufficientEvidence": o.insufficient_evidence,
                }
                for o in self.outcomes
            ],
        }
