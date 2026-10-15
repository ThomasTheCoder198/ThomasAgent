import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AgentDetail } from "./contract";
import { useAgentEditor } from "./useAgentEditor";

const { apiFetch, refresh, notify } = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  refresh: vi.fn(),
  notify: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/lib/api/client", () => ({ apiFetch }));
vi.mock("@/lib/useNotify", () => ({ useNotify: () => notify }));
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
  notify.success.mockReset();
  notify.error.mockReset();
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

  it("confirms a successful save with a success toast and refreshes the page data", async () => {
    apiFetch.mockResolvedValueOnce({ ...agent, instructions: "Mới" });
    const { result } = renderHook(() => useAgentEditor(agent));
    act(() => result.current.update({ instructions: "Mới" }));
    await act(() => result.current.persist());
    expect(notify.success).toHaveBeenCalledWith("saved");
    expect(notify.error).not.toHaveBeenCalled();
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("reports a failed save by toast and keeps the draft so the owner can retry", async () => {
    const failure = new Error("boom");
    apiFetch.mockRejectedValueOnce(failure);
    const { result } = renderHook(() => useAgentEditor(agent));
    act(() => result.current.update({ instructions: "Mới" }));
    await act(() => result.current.persist());
    expect(notify.error).toHaveBeenCalledWith(failure);
    expect(notify.success).not.toHaveBeenCalled();
    expect(result.current.saving).toBe(false);
    expect(result.current.dirty).toBe(true);
    expect(result.current.draft.instructions).toBe("Mới");
  });
});
