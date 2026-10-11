"use client";

import { usePathname } from "next/navigation";
import { useState } from "react";

import { orgIdFromPath } from "./current";

/**
 * The workspace layout stays mounted when the operator switches org, so the org it resolved on the server goes
 * stale. The plate and the Knowledge Base link both read this one answer: the org in the live URL wins, off an org
 * page the last one seen stays, and the server's org is only the first-load fallback. Ids outside `orgIds` are ignored.
 */
export function useLiveOrgId(orgIds: readonly string[], serverOrgId: string | null): string | null {
  const pathId = orgIdFromPath(usePathname() ?? "");
  const valid = pathId && orgIds.includes(pathId) ? pathId : null;
  const [seenId, setSeenId] = useState(valid);
  if (valid && valid !== seenId) setSeenId(valid);
  return valid ?? seenId ?? serverOrgId;
}
