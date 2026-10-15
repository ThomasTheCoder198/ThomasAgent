"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent, type RefObject } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent, DialogHeading } from "@/components/ui/Dialog";
import { Field, SwitchField } from "@/components/ui/Field";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { Input } from "@/components/ui/Input";
import type { Locale } from "@/i18n/config";
import { ApiError, describeApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";

import { createOrganization, retryOrganization, updateOrganization } from "./api";
import { orgHome, validateOrgName, type Organization, type OrgNameProblem } from "./contract";

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
};
type Props = ({ mode: "create" } & DialogProps) | ({ mode: "edit"; org: Organization } & DialogProps);

const NAME_PROBLEM_KEY: Record<OrgNameProblem, "nameRequired" | "nameTooLong" | "nameInvalid"> = {
  required: "nameRequired",
  tooLong: "nameTooLong",
  invalid: "nameInvalid",
};

function takenName(err: unknown): boolean {
  const fields =
    err instanceof ApiError
      ? (err.details?.fields as { field: string; code: string }[] | undefined)
      : undefined;
  return (
    err instanceof ApiError && err.code === ErrorCode.CONFLICT && !!fields?.some((f) => f.field === "name")
  );
}

/** Retry provisioning; `shown` is core's answer at once, and the refresh brings the same state to the sidebar. */
function useProvisionRetry(org: Organization | null) {
  const t = useTranslations("orgs");
  const tErr = useTranslations("errors");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [retried, setRetried] = useState<Organization>();
  const [failure, setFailure] = useState<string>();
  const [pending, setPending] = useState(false);

  async function retry() {
    if (!org) return;
    setPending(true);
    setFailure(undefined);
    try {
      setRetried(await retryOrganization(org.id, locale));
      router.refresh();
    } catch (err) {
      setFailure(describeApiError(err, t("errors.retryFailed"), (id) => tErr("requestId", { id })));
    } finally {
      setPending(false);
    }
  }

  return { shown: retried ?? org, retry, failure, pending };
}

type Draft = { name: string; externalParsing: boolean };

/** Validate, save and navigate; the field error and the failure alert are the only things the form shows. */
function useOrgSave(editing: Organization | null, onSaved: () => void) {
  const t = useTranslations("orgs");
  const tErr = useTranslations("errors");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [nameError, setNameError] = useState<string>();
  const [failure, setFailure] = useState<string>();
  const [saving, setSaving] = useState(false);

  async function save({ name, externalParsing }: Draft) {
    const problem = validateOrgName(name);
    setNameError(problem ? t(`errors.${NAME_PROBLEM_KEY[problem]}`) : undefined);
    if (problem) return;
    setSaving(true);
    setFailure(undefined);
    try {
      const input = { name: name.trim(), externalParsing };
      const saved = editing
        ? await updateOrganization(editing.id, input, locale)
        : await createOrganization(input, locale);
      onSaved();
      if (!editing) router.push(orgHome(saved.id));
      // The shell layout stays mounted across the navigation, so its org list and plate need a server refresh either way.
      router.refresh();
    } catch (err) {
      if (takenName(err)) setNameError(t("errors.nameTaken"));
      else setFailure(describeApiError(err, t("errors.saveFailed"), (id) => tErr("requestId", { id })));
    } finally {
      setSaving(false);
    }
  }

  return { save, nameError, failure, saving };
}

const editingOf = (props: Props) => (props.mode === "edit" ? props.org : null);

/** An edit counts only when it would change the saved org, so an unchanged form never sends a PATCH. */
const changesOrg = (editing: Organization | null, { name, externalParsing }: Draft) =>
  !editing || name.trim() !== editing.name || externalParsing !== editing.externalParsing;

function useOrgCopy(editing: Organization | null) {
  const t = useTranslations("orgs");
  const common = useTranslations("common");
  return editing
    ? { title: t("settingsTitle"), description: undefined, submit: common("save") }
    : { title: t("createTitle"), description: t("createBody"), submit: t("create") };
}

export function OrgDialog(props: Props) {
  const t = useTranslations("orgs");
  const editing = editingOf(props);
  const copy = useOrgCopy(editing);
  const [name, setName] = useState(editing?.name ?? "");
  const [externalParsing, setExternalParsing] = useState(editing?.externalParsing ?? true);
  const saving = useOrgSave(editing, () => props.onOpenChange(false));
  const retrying = useProvisionRetry(editing);
  const pending = saving.saving || retrying.pending;
  const failure = saving.failure ?? retrying.failure;
  const dirty = changesOrg(editing, { name, externalParsing });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (dirty) void saving.save({ name, externalParsing });
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent returnFocusRef={props.returnFocusRef}>
        <DialogHeading title={copy.title} description={copy.description} />
        <form onSubmit={submit} noValidate className="flex flex-col gap-5">
          <Field label={t("name")} error={saving.nameError}>
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
          <SwitchField
            label={t("externalParsing")}
            hint={t("externalParsingHint")}
            checked={externalParsing}
            onCheckedChange={setExternalParsing}
          />
          {retrying.shown && (
            <ProvisioningState org={retrying.shown} pending={pending} onRetry={retrying.retry} />
          )}
          {failure && <InlineAlert title={failure} />}
          <FormActions submitLabel={copy.submit} pending={pending} dirty={dirty} />
        </form>
      </DialogContent>
    </Dialog>
  );
}

function FormActions({
  submitLabel,
  pending,
  dirty,
}: {
  submitLabel: string;
  pending: boolean;
  dirty: boolean;
}) {
  const common = useTranslations("common");
  return (
    <div className="flex flex-wrap justify-end gap-2">
      <DialogClose asChild>
        <Button variant="ghost">{common("cancel")}</Button>
      </DialogClose>
      <Button type="submit" pending={pending} disabled={!dirty}>
        {submitLabel}
      </Button>
    </div>
  );
}

function ProvisioningState({
  org,
  pending,
  onRetry,
}: {
  org: Organization;
  pending: boolean;
  onRetry: () => void;
}) {
  const t = useTranslations("orgs");
  // Provisioning is synchronous, so a pending org is stuck (a process died mid-call): Retry is the way out.
  const needsRetry = org.ragxStatus !== "ready" || !org.settingsSynced;
  const statusRef = useRef<HTMLParagraphElement>(null);
  const retryClicked = useRef(false);
  // The Retry button unmounts (or is disabled) when core answers, which would drop focus to the dialog:
  // keep it on the status line, which is also the live region that announces the new state.
  useEffect(() => {
    if (retryClicked.current && !pending) {
      retryClicked.current = false;
      statusRef.current?.focus();
    }
  }, [org, pending]);
  return (
    <div className="border-border flex flex-wrap items-center justify-between gap-3 border-t pt-4 text-sm">
      <p ref={statusRef} role="status" tabIndex={-1} className="outline-none">
        <span className="font-medium">{t(`status.${org.ragxStatus}`)}</span>
        {!org.settingsSynced && org.ragxStatus === "ready" && (
          <span className="text-muted-foreground"> · {t("settingsSyncing")}</span>
        )}
        {org.ragxErrorCode && (
          <span className="tabular text-destructive mt-0.5 block text-xs">{org.ragxErrorCode}</span>
        )}
      </p>
      {needsRetry && (
        <Button
          variant="outline"
          onClick={() => {
            retryClicked.current = true;
            onRetry();
          }}
          pending={pending}
        >
          {t("retry")}
        </Button>
      )}
    </div>
  );
}
