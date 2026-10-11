import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { CURRENT_ORG_COOKIE, orgHome } from "@/features/orgs/contract";
import { resolveCurrentOrg } from "@/features/orgs/current";
import { listOrganizations } from "@/features/orgs/server";
import { EmptyState } from "@/features/shell/EmptyState";

/** The sidebar's KB entry outside any org: go to the current org, or explain that an org comes first. */
export default async function KnowledgeBasesIndex() {
  const [loaded, jar, t] = await Promise.all([listOrganizations(), cookies(), getTranslations("kb")]);
  const current = loaded.ok ? resolveCurrentOrg(loaded.data, null, jar.get(CURRENT_ORG_COOKIE)?.value) : null;
  if (current) redirect(orgHome(current.id));
  return <EmptyState line="kb" title={t("noOrgTitle")} body={loaded.ok ? t("noOrgBody") : loaded.message} />;
}
