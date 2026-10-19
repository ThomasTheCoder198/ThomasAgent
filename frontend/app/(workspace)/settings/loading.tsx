import { getTranslations } from "next-intl/server";

import { SettingsSkeleton } from "@/features/settings/SettingsSkeleton";

export default async function SettingsLoading() {
  const t = await getTranslations("common");
  return (
    <div role="status" aria-busy className="h-full overflow-y-auto">
      <span className="sr-only">{t("loading")}</span>
      <SettingsSkeleton />
    </div>
  );
}
