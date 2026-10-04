import { randomUUID } from "node:crypto";

import type { Model, RoleAssignment } from "../features/registry/types.ts";

/** SYNTHETIC registry rows in the documented M0.2 shapes; the real ones come from the owner's providers. */
const providerId = randomUUID();

export const models: Model[] = [
  {
    id: randomUUID(),
    providerId,
    modelRef: "anthropic/claude-sonnet-5.5",
    displayName: "Claude Sonnet 5.5",
    capabilities: ["chat", "tools", "reasoning", "vision"],
    contextWindow: 200_000,
    inputPricePerMTok: 3,
    outputPricePerMTok: 15,
    source: "sync",
  },
  {
    id: randomUUID(),
    providerId,
    modelRef: "openai/gpt-5-mini",
    displayName: "GPT-5 mini",
    capabilities: ["chat", "tools"],
    contextWindow: 128_000,
    inputPricePerMTok: 0.25,
    outputPricePerMTok: 2,
    source: "sync",
  },
];

export const roles: RoleAssignment[] = [
  { role: "chat.default", modelId: models[0]?.id ?? "" },
  { role: "chat.fast", modelId: models[1]?.id ?? "" },
];
