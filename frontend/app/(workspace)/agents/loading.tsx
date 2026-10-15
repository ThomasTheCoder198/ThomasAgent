import { getTranslations } from "next-intl/server";

import { AgentListSkeleton } from "@/features/agents/AgentListSkeleton";
import { Skeleton } from "@/components/ui/Skeleton";

export default async function AgentsLoading() {
  const t = await getTranslations("common");
  return (
    <div role="status" aria-busy className="h-full overflow-y-auto">
      <span className="sr-only">{t("loading")}</span>
      <div className="mx-auto w-full max-w-5xl px-4 pt-6 pb-6 md:px-6 md:pt-10 md:pb-8 lg:px-8">
        <Skeleton className="h-9 w-40" />
        <Skeleton className="mt-3 h-5 w-96 max-w-full" />
      </div>
      <div className="mx-auto w-full max-w-5xl px-4 pb-16 md:px-6 lg:px-8">
        <AgentListSkeleton />
      </div>
    </div>
  );
}
