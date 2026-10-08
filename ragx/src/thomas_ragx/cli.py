import asyncio
import contextlib
import os
import signal
from typing import Final

import structlog
import typer
import uvicorn

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx.api.app import create_app
from thomas_ragx.db.migrate import migrate_down, migrate_up
from thomas_ragx.platform.config import Config, load_config
from thomas_ragx.platform.litellm_policy import apply_litellm_policy
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.tracing import (
    apply_trace_content_policy,
    configure_tracing,
    install_global_tracer_provider,
)
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.resources import Resources
from thomas_ragx.worker.run import run_worker

DEV_ENVIRONMENT: Final = "dev"
log = structlog.get_logger(__name__)

app = typer.Typer(no_args_is_help=True)
migrate_app = typer.Typer(no_args_is_help=True)
app.add_typer(migrate_app, name="migrate")


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
