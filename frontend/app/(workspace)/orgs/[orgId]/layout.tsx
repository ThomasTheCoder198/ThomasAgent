import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { OrgStatusStation } from "@/features/knowledge-bases/components/OrgStatusStation";
import { getOrganization } from "@/features/orgs/server";

/** Everything under an org needs ragx to have prepared it: until then, one station says why and offers Retry. */
export default async function OrgLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ orgId: string }>;
}) {
  const { orgId } = await params;
  const org = await getOrganization(decodeURIComponent(orgId));
  if (!org) notFound();
  if (org.ragxStatus !== "ready") return <OrgStatusStation org={org} />;
  return children;
}
