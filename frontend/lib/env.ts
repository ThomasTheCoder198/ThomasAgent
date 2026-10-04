import { z } from "zod";

const schema = z.object({
  WEB_CORE_URL: z.url().default("http://localhost:8080"),
});

export const env = schema.parse({ WEB_CORE_URL: process.env.WEB_CORE_URL });
