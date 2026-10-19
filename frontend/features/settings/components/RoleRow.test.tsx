import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { RoleRow } from "./RoleRow";

const providers = [
  {
    id: "p1",
    kind: "openrouter",
    name: "OpenRouter",
    baseUrl: "",
    hasApiKey: true,
    enabled: true,
    createdAt: "",
    updatedAt: "",
  },
];
const models = [
  {
    id: "m1",
    providerId: "p1",
    modelRef: "a/one",
    displayName: "One",
    capabilities: ["chat", "tools"],
    contextWindow: 200000,
    inputPricePerMTok: 3,
    outputPricePerMTok: 15,
    source: "sync",
  },
  {
    id: "m2",
    providerId: "p1",
    modelRef: "a/two",
    displayName: "Two",
    capabilities: [],
    contextWindow: null,
    source: "sync",
  },
];

const wrap = (node: React.ReactNode) =>
  render(
    <NextIntlClientProvider locale="en" messages={messages}>
      {node}
    </NextIntlClientProvider>,
  );

describe("RoleRow", () => {
  it("shows the chosen model's capabilities, context and price", () => {
    wrap(
      <RoleRow
        role="chat.default"
        scope={{ scope: "platform" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m1", source: "platform" }}
        change={undefined}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText("tools")).toBeVisible();
    expect(screen.getByText(/200k ctx/)).toBeVisible();
  });

  it("says when capabilities are unknown", () => {
    wrap(
      <RoleRow
        role="chat.fast"
        scope={{ scope: "platform" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m2", source: "platform" }}
        change={undefined}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText(messages.settings.unknownCapabilities)).toBeVisible();
  });

  it("shows a missing override and offers to clear it", async () => {
    const onChange = vi.fn();
    wrap(
      <RoleRow
        role="chat.fast"
        scope={{ scope: "org", orgId: "o1" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "gone", source: "org" }}
        change={undefined}
        onChange={onChange}
      />,
    );
    expect(screen.getByText(messages.settings.modelMissing)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: messages.settings.clearOverride }));
    expect(onChange).toHaveBeenCalledWith({ kind: "clear" });
  });

  it("on an org tab, an inherited role says where it comes from", () => {
    wrap(
      <RoleRow
        role="chat.fast"
        scope={{ scope: "org", orgId: "o1" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m1", source: "platform" }}
        change={undefined}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/Platform default: One/)).toBeVisible();
  });

  it("labels an org decision fallback as the fast chat model, including an org override", () => {
    wrap(
      <RoleRow
        role="decision"
        scope={{ scope: "org", orgId: "o1" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m2", source: "fallback" }}
        platformModelId="m1"
        fastModel={models[1] as never}
        change={undefined}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText("Using the fast chat model: Two")).toBeVisible();
    expect(screen.queryByText(/Platform default: Two/)).not.toBeInTheDocument();
  });

  it("on the platform tab, the decision row can fall back to the fast chat model", async () => {
    const onChange = vi.fn();
    wrap(
      <RoleRow
        role="decision"
        scope={{ scope: "platform" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m1", source: "platform" }}
        change={undefined}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("radio", { name: messages.settings.useFast }));
    expect(onChange).toHaveBeenCalledWith({ kind: "clear" });
  });

  it("an org row can start an override from the inherited sentence", async () => {
    wrap(
      <RoleRow
        role="chat.fast"
        scope={{ scope: "org", orgId: "o1" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m1", source: "platform" }}
        change={undefined}
        onChange={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: messages.settings.override }));
    expect(screen.getByRole("button", { name: messages.settings.dropOverride })).toBeVisible();
    expect(screen.getByRole("combobox", { name: messages.settings.model })).toBeVisible();
  });

  it("offers to fix unknown capabilities when core can edit them", async () => {
    const onEditCapabilities = vi.fn();
    wrap(
      <RoleRow
        role="chat.fast"
        scope={{ scope: "platform" }}
        providers={providers as never}
        models={models as never}
        current={{ modelId: "m2", source: "platform" }}
        change={undefined}
        onChange={vi.fn()}
        onEditCapabilities={onEditCapabilities}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: messages.settings.edit }));
    expect(onEditCapabilities).toHaveBeenCalledWith(expect.objectContaining({ id: "m2" }));
  });
});
