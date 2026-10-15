"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import type { Locale } from "@/i18n/config";
import { apiFetch } from "@/lib/api/client";
import { useNotify } from "@/lib/useNotify";

import { hasProblems, validateCredentials, type CredentialProblems } from "./credentials";
import { AUTH_PATHS } from "./paths";

/** Field messages stay in the form; a failed sign-in (wrong password, core down) is an error toast. */
export function LoginForm({ next }: { next: string }) {
  const t = useTranslations("auth");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [problems, setProblems] = useState<CredentialProblems>({});
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const email = String(form.get("email") ?? "");
    const password = String(form.get("password") ?? "");
    const found = validateCredentials(email, password);
    setProblems(found);
    if (hasProblems(found)) return;
    setPending(true);
    try {
      await apiFetch(AUTH_PATHS.login, {
        method: "POST",
        locale,
        body: JSON.stringify({ email: email.trim(), password }),
      });
      router.replace(next);
    } catch (err) {
      notify.error(err);
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <Field label={t("email")} error={problems.email && t(problems.email)}>
        {(control) => <Input {...control} name="email" type="email" autoComplete="username" required />}
      </Field>
      <Field label={t("password")} error={problems.password && t(problems.password)}>
        {(control) => (
          <Input {...control} name="password" type="password" autoComplete="current-password" required />
        )}
      </Field>
      <Button type="submit" pending={pending} size="lg" className="mt-2 w-full">
        {pending ? t("submitting") : t("submit")}
      </Button>
    </form>
  );
}
