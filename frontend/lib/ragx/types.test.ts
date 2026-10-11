import { describe, expectTypeOf, it } from "vitest";

import type { DownloadUrl, FileStatus, KbFile, KnowledgeBase, UploadTicket } from "./types";

describe("ragx types generated from contracts/openapi/ragx.v1.yaml", () => {
  it("expose the shapes the Knowledge Base screens use", () => {
    expectTypeOf<KbFile["kbId"]>().toEqualTypeOf<string>();
    expectTypeOf<KnowledgeBase["isDefault"]>().toEqualTypeOf<boolean>();
    expectTypeOf<FileStatus>().toEqualTypeOf<
      | "pending"
      | "parsing"
      | "enriching"
      | "chunking"
      | "embedding"
      | "indexing"
      | "ready"
      | "failed"
      | "degraded"
    >();
    expectTypeOf<UploadTicket["upload"]["fields"]>().toEqualTypeOf<{ [key: string]: string }>();
    expectTypeOf<DownloadUrl["url"]>().toEqualTypeOf<string>();
  });
});
