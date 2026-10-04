/**
 * The desktop sidebar's collapsed preference lives in a cookie so the server renders the right width on the
 * first paint (no flash). It is a per-viewer convenience, not data.
 */
export const SIDEBAR_COOKIE = "thomas_sidebar";
export const SIDEBAR_COLLAPSED = "collapsed";
const SIDEBAR_OPEN = "open";
const ONE_YEAR_SECONDS = 31_536_000;

/** ⌘B on Apple platforms, Ctrl+B elsewhere — the common "toggle sidebar" binding. */
const SHORTCUT_KEY = "b";
const APPLE_PLATFORM = /mac|iphone|ipad/i;
const MAC_HINT = "⌘B";
const OTHER_HINT = "Ctrl+B";
const EDITABLE_HOST = '[contenteditable=""], [contenteditable="true"], [contenteditable="plaintext-only"]';

export function isCollapsedValue(value: string | undefined): boolean {
  return value === SIDEBAR_COLLAPSED;
}

export function writeSidebarCookie(collapsed: boolean) {
  const value = collapsed ? SIDEBAR_COLLAPSED : SIDEBAR_OPEN;
  document.cookie = `${SIDEBAR_COOKIE}=${value}; path=/; max-age=${ONE_YEAR_SECONDS}; samesite=lax`;
}

export function isToggleShortcut(event: KeyboardEvent): boolean {
  return (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === SHORTCUT_KEY;
}

/** Typing Ctrl+B in a field means "bold" or nothing — never "hide the sidebar". */
export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)) return true;
  return target.isContentEditable || target.closest(EDITABLE_HOST) !== null;
}

export function shortcutHint(platform: string): string {
  return APPLE_PLATFORM.test(platform) ? MAC_HINT : OTHER_HINT;
}
