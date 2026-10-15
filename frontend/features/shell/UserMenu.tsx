"use client";

import { LogOut, Settings } from "lucide-react";
import { useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useCallback, useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import { useDismiss } from "@/components/ui/useDismiss";
import { AUTH_PATHS, LOGIN_ROUTE } from "@/features/auth/paths";
import { LocaleSwitcher } from "@/features/preferences/LocaleSwitcher";
import { ThemeSwitcher } from "@/features/preferences/ThemeSwitcher";
import { apiFetch } from "@/lib/api/client";

export function UserMenu() {
  const t = useTranslations();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, close, ref);

  async function logout() {
    await apiFetch(AUTH_PATHS.logout, { method: "POST" }).catch(() => undefined);
    router.replace(LOGIN_ROUTE);
  }

  return (
    <div ref={ref} className="relative ml-auto">
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("shell.settings")}
        title={t("shell.settings")}
        aria-expanded={open}
        aria-haspopup="true"
        onClick={() => setOpen((v) => !v)}
      >
        <Settings aria-hidden />
      </Button>
      {open && (
        <div className="bg-sidebar border-sidebar-border shadow-lift absolute bottom-11 left-0 z-20 flex w-60 flex-col gap-3.5 rounded-xl border p-3.5">
          <ThemeSwitcher
            labels={{
              light: t("preferences.light"),
              dark: t("preferences.dark"),
              system: t("preferences.system"),
              legend: t("preferences.theme"),
            }}
          />
          <LocaleSwitcher />
          <button
            type="button"
            onClick={logout}
            className="text-sidebar-foreground hover:bg-sidebar-accent -mx-1 flex items-center gap-2 rounded-md px-1 py-1.5 text-sm"
          >
            <LogOut aria-hidden className="size-4" />
            {t("auth.logout")}
          </button>
        </div>
      )}
    </div>
  );
}
