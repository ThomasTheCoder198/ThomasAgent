"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/Select";
import type { Model, Provider } from "@/features/registry/types";

import type { SettingsRole } from "../role-fit";
import { ModelCombobox } from "./ModelCombobox";

type Props = {
  role: SettingsRole;
  roleLabel: string;
  providers: Provider[];
  models: Model[];
  providerId: string | undefined;
  onProviderChange: (providerId: string) => void;
  /** The model shown in the picker (belongs to `providerId`), if any. */
  value: string | undefined;
  onPick: (modelId: string) => void;
  canTest: boolean;
  testing: boolean;
  onTest: () => void;
  onSyncProvider?: (providerId: string) => Promise<void>;
};

/** Provider, model and Test side by side from sm; stacked on phones with a full-width 44px Test. */
export function RoleControls({
  role,
  roleLabel,
  providers,
  models,
  providerId,
  onProviderChange,
  value,
  onPick,
  canTest,
  testing,
  onTest,
  onSyncProvider,
}: Props) {
  const t = useTranslations("settings");
  const sync = providerId && onSyncProvider ? () => onSyncProvider(providerId) : undefined;
  return (
    <div className="grid gap-3 sm:grid-cols-[minmax(0,11rem)_minmax(0,1fr)_auto] sm:items-end">
      <Field label={t("provider")}>
        {(control) => (
          <Select value={providerId} onValueChange={onProviderChange}>
            <SelectTrigger {...control} className="w-full">
              <SelectValue placeholder={t("pickProvider")} />
            </SelectTrigger>
            <SelectContent>
              {providers.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      <Field label={t("model")}>
        {(control) => (
          <ModelCombobox
            control={control}
            role={role}
            roleLabel={roleLabel}
            models={models.filter((m) => m.providerId === providerId)}
            value={value}
            onChange={onPick}
            disabled={!providerId}
            placeholder={providerId ? t("pickModel") : t("pickProviderFirst")}
            onSync={sync}
          />
        )}
      </Field>
      <Button
        variant="outline"
        onClick={onTest}
        disabled={!canTest}
        pending={testing}
        className="max-sm:w-full"
      >
        {t("test")}
      </Button>
    </div>
  );
}
