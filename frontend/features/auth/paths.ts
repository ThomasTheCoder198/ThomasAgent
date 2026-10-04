/** Core auth endpoints (contracts/openapi/core.v1.yaml) and the web routes around them. */
export const AUTH_PATHS = {
  login: "/api/v1/auth/login",
  me: "/api/v1/auth/me",
  logout: "/api/v1/auth/logout",
} as const;

export const SESSION_COOKIE = "thomas_session";
export const LOGIN_ROUTE = "/login";
export const HOME_ROUTE = "/chat";

/** Only same-origin paths may follow a login, so `next` cannot bounce the owner to another site. */
export function safeNext(next: string | undefined): string {
  return next?.startsWith("/") && !next.startsWith("//") ? next : HOME_ROUTE;
}

export function loginUrl(nextPath: string): string {
  return `${LOGIN_ROUTE}?next=${encodeURIComponent(nextPath)}`;
}
