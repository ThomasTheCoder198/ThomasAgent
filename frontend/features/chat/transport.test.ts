import { describe, expect, it } from "vitest";

import type { ChatMessage } from "./contract";
import { toChatBody } from "./transport";

const message: ChatMessage = { id: "m1", role: "user", parts: [{ type: "text", text: "Xin chào" }] };

describe("toChatBody", () => {
  it("sends only the newest message with the chosen agent and model", () => {
    expect(
      toChatBody({
        id: "c1",
        trigger: "submit-message",
        messageId: undefined,
        messages: [message],
        options: { modelId: "m-2", agentId: "a-1" },
      }),
    ).toEqual({
      id: "c1",
      trigger: "submit-message",
      messageId: undefined,
      message,
      modelId: "m-2",
      agentId: "a-1",
    });
  });

  it("drops options that are not strings", () => {
    const body = toChatBody({
      id: "c1",
      trigger: "regenerate-message",
      messageId: "m1",
      messages: [message],
      options: { modelId: 3, agentId: null },
    });
    expect(body.modelId).toBeUndefined();
    expect(body.agentId).toBeUndefined();
  });
});
