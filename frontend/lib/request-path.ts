import { HOME_ROUTE } from "@/features/auth/paths";

/** Set by proxy.ts so server layouts know which route they render. */
export const PATHNAME_HEADER = "x-pathname";

export function pathFromHeaders(headers: Headers): string {
  return headers.get(PATHNAME_HEADER) ?? HOME_ROUTE;
}
