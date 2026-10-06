// Values mirror contracts/request-id.json; request-id.test.ts fails when they drift.
export const REQUEST_ID_HEADER = "X-Request-Id";
export const REQUEST_ID_PATTERN = /^(req|job)_[A-Za-z0-9-]{8,64}$/;
export const REQUEST_ID_PREFIX = "req_";
export const JOB_ID_PREFIX = "job_";

const RANDOM_BYTES = 16;
const HEX_RADIX = 16;
const HEX_DIGITS_PER_BYTE = 2;

/**
 * One id per API call; a retry gets a new one. `crypto.randomUUID` only exists in secure contexts, so a plain-HTTP
 * origin falls back to random bytes instead of breaking every request.
 */
export function newRequestId(): string {
  if (typeof crypto.randomUUID === "function") return REQUEST_ID_PREFIX + crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(RANDOM_BYTES));
  return (
    REQUEST_ID_PREFIX +
    Array.from(bytes, (byte) => byte.toString(HEX_RADIX).padStart(HEX_DIGITS_PER_BYTE, "0")).join("")
  );
}
