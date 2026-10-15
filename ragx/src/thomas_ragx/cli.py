import asyncio
import contextlib
import json
import os
import signal
from pathlib import Path
from time import perf_counter
from typing import Annotated, Any, Final
from uuid import UUID

import structlog
import typer
import uvicorn

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx.api.app import create_app
from thomas_ragx.db.migrate import migrate_down, migrate_up
from thomas_ragx.db.outbox import KNOWN_STREAMS
from thomas_ragx.eval.golden import load_golden
from thomas_ragx.eval.runner import EvalRunner
from thomas_ragx.kb import repository as kbs
from thomas_ragx.platform.config import Config, load_config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.platform.litellm_policy import apply_litellm_policy
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.request_id import new_job_id
from thomas_ragx.platform.timing import elapsed_ms
from thomas_ragx.platform.tracing import (
    apply_trace_content_policy,
    configure_tracing,
    install_global_tracer_provider,
    instrument_litellm,
)
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.resources import Resources, create_redis
from thomas_ragx.retrieval.factory import create_search_service
from thomas_ragx.worker.dlq import DeadLetterReplayer
from thomas_ragx.worker.metrics import configure_worker_metrics
from thomas_ragx.worker.run import run_worker

DEV_ENVIRONMENT: Final = "dev"
log = structlog.get_logger(__name__)

app = typer.Typer(no_args_is_help=True)
migrate_app = typer.Typer(no_args_is_help=True)
app.add_typer(migrate_app, name="migrate")
dlq_app = typer.Typer(no_args_is_help=True)
app.add_typer(dlq_app, name="dlq")
eval_app = typer.Typer(no_args_is_help=True)
app.add_typer(eval_app, name="eval")


@migrate_app.command("up")
def migrate_up_command() -> None:
    apply_litellm_policy(os.environ)
    config = load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    migrate_up(config.database_url.get_secret_value())


@migrate_app.command("down")
def migrate_down_command(steps: int = typer.Option(1, min=1)) -> None:
    apply_litellm_policy(os.environ)
    config = load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    migrate_down(config.database_url.get_secret_value(), steps)


@app.command()
def serve() -> None:
    apply_litellm_policy(os.environ)
    config = load_config()
    apply_trace_content_policy(config, os.environ)
    api = create_app(config)
    install_global_tracer_provider(api.state.tracer_provider)
    instrument_litellm(api.state.tracer_provider)
    try:
        uvicorn.run(api, host=config.http_host, port=config.http_port, log_config=None)
    finally:
        api.state.tracer_provider.shutdown()


@app.command()
def bootstrap() -> None:
    apply_litellm_policy(os.environ)
    config = load_config()
    apply_trace_content_policy(config, os.environ)
    configure_logging(config.log_level, config.service_name, config.environment)
    bootstrap_module.run(config)


async def _run_worker_process(config: Config) -> None:
    resources = Resources.from_config(config)
    provider = configure_tracing(config)
    install_global_tracer_provider(provider)
    instrument_litellm(provider)
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for signum in (signal.SIGTERM, signal.SIGINT):
        # Windows has no add_signal_handler; Ctrl+C still raises KeyboardInterrupt there.
        with contextlib.suppress(NotImplementedError):
            loop.add_signal_handler(signum, stop.set)
    await resources.open()
    try:
        await run_worker(config, resources, provider.get_tracer(__name__), stop)
    finally:
        await resources.close()
        provider.shutdown()


@app.command()
def worker() -> None:
    apply_litellm_policy(os.environ)
    config = load_config()
    apply_trace_content_policy(config, os.environ)
    configure_logging(config.log_level, config.service_name, config.environment)
    asyncio.run(_run_worker_process(config))


async def _seed_dev(config: Config) -> None:
    resources = Resources.from_config(config)
    await resources.open()
    try:
        model = await seed_dev_registry(create_registry_service(config, resources.pool))
        log.info("dev registry seeded", model=model.name, dims=model.embedding_dims)
    finally:
        await resources.close()


