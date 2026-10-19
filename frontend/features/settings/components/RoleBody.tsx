"use client";

import { TriangleAlert } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import type { Model, Provider } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { useNotify } from "@/lib/useNotify";

import { testModel } from "../api";
import type { SettingsScope } from "../contract";
import type { SettingsRole } from "../role-fit";
import { ownModelId, type RoleCurrent } from "../role-view";
import type { RoleChange } from "../useRoleDraft";
import { RoleControls } from "./RoleControls";
import { RoleFacts } from "./RoleFacts";

/** The model the stored or drafted role points at no longer exists (a deleted model or provider). */
export function MissingModel({ onClear }: { onClear: () => void }) {
  const t = useTranslations("settings");
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Badge variant="destructive">
        <TriangleAlert aria-hidden />
        {t("modelMissing")}
      </Badge>
      <Button variant="outline" size="sm" onClick={onClear}>
        {t("clearOverride")}
      </Button>
    </div>
  );
}

/** The platform decision role while it follows the fast chat model. */
export function FastNote({ model }: { model: Model | undefined }) {
  const t = useTranslations("settings");
  return (
    <p className="text-muted-foreground flex min-h-10 items-center text-sm">
      {model ? t("usingFast", { model: model.displayName }) : t("usingFastNone")}
    </p>
  );
}

/** An org role that follows the platform: where it comes from, and the way to override it. */
export function InheritedLine({ text, onOverride }: { text: string; onOverride: () => void }) {
  const t = useTranslations("settings");
  return (
    <div className="flex min-h-10 flex-wrap items-center justify-between gap-3">
      <p className="text-muted-foreground text-sm">{text}</p>
      <Button variant="outline" size="sm" onClick={onOverride}>
        {t("override")}
      </Button>
    </div>
  );
}

/** Runs the model Test; the result lives only for this session (it disappears on reload, gate answer 5). */
function useModelTest(model: Model | undefined) {
  const locale = useLocale() as Locale;
  const notify = useNotify();
  const [testedMs, setTestedMs] = useState<number | null>(null);
  const [testing, setTesting] = useState(false);
  async function run() {
    if (!model) return;
    setTesting(true);
    try {
      setTestedMs((await testModel(model.id, locale)).latencyMs);
    } catch (err) {
      setTestedMs(null);
      notify.error(err);
    } finally {
      setTesting(false);
    }
  }
  return { testedMs, testing, run: () => void run(), forget: () => setTestedMs(null) };
}

type EditableProps = {
  role: SettingsRole;
  roleLabel: string;
  scope: SettingsScope;
  providers: Provider[];
  models: Model[];
  current: RoleCurrent;
  providerId: string | undefined;
  onProviderChange: (providerId: string) => void;
  /** The chosen model when it belongs to the chosen provider. */
  shown: Model | undefined;
  onChange: (next: RoleChange | undefined) => void;
  /** The org tab's "Remove override" (also shows "Overriding the platform default (X)"). */
  onDropOverride: () => void;
  platformName: string | undefined;
  onEditCapabilities?: (model: Model) => void;
  onSyncProvider?: (providerId: string) => Promise<void>;
};

/** Provider, model and Test, then the facts line; on an org tab with a line above that says it overrides. */
export function EditableRole(props: EditableProps) {
  const { role, scope, shown, current, onChange, platformName } = props;
  const t = useTranslations("settings");
  const test = useModelTest(shown);
  return (
    <>
      {scope.scope === "org" && (
        <div className="mb-3 flex items-center justify-between gap-3">
          <p className="text-muted-foreground text-sm">
            {platformName ? t("overridingModel", { model: platformName }) : t("overriding")}
          </p>
          <Button variant="ghost" size="sm" onClick={props.onDropOverride}>
            {t("dropOverride")}
          </Button>
        </div>
      )}
      <RoleControls
        role={role}
        roleLabel={props.roleLabel}
        providers={props.providers}
        models={props.models}
        providerId={props.providerId}
        onProviderChange={props.onProviderChange}
        value={shown?.id}
        onPick={(modelId) => {
          test.forget();
          onChange(modelId === ownModelId(scope, current) ? undefined : { kind: "set", modelId });
        }}
        canTest={!!shown}
        testing={test.testing}
        onTest={test.run}
        onSyncProvider={props.onSyncProvider}
      />
      <RoleFacts
        role={role}
        model={shown}
        testedMs={test.testedMs}
        onEditCapabilities={props.onEditCapabilities}
      />
    </>
  );
}
