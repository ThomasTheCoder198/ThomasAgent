/** Locale-aware numerals for the mono measurements: durations, tokens, cost, percentages. */
const MS_PER_SECOND = 1000;
const COST_DIGITS = 4;
const SMALL_PERCENT = 10;
export const PERCENT = 100;

export function formatSeconds(ms: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 1, minimumFractionDigits: 1 }).format(
    ms / MS_PER_SECOND,
  );
}

export function formatTokens(count: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(count);
}

export function formatCost(usd: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: COST_DIGITS,
  }).format(usd);
}

const BYTE_STEP = 1024;
const BYTE_UNITS = ["byte", "kilobyte", "megabyte", "gigabyte"] as const;

/** File sizes in the largest unit that keeps the number ≥ 1, with the locale's unit names and decimals. */
export function formatBytes(bytes: number, locale: string): string {
  let value = bytes;
  let unit = 0;
  while (value >= BYTE_STEP && unit < BYTE_UNITS.length - 1) {
    value /= BYTE_STEP;
    unit += 1;
  }
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: BYTE_UNITS[unit],
    unitDisplay: "short",
    maximumFractionDigits: 1,
  }).format(value);
}

export function formatDateTime(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso));
}

/** Below 10% one decimal still moves when a message lands; above it, whole points read cleaner. */
export function formatPercent(ratio: number, locale: string): string {
  const percent = ratio * PERCENT;
  return new Intl.NumberFormat(locale, { maximumFractionDigits: percent < SMALL_PERCENT ? 1 : 0 }).format(
    percent,
  );
}
