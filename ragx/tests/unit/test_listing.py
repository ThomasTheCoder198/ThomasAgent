import pytest
from pydantic import SecretStr

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.listing import LISTING_ROUTES, ListingMethod, ModelListing
from thomas_ragx.registry.model import ProviderAccess, ProviderKind


class RecordingLister:
    def __init__(self, result: list[str] | None = None, error: Exception | None = None) -> None:
        self.calls: list[ProviderAccess] = []
        self.result = result or ["m-1"]
        self.error = error

    async def list_models(self, access: ProviderAccess) -> list[str]:
        self.calls.append(access)
        if self.error:
            raise self.error
        return self.result


def test_every_kind_has_a_listing_route() -> None:
    assert set(LISTING_ROUTES) == set(ProviderKind)


@pytest.mark.parametrize("kind", [k for k, r in LISTING_ROUTES.items() if r.method is ListingMethod.MANUAL])
async def test_manual_kinds_report_sync_unsupported(kind: ProviderKind) -> None:
    listing = ModelListing(RecordingLister(), RecordingLister())
    with pytest.raises(AppError) as failure:
        await listing.list_models(ProviderAccess(kind, "", SecretStr("k")))
    assert failure.value.code is ErrorCode.REGISTRY_SYNC_UNSUPPORTED


async def test_openrouter_goes_through_the_openai_sdk_lister() -> None:
    litellm, openai_sdk = RecordingLister(), RecordingLister(["openai/gpt-4o-mini"])
    models = await ModelListing(litellm, openai_sdk).list_models(
        ProviderAccess(ProviderKind.OPENROUTER, "", SecretStr("k"))
    )
    assert models == ["openai/gpt-4o-mini"]
    assert litellm.calls == []


async def test_keyed_kind_without_a_key_is_a_validation_error() -> None:
    with pytest.raises(AppError) as failure:
        await ModelListing(RecordingLister(), RecordingLister()).list_models(
            ProviderAccess(ProviderKind.OPENAI, "", None)
        )
    assert failure.value.code is ErrorCode.VALIDATION_FAILED


async def test_provider_failure_becomes_rejected_without_its_message() -> None:
    failing = RecordingLister(error=RuntimeError("401 invalid key sk-secret"))
    with pytest.raises(AppError) as failure:
        await ModelListing(failing, RecordingLister()).list_models(
            ProviderAccess(ProviderKind.ANTHROPIC, "", SecretStr("sk-secret"))
        )
    assert failure.value.code is ErrorCode.REGISTRY_PROVIDER_REJECTED
    assert "sk-secret" not in str(failure.value)
