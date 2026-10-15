import { describe, expect, it } from "vitest";

import en from "@/messages/en.json";
import vi from "@/messages/vi.json";

import { STORAGE_PROBLEMS, storageProblem } from "./storage-errors";
import { INTERRUPTED_CODE, NETWORK_CODE, StorageUploadError } from "./upload";

describe("storageProblem", () => {
  it.each([
    [400, "EntityTooLarge", "sizeMismatch"],
    [400, "EntityTooSmall", "sizeMismatch"],
    [403, "AccessDenied", "denied"],
    [403, "RequestTimeTooSkewed", "denied"],
    [403, "SignatureDoesNotMatch", "denied"],
    [403, "InvalidAccessKeyId", "denied"],
    [400, "RequestTimeout", "timeout"],
    [503, "SlowDown", "busy"],
    [503, "ServiceUnavailable", "busy"],
    [500, "InternalError", "fault"],
    [0, NETWORK_CODE, "unreachable"],
    [0, INTERRUPTED_CODE, "interrupted"],
    [400, "MalformedPOSTRequest", "rejected"],
    [502, "502", "fault"],
    [503, "503", "busy"],
  ] as const)("maps %i %s to %s", (status, code, problem) => {
    expect(storageProblem(new StorageUploadError(status, code))).toBe(problem);
  });

  it("has a message for every problem in both locales", () => {
    for (const messages of [en, vi]) {
      for (const problem of STORAGE_PROBLEMS) {
        expect(messages.kb.storage[problem]).toContain("<mono>{code}</mono>");
      }
    }
  });
});
