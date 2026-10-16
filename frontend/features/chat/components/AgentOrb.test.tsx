import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AgentOrb, orbFor } from "./AgentOrb";

const orb = vi.hoisted(() => vi.fn());
// jsdom has no canvas; the props AgentOrb hands the orb are what it controls. Motion lives in Orb.test.tsx.
vi.mock("@/components/Orb", () => ({
  Orb: (props: Record<string, unknown>) => {
    orb(props);
    return null;
  },
}));

const lastProps = () => orb.mock.calls.at(-1)?.[0] as Record<string, unknown>;
afterEach(() => {
  document.documentElement.removeAttribute("style");
  orb.mockClear();
});

describe("AgentOrb", () => {
  it("tints with a hex, falling back to the light token where the browser cannot resolve it", () => {
    render(<AgentOrb state="solving" size={32} />);
    expect(lastProps().color).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it("follows the token when the theme changes it", async () => {
    render(<AgentOrb state="searching" size={20} tint="kb" />);
    await act(async () => {
      document.documentElement.style.setProperty("--kb", "#4cc27a");
      document.documentElement.setAttribute("data-theme", "dark");
    });
    expect(lastProps().color).toBe("#4cc27a");
    document.documentElement.removeAttribute("data-theme");
  });

  it("maps each line to the state its work looks like", () => {
    expect(orbFor("kb")).toBe("searching");
    expect(orbFor("model")).toBe("solving");
    expect(orbFor("mcp")).toBe("connecting");
    expect(orbFor("cmp")).toBe("connecting");
    expect(orbFor("app")).toBe("connecting");
  });
});
