"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/Button";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { RouteStatus } from "@/features/shell/RouteStatus";

/**
 * Ragx or core is down or unreachable (status >= 500): the whole page is the retryable service error, with
 * the request id in mono. It replaces the page, header included, so there is one heading and one menu button.
 */
export function ServiceUnavailable({ requestId }: { requestId?: string }) {
  const router = useRouter();
  return <RouteStatus kind="error" workspace requestId={requestId} retry={() => router.refresh()} />;
}

/** Any other refusal (not a 404, not a 5xx) shows core's own words under the page header, with Retry. */
export function LoadFailure({ message, requestId }: { message: string; requestId?: string }) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const router = useRouter();
  return (
    <div className="flex flex-col items-start gap-4 py-6">
      <InlineAlert title={t("loadFailed")} detail={message}>
        {requestId && (
          <p className="text-muted-foreground tabular mt-0.5 text-xs break-all">
            {tErr("requestId", { id: requestId })}
          </p>
        )}
      </InlineAlert>
      <Button variant="outline" onClick={() => router.refresh()}>
        {t("retry")}
      </Button>
    </div>
  );
}
