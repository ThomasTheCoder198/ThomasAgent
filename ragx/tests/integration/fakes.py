import asyncio

from thomas_ragx.registry.listing import ListedModel
from thomas_ragx.registry.model import ModelRecord, ProviderAccess

MEASURED_DIMS = 768


class FakeLister:
    def __init__(self, models: list[str | ListedModel] | None = None) -> None:
        self.models = [ListedModel(m) if isinstance(m, str) else m for m in models or []]
        self.calls: list[ProviderAccess] = []

    async def list_models(self, access: ProviderAccess) -> list[ListedModel]:
        self.calls.append(access)
        return list(self.models)


class FakeChecker:
    def __init__(
        self, dims: int = MEASURED_DIMS, error: Exception | None = None, gate: asyncio.Event | None = None
    ) -> None:
        self.dims = dims
        self.error = error
        # A closed gate holds the probe so a test can change the model while the provider call is in flight.
        self.gate = gate
        self.probing = asyncio.Event()
        self.calls: list[tuple[ProviderAccess, ModelRecord]] = []
        self.measured: list[tuple[ProviderAccess, ModelRecord]] = []

    async def check(self, access: ProviderAccess, model: ModelRecord) -> None:
        self.calls.append((access, model))

    async def measure_embedding_dims(self, access: ProviderAccess, model: ModelRecord) -> int:
        self.measured.append((access, model))
        self.probing.set()
        if self.gate is not None:
            await self.gate.wait()
        if self.error:
            raise self.error
        return self.dims
