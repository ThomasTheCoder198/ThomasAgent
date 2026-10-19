"use client";

import { Plus } from "lucide-react";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeading } from "@/components/ui/Dialog";
import { Field } from "@/components/ui/Field";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { Input } from "@/components/ui/Input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import { Textarea } from "@/components/ui/Textarea";
import { orgHome } from "@/features/orgs/contract";
import type { Locale } from "@/i18n/config";
import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KnowledgeBase } from "@/lib/ragx/types";
import { useNotify } from "@/lib/useNotify";

import { createAgent } from "../api";
import { AGENT_DESCRIPTION_MAX, AGENT_NAME_MAX, agentPage } from "../contract";

type Props = {
  orgId: string;
  knowledgeBases: KnowledgeBase[];
  onOpenChange: (open: boolean) => void;
};

const nameTaken = (err: unknown) =>
  err instanceof ApiError &&
  err.code === ErrorCode.CONFLICT &&
  !!(err.details?.fields as { field: string }[] | undefined)?.some((f) => f.field === "name");

const runes = (value: string) => [...value].length;

/**
 * Name, optional description and the one KB (R12, index "Agents"). Instructions and model overrides are edited on
 * the Agent's page afterwards, so creating stays one short step.
 */
export function NewAgentDialog({ orgId, knowledgeBases, onOpenChange }: Props) {
  const t = useTranslations("agents");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [kbId, setKbId] = useState(knowledgeBases[0]?.id ?? "");
  const [nameError, setNameError] = useState<string>();
  const [saving, setSaving] = useState(false);
  const hasKb = knowledgeBases.length > 0;

  function validate(): boolean {
    const trimmed = name.trim();
    const problem =
      trimmed === "" ? t("nameRequired") : runes(trimmed) > AGENT_NAME_MAX ? t("nameTooLong") : undefined;
    setNameError(problem);
    return !problem && runes(description) <= AGENT_DESCRIPTION_MAX && kbId !== "";
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!validate()) return;
    setSaving(true);
    try {
      const created = await createAgent(
        orgId,
        { name: name.trim(), description: description.trim(), kbId },
        locale,
      );
      notify.success(t("created", { name: created.name }));
      onOpenChange(false);
      router.push(agentPage(created.id));
    } catch (err) {
      if (nameTaken(err)) setNameError(t("nameTaken"));
      else notify.error(err);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeading title={t("createTitle")} description={t("createBody")} />
        <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-4">
          <Field label={t("name")} error={nameError}>
            {(control) => (
              <Input
                {...control}
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setNameError(undefined);
                }}
                autoComplete="off"
                required
              />
            )}
          </Field>
          <Field
            label={t("description")}
            error={runes(description) > AGENT_DESCRIPTION_MAX ? t("descriptionTooLong") : undefined}
          >
            {(control) => (
              <Textarea
                {...control}
                rows={2}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            )}
          </Field>
          {hasKb ? (
            <Field label={t("tabKnowledge")}>
              {(control) => (
                <Select value={kbId} onValueChange={setKbId}>
                  <SelectTrigger {...control} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {knowledgeBases.map((kb) => (
                      <SelectItem key={kb.id} value={kb.id}>
                        {kb.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </Field>
          ) : (
            // `InlineAlert.detail` takes a string only, so the link sits in its children.
            <InlineAlert title={t("needKb")}>
              <Link href={orgHome(orgId)} className="text-primary mt-1 inline-block underline">
                {t("needKbAction")}
              </Link>
            </InlineAlert>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="ghost">{t("cancel")}</Button>
            </DialogClose>
            <Button type="submit" pending={saving} disabled={!hasKb}>
              {t("create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** The page header's action: the trigger and its dialog together. */
export function NewAgentButton({ orgId, knowledgeBases }: Omit<Props, "onOpenChange">) {
  const t = useTranslations("agents");
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        <Plus aria-hidden />
        {t("new")}
      </Button>
      {open && <NewAgentDialog orgId={orgId} knowledgeBases={knowledgeBases} onOpenChange={setOpen} />}
    </>
  );
}
