import { NextResponse, type NextRequest } from "next/server";

import { LOGIN_ROUTE, SESSION_COOKIE } from "@/features/auth/paths";
import { CURRENT_ORG_COOKIE } from "@/features/orgs/contract";
import { orgIdFromPath } from "@/features/orgs/current";
import { PATHNAME_HEADER } from "@/lib/request-path";

export function proxy(request: NextRequest) {
  if (!request.cookies.has(SESSION_COOKIE)) {
    const url = new URL(LOGIN_ROUTE, request.url);
    url.searchParams.set("next", request.nextUrl.pathname);
    return NextResponse.redirect(url);
  }
  // Server components cannot read the current path; forward it as a request header for the workspace layout.
  const headers = new Headers(request.headers);
  headers.set(PATHNAME_HEADER, request.nextUrl.pathname);
  const response = NextResponse.next({ request: { headers } });
  const orgId = orgIdFromPath(request.nextUrl.pathname);
  if (orgId) {
    // Remember the org for pages outside /orgs (the sidebar link, /knowledge-bases); core still checks every request.
    response.cookies.set(CURRENT_ORG_COOKIE, orgId, {
      path: "/",
      sameSite: "lax",
      httpOnly: true,
      maxAge: CURRENT_ORG_MAX_AGE_SECONDS,
    });
  }
  return response;
}

const SECONDS_PER_DAY = 86_400;
const DAYS_PER_YEAR = 365;
const CURRENT_ORG_MAX_AGE_SECONDS = SECONDS_PER_DAY * DAYS_PER_YEAR;

export const config = {
  matcher: ["/((?!api|login|_next/static|_next/image|favicon.ico|robots.txt).*)"],
};
