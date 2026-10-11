// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import { UPLOAD_EXTENSIONS, UPLOAD_MAX_FILE_BYTES, checkUpload } from "./upload-rules";

const contract = JSON.parse(
  readFileSync(fileURLToPath(new URL("../../../contracts/kb-upload.json", import.meta.url)), "utf8"),
) as { maxFileBytes: number; extensions: string[] };

describe("upload rules", () => {
  it("match contracts/kb-upload.json", () => {
    expect(UPLOAD_MAX_FILE_BYTES).toBe(contract.maxFileBytes);
    expect([...UPLOAD_EXTENSIONS]).toEqual(contract.extensions);
  });
  it("check type by extension (case-insensitive), size and emptiness", () => {
    expect(checkUpload({ name: "Hợp đồng.PDF", size: 10 })).toBeNull();
    expect(checkUpload({ name: "virus.exe", size: 10 })).toBe("type");
    expect(checkUpload({ name: "noext", size: 10 })).toBe("type");
    expect(checkUpload({ name: "a.pdf", size: UPLOAD_MAX_FILE_BYTES + 1 })).toBe("size");
    expect(checkUpload({ name: "a.pdf", size: 0 })).toBe("empty");
  });
});
