import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { FileList } from "@/features/knowledge-bases/components/FileList";
import { KbPageHeader } from "@/features/knowledge-bases/components/KbPageHeader";
import { KnowledgeBaseHeader } from "@/features/knowledge-bases/components/KnowledgeBaseHeader";
import { LoadFailure, ServiceUnavailable } from "@/features/knowledge-bases/components/LoadFailure";
import { PAGE_COLUMN } from "@/features/knowledge-bases/layout";
import { getKnowledgeBasePage } from "@/features/knowledge-bases/server";
import { orgHome } from "@/features/orgs/contract";

export default async function KnowledgeBasePage({
  params,
}: {
  params: Promise<{ orgId: string; kbId: string }>;
}) {
  const { orgId: rawOrgId, kbId: rawKbId } = await params;
  const orgId = decodeURIComponent(rawOrgId);
  const kbId = decodeURIComponent(rawKbId);
  const loaded = await getKnowledgeBasePage(orgId, kbId);
  if (!loaded.ok && loaded.notFound) notFound();
  if (!loaded.ok && loaded.serviceDown) return <ServiceUnavailable requestId={loaded.requestId} />;
  if (!loaded.ok) {
    const t = await getTranslations("kb");
    return (
      <div className="h-full overflow-y-auto">
        <KbPageHeader title={t("title")} back={{ href: orgHome(orgId), label: t("back") }} />
        <div className={`${PAGE_COLUMN} pb-16`}>
          <LoadFailure message={loaded.message} requestId={loaded.requestId} />
        </div>
      </div>
    );
  }
  const { knowledgeBase, files, cursor } = loaded.data;
  return (
    <div className="h-full overflow-y-auto">
      <KnowledgeBaseHeader orgId={orgId} knowledgeBase={knowledgeBase} />
      <FileList
        key={knowledgeBase.id}
        orgId={orgId}
        kbId={knowledgeBase.id}
        initialFiles={files}
        initialCursor={cursor}
      />
    </div>
  );
}
