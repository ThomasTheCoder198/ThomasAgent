import os

import typer
import uvicorn

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import load_config
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.tracing import apply_trace_content_policy, install_global_tracer_provider

app = typer.Typer(no_args_is_help=True)


@app.command()
def serve() -> None:
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
    config = load_config()
    apply_trace_content_policy(config, os.environ)
    configure_logging(config.log_level, config.service_name, config.environment)
    bootstrap_module.run(config)
