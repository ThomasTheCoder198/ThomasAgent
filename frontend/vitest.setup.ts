import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library only auto-unmounts when Vitest globals are on; they are off here, so unmount explicitly.
afterEach(cleanup);

// Files marked `@vitest-environment node` (pure data checks) have no window to patch.
if (typeof window !== "undefined") {
  // jsdom has no matchMedia; next-themes needs it when enableSystem is on.
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });

  // jsdom has no pointer capture; Sonner captures the pointer when a toast is pressed.
  Element.prototype.setPointerCapture ??= () => undefined;
  Element.prototype.releasePointerCapture ??= () => undefined;
  Element.prototype.hasPointerCapture ??= () => false;

  // jsdom has no ResizeObserver; Radix's Switch measures its control with one.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}
