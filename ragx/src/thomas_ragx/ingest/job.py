from dataclasses import dataclass
from typing import Final
from uuid import UUID

from thomas_ragx.platform.error_codes_gen import ErrorCode
from thomas_ragx.platform.errors import AppError
from thomas_ragx.worker.consumer import StreamMessage

FIELD_KB_ID: Final = "kb_id"
FIELD_FILE_ID: Final = "file_id"
FIELD_VERSION: Final = "version"


@dataclass(frozen=True)
class IngestJob:
    org_id: str
    kb_id: UUID
    file_id: UUID
    version: int
    request_id: str

    @classmethod
    def from_message(cls, message: StreamMessage) -> "IngestJob":
        try:
            return cls(
                org_id=message.org_id,
                kb_id=UUID(str(message.payload[FIELD_KB_ID])),
                file_id=UUID(str(message.payload[FIELD_FILE_ID])),
                version=int(message.payload[FIELD_VERSION]),
                request_id=message.request_id,
            )
        except (KeyError, ValueError, TypeError):
            raise AppError(ErrorCode.VALIDATION_FAILED) from None  # a malformed job never becomes valid
