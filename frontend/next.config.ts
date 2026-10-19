import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

import { env } from "./lib/env";

const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

/**
 * Spike S8: stock Next cuts a proxied response that is quiet for about 30 s. Core heartbeats every 15 s and a run
 * lasts at most 5 minutes (CORE_CHAT_RUN_DEADLINE), so the rewrite must outlive a whole run with margin.
 */
export const PROXY_TIMEOUT_MS = 360_000;

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  experimental: { proxyTimeout: PROXY_TIMEOUT_MS },
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: `${env.WEB_CORE_URL}/api/v1/:path*` }];
  },
};

export default withNextIntl(nextConfig);
