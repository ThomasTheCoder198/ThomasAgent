import Link from "next/link";
import { getTranslations } from "next-intl/server";

import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/features/shell/EmptyState";

/** Business pages need an org (R1); before the first one exists they point the operator at /orgs. */
export async function NoOrgState() {
  const t = await getTranslations("orgs");
  return (
    <div className="flex h-full flex-col items-center justify-center p-6">
      <EmptyState level="h1" line="model" title={t("noOrgTitle")} body={t("noOrgBody")}>
        <Button asChild className="mt-3">
          <Link href="/orgs">{t("noOrgAction")}</Link>
        </Button>
      </EmptyState>
    </div>
  );
}
