"use client";

import { useTranslations } from "next-intl";

import { Field, SwitchField } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import type { ProviderKind } from "@/features/registry/types";

import { CHAT_PROVIDER_KINDS } from "../contract";
import {
  keyHint,
  needsBaseUrl,
  problemKey,
  PROVIDER_KIND_LABELS,
  showsKey,
  type FieldName,
  type FieldProblems,
  type ProviderFields,
} from "../provider-form";

type Props = {
  fields: ProviderFields;
  onChange: (next: Partial<ProviderFields>) => void;
  problems: FieldProblems;
  editing: boolean;
  /** The address differs from the stored one: core will drop the key unless a new one is typed. */
  baseUrlDirty: boolean;
  sync: boolean;
  onSyncChange: (sync: boolean) => void;
};

/** The kind is fixed when editing; the key never shows a stored value; the address only for kinds that need one. */
export function ProviderFormFields({
  fields,
  onChange,
  problems,
  editing,
  baseUrlDirty,
  sync,
  onSyncChange,
}: Props) {
  const t = useTranslations("settings");
  const { kind, name, baseUrl, apiKey } = fields;
  const problemText = (field: FieldName) => {
    const problem = problems[field];
    return problem ? t(problemKey(problem)) : undefined;
  };
  return (
    <>
      <Field label={t("kind")}>
        {(control) => (
          <Select
            value={kind}
            onValueChange={(next) => onChange({ kind: next as ProviderKind })}
            disabled={editing}
          >
            <SelectTrigger {...control} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(editing ? [kind] : CHAT_PROVIDER_KINDS).map((k) => (
                <SelectItem key={k} value={k}>
                  {PROVIDER_KIND_LABELS[k]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      <Field label={t("providerName")} error={problemText("name")}>
        {(control) => (
          <Input
            {...control}
            value={name}
            onChange={(e) => onChange({ name: e.target.value })}
            placeholder={PROVIDER_KIND_LABELS[kind]}
            autoComplete="off"
          />
        )}
      </Field>
      {needsBaseUrl(kind) && (
        <Field label={t("baseUrl")} hint={t("baseUrlHint")} error={problemText("baseUrl")}>
          {(control) => (
            <Input
              {...control}
              type="url"
              inputMode="url"
              value={baseUrl}
              onChange={(e) => onChange({ baseUrl: e.target.value })}
              placeholder="http://localhost:8000/v1"
              required
            />
          )}
        </Field>
      )}
      {showsKey(kind) && (
        <Field
          label={t("apiKey")}
          hint={t(keyHint(kind, editing, baseUrlDirty, apiKey !== ""))}
          error={problemText("apiKey")}
        >
          {(control) => (
            <Input
              {...control}
              type="password"
              autoComplete="off"
              value={apiKey}
              onChange={(e) => onChange({ apiKey: e.target.value })}
              placeholder="sk-…"
            />
          )}
        </Field>
      )}
      {!editing && (
        <SwitchField
          label={t("syncAfterSave")}
          hint={t("syncAfterSaveHint")}
          checked={sync}
          onCheckedChange={onSyncChange}
        />
      )}
    </>
  );
}
