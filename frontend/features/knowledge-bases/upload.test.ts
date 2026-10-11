import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StorageUploadError, confirmUpload, fetchDownloadUrl, postToStorage, presignUpload } from "./upload";
import type { UploadTicket } from "@/lib/ragx/types";

class FakeXhr {
  static last: FakeXhr;
  method = "";
  url = "";
  withCredentials = false;
  status = 0;
  responseText = "";
  headers: Record<string, string> = {};
  sent: FormData | null = null;
  upload: { onprogress: ((e: { loaded: number; total: number; lengthComputable: boolean }) => void) | null } =
    { onprogress: null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  aborted = false;
  constructor() {
    FakeXhr.last = this;
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(name: string, value: string) {
    this.headers[name] = value;
  }
  send(body: FormData) {
    this.sent = body;
  }
  abort() {
    this.aborted = true;
  }
  respond(status: number, text = "") {
    this.status = status;
    this.responseText = text;
    this.onload?.();
  }
}

const ticket: UploadTicket = {
  fileId: "f1",
  upload: {
    url: "http://localhost:9000/kb-raw",
    fields: { key: "o/k/f1", policy: "p", "x-amz-signature": "s" },
  },
  expiresAt: "2030-01-01T00:00:00Z",
};

beforeEach(() => vi.stubGlobal("XMLHttpRequest", FakeXhr));
afterEach(() => vi.unstubAllGlobals());

describe("postToStorage", () => {
  it("POSTs the policy fields first and the file last, with no credentials or custom headers", async () => {
    const file = new File(["hello"], "Tài liệu.md");
    const progress = vi.fn();
    const done = postToStorage(ticket, file, progress, new AbortController().signal);
    const xhr = FakeXhr.last;
    expect([xhr.method, xhr.url, xhr.withCredentials]).toEqual(["POST", ticket.upload.url, false]);
    expect(xhr.headers).toEqual({});
    expect([...xhr.sent!.keys()]).toEqual(["key", "policy", "x-amz-signature", "file"]);
    xhr.upload.onprogress?.({ loaded: 2, total: 5, lengthComputable: true });
    expect(progress).toHaveBeenCalledWith(2, 5);
    xhr.respond(204);
    await expect(done).resolves.toBeUndefined();
  });
  it("turns MinIO's XML error into a StorageUploadError with the S3 code", async () => {
    const done = postToStorage(ticket, new File(["x"], "a.md"), vi.fn(), new AbortController().signal);
    FakeXhr.last.respond(400, "<Error><Code>EntityTooLarge</Code><Message>…</Message></Error>");
    await expect(done).rejects.toMatchObject({
      name: "StorageUploadError",
      status: 400,
      code: "EntityTooLarge",
    });
  });
  it("reports a network failure and honours cancel", async () => {
    const failed = postToStorage(ticket, new File(["x"], "a.md"), vi.fn(), new AbortController().signal);
    FakeXhr.last.onerror?.();
    await expect(failed).rejects.toBeInstanceOf(StorageUploadError);
    const controller = new AbortController();
    const cancelled = postToStorage(ticket, new File(["x"], "a.md"), vi.fn(), controller.signal);
    controller.abort();
    expect(FakeXhr.last.aborted).toBe(true);
    await expect(cancelled).rejects.toMatchObject({ name: "AbortError" });
  });
});

describe("proxy calls", () => {
  it("presigns, confirms and fetches the download link through core's app proxy", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ data: { ok: true }, meta: { requestId: "req_1" } }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const signal = new AbortController().signal;
    await presignUpload("o 1", "k1", new File(["abc"], "a.md"), "en", signal);
    await confirmUpload("o 1", "k1", "f1", "en", signal);
    await fetchDownloadUrl("o 1", "k1", "f1", "en");
    const calls = fetchMock.mock.calls.map(([url, init]) => [url, init?.method ?? "GET", init?.body]);
    expect(calls).toEqual([
      [
        "/api/v1/orgs/o%201/apps/ragx/knowledge-bases/k1/uploads",
        "POST",
        JSON.stringify({ name: "a.md", sizeBytes: 3 }),
      ],
      ["/api/v1/orgs/o%201/apps/ragx/knowledge-bases/k1/files/f1/confirm", "POST", undefined],
      ["/api/v1/orgs/o%201/apps/ragx/knowledge-bases/k1/files/f1/download-url", "GET", undefined],
    ]);
  });
});
