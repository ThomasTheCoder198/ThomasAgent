from typing import Annotated, Any

from fastapi import APIRouter, Path, Request

from thomas_ragx.api.services import KbServiceDep
from thomas_ragx.auth.caller import ORG_ID_PATTERN
from thomas_ragx.auth.dependencies import ProvisionCaller
from thomas_ragx.kb.dto import OrgProvisionRequest, OrgResponse, to_json
from thomas_ragx.platform.http_response import request_id_of, success_body

router = APIRouter(prefix="/v1")


@router.put("/orgs/{org_id}")
async def provision_org(
    org_id: Annotated[str, Path(pattern=ORG_ID_PATTERN.pattern)],
    body: OrgProvisionRequest,
    caller: ProvisionCaller,
    service: KbServiceDep,
    request: Request,
) -> dict[str, Any]:
    org = await service.provision(caller, org_id, body.name, body.external_parsing)
    return success_body(to_json(OrgResponse.from_model(org)), request_id_of(request))
