import asyncio
import json
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass
from time import perf_counter
from typing import Any, Final, Protocol, cast

import structlog
from redis.asyncio import Redis
from redis.exceptions import ResponseError

from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.request_id import is_valid_request_id, new_job_id
from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.worker.metrics import OUTCOME_DEAD_LETTER, OUTCOME_RETRY, OUTCOME_SUCCESS
from thomas_ragx.worker.relay import FIELD_ORG_ID, FIELD_OUTBOX_ID, FIELD_PAYLOAD, FIELD_REQUEST_ID

CONSUMER_GROUP: Final = "ragx-worker"
DEAD_LETTER_SUFFIX: Final = ".dlq"
FIELD_ERROR: Final = "error"
FIELD_DELIVERIES: Final = "deliveries"
FIELD_ORIGINAL_ID: Final = "original_id"
FIELD_ATTEMPT: Final = "attempt"
BUSY_GROUP_PREFIX: Final = "BUSYGROUP"
GROUP_START_ID: Final = "0"
NEW_MESSAGES_ID: Final = ">"
AUTOCLAIM_START_ID: Final = "0-0"
log = structlog.get_logger(__name__)


@dataclass(frozen=True)
class StreamMessage:
    stream: str
    entry_id: str
    outbox_id: str
    org_id: str
    request_id: str
    payload: Mapping[str, Any]
    deliveries: int


type Handler = Callable[[StreamMessage], Awaitable[None]]
type DeadLetterHook = Callable[[StreamMessage, str], Awaitable[None]]


class JobRecorder(Protocol):
    def observe_job(self, stream: str, outcome: str, duration_seconds: float) -> None: ...


def _request_id(fields: Mapping[str, str]) -> str:
    # Only well-formed ids are trusted; anything else gets a fresh job id (same rule as core's consumer).
    value = fields.get(FIELD_REQUEST_ID)
    return value if is_valid_request_id(value) else new_job_id()


def _error_code(exc: Exception) -> str:
    return exc.code.value if isinstance(exc, AppError) else type(exc).__name__


def _should_retry(exc: Exception) -> bool:
    # Catalog errors say whether they are retryable; anything else (driver, network) retries too (rule 5).
    return exc.retryable() if isinstance(exc, AppError) else True


def _attempt(fields: Mapping[str, str]) -> int:
    try:
        return max(int(fields.get(FIELD_ATTEMPT, "1")), 1)
    except ValueError:
        return 1


def no_delay(_: int) -> float:
    return 0.0


