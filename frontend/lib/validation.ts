/** Display-name rules shared by organizations and Knowledge Bases (core and ragx: trimmed, bounded, no control characters). */
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f-\u009f]/;

export type NameProblem = "required" | "tooLong" | "invalid";

export function validateDisplayName(raw: string, maxLength: number): NameProblem | null {
  const name = raw.trim();
  if (name === "") return "required";
  if ([...name].length > maxLength) return "tooLong";
  if (CONTROL_CHARACTERS.test(name)) return "invalid";
  return null;
}
