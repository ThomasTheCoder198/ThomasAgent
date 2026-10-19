import { Check } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import type { Model } from "@/features/registry/types";
import { formatTokens } from "@/lib/format";

import { modelFacts } from "../model-facts";
import type { SettingsRole } from "../role-fit";

type Props = {
  role: SettingsRole;
  /** The model the row shows now; undefined when nothing is chosen. */
  model: Model | undefined;
  /** The last Test round-trip in milliseconds; session-only (it disappears on reload, gate answer 5). */
  testedMs: number | null;
  /** Offered only when core serves PATCH /models/{id} (Q12) and the capabilities are unknown. */
  onEditCapabilities?: (model: Model) => void;
};

/** Capability chips, context and price, the Test result and the role key on one wrapping line. */
export function RoleFacts({ role, model, testedMs, onEditCapabilities }: Props) {
  const t = useTranslations("settings");
  const locale = useLocale();
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-2">
      {model ? (
        <>
          {model.capabilities.length === 0 ? (
            <>
              <span className="text-muted-foreground text-xs">{t("unknownCapabilities")}</span>
              {onEditCapabilities && (
                <Button variant="link" className="text-xs" onClick={() => onEditCapabilities(model)}>
                  {t("edit")}
                </Button>
              )}
            </>
          ) : (
            model.capabilities.map((c) => <Badge key={c}>{c}</Badge>)
          )}
          <span className="tabular text-muted-foreground text-xs">{modelFacts(model, t("perMillion"))}</span>
        </>
      ) : (
        <Badge variant="warning">{role === "vision" ? t("unassignedVision") : t("unassigned")}</Badge>
      )}
      {testedMs !== null && (
        <Badge variant="success" className="animate-rise">
          <Check aria-hidden />
          {t("tested", { ms: formatTokens(testedMs, locale) })}
        </Badge>
      )}
      <code className="tabular text-muted-foreground ml-auto text-xs">{role}</code>
    </div>
  );
}
