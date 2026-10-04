import "server-only";

import { redirect } from "next/navigation";

import { ApiError } from "@/lib/api/client";
import { serverFetch } from "@/lib/api/server";
import { ErrorCode } from "@/lib/errors/codes.gen";

import { AUTH_PATHS, loginUrl } from "./paths";
import type { SessionUser } from "./types";

export type { SessionUser } from "./types";

const SIGNED_OUT_CODES = new Set<string>([ErrorCode.UNAUTHENTICATED, ErrorCode.AUTH_SESSION_EXPIRED]);

export async function requireUser(nextPath: string): Promise<SessionUser> {
  try {
    const { user } = await serverFetch<{ user: SessionUser }>(AUTH_PATHS.me);
    return user;
  } catch (err) {
    if (err instanceof ApiError && SIGNED_OUT_CODES.has(err.code)) {
      redirect(loginUrl(nextPath));
    }
    throw err;
  }
}