class StreamConsumer:
    def __init__(
        self,
        redis: Redis,
        stream: str,
        consumer_name: str,
        max_deliveries: int,
        visibility_timeout_ms: int,
        block_ms: int,
        batch_size: int,
        metrics: JobRecorder | None = None,
        on_dead_letter: DeadLetterHook | None = None,
        retry_delay: Callable[[int], float] = no_delay,
    ) -> None:
        self.stream = stream
        self._redis = redis
        self._consumer = consumer_name
        self._max_deliveries = max_deliveries
        self._visibility_ms = visibility_timeout_ms
        self._block_ms = block_ms
        self._batch_size = batch_size
        self._metrics = metrics
        self._on_dead_letter = on_dead_letter
        self._retry_delay = retry_delay

    def _observe(self, outcome: str, started: float) -> None:
        if self._metrics is not None:
            self._metrics.observe_job(self.stream, outcome, perf_counter() - started)

    async def ensure_group(self) -> None:
        try:
            await self._redis.xgroup_create(self.stream, CONSUMER_GROUP, id=GROUP_START_ID, mkstream=True)
        except ResponseError as exc:
            if not str(exc).startswith(BUSY_GROUP_PREFIX):
                raise

    async def poll_once(self, handler: Handler) -> None:
        claimed = await self._redis.xautoclaim(
            self.stream,
            CONSUMER_GROUP,
            self._consumer,
            min_idle_time=self._visibility_ms,
            start_id=AUTOCLAIM_START_ID,
            count=self._batch_size,
        )
        claimed_entries = cast(list[tuple[str, dict[str, str]]], claimed[1])
        for entry_id, fields in claimed_entries:
            await self._handle(entry_id, fields, handler)
        streams = await self._redis.xreadgroup(
            CONSUMER_GROUP,
            self._consumer,
            {self.stream: NEW_MESSAGES_ID},
            count=self._batch_size,
            block=self._block_ms,
        )
        stream_entries = cast(list[tuple[str, list[tuple[str, dict[str, str]]]]], streams or [])
        for _, entries in stream_entries:
            for entry_id, fields in entries:
                await self._handle(entry_id, fields, handler)

    async def _deliveries(self, entry_id: str) -> int:
        pending = await self._redis.xpending_range(
            self.stream, CONSUMER_GROUP, min=entry_id, max=entry_id, count=1
        )
        return int(pending[0]["times_delivered"]) if pending else 1

    async def _handle(self, entry_id: str, fields: Mapping[str, str], handler: Handler) -> None:
        request_id = _request_id(fields)
        context = {
            "request_id": request_id,
            "org_id": fields.get(FIELD_ORG_ID, ""),
            "outbox_id": fields.get(FIELD_OUTBOX_ID, ""),
            "stream": self.stream,
        }
        with structlog.contextvars.bound_contextvars(**context):
            # A requeued retry is a new entry, so its attempt count rides in the message (ruling A25).
            deliveries = max(_attempt(fields), await self._deliveries(entry_id))
            started = perf_counter()
            message: StreamMessage | None = None
            try:
                message = StreamMessage(
                    stream=self.stream,
                    entry_id=entry_id,
                    outbox_id=context["outbox_id"],
                    org_id=context["org_id"],
                    request_id=request_id,
                    payload=json.loads(fields.get(FIELD_PAYLOAD, "{}")),
                    deliveries=deliveries,
                )
                await handler(message)
            except Exception as exc:  # Retryable failures retry; the rest dead-letter now (ruling A12)
                await self._failed(entry_id, fields, request_id, deliveries, exc, started, message)
                return
            await self._redis.xack(self.stream, CONSUMER_GROUP, entry_id)
            self._observe(OUTCOME_SUCCESS, started)
            log.info("job handled", id=entry_id, deliveries=deliveries, duration_ms=elapsed_ms(started))

    async def _failed(
        self,
        entry_id: str,
        fields: Mapping[str, str],
        request_id: str,
        deliveries: int,
        exc: Exception,
        started: float,
        message: StreamMessage | None,
    ) -> None:
        code = _error_code(exc)
        if deliveries < self._max_deliveries and _should_retry(exc):
            self._observe(OUTCOME_RETRY, started)
            log.warning(
                "job failed, will retry",
                id=entry_id,
                deliveries=deliveries,
                error_code=code,
                duration_ms=elapsed_ms(started),
            )
            await asyncio.sleep(self._retry_delay(deliveries))  # backoff + jitter (rule 5)
            await self._redis.xadd(
                self.stream,
                {name: value for name, value in [*fields.items(), (FIELD_ATTEMPT, str(deliveries + 1))]},
            )
            await self._redis.xack(self.stream, CONSUMER_GROUP, entry_id)
            return
        await self._redis.xadd(
            self.stream + DEAD_LETTER_SUFFIX,
            {
                FIELD_PAYLOAD: fields.get(FIELD_PAYLOAD, ""),
                FIELD_REQUEST_ID: request_id,
                FIELD_ORG_ID: fields.get(FIELD_ORG_ID, ""),
                FIELD_OUTBOX_ID: fields.get(FIELD_OUTBOX_ID, ""),
                FIELD_ERROR: code,
                FIELD_DELIVERIES: str(deliveries),
                FIELD_ORIGINAL_ID: entry_id,
            },
        )
        if self._on_dead_letter is not None and message is not None:
            try:
                await self._on_dead_letter(message, code)
            except Exception as hook_exc:  # the entry is in the DLQ either way; never leave it pending
                log.error("dead letter hook failed", id=entry_id, error_type=type(hook_exc).__name__)  # noqa: TRY400
        await self._redis.xack(self.stream, CONSUMER_GROUP, entry_id)
        self._observe(OUTCOME_DEAD_LETTER, started)
        log.error(
            "job moved to DLQ",
            id=entry_id,
            deliveries=deliveries,
            error_code=code,
            duration_ms=elapsed_ms(started),
        )
