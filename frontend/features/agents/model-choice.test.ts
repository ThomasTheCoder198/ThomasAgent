import { describe, expect, it } from "vitest";

import { fromSelectValue, INHERIT_VALUE, toSelectValue, withOverride } from "./model-choice";

describe("model choice", () => {
  it("never hands Radix an empty-string value", () => {
    expect(INHERIT_VALUE).not.toBe("");
    expect(toSelectValue(undefined)).toBe(INHERIT_VALUE);
  });

  it("round-trips a model id and maps the sentinel back to no override", () => {
    expect(toSelectValue("m-1")).toBe("m-1");
    expect(fromSelectValue("m-1")).toBe("m-1");
    expect(fromSelectValue(INHERIT_VALUE)).toBeUndefined();
  });

  it("sets an override for the role and keeps the others", () => {
    expect(withOverride({ vision: "v-1" }, "chat.default", "m-1")).toEqual({
      vision: "v-1",
      "chat.default": "m-1",
    });
  });

  it("drops the role's entry when the owner picks inherit", () => {
    expect(withOverride({ vision: "v-1", "chat.default": "m-1" }, "chat.default", INHERIT_VALUE)).toEqual({
      vision: "v-1",
    });
  });
});