@app.command("seed-dev")
def seed_dev() -> None:
    """Offline embedding model + role so a dev stack can provision orgs; refused outside RAG_ENV=dev."""
    apply_litellm_policy(os.environ)
    config = load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    if config.environment != DEV_ENVIRONMENT:
        log.error("seed-dev refused outside dev", environment=config.environment)
        raise typer.Exit(code=1)
    asyncio.run(_seed_dev(config))


async def _replay(config: Config, stream: str, entry_ids: list[str] | None) -> None:
    redis = create_redis(config)
    try:
        with structlog.contextvars.bound_contextvars(request_id=new_job_id()):
            replayed = await DeadLetterReplayer(redis).replay(stream, entry_ids)
            log.info("dlq replay finished", stream=stream, count=replayed)
    finally:
        await redis.aclose()


@dlq_app.command("replay")
def dlq_replay(
    stream: str,
    entry_id: Annotated[
        list[str] | None, typer.Option("--entry-id", help="Replay only these DLQ entry ids")
    ] = None,
    replay_all: Annotated[bool, typer.Option("--all", help="Replay every entry of the stream's DLQ")] = False,
) -> None:
    """Move dead letters back to their stream; run it inside the worker container."""
    apply_litellm_policy(os.environ)
    config = load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    if stream not in KNOWN_STREAMS or (not entry_id and not replay_all):
        log.error("dlq replay needs a known stream and --entry-id or --all", stream=stream)
        raise typer.Exit(code=1)
    asyncio.run(_replay(config, stream, entry_id or None))


async def _eval(config: Config, golden: Path, org_id: str, kb_id: UUID) -> dict[str, Any]:
    resources = Resources.from_config(config)
    provider = configure_tracing(config)
    install_global_tracer_provider(provider)
    instrument_litellm(provider)
    metrics_provider, metrics = configure_worker_metrics(config)
    try:
        await resources.open()
        async with resources.pool.connection() as conn:
            kb = await kbs.get_kb(conn, org_id, kb_id)
        if kb is None:
            raise AppError(ErrorCode.KB_NOT_FOUND)
        registry = create_registry_service(config, resources.pool)
        search = create_search_service(config, resources, registry, provider.get_tracer(__name__), metrics)
        return (await EvalRunner(resources.pool, search).run(org_id, kb, load_golden(golden))).to_json()
    finally:
        await resources.close()
        provider.shutdown()
        metrics_provider.shutdown()


@eval_app.command("run")
def eval_run(
    golden: Annotated[Path, typer.Option("--set", exists=True, dir_okay=False, help="Golden set JSONL")],
    org: Annotated[str, typer.Option("--org")],
    kb: Annotated[str, typer.Option("--kb")],
    out: Annotated[Path | None, typer.Option("--out", help="Write the JSON report here")] = None,
) -> None:
    """Score retrieval of a golden set against a live KB; run it inside the worker container."""
    apply_litellm_policy(os.environ)
    config = load_config()
    apply_trace_content_policy(config, os.environ)
    configure_logging(config.log_level, config.service_name, config.environment)
    started = perf_counter()
    # Bound here so the context is copied into asyncio.run and covers the completion line too (rule 6).
    with structlog.contextvars.bound_contextvars(request_id=new_job_id(), org_id=org):
        try:
            report = asyncio.run(_eval(config, golden, org, UUID(kb)))
        except AppError as exc:
            # log.exception would attach the traceback; the code alone is the boundary record (rule 4).
            log.error("eval failed", error_code=exc.code.value, duration_ms=elapsed_ms(started))  # noqa: TRY400
            raise typer.Exit(code=1) from None
        except Exception as exc:
            # A malformed --kb or a down dependency: the type is the record, never the text.
            log.error("eval failed", error_type=type(exc).__name__, duration_ms=elapsed_ms(started))  # noqa: TRY400
            raise typer.Exit(code=1) from None
        if out is not None:
            out.write_text(json.dumps(report, ensure_ascii=False, indent=1), encoding="utf-8")
        log.info(
            "eval finished",
            scored=report["scored"],
            recall_at_5=report["recall"]["5"],
            mrr=report["mrr"],
            duration_ms=elapsed_ms(started),
        )
