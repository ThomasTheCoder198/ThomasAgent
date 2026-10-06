import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  JOB_ID_PREFIX,
  REQUEST_ID_HEADER,
  REQUEST_ID_PATTERN,
  REQUEST_ID_PREFIX,
  newRequestId,
} from "./request-id";

type SharedDefinition = {
  header: string;
  pattern: string;
  requestPrefix: string;
  jobPrefix: string;
  valid: string[];
  invalid: string[];
};

const contract = JSON.parse(
  readFileSync(
    resolve(dirname(fileURLToPath(import.meta.url)), "../../../contracts/request-id.json"),
    "utf8",
  ),
) as SharedDefinition;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("request id", () => {
  it("matches the shared contract", () => {
    expect(REQUEST_ID_HEADER).toBe(contract.header);
    expect(REQUEST_ID_PATTERN.source).toBe(contract.pattern);
    expect(REQUEST_ID_PREFIX).toBe(contract.requestPrefix);
    expect(JOB_ID_PREFIX).toBe(contract.jobPrefix);
  });

  it("follows the shared examples", () => {
    for (const id of contract.valid) expect(REQUEST_ID_PATTERN.test(id), id).toBe(true);
    for (const id of contract.invalid) expect(REQUEST_ID_PATTERN.test(id), id).toBe(false);
  });

  it("creates a fresh valid id each call", () => {
    const first = newRequestId();
    expect(first.startsWith(REQUEST_ID_PREFIX)).toBe(true);
    expect(REQUEST_ID_PATTERN.test(first)).toBe(true);
    expect(newRequestId()).not.toBe(first);
  });

  it("falls back to random bytes without randomUUID", () => {
    const getRandomValues = crypto.getRandomValues.bind(crypto);
    vi.stubGlobal("crypto", { getRandomValues });
    const id = newRequestId();
    expect(REQUEST_ID_PATTERN.test(id)).toBe(true);
    expect(id.startsWith(REQUEST_ID_PREFIX)).toBe(true);
  });
});
