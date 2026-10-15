"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import type { Locale } from "@/i18n/config";
import { apiFetch, describeApiError } from "@/lib/api/client";

import { AUTH_PATHS } from "./paths";

export function LoginForm({ next }: { next: string }) {
  const t = useTranslations("auth");
  const tErr = useTranslations("errors");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setPending(true);
    setError(null);
    try {
      await apiFetch(AUTH_PATHS.login, {
        method: "POST",
        locale,
        body: JSON.stringify({ email: form.get("email"), password: form.get("password") }),
      });
      router.replace(next);
    } catch (err) {
      setError(describeApiError(err, tErr("generic"), (id) => tErr("requestId", { id })));
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <Field label={t("email")}>
        {(control) => <Input {...control} name="email" type="email" autoComplete="username" required />}
      </Field>
      <Field label={t("password")}>
        {(control) => (
          <Input {...control} name="password" type="password" autoComplete="current-password" required />
        )}
      </Field>
      {error && (
        <p role="alert" className="text-destructive animate-rise text-sm font-medium">
          {error}
        </p>
      )}
      <Button type="submit" pending={pending} size="lg" className="mt-2">
        {pending ? t("submitting") : t("submit")}
      </Button>
    </form>
  );
}
