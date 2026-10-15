import { BarChart3 } from "lucide-react";
import { getTranslations } from "next-intl/server";

import { EmptyState } from "@/features/shell/EmptyState";

export default async function UsagePage() {
  const t = await getTranslations("empty");
  return <EmptyState line="model" icon={BarChart3} title={t("usageTitle")} body={t("usageBody")} />;
}
