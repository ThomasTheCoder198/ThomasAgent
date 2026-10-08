from thomas_ragx.registry.model import ModelRecord, ProviderAccess


class FakeLister:
    def __init__(self, models: list[str] | None = None) -> None:
        self.models = models or []
        self.calls: list[ProviderAccess] = []

    async def list_models(self, access: ProviderAccess) -> list[str]:
        self.calls.append(access)
        return list(self.models)


class FakeChecker:
    def __init__(self) -> None:
        self.calls: list[tuple[ProviderAccess, ModelRecord]] = []

    async def check(self, access: ProviderAccess, model: ModelRecord) -> None:
        self.calls.append((access, model))
