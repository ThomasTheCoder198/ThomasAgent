"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { RouteStatus } from "@/features/shell/RouteStatus";

type Props = { serviceDown: boolean; message: string; requestId?: string };

/** `serviceDown` (a 5xx or unreachable) is the retryable service error; any other refusal shows core's own words. */
export function LoadFailure({ serviceDown, message, requestId }: Props) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const router = useRouter();
  if (serviceDown) {
    return <RouteStatus kind="error" workspace requestId={requestId} retry={() => router.refresh()} />;
  }
  return (
    <div className="flex flex-col items-start gap-4 py-6">
      <InlineAlert
        title={t("loadFailed")}
        detail={requestId ? `${message} ${tErr("requestId", { id: requestId })}` : message}
      />
      <Button variant="outline" onClick={() => router.refresh()}>
        {t("retry")}
      </Button>
    </div>
  );
}
