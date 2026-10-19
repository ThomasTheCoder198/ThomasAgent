import type { Provider, ProviderInput, ProviderKind } from "@/features/registry/types";
import { ApiError } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";

import { KINDS_NEEDING_BASE_URL, KINDS_WITHOUT_KEY, KINDS_WITH_OPTIONAL_KEY } from "./contract";

export type ProviderFields = {
  kind: ProviderKind;
  name: string;
  baseUrl: string;
  apiKey: string;
};

export type FieldName = "name" | "baseUrl" | "apiKey";
export type FieldProblem = "required" | "invalid" | "taken";
export type FieldProblems = Partial<Record<FieldName, FieldProblem>>;

/** Proper names, not translated. */
export const PROVIDER_KIND_LABELS: Record<ProviderKind, string> = {
  openrouter: "OpenRouter",
  openai: "OpenAI",
  anthropic: "Anthropic",
  google: "Google",
  azure: "Azure OpenAI",
  ollama: "Ollama",
  openai_compatible: "OpenAI-compatible",
  cohere: "Cohere",
  voyage: "Voyage",
  jina: "Jina",
  typesafe: "Typesafe",
};

const DEFAULT_KIND: ProviderKind = "openrouter";

/** The form starts empty for a new provider, or from the stored one (never with its key). */
export function initialFields(provider: Provider | undefined): ProviderFields {
  return {
    kind: provider?.kind ?? DEFAULT_KIND,
    name: provider?.name ?? "",
    baseUrl: provider?.baseUrl ?? "",
    apiKey: "",
  };
}

export const showsKey = (kind: ProviderKind) => !KINDS_WITHOUT_KEY.has(kind);
export const needsBaseUrl = (kind: ProviderKind) => KINDS_NEEDING_BASE_URL.has(kind);
export const keyIsOptional = (kind: ProviderKind) => KINDS_WITH_OPTIONAL_KEY.has(kind);

/** Required fields the form can check before asking core (frontend rule 10); core stays the authority. */
export function validateProvider(fields: ProviderFields, editing: boolean): FieldProblems {
  const problems: FieldProblems = {};
  if (needsBaseUrl(fields.kind) && fields.baseUrl.trim() === "") problems.baseUrl = "required";
  if (!editing && showsKey(fields.kind) && !keyIsOptional(fields.kind) && fields.apiKey === "") {
    problems.apiKey = "required";
  }
  return problems;
}

/** A create body without empty fields; an unnamed provider takes its kind's name. */
export function createInput(fields: ProviderFields): ProviderInput {
  const input: ProviderInput = {
    kind: fields.kind,
    name: fields.name.trim() || PROVIDER_KIND_LABELS[fields.kind],
  };
  if (needsBaseUrl(fields.kind)) input.baseUrl = fields.baseUrl.trim();
  if (showsKey(fields.kind) && fields.apiKey !== "") input.apiKey = fields.apiKey;
  return input;
}

/** An edit sends only what changed; `apiKey` only when typed (core clears the key if the address changes without one). */
export function editInput(provider: Provider, fields: ProviderFields): ProviderInput {
  const input: ProviderInput = {};
  const name = fields.name.trim();
  if (name !== "" && name !== provider.name) input.name = name;
  if (needsBaseUrl(provider.kind) && fields.baseUrl.trim() !== provider.baseUrl) {
    input.baseUrl = fields.baseUrl.trim();
  }
  if (showsKey(provider.kind) && fields.apiKey !== "") input.apiKey = fields.apiKey;
  return input;
}

type WireField = { field: string; code: string };

/** Core's `VALIDATION_FAILED` / conflict answers name the offending fields in `details.fields`. */
export function problemsFromCore(details: Record<string, unknown> | undefined): FieldProblems {
  const fields = (details?.fields as WireField[] | undefined) ?? [];
  const problems: FieldProblems = {};
  for (const { field, code } of fields) {
    if (field === "name" || field === "baseUrl" || field === "apiKey") {
      problems[field] = code === "REQUIRED" ? "required" : "invalid";
    }
  }
  return problems;
}

export type KeyHint = "apiKeyCleared" | "apiKeyKeep" | "apiKeyOptional" | "apiKeyHint";

/** The line under the key field: what happens to the stored key, or what the field is for. */
export function keyHint(
  kind: ProviderKind,
  editing: boolean,
  baseUrlDirty: boolean,
  keyTyped: boolean,
): KeyHint {
  if (baseUrlDirty && !keyTyped) return "apiKeyCleared";
  if (editing) return "apiKeyKeep";
  return keyIsOptional(kind) ? "apiKeyOptional" : "apiKeyHint";
}

export type ProblemKey = "fieldRequired" | "fieldInvalid" | "nameTaken";

export function problemKey(problem: FieldProblem): ProblemKey {
  if (problem === "required") return "fieldRequired";
  return problem === "taken" ? "nameTaken" : "fieldInvalid";
}

/** Field problems core reported for a failed save, or none when the failure is not about a field. */
export function problemsFromError(err: unknown): FieldProblems {
  if (!(err instanceof ApiError)) return {};
  if (err.code === ErrorCode.LLM_NAME_TAKEN) return { name: "taken" };
  return problemsFromCore(err.details);
}
