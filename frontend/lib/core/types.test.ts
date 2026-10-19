import { describe, expectTypeOf, it } from "vitest";

import type {
  Agent,
  AgentPatch,
  ChatRequest,
  ChatScope,
  CitationLocatorWire,
  OrgModelRole,
  RunUsage,
} from "./types";

describe("core types generated from contracts/openapi/core.v1.yaml", () => {
  it("expose the M2 shapes the web uses", () => {
    expectTypeOf<Agent["kbId"]>().toEqualTypeOf<string>();
    expectTypeOf<Agent>().toHaveProperty("kbMissing");
    expectTypeOf<AgentPatch>().toHaveProperty("modelOverrides");
    expectTypeOf<ChatRequest["trigger"]>().toEqualTypeOf<"submit-message" | "regenerate-message">();
    expectTypeOf<ChatRequest>().toHaveProperty("agentId");
    expectTypeOf<ChatScope["kb"]>().toHaveProperty("missing");
    expectTypeOf<OrgModelRole["source"]>().toEqualTypeOf<"org" | "platform" | "fallback" | "none">();
    expectTypeOf<RunUsage>().toHaveProperty("ttftMs");
    expectTypeOf<CitationLocatorWire["fileVersion"]>().toEqualTypeOf<number | undefined>();
  });
});
