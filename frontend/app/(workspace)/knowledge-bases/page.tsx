import { getTranslations } from "next-intl/server";

import { EmptyState } from "@/features/shell/EmptyState";

export default async function KnowledgeBasesPage() {
  const t = await getTranslations("empty");
  return <EmptyState line="kb" title={t("kbTitle")} body={t("kbBody")} />;
}
