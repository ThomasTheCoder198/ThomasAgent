"use client";

import { RouteStatus } from "@/features/shell/RouteStatus";

export default function ErrorPage({
  error,
  retry,
}: {
  error: Error & { digest?: string; requestId?: string };
  retry: () => void;
}) {
  return <RouteStatus kind="error" requestId={error.requestId} retry={retry} />;
}
