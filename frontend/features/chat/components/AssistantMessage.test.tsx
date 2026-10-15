import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import type { ChatMessage } from "../contract";
import { AssistantMessage } from "./AssistantMessage";
import { HeroSources } from "./ChatHero";

const orbs = vi.hoisted(() => vi.fn());
vi.mock("thinking-orbs", () => ({
  ThinkingOrb: (props: Record<string, unknown>) => {
    orbs(props);
    return null;
  },
}));

const handlers = { onCite: vi.fn(), onDecide: vi.fn(), onInspect: vi.fn() };

function renderIntl(ui: React.ReactNode) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe("AssistantMessage", () => {
  it("shows one announced thinking line before the first part arrives", () => {
    orbs.mockClear();
    renderIntl(
      <AssistantMessage message={{ id: "a", role: "assistant", parts: [] }} streaming {...handlers} />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Thinking…");
    expect(orbs).toHaveBeenCalledWith(expect.objectContaining({ state: "solving", size: 32 }));
  });

  it("puts a tinted orb on the live tool station", () => {
    orbs.mockClear();
    const message: ChatMessage = {
      id: "a",
      role: "assistant",
      parts: [
        {
          type: "dynamic-tool",
          toolName: "kb_search",
          toolCallId: "k1",
          state: "input-available",
          input: { query: "refund" },
          toolMetadata: { line: "kb", source: "Policy" },
        },
      ],
    };
    renderIntl(<AssistantMessage message={message} streaming {...handlers} />);
    expect(orbs).toHaveBeenCalledWith(expect.objectContaining({ state: "searching", size: 20 }));
  });

  it("offers copy, inspect and regenerate on a finished answer", () => {
    const message: ChatMessage = {
      id: "a",
      role: "assistant",
      parts: [{ type: "text", text: "Done.", state: "done" }],
    };
    renderIntl(<AssistantMessage message={message} streaming={false} onRegenerate={vi.fn()} {...handlers} />);
    for (const name of ["Copy", "Retry", "Open Run Inspector"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });
});

describe("HeroSources", () => {
  it("names every source the agent can reach", () => {
    renderIntl(<HeroSources />);
    const list = screen.getByRole("list", { name: "Tool sources" });
    expect(list).toHaveTextContent(/Model.*KB.*MCP.*Composio.*Apps/);
  });
});
