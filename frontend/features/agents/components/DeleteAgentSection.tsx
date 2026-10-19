"use client";

import { Trash2 } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { SettingsSection } from "@/components/ui/SettingsSection";
import { ConfirmDelete } from "@/features/knowledge-bases/components/ConfirmDelete";
import type { Locale } from "@/i18n/config";

import { deleteAgent } from "../api";

type Props = { orgId: string; id: string; name: string; index: number };

/**
 * Deleting an Agent also deletes its conversations (index: conversations.agent_id ON DELETE CASCADE), so the
 * confirmation says so. The KB and its files stay in ragx.
 */
export function DeleteAgentSection({ orgId, id, name, index }: Props) {
  const t = useTranslations("agents");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [open, setOpen] = useState(false);
  return (
    <SettingsSection index={index} title={t("deleteTitle")} hint={t("deleteHint")}>
      <Button variant="destructive" onClick={() => setOpen(true)}>
        <Trash2 aria-hidden />
        {t("delete")}
      </Button>
      {open && (
        <ConfirmDelete
          onOpenChange={setOpen}
          title={t("deleteConfirmTitle", { name })}
          body={t("deleteConfirmBody")}
          successMessage={t("deleted", { name })}
          onConfirm={async () => {
            await deleteAgent(orgId, id, locale);
            router.push("/agents");
            router.refresh();
          }}
        />
      )}
    </SettingsSection>
  );
}
