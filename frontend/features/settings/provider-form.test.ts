import { describe, expect, it } from "vitest";

import type { Provider } from "@/features/registry/types";
import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";

import {
  createInput,
  editInput,
  keyHint,
  problemsFromError,
  validateProvider,
  type ProviderFields,
} from "./provider-form";

const fields = (over: Partial<ProviderFields> = {}): ProviderFields => ({
  kind: "openrouter",
  name: "",
  baseUrl: "",
  apiKey: "",
  ...over,
});

const stored: Provider = {
  id: "p1",
  kind: "openai_compatible",
  name: "vLLM",
  baseUrl: "http://a/v1",
  hasApiKey: false,
  enabled: true,
  createdAt: "",
  updatedAt: "",
};

describe("provider form", () => {
  it("asks for a key on built-in kinds and an address on self-hosted ones", () => {
    expect(validateProvider(fields(), false)).toEqual({ apiKey: "required" });
    expect(validateProvider(fields({ kind: "openai_compatible" }), false)).toEqual({ baseUrl: "required" });
    expect(validateProvider(fields({ kind: "ollama", baseUrl: "http://x" }), false)).toEqual({});
    expect(validateProvider(fields({ apiKey: "" }), true)).toEqual({});
  });

  it("builds a create body without empty fields, naming an unnamed provider after its kind", () => {
    expect(createInput(fields({ apiKey: "sk-1" }))).toEqual({
      kind: "openrouter",
      name: "OpenRouter",
      apiKey: "sk-1",
    });
    expect(
      createInput(fields({ kind: "ollama", name: " Local ", baseUrl: " http://x ", apiKey: "ignored" })),
    ).toEqual({
      kind: "ollama",
      name: "Local",
      baseUrl: "http://x",
    });
  });

  it("sends only what changed on edit, and a key only when typed", () => {
    expect(editInput(stored, fields({ kind: stored.kind, name: "vLLM", baseUrl: "http://a/v1" }))).toEqual(
      {},
    );
    expect(
      editInput(stored, fields({ kind: stored.kind, name: "Mới", baseUrl: "http://b/v1", apiKey: "k" })),
    ).toEqual({ name: "Mới", baseUrl: "http://b/v1", apiKey: "k" });
  });

  it("warns that a new address without a key drops the stored key", () => {
    expect(keyHint("openai_compatible", true, true, false)).toBe("apiKeyCleared");
    expect(keyHint("openai_compatible", true, false, false)).toBe("apiKeyKeep");
    expect(keyHint("openai_compatible", false, false, false)).toBe("apiKeyOptional");
    expect(keyHint("openrouter", false, false, false)).toBe("apiKeyHint");
  });

  it("reads field problems from core's answer", () => {
    const invalid = new ApiError(ErrorCode.VALIDATION_FAILED, "x", 422, {
      fields: [{ field: "baseUrl", code: "INVALID" }],
    });
    expect(problemsFromError(invalid)).toEqual({ baseUrl: "invalid" });
    expect(problemsFromError(new ApiError(ErrorCode.LLM_NAME_TAKEN, "x", 409))).toEqual({ name: "taken" });
    expect(problemsFromError(new Error("boom"))).toEqual({});
  });
});
