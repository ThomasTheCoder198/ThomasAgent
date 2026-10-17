import pytest
from pydantic import SecretStr

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.listing import LISTING_ROUTES, ListedModel, ListingMethod, ModelListing
from thomas_ragx.registry.model import KIND_RULES, BaseUrlRule, ProviderAccess, ProviderKind

CATALOG_KINDS = (ProviderKind.COHERE, ProviderKind.VOYAGE, ProviderKind.JINA)


class RecordingLister:
    def __init__(self, result: list[ListedModel] | None = None, error: Exception | None = None) -> None:
        self.calls: list[ProviderAccess] = []
        self.result = result or [ListedModel("m-1")]
        self.error = error

    async def list_models(self, access: ProviderAccess) -> list[ListedModel]:
        self.calls.append(access)
        if self.error:
            raise self.error
        return self.result


def _listing(
    litellm: RecordingLister | None = None,
    openai_sdk: RecordingLister | None = None,
    openrouter: RecordingLister | None = None,
    catalog: RecordingLister | None = None,
) -> tuple[ModelListing, dict[str, RecordingLister]]:
    listers = {
        "litellm": litellm or RecordingLister(),
        "openai_sdk": openai_sdk or RecordingLister(),
        "openrouter": openrouter or RecordingLister(),
        "catalog": catalog or RecordingLister(),
    }
    return ModelListing(**listers), listers


def test_every_kind_has_a_listing_route_and_kind_rules() -> None:
    assert set(LISTING_ROUTES) == set(ProviderKind)
    assert set(KIND_RULES) == set(ProviderKind)


def test_only_azure_is_still_manual() -> None:
    manual = {kind for kind, route in LISTING_ROUTES.items() if route.method is ListingMethod.MANUAL}
    assert manual == {ProviderKind.AZURE}


def test_base_url_is_hidden_only_for_built_in_kinds() -> None:
    shown = {kind for kind, rules in KIND_RULES.items() if rules.base_url is not BaseUrlRule.HIDDEN}
    assert shown == {ProviderKind.AZURE, ProviderKind.OLLAMA, ProviderKind.OPENAI_COMPATIBLE}


def test_key_is_optional_only_for_local_servers() -> None:
    keyless = {kind for kind, rules in KIND_RULES.items() if not rules.api_key_required}
    assert keyless == {ProviderKind.OLLAMA, ProviderKind.OPENAI_COMPATIBLE}


async def test_azure_reports_sync_unsupported() -> None:
    listing, _ = _listing()
    with pytest.raises(AppError) as failure:
        await listing.list_models(ProviderAccess(ProviderKind.AZURE, "https://x", SecretStr("k")))
    assert failure.value.code is ErrorCode.LLM_PROVIDER_SYNC_UNSUPPORTED


async def test_openrouter_goes_through_its_own_lister_without_a_key() -> None:
    openrouter = RecordingLister([ListedModel("openai/gpt-4o-mini")])
    listing, listers = _listing(openrouter=openrouter)
    models = await listing.list_models(ProviderAccess(ProviderKind.OPENROUTER, "", None))
    assert [model.name for model in models] == ["openai/gpt-4o-mini"]
    assert listers["litellm"].calls == listers["openai_sdk"].calls == []


@pytest.mark.parametrize("kind", CATALOG_KINDS)
async def test_catalog_kinds_list_without_a_key(kind: ProviderKind) -> None:
    catalog = RecordingLister([ListedModel("rerank-x")])
    listing, _ = _listing(catalog=catalog)
    assert await listing.list_models(ProviderAccess(kind, "", None)) == [ListedModel("rerank-x")]
    assert len(catalog.calls) == 1


async def test_keyed_kind_without_a_key_is_a_validation_error() -> None:
    listing, _ = _listing()
    with pytest.raises(AppError) as failure:
        await listing.list_models(ProviderAccess(ProviderKind.OPENAI, "", None))
    assert failure.value.code is ErrorCode.VALIDATION_FAILED
    assert failure.value.details == {"fields": [{"field": "apiKey", "code": "REQUIRED"}]}


async def test_provider_failure_becomes_rejected_without_its_message() -> None:
    failing = RecordingLister(error=RuntimeError("401 invalid key sk-secret"))
    listing, _ = _listing(litellm=failing)
    with pytest.raises(AppError) as failure:
        await listing.list_models(ProviderAccess(ProviderKind.ANTHROPIC, "", SecretStr("sk-secret")))
    assert failure.value.code is ErrorCode.LLM_PROVIDER_REJECTED
    assert "sk-secret" not in str(failure.value)


async def test_a_listing_attempt_logs_exactly_one_event_with_its_duration(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    import structlog
    from structlog.testing import capture_logs

    from thomas_ragx.registry import listing as listing_module

    # Earlier tests may have used (and cached) the module logger under the real configuration.
    monkeypatch.setattr(listing_module, "log", structlog.get_logger("test"))
    failing = RecordingLister(error=RuntimeError("boom sk-secret"))
    listing, _ = _listing(litellm=failing)
    with capture_logs() as logs, pytest.raises(AppError):
        await listing.list_models(ProviderAccess(ProviderKind.ANTHROPIC, "", SecretStr("sk-secret")))
    assert len(logs) == 1
    assert (logs[0]["outcome"], logs[0]["error_type"]) == ("error", "RuntimeError")
    assert "duration_ms" in logs[0]
    assert "sk-secret" not in str(logs)
    with capture_logs() as ok_logs:
        await _listing()[0].list_models(ProviderAccess(ProviderKind.OPENROUTER, "", None))
    assert [(event["outcome"], event["log_level"]) for event in ok_logs] == [("success", "info")]
