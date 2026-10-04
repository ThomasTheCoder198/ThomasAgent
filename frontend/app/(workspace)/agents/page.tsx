import { getTranslations } from "next-intl/server";

import { EmptyState } from "@/features/shell/EmptyState";

export default async function AgentsPage() {
  const t = await getTranslations("empty");
  return <EmptyState line="model" title={t("agentsTitle")} body={t("agentsBody")} />;
}
