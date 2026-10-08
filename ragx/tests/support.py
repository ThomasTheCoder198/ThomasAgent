from typing import Any


class FakeResources:
    """Stands in for Postgres/Redis/MinIO in unit tests.

    Routes that need them are tested in tests/integration.
    """

    def __init__(self, ready: dict[str, bool] | None = None) -> None:
        self.ready = ready if ready is not None else {"postgres": True, "redis": True, "storage": True}
        self.opened = False
        self.closed = False
        self.pool: Any = None
        self.redis: Any = None
        self.storage: Any = None

    async def open(self) -> None:
        self.opened = True

    async def close(self) -> None:
        self.closed = True

    async def readiness(self) -> dict[str, bool]:
        return dict(self.ready)
