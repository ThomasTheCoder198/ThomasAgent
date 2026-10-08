from psycopg import AsyncConnection
from psycopg.rows import DictRow, dict_row
from psycopg_pool import AsyncConnectionPool

type Pool = AsyncConnectionPool[AsyncConnection[DictRow]]


def create_pool(conninfo: str, min_size: int, max_size: int) -> Pool:
    """The caller opens the pool, so constructing an app never touches the network."""
    return AsyncConnectionPool(
        conninfo,
        min_size=min_size,
        max_size=max_size,
        open=False,
        connection_class=AsyncConnection[DictRow],
        kwargs={"row_factory": dict_row},
    )
