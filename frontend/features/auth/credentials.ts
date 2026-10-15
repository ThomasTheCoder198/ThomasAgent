/** Only the shape of an address: core decides whether the owner exists, so this just catches typos early. */
const EMAIL_SHAPE = /^\S+@\S+$/;

export type CredentialProblems = {
  email?: "emailRequired" | "emailInvalid";
  password?: "passwordRequired";
};

export function validateCredentials(email: string, password: string): CredentialProblems {
  const trimmed = email.trim();
  return {
    email: trimmed === "" ? "emailRequired" : EMAIL_SHAPE.test(trimmed) ? undefined : "emailInvalid",
    password: password === "" ? "passwordRequired" : undefined,
  };
}

export const hasProblems = (problems: CredentialProblems) => Object.values(problems).some(Boolean);
