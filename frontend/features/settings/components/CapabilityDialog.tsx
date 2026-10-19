"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeading } from "@/components/ui/Dialog";
import { SwitchField } from "@/components/ui/Field";
import type { Capability, Model } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import { updateCapabilities } from "../api";

/** The capabilities core's chat roles use; embedding and rerank belong to ragx (O6). */
const EDITABLE_CAPABILITIES: readonly Capability[] = ["chat", "tools", "vision", "reasoning", "decision"];

type Props = { model: Model; onOpenChange: (open: boolean) => void };

/**
 * O8's "Sửa" (Q12): fixes a model whose provider did not report its capabilities. Core keeps the edit across
 * syncs and refuses one that would break a role still pinned to the model.
 */
export function CapabilityDialog({ model, onOpenChange }: Props) {
  const t = useTranslations("settings");
  const common = useTranslations("common");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [selected, setSelected] = useState<Capability[]>(model.capabilities);
  const [saving, setSaving] = useState(false);

  const toggle = (capability: Capability, on: boolean) =>
    setSelected((current) => (on ? [...current, capability] : current.filter((c) => c !== capability)));

  async function save() {
    setSaving(true);
    try {
      await updateCapabilities(model.id, selected, locale);
      notify.success(t("capabilitiesSaved"));
      onOpenChange(false);
      router.refresh();
    } catch (err) {
      notify.error(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeading
          title={t("capabilitiesTitle", { model: model.displayName })}
          description={t("capabilitiesBody")}
        />
        <div className="flex flex-col gap-3">
          {EDITABLE_CAPABILITIES.map((capability) => (
            <SwitchField
              key={capability}
              label={capability}
              checked={selected.includes(capability)}
              onCheckedChange={(on) => toggle(capability, on)}
            />
          ))}
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="ghost">
              {common("cancel")}
            </Button>
          </DialogClose>
          <Button onClick={() => void save()} pending={saving} disabled={selected.length === 0}>
            {common("save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
