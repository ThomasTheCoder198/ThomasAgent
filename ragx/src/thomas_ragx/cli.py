import typer
import uvicorn

from thomas_ragx import bootstrap as bootstrap_module
from thomas_ragx.api.app import create_app
from thomas_ragx.platform.logging import configure_logging
from thomas_ragx.platform.settings import get_settings

app = typer.Typer(no_args_is_help=True)


@app.command()
def serve() -> None:
    settings = get_settings()
    uvicorn.run(create_app(settings), host=settings.http_host, port=settings.http_port, log_config=None)


@app.command()
def bootstrap() -> None:
    settings = get_settings()
    configure_logging(settings.log_level, settings.service_name)
    bootstrap_module.run(settings)
