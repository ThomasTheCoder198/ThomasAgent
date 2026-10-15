import type { AgentRole } from "./contract";

/**
 * Radix Select forbids an empty-string item value, so "inherit the platform role" gets a sentinel of its own.
 * It is mapped to and from "no override" here, at the boundary, and never reaches the draft or the PATCH.
 */
export const INHERIT_VALUE = "__inherit__";

type Overrides = Partial<Record<AgentRole, string>>;

export function toSelectValue(override: string | undefined): string {
  return override ?? INHERIT_VALUE;
}

export function fromSelectValue(value: string): string | undefined {
  return value === INHERIT_VALUE ? undefined : value;
}

/** The overrides after the owner picked `selected` for `role`; choosing "inherit" drops the role's entry. */
export function withOverride(overrides: Overrides, role: AgentRole, selected: string): Overrides {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- dropped on purpose: the role inherits again
  const { [role]: _removed, ...rest } = overrides;
  const modelId = fromSelectValue(selected);
  return modelId === undefined ? rest : { ...rest, [role]: modelId };
}
