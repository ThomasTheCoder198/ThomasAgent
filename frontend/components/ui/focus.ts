/**
 * Shared focus looks. Controls that are pressed (buttons, tabs, toggles) show a ring on keyboard focus only, so a
 * mouse click never leaves one behind; fields show theirs on any focus because the caret already says "typing here".
 * Neither uses a ring offset or an outline: the halo is one soft 3px bloom.
 */
export const pressFocus = "focus-visible:ring-ring/40 focus-visible:ring-[3px] focus-visible:outline-none";

export const fieldFocus =
  "focus-visible:border-primary/60 focus-visible:ring-halo focus-visible:ring-[3px] focus-visible:outline-none";
