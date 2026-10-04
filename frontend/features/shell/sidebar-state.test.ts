import { describe, expect, it } from "vitest";

import { isEditableTarget, isToggleShortcut, shortcutHint } from "./sidebar-state";

const key = (init: KeyboardEventInit) => new KeyboardEvent("keydown", init);

describe("sidebar shortcut", () => {
  it("recognises Ctrl+B and Cmd+B only", () => {
    expect(isToggleShortcut(key({ key: "b", ctrlKey: true }))).toBe(true);
    expect(isToggleShortcut(key({ key: "B", metaKey: true }))).toBe(true);
    expect(isToggleShortcut(key({ key: "b" }))).toBe(false);
    expect(isToggleShortcut(key({ key: "k", ctrlKey: true }))).toBe(false);
  });

  it("treats inputs, textareas, selects and contenteditable as editing targets", () => {
    const editable = document.createElement("div");
    // jsdom does not reflect the contentEditable property to the attribute (browsers do); author it directly.
    editable.setAttribute("contenteditable", "true");
    for (const el of [
      document.createElement("input"),
      document.createElement("textarea"),
      document.createElement("select"),
      editable,
    ]) {
      expect(isEditableTarget(el)).toBe(true);
    }
    expect(isEditableTarget(document.createElement("button"))).toBe(false);
    expect(isEditableTarget(null)).toBe(false);
  });

  it("names the shortcut the way the platform writes it", () => {
    expect(shortcutHint("MacIntel")).toBe("⌘B");
    expect(shortcutHint("Win32")).toBe("Ctrl+B");
  });
});
