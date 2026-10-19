import { SettingsModels } from "@/features/settings/components/SettingsModels";
import { getSettingsData } from "@/features/settings/server";
import { getCurrentOrg } from "@/features/orgs/server";

/**
 * Two scope tabs (owner 2026-10-18): the platform default and the org picked in the sidebar switcher, resolved
 * here like the other business pages (ruling C2-1). Both are loaded together so the org tab can badge its overrides.
 */
export default async function SettingsPage() {
  const { org, failed } = await getCurrentOrg();
  const data = await getSettingsData(org?.id ?? null);
  return <SettingsModels org={org ? { id: org.id, name: org.name } : null} orgsFailed={failed} {...data} />;
}
