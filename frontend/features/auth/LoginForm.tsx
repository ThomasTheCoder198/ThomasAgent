"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { TextField } from "@/components/ui/TextField";
import type { Locale } from "@/i18n/config";
import { ApiError, apiFetch } from "@/lib/api/client";

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
      setError(err instanceof ApiError ? err.message : tErr("generic"));
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      <TextField label={t("email")} name="email" type="email" autoComplete="username" required />
      <TextField
        label={t("password")}
        name="password"
        type="password"
        autoComplete="current-password"
        required
      />
      {error && (
        <p role="alert" className="text-app text-sm font-medium">
          {error}
        </p>
      )}
      <Button type="submit" disabled={pending} className="mt-2 py-2.5 text-[15px]">
        {pending ? t("submitting") : t("submit")}
      </Button>
    </form>
  );
}
