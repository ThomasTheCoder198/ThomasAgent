from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True)
class AdminSession:
    admin_user_id: UUID
    email: str
    csrf_token: str
    expires_at: datetime
