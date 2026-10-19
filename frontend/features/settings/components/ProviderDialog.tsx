"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeading } from "@/components/ui/Dialog";
import type { Provider } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import { createProvider, syncProvider, updateProvider } from "../api";
import {
  createInput,
  editInput,
  initialFields,
  problemsFromError,
  validateProvider,
  type FieldProblems,
  type ProviderFields,
} from "../provider-form";
import { ProviderFormFields } from "./ProviderFormFields";

type Props = {
  /** Set to edit that provider: its kind is fixed and the key stays unless a new one is typed. */
  provider?: Provider;
  onOpenChange: (open: boolean) => void;
};

/** The key field is write-only: it is never prefilled and never shown again after saving. */
export function ProviderDialog({ provider, onOpenChange }: Props) {
  const t = useTranslations("settings");
  const common = useTranslations("common");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const editing = provider !== undefined;
  const [fields, setFields] = useState<ProviderFields>(() => initialFields(provider));
  const [sync, setSync] = useState(true);
  const [problems, setProblems] = useState<FieldProblems>({});
  const [saving, setSaving] = useState(false);

  async function save() {
    const saved = provider
      ? await updateProvider(provider.id, editInput(provider, fields), locale)
      : await createProvider(createInput(fields), locale);
    notify.success(t("providerSaved", { name: saved.name }));
    onOpenChange(false);
    router.refresh();
    if (!provider && sync) {
      try {
        await syncProvider(saved.id, locale);
      } catch (err) {
        notify.error(err);
      }
      router.refresh();
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const found = validateProvider(fields, editing);
    setProblems(found);
    if (Object.keys(found).length > 0) return;
    setSaving(true);
    try {
      await save();
    } catch (err) {
      const fromCore = problemsFromError(err);
      if (Object.keys(fromCore).length > 0) setProblems(fromCore);
      else notify.error(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeading
          title={editing ? t("editProvider") : t("addProvider")}
          description={t("addProviderBody")}
        />
        <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-4">
          <ProviderFormFields
            fields={fields}
            onChange={(next) => setFields((current) => ({ ...current, ...next }))}
            problems={problems}
            editing={editing}
            baseUrlDirty={!!provider && fields.baseUrl.trim() !== provider.baseUrl}
            sync={sync}
            onSyncChange={setSync}
          />
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost">
                {common("cancel")}
              </Button>
            </DialogClose>
            <Button type="submit" pending={saving}>
              {!editing && sync ? t("saveAndSync") : t("saveProvider")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
