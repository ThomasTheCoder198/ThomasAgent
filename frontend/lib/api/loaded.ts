/** Data a page needs but can survive without: either the value, or the localized reason it is missing. */
export type Loaded<T> = { ok: true; data: T } | { ok: false; message: string };

/** The value when loaded, otherwise the fallback — for places that only decorate (e.g. a header label). */
export function loadedOr<T>(loaded: Loaded<T>, fallback: T): T {
  return loaded.ok ? loaded.data : fallback;
}
