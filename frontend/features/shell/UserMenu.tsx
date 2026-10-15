"use client";

import { LogOut, Settings } from "lucide-react";
import { useTranslations } from "next-intl";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { AUTH_PATHS, LOGIN_ROUTE } from "@/features/auth/paths";
import { LocaleMenuGroup } from "@/features/preferences/LocaleMenuGroup";
import { ThemeMenuGroup } from "@/features/preferences/ThemeMenuGroup";
import { ApiError, apiFetch } from "@/lib/api/client";
import { ErrorCode } from "@/lib/errors/codes.gen";
import { useNotify } from "@/lib/useNotify";

/** The owner's menu: theme, language and sign out. */
export function UserMenu() {
  const t = useTranslations();
  const router = useRouter();
  const notify = useNotify();

  async function logout() {
    try {
      await apiFetch(AUTH_PATHS.logout, { method: "POST" });
    } catch (err) {
      // An expired session is already signed out; any other failure leaves the cookie, so say so and stay.
      const expired = err instanceof ApiError && err.code === ErrorCode.UNAUTHENTICATED;
      if (!expired) {
        notify.error(err);
        return;
      }
    }
    router.replace(LOGIN_ROUTE);
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("shell.settings")} title={t("shell.settings")}>
          <Settings aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start">
        <ThemeMenuGroup />
        <DropdownMenuSeparator />
        <LocaleMenuGroup />
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void logout()}>
          <LogOut aria-hidden />
          {t("auth.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
