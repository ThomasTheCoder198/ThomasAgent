import { z } from "zod";

const schema = z.object({
  WEB_CORE_URL: z.url().default("http://localhost:8080"),
  /** Ragx's own model-settings UI (spec ragx-custom-app §14.6); the sidebar link is hidden when unset. */
  WEB_RAGX_UI_URL: z.url().optional(),
});

export const env = schema.parse({
  WEB_CORE_URL: process.env.WEB_CORE_URL,
  WEB_RAGX_UI_URL: process.env.WEB_RAGX_UI_URL || undefined,
});
