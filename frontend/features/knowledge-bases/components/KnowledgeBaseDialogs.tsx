"use client";

import { Plus } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeading } from "@/components/ui/Dialog";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import type { Locale } from "@/i18n/config";
import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KnowledgeBase } from "@/lib/ragx/types";
import { useNotify } from "@/lib/useNotify";
import { validateDisplayName, type NameProblem } from "@/lib/validation";

import { createKnowledgeBase, renameKnowledgeBase } from "../api";
import { KB_NAME_MAX_LENGTH, kbPage } from "../contract";

const PROBLEM_KEY: Record<NameProblem, "nameRequired" | "nameTooLong" | "nameInvalid"> = {
  required: "nameRequired",
  tooLong: "nameTooLong",
  invalid: "nameInvalid",
};

const isTaken = (err: unknown) =>
  err instanceof ApiError &&
  err.code === ErrorCode.CONFLICT &&
  !!(err.details?.fields as { field: string }[] | undefined)?.some((f) => f.field === "name");

type NameDialogProps = {
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  submitLabel: string;
  initialName: string;
  /** Resolves when saved (the caller toasts its own success); a rejection is mapped here. */
  onSubmit: (name: string) => Promise<void>;
};

/**
 * One name field, validated like an organization's name. A name that is already taken stays on the field
 * (the operator is typing it); every other failure is a catalog toast.
 */
function NameDialog({
  onOpenChange,
  title,
  description,
  submitLabel,
  initialName,
  onSubmit,
}: NameDialogProps) {
  const t = useTranslations("kb");
  const notify = useNotify();
  const [name, setName] = useState(initialName);
  const [nameError, setNameError] = useState<string>();
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const problem = validateDisplayName(name, KB_NAME_MAX_LENGTH);
    setNameError(problem ? t(`errors.${PROBLEM_KEY[problem]}`) : undefined);
    if (problem) return;
    setSaving(true);
    try {
      await onSubmit(name.trim());
      onOpenChange(false);
    } catch (err) {
      if (isTaken(err)) setNameError(t("errors.nameTaken"));
      else notify.error(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeading title={title} description={description} />
        <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-4">
          <Field label={t("name")} error={nameError}>
            {(control) => (
              <Input
                {...control}
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoComplete="off"
                required
              />
            )}
          </Field>
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="ghost">{t("cancel")}</Button>
            </DialogClose>
            <Button type="submit" pending={saving}>
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function NewKnowledgeBaseDialog({
  orgId,
  onOpenChange,
}: {
  orgId: string;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("kb");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  return (
    <NameDialog
      onOpenChange={onOpenChange}
      title={t("createTitle")}
      description={t("createBody")}
      submitLabel={t("create")}
      initialName=""
      onSubmit={async (name) => {
        const created = await createKnowledgeBase(orgId, { name }, locale);
        notify.success(t("toast.created", { name: created.name }));
        router.push(kbPage(orgId, created.id));
      }}
    />
  );
}

export function RenameKnowledgeBaseDialog({
  orgId,
  knowledgeBase,
  onOpenChange,
}: {
  orgId: string;
  knowledgeBase: KnowledgeBase;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("kb");
  const common = useTranslations("common");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  return (
    <NameDialog
      onOpenChange={onOpenChange}
      title={t("renameTitle")}
      submitLabel={common("save")}
      initialName={knowledgeBase.name}
      onSubmit={async (name) => {
        const renamed = await renameKnowledgeBase(orgId, knowledgeBase.id, { name }, locale);
        notify.success(t("toast.renamed", { name: renamed.name }));
        router.refresh();
      }}
    />
  );
}

/** The page header's action: the trigger and its dialog together (F15). */
export function NewKnowledgeBaseButton({ orgId }: { orgId: string }) {
  const t = useTranslations("kb");
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        <Plus aria-hidden />
        {t("new")}
      </Button>
      {open && <NewKnowledgeBaseDialog orgId={orgId} onOpenChange={setOpen} />}
    </>
  );
}
