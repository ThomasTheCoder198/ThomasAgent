import { NextResponse, type NextRequest } from "next/server";

import { LOGIN_ROUTE, SESSION_COOKIE } from "@/features/auth/paths";
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
  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!api|login|_next/static|_next/image|favicon.ico|robots.txt).*)"],
};
