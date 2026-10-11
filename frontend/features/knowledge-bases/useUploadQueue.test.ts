import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import type { KbFile, UploadTicket } from "@/lib/ragx/types";

import { StorageUploadError } from "./upload";
import { useUploadQueue } from "./useUploadQueue";

vi.mock("next-intl", () => ({ useLocale: () => "en" }));
const presignUpload = vi.fn();
const postToStorage = vi.fn();
const confirmUpload = vi.fn();
vi.mock("./upload", async (original) => ({
  ...(await original<typeof import("./upload")>()),
  presignUpload: (...a: unknown[]) => presignUpload(...a),
  postToStorage: (...a: unknown[]) => postToStorage(...a),
  confirmUpload: (...a: unknown[]) => confirmUpload(...a),
}));

const ticket = (fileId: string, expiresAt = "2999-01-01T00:00:00Z"): UploadTicket => ({
  fileId,
  upload: { url: "http://s3/kb-raw", fields: { key: fileId } },
  expiresAt,
});
const stored = (id: string) => ({ id, status: "pending" }) as KbFile;
const file = (name: string, size = 3) => new File(["x".repeat(size)], name);
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  [presignUpload, postToStorage, confirmUpload].forEach((m) => m.mockReset());
  presignUpload.mockImplementation(async (_o, _k, f: File) => ticket(`id-${f.name}`));
  postToStorage.mockResolvedValue(undefined);
  confirmUpload.mockImplementation(async (_o, _k, fileId: string) => stored(fileId));
});

describe("useUploadQueue", () => {
  it("runs files one at a time through presign → storage → confirm", async () => {
    const gate = deferred<void>();
    postToStorage.mockImplementationOnce(() => gate.promise);
    const onUploaded = vi.fn();
    const { result } = renderHook(() => useUploadQueue("o", "k", onUploaded));
    act(() => result.current.add([file("a.md"), file("b.md")]));
    await waitFor(() => expect(result.current.items[0]?.state).toBe("uploading"));
    expect(result.current.items[1]?.state).toBe("queued");
    expect(presignUpload).toHaveBeenCalledTimes(1);
    await act(async () => gate.resolve());
    await waitFor(() => expect(onUploaded).toHaveBeenCalledTimes(2));
    expect(onUploaded.mock.calls.map(([f]) => f.id)).toEqual(["id-a.md", "id-b.md"]);
    expect(result.current.items).toEqual([]);
  });

  it("rejects invalid files locally without any call", async () => {
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.exe")]));
    expect(result.current.items[0]).toMatchObject({ state: "failed", problem: "type" });
    expect(presignUpload).not.toHaveBeenCalled();
  });

  it("retries a storage failure without presigning again while the ticket is valid", async () => {
    postToStorage.mockRejectedValueOnce(new StorageUploadError(0, "NETWORK"));
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.md")]));
    await waitFor(() =>
      expect(result.current.items[0]).toMatchObject({ state: "failed", failedStep: "storage" }),
    );
    act(() => result.current.retry(result.current.items[0]!.id));
    await waitFor(() => expect(confirmUpload).toHaveBeenCalledTimes(1));
    expect(presignUpload).toHaveBeenCalledTimes(1);
    expect(postToStorage).toHaveBeenCalledTimes(2);
  });

  it("presigns again when the failed upload's ticket has expired", async () => {
    presignUpload.mockResolvedValueOnce(ticket("old", "2000-01-01T00:00:00Z"));
    postToStorage.mockRejectedValueOnce(new StorageUploadError(403, "AccessDenied"));
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.md")]));
    await waitFor(() => expect(result.current.items[0]?.state).toBe("failed"));
    act(() => result.current.retry(result.current.items[0]!.id));
    await waitFor(() =>
      expect(confirmUpload).toHaveBeenCalledWith("o", "k", "id-a.md", "en", expect.anything()),
    );
    expect(presignUpload).toHaveBeenCalledTimes(2);
  });

  it("presigns again after a storage 403, because the same ticket would be refused again", async () => {
    postToStorage.mockRejectedValueOnce(new StorageUploadError(403, "AccessDenied"));
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.md")]));
    await waitFor(() => expect(result.current.items[0]?.state).toBe("failed"));
    expect(result.current.items[0]?.ticket).toBeUndefined();
    act(() => result.current.retry(result.current.items[0]!.id));
    await waitFor(() => expect(confirmUpload).toHaveBeenCalledTimes(1));
    expect(presignUpload).toHaveBeenCalledTimes(2);
  });

  it("retries a confirm failure with confirm only, but re-POSTs when confirm says the file is missing", async () => {
    confirmUpload.mockRejectedValueOnce(
      new ApiError(ErrorCode.INTERNAL_ERROR, "x", 503, undefined, "req_abcdefgh"),
    );
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.md")]));
    await waitFor(() =>
      expect(result.current.items[0]).toMatchObject({ state: "failed", failedStep: "confirm" }),
    );
    act(() => result.current.retry(result.current.items[0]!.id));
    await waitFor(() => expect(confirmUpload).toHaveBeenCalledTimes(2));
    expect(postToStorage).toHaveBeenCalledTimes(1);

    confirmUpload.mockRejectedValueOnce(
      new ApiError(ErrorCode.VALIDATION_FAILED, "x", 400, { fields: [{ field: "file", code: "MISSING" }] }),
    );
    act(() => result.current.add([file("b.md")]));
    await waitFor(() =>
      expect(result.current.items[0]).toMatchObject({ state: "failed", failedStep: "storage" }),
    );
    act(() => result.current.retry(result.current.items[0]!.id));
    await waitFor(() => expect(confirmUpload).toHaveBeenCalledTimes(4));
    expect(postToStorage).toHaveBeenCalledTimes(3);
  });

  it("cancels a queued file by removing it and an uploading file by aborting it", async () => {
    const gate = deferred<void>();
    postToStorage.mockImplementationOnce((_t, _f, _p, signal: AbortSignal) => {
      signal.addEventListener("abort", () => gate.reject(new DOMException("cancelled", "AbortError")));
      return gate.promise;
    });
    const { result } = renderHook(() => useUploadQueue("o", "k", vi.fn()));
    act(() => result.current.add([file("a.md"), file("b.md")]));
    await waitFor(() => expect(result.current.items[0]?.state).toBe("uploading"));
    act(() => result.current.cancel(result.current.items[1]!.id));
    expect(result.current.items).toHaveLength(1);
    act(() => result.current.cancel(result.current.items[0]!.id));
    await waitFor(() => expect(result.current.items).toEqual([]));
    expect(confirmUpload).not.toHaveBeenCalled();
  });

  it("aborts the running upload when the queue unmounts, and never reports it", async () => {
    const gate = deferred<void>();
    let signal!: AbortSignal;
    postToStorage.mockImplementationOnce((_t, _f, _p, s: AbortSignal) => {
      signal = s;
      s.addEventListener("abort", () => gate.reject(new DOMException("cancelled", "AbortError")));
      return gate.promise;
    });
    const onUploaded = vi.fn();
    const { result, unmount } = renderHook(() => useUploadQueue("o", "k", onUploaded));
    act(() => result.current.add([file("a.md"), file("b.md")]));
    await waitFor(() => expect(result.current.items[0]?.state).toBe("uploading"));
    unmount();
    expect(signal.aborted).toBe(true);
    await Promise.resolve();
    expect(confirmUpload).not.toHaveBeenCalled();
    expect(onUploaded).not.toHaveBeenCalled();
  });
});
