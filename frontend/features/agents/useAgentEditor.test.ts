import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AgentDetail } from "./contract";
import { useAgentEditor } from "./useAgentEditor";

const { apiFetch, refresh } = vi.hoisted(() => ({ apiFetch: vi.fn(), refresh: vi.fn() }));
vi.mock("@/lib/api/client", () => ({ apiFetch, ApiError: class extends Error {} }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("next-intl", () => ({ useLocale: () => "vi", useTranslations: () => (key: string) => key }));

const agent: AgentDetail = {
  id: "a1",
  name: "Pháp chế",
  description: "",
  instructions: "Cũ",
  kbId: "kb-1",
  toolSources: [],
  modelOverrides: {},
  contextFiles: [],
  updatedAt: "",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

beforeEach(() => {
  apiFetch.mockReset();
  refresh.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useAgentEditor", () => {
  it("freezes edits while a save is in flight so nothing typed meanwhile is lost", async () => {
    const response = deferred<AgentDetail>();
    apiFetch.mockReturnValueOnce(response.promise);
    const { result } = renderHook(() => useAgentEditor(agent));

    act(() => result.current.update({ instructions: "Mới" }));
    let saving!: Promise<void>;
    act(() => {
      saving = result.current.persist();
    });
    expect(result.current.saving).toBe(true);

    act(() => result.current.update({ instructions: "Gõ trong lúc lưu" }));
    expect(result.current.draft.instructions).toBe("Mới");

    await act(async () => {
      response.resolve({ ...agent, instructions: "Mới" });
      await saving;
    });
    expect(result.current.saving).toBe(false);
    expect(result.current.draft.instructions).toBe("Mới");
    expect(result.current.dirty).toBe(false);
    expect(apiFetch).toHaveBeenCalledWith(
      "/api/v1/agents/a1",
      expect.objectContaining({ body: '{"instructions":"Mới"}' }),
    );
  });

  it("never sends a draft with an empty name", async () => {
    const { result } = renderHook(() => useAgentEditor(agent));
    act(() => result.current.update({ name: "  " }));
    expect(result.current.canSave).toBe(false);
    await act(() => result.current.persist());
    expect(apiFetch).not.toHaveBeenCalled();
  });

  it("clears the saved-flash timer on the next save and on unmount", async () => {
    vi.useFakeTimers();
    const clear = vi.spyOn(globalThis, "clearTimeout");
    apiFetch.mockImplementation(async (_path: string, init: { body: string }) => ({
      ...agent,
      ...JSON.parse(init.body),
    }));
    const { result, unmount } = renderHook(() => useAgentEditor(agent));

    act(() => result.current.update({ instructions: "Một" }));
    await act(() => result.current.persist());
    expect(result.current.save.status).toBe("saved");

    const before = clear.mock.calls.length;
    act(() => result.current.update({ instructions: "Hai" }));
    await act(() => result.current.persist());
    expect(clear.mock.calls.length).toBeGreaterThan(before);

    const beforeUnmount = clear.mock.calls.length;
    unmount();
    expect(clear.mock.calls.length).toBeGreaterThan(beforeUnmount);
  });
});
