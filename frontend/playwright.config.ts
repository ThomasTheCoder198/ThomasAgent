import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  // Review captures run only on demand: `SCREENS=1 npx playwright test --grep @screens --project desktop`.
  grepInvert: process.env.SCREENS ? undefined : /@screens/,
  // The app picks its locale from Accept-Language; the owner's default is Vietnamese.
  use: { baseURL: process.env.WEB_BASE_URL ?? "http://localhost:3000", locale: "vi-VN" },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
});
