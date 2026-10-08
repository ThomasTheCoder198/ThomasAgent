from typer.testing import CliRunner

from thomas_ragx import cli


def test_seed_dev_refuses_outside_dev(monkeypatch) -> None:  # type: ignore[no-untyped-def]
    monkeypatch.setenv("RAG_ENV", "production")
    monkeypatch.setenv("RAG_MINIO_ACCESS_KEY", "a")
    monkeypatch.setenv("RAG_MINIO_SECRET_KEY", "s")
    cli.load_config.cache_clear()
    try:
        result = CliRunner().invoke(cli.app, ["seed-dev"])
    finally:
        cli.load_config.cache_clear()
    assert result.exit_code == 1
    assert "seed-dev refused outside dev" in result.output
