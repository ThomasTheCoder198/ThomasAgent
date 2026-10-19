import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { AgentPicker } from "./AgentPicker";

const agents = [
  { id: "a1", name: "Pháp chế", kbName: "Chính sách", kbMissing: false },
  { id: "a2", name: "CSKH", kbName: "FAQ", kbMissing: false },
];

describe("AgentPicker", () => {
  it("lets the owner pick the agent for a new chat", async () => {
    const onChange = vi.fn();
    render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <AgentPicker agents={agents as never} agentId="a1" onChange={onChange} />
      </NextIntlClientProvider>,
    );
    await userEvent.click(screen.getByRole("combobox", { name: messages.chat.agent }));
    await userEvent.click(await screen.findByRole("option", { name: /CSKH/ }));
    expect(onChange).toHaveBeenCalledWith("a2");
  });

  it("is not shown when there is only one agent", () => {
    const { container } = render(
      <NextIntlClientProvider locale="en" messages={messages}>
        <AgentPicker agents={agents.slice(0, 1) as never} agentId="a1" onChange={vi.fn()} />
      </NextIntlClientProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
