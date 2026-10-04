import { describe, expect, it } from "vitest";

import { formatBytes, formatPercent, formatSeconds } from "./format";

describe("formatBytes", () => {
  it("uses the largest unit that keeps the number at least 1", () => {
    expect(formatBytes(512, "en")).toBe("512 byte");
    expect(formatBytes(8_412, "en")).toBe("8.2 kB");
    expect(formatBytes(5_400_000, "en")).toBe("5.1 MB");
  });

  it("follows the locale's decimal separator", () => {
    expect(formatBytes(8_412, "vi")).toBe("8,2 kB");
  });
});

describe("formatSeconds / formatPercent", () => {
  it("keeps one decimal for durations and small percentages", () => {
    expect(formatSeconds(1200, "en")).toBe("1.2");
    expect(formatPercent(0.017, "en")).toBe("1.7");
    expect(formatPercent(0.42, "en")).toBe("42");
  });
});
