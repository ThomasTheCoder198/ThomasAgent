import typer
import uvicorn

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.config import load_config
from thomas_ragx.platform.logging import configure_logging

app = typer.Typer(no_args_is_help=True)


@app.command()
def serve() -> None:
    config = load_config()
    uvicorn.run(create_app(config), host=config.http_host, port=config.http_port, log_config=None)


@app.command()
def bootstrap() -> None:
    config = load_config()
    configure_logging(config.log_level, config.service_name, config.environment)
    bootstrap_module.run(config)
