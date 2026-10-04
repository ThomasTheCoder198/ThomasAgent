import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

import { env } from "./lib/env";

const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: `${env.WEB_CORE_URL}/api/v1/:path*` }];
  },
};

export default withNextIntl(nextConfig);
