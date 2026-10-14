import uuid

import pytest

from thomas_ragx.db.pool import Pool
from thomas_ragx.platform.config import Config
from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.registry.dev_seed import seed_dev_registry
from thomas_ragx.registry.factory import create_registry_service
from thomas_ragx.registry.model import Role

pytestmark = pytest.mark.integration


async def test_model_handle_and_role_handle(pool: Pool) -> None:
    service = create_registry_service(Config(minio_access_key="a", minio_secret_key="s"), pool)
    model = await seed_dev_registry(service)
    handle = await service.model_handle(model.id)
    assert handle.record.id == model.id and handle.litellm_model == "openai/dev-embedding"
    assert (await service.role_handle(Role.EMBEDDING)).record.id == model.id
    with pytest.raises(AppError) as missing:
        await service.model_handle(uuid.uuid4())
    assert missing.value.code is ErrorCode.LLM_MODEL_NOT_FOUND
    with pytest.raises(AppError) as unassigned:
        await service.role_handle(Role.RERANK)
    assert unassigned.value.code is ErrorCode.LLM_ROLE_NOT_ASSIGNED
