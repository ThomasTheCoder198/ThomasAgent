import { getTranslations } from "next-intl/server";

import { AgentSettingsSkeleton } from "@/features/agents/AgentSettingsSkeleton";

export default async function AgentLoading() {
  const t = await getTranslations("common");
  return (
    <div role="status" aria-busy className="h-full overflow-y-auto">
      <span className="sr-only">{t("loading")}</span>
      <AgentSettingsSkeleton />
    </div>
  );
}
