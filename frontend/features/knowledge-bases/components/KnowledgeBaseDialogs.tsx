"use client";

import { Plus } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent } from "@/components/ui/Dialog";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { TextField } from "@/components/ui/TextField";
import type { Locale } from "@/i18n/config";
import { ApiError, describeApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KnowledgeBase } from "@/lib/ragx/types";
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
  onSubmit: (name: string) => Promise<void>;
};

/** One name field, validated like an organization's name; server field errors map back onto the field. */
function NameDialog({
  onOpenChange,
  title,
  description,
  submitLabel,
  initialName,
  onSubmit,
}: NameDialogProps) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const [name, setName] = useState(initialName);
  const [nameError, setNameError] = useState<string>();
  const [failure, setFailure] = useState<string>();
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const problem = validateDisplayName(name, KB_NAME_MAX_LENGTH);
    setNameError(problem ? t(`errors.${PROBLEM_KEY[problem]}`) : undefined);
    if (problem) return;
    setSaving(true);
    setFailure(undefined);
    try {
      await onSubmit(name.trim());
      onOpenChange(false);
    } catch (err) {
      if (isTaken(err)) setNameError(t("errors.nameTaken"));
      else setFailure(describeApiError(err, t("errors.saveFailed"), (id) => tErr("requestId", { id })));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent title={title} description={description}>
        <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-5">
          <TextField
            label={t("name")}
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={nameError}
            autoComplete="off"
            required
          />
          {failure && <InlineAlert title={failure} />}
          <div className="flex flex-wrap justify-end gap-2">
            <DialogClose asChild>
              <Button variant="ghost">{t("cancel")}</Button>
            </DialogClose>
            <Button type="submit" pending={saving} disabled={saving}>
              {submitLabel}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function NewKnowledgeBaseDialog({
  orgId,
  onOpenChange,
}: {
  orgId: string;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("kb");
  const locale = useLocale() as Locale;
  const router = useRouter();
  return (
    <NameDialog
      onOpenChange={onOpenChange}
      title={t("createTitle")}
      description={t("createBody")}
      submitLabel={t("create")}
      initialName=""
      onSubmit={async (name) => {
        const created = await createKnowledgeBase(orgId, { name }, locale);
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
  return (
    <NameDialog
      onOpenChange={onOpenChange}
      title={t("renameTitle")}
      submitLabel={common("save")}
      initialName={knowledgeBase.name}
      onSubmit={async (name) => {
        await renameKnowledgeBase(orgId, knowledgeBase.id, { name }, locale);
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
      {/* Icon-only on phones so the title keeps its width; the label stays for assistive tech. */}
      <Button
        onClick={() => setOpen(true)}
        className="min-h-touch max-sm:min-w-touch shrink-0 max-sm:px-0 md:min-h-0"
      >
        <Plus aria-hidden className="size-4" />
        <span className="max-sm:sr-only">{t("new")}</span>
      </Button>
      {open && <NewKnowledgeBaseDialog orgId={orgId} onOpenChange={setOpen} />}
    </>
  );
}
