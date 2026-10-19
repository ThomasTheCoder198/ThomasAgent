"use client";

import { useTranslations } from "next-intl";

import { InlineAlert } from "@/components/ui/InlineAlert";
import type { Model, Provider } from "@/features/registry/types";
import type { Loaded } from "@/lib/api/loaded";

import { ProviderTable } from "./ProviderTable";

/** Providers and models are platform-wide, so this section sits below the tab panel on every tab (mockup). */
export function ProvidersSection({
  providers,
  models,
}: {
  providers: Loaded<Provider[]>;
  models: Loaded<Model[]>;
}) {
  const t = useTranslations("settings");
  return (
    <section
      aria-labelledby="providers-title"
      className="mx-auto w-full max-w-5xl flex-1 px-4 pt-2 pb-28 md:px-6 lg:px-8"
    >
      <div className="border-border mb-4 border-t pt-8">
        <h2 id="providers-title" className="text-lg font-semibold tracking-tight">
          {t("providers")}
        </h2>
        <p className="text-muted-foreground mt-1 max-w-[70ch] text-sm">{t("providersHint")}</p>
      </div>
      {providers.ok ? (
        <ProviderTable providers={providers.data} models={models.ok ? models.data : []} />
      ) : (
        <InlineAlert title={t("providersFailed")} detail={providers.message} />
      )}
    </section>
  );
}
