import { notFound } from "next/navigation";

import { FileList } from "@/features/knowledge-bases/components/FileList";
import { KnowledgeBaseSign } from "@/features/knowledge-bases/components/KnowledgeBaseSign";
import { LoadFailure } from "@/features/knowledge-bases/components/LoadFailure";
import { getKnowledgeBasePage } from "@/features/knowledge-bases/server";

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
  if (!loaded.ok) {
    return (
      <div className="h-full overflow-y-auto">
        <div className="mx-auto w-full max-w-272 px-4 py-6 md:px-8">
          <LoadFailure
            serviceDown={loaded.serviceDown}
            message={loaded.message}
            requestId={loaded.requestId}
          />
        </div>
      </div>
    );
  }
  const { knowledgeBase, files, cursor } = loaded.data;
  return (
    <div className="h-full overflow-y-auto">
      <KnowledgeBaseSign orgId={orgId} knowledgeBase={knowledgeBase} />
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
