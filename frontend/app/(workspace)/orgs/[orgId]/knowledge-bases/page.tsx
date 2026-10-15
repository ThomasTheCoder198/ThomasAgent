import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { NewKnowledgeBaseButton } from "@/features/knowledge-bases/components/KnowledgeBaseDialogs";
import { KnowledgeBaseList } from "@/features/knowledge-bases/components/KnowledgeBaseList";
import { LoadFailure, ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { PAGE_COLUMN } from "@/features/knowledge-bases/layout";
import { listKnowledgeBases } from "@/features/knowledge-bases/server";
import { PageHeader } from "@/features/shell/PageHeader";

export default async function KnowledgeBasesPage({ params }: { params: Promise<{ orgId: string }> }) {
  const orgId = decodeURIComponent((await params).orgId);
  const [t, loaded] = await Promise.all([getTranslations("kb"), listKnowledgeBases(orgId)]);
  if (!loaded.ok && loaded.notFound) notFound();
  if (!loaded.ok && loaded.serviceDown) return <ServiceUnavailable requestId={loaded.requestId} />;
  return (
    <div className="h-full overflow-y-auto">
      <PageHeader title={t("title")} body={t("body")} actions={<NewKnowledgeBaseButton orgId={orgId} />} />
      <div className={`${PAGE_COLUMN} pb-16`}>
        {loaded.ok ? (
          <KnowledgeBaseList orgId={orgId} items={loaded.data} />
        ) : (
          <LoadFailure message={loaded.message} requestId={loaded.requestId} />
        )}
      </div>
    </div>
  );
}
