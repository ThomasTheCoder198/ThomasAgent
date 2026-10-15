import { Waypoints } from "lucide-react";
import { getTranslations } from "next-intl/server";

import { EmptyState } from "@/features/shell/EmptyState";

export default async function ToolsPage() {
  const t = await getTranslations("empty");
  return <EmptyState line="mcp" icon={Waypoints} title={t("toolsTitle")} body={t("toolsBody")} />;
}
