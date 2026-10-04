import { describe, expect, it } from "vitest";

import en from "./en.json";
import vi from "./vi.json";

function keys(obj: Record<string, unknown>, prefix = ""): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    v && typeof v === "object" ? keys(v as Record<string, unknown>, `${prefix}${k}.`) : [`${prefix}${k}`],
  );
}

describe("messages", () => {
  it("vi and en have identical keys", () => {
    expect(keys(en).sort()).toEqual(keys(vi).sort());
  });
  it("has no empty strings", () => {
    for (const [locale, file] of Object.entries({ vi, en })) {
      const empty = keys(file).filter(
        (k) => k.split(".").reduce<unknown>((o, p) => (o as Record<string, unknown>)[p], file) === "",
      );
      expect(empty, locale).toEqual([]);
    }
  });
});
