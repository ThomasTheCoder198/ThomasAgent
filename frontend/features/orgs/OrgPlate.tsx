"use client";

import { Check, ChevronsUpDown, ExternalLink, Plus, Settings2 } from "lucide-react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { useRef, useState } from "react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { cn } from "@/lib/cn";

import { orgHome, type Organization } from "./contract";
import { OrgDialog } from "./OrgDialog";
import { OrgStatusDot } from "./OrgStatusDot";
import { useLiveOrgId } from "./useLiveOrgId";

const RAIL_TEXT = "md:group-data-[collapsed=true]/shell:sr-only";

type Props = {
  orgs: Organization[];
  current: Organization | null;
  failed?: boolean;
  /** Ragx's own model-settings UI; the item is absent when unset. Read on the server, passed down as a prop. */
  ragxUiUrl?: string;
};

/** First code point, not UTF-16 unit: slice(0, 1) would split a surrogate pair into a broken glyph. */
const initialOf = (name: string) => ([...name][0] ?? "").toUpperCase();

function useLiveCurrentOrg(orgs: Organization[], serverCurrent: Organization | null): Organization | null {
  const liveId = useLiveOrgId(
    orgs.map((org) => org.id),
    serverCurrent?.id ?? null,
  );
  return orgs.find((org) => org.id === liveId) ?? serverCurrent;
}

/** The org the operator is working in, on the sign under the brand; switching is a link, not a client state. */
export function OrgPlate({ orgs, current: serverCurrent, failed, ragxUiUrl }: Props) {
  const t = useTranslations("orgs");
  const current = useLiveCurrentOrg(orgs, serverCurrent);
  const [dialog, setDialog] = useState<"create" | "edit" | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const close = (open: boolean) => !open && setDialog(null);
  const name = current?.name ?? (failed ? t("unavailable") : t("none"));
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          ref={triggerRef}
          className={cn(
            "border-sign-rule hover:bg-sign-hover data-[state=open]:bg-sign-active min-h-touch mb-3 flex w-full items-center gap-2.5 rounded-lg border px-2.5 py-2 text-left transition-colors",
            "md:group-data-[collapsed=true]/shell:justify-center md:group-data-[collapsed=true]/shell:border-transparent md:group-data-[collapsed=true]/shell:px-0",
          )}
          aria-label={t("switchLabel", { name })}
        >
          <span
            aria-hidden
            className="bg-sign-active grid size-7 shrink-0 place-items-center rounded-full text-[13px] font-bold"
          >
            {initialOf(name)}
          </span>
          <span className={cn("min-w-0 flex-1", RAIL_TEXT)}>
            <span className="block truncate text-sm font-semibold">{name}</span>
            {current && (
              <span className="text-sign-ink-2 flex items-center gap-1.5 text-xs">
                <OrgStatusDot status={current.ragxStatus} label={t(`status.${current.ragxStatus}`)} />
                {t(`status.${current.ragxStatus}`)}
              </span>
            )}
          </span>
          <ChevronsUpDown aria-hidden className={cn("text-sign-ink-2 size-4 shrink-0", RAIL_TEXT)} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>{t("menuLabel")}</DropdownMenuLabel>
          {orgs.map((org) => (
            <DropdownMenuItem key={org.id} asChild>
              <Link href={orgHome(org.id)} aria-current={org.id === current?.id ? "true" : undefined}>
                <OrgStatusDot status={org.ragxStatus} label={t(`status.${org.ragxStatus}`)} />
                <span className="min-w-0 flex-1 truncate">{org.name}</span>
                {org.id === current?.id && <Check aria-hidden className="size-4" />}
              </Link>
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          {current && (
            <DropdownMenuItem onSelect={() => setDialog("edit")}>
              <Settings2 aria-hidden className="size-4" />
              {t("settings")}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => setDialog("create")}>
            <Plus aria-hidden className="size-4" />
            {t("new")}
          </DropdownMenuItem>
          {ragxUiUrl && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <a href={ragxUiUrl} target="_blank" rel="noopener noreferrer">
                  <ExternalLink aria-hidden className="size-4" />
                  {t("ragxModels")}
                  <span className="sr-only"> {t("opensInNewTab")}</span>
                </a>
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog === "create" && (
        <OrgDialog mode="create" open onOpenChange={close} returnFocusRef={triggerRef} />
      )}
      {dialog === "edit" && current && (
        <OrgDialog mode="edit" org={current} open onOpenChange={close} returnFocusRef={triggerRef} />
      )}
    </>
  );
}
