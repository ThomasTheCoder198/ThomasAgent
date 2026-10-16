import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Orb } from "./Orb";

const realMatchMedia = window.matchMedia;
const realGetContext = HTMLCanvasElement.prototype.getContext;

function mockReducedMotion(matches: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: matches && query.includes("reduced-motion"),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  })) as unknown as typeof window.matchMedia;
}

describe("Orb", () => {
  const clearRect = vi.fn();
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame", "performance"] });
    clearRect.mockClear();
    HTMLCanvasElement.prototype.getContext = (() => ({
      setTransform: () => undefined,
      clearRect,
      beginPath: () => undefined,
      arc: () => undefined,
      fill: () => undefined,
      moveTo: () => undefined,
      lineTo: () => undefined,
      stroke: () => undefined,
    })) as unknown as typeof realGetContext;
  });
  afterEach(() => {
    vi.useRealTimers();
    window.matchMedia = realMatchMedia;
    HTMLCanvasElement.prototype.getContext = realGetContext;
  });

  it("keeps animating when the OS asks for reduced motion (owner decision 2026-10-16)", () => {
    mockReducedMotion(true);
    render(<Orb state="working" size={64} color="#5558e6" />);
    const first = clearRect.mock.calls.length;
    vi.advanceTimersByTime(200);
    expect(clearRect.mock.calls.length).toBeGreaterThan(first);
  });

  it("animates the same without the preference, and is hidden from assistive tech", () => {
    mockReducedMotion(false);
    const { container } = render(<Orb state="searching" size={20} />);
    const first = clearRect.mock.calls.length;
    vi.advanceTimersByTime(200);
    expect(clearRect.mock.calls.length).toBeGreaterThan(first);
    expect(container.querySelector("canvas")).toHaveAttribute("aria-hidden", "true");
  });
});
