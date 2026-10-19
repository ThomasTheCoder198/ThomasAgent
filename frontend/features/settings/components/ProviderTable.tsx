"use client";

import { MoreHorizontal } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { ConfirmDelete } from "@/features/knowledge-bases/components/ConfirmDelete";
import type { Model, Provider } from "@/features/registry/types";
import type { Locale } from "@/i18n/config";
import { staggerStyle } from "@/lib/stagger";
import { useNotify } from "@/lib/useNotify";

import { deleteProvider, syncProvider } from "../api";
import { KINDS_WITHOUT_KEY } from "../contract";
import { PROVIDER_KIND_LABELS } from "../provider-form";
import { ProviderDialog } from "./ProviderDialog";

/** From md: name, kind, model count, actions. Below md the facts fold into one line under the name. */
const COLUMNS = "grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,2fr)_minmax(0,1.5fr)_4rem_2.25rem]";

const isReady = (p: Provider) => p.hasApiKey || KINDS_WITHOUT_KEY.has(p.kind);

function ProviderRow({
  provider,
  modelCount,
  index,
}: {
  provider: Provider;
  modelCount: number;
  index: number;
}) {
  const t = useTranslations("settings");
  const locale = useLocale() as Locale;
  const router = useRouter();
  const notify = useNotify();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const menuTrigger = useRef<HTMLButtonElement>(null);
  const kind = PROVIDER_KIND_LABELS[provider.kind];
  const ready = isReady(provider);

  async function sync() {
    try {
      const result = await syncProvider(provider.id, locale);
      notify.success(t("synced", { added: result.added, updated: result.updated }));
      router.refresh();
    } catch (err) {
      notify.error(err);
    }
  }

  return (
    <li
      style={staggerStyle(index)}
      className={`animate-rise border-border grid items-center gap-x-4 border-b py-3 max-md:min-h-14 ${COLUMNS}`}
    >
      <div className="min-w-0">
        <p className="flex items-center gap-2 font-medium">
          <span
            aria-hidden
            className={`size-2 shrink-0 rounded-full ${ready ? "bg-success" : "bg-warning"}`}
          />
          <span className="truncate">{provider.name}</span>
          <span className="sr-only">{ready ? t("statusReady") : t("statusNoKey")}</span>
        </p>
        <p className="text-muted-foreground mt-0.5 text-sm md:hidden">
          {kind} · {t("modelCount", { count: modelCount })}
        </p>
      </div>
      <span className="text-muted-foreground hidden text-sm md:block">{kind}</span>
      <span className="tabular hidden text-right text-sm md:block">{modelCount}</span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={menuTrigger}
            variant="ghost"
            size="icon-sm"
            aria-label={t("providerActions", { name: provider.name })}
          >
            <MoreHorizontal aria-hidden />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => void sync()}>{t("sync")}</DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setEditing(true)}>{t("edit")}</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirming(true)}>
            {t("deleteProvider")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {editing && <ProviderDialog provider={provider} onOpenChange={setEditing} />}
      {confirming && (
        <ConfirmDelete
          onOpenChange={setConfirming}
          title={`${t("deleteProvider")}: ${provider.name}`}
          body={t("deleteProviderBody")}
          successMessage={t("providerDeleted")}
          returnFocusRef={menuTrigger}
          onConfirm={async () => {
            await deleteProvider(provider.id, locale);
            router.refresh();
          }}
        />
      )}
    </li>
  );
}

/**
 * Ruled rows, like the Agent list, because components/ui has no Table. A header row names the columns from md.
 * There is no last-sync column: core stores none (O8).
 */
export function ProviderTable({ providers, models }: { providers: Provider[]; models: Model[] }) {
  const t = useTranslations("settings");
  if (providers.length === 0) return <p className="text-muted-foreground text-sm">{t("noProviders")}</p>;
  return (
    <ul aria-label={t("providers")}>
      <li
        aria-hidden
        className={`text-muted-foreground border-border hidden gap-x-4 border-b pb-2 text-[0.8125rem] md:grid ${COLUMNS}`}
      >
        <span>{t("colName")}</span>
        <span>{t("colKind")}</span>
        <span className="text-right">{t("colModels")}</span>
        <span />
      </li>
      {providers.map((p, index) => (
        <ProviderRow
          key={p.id}
          provider={p}
          index={index}
          modelCount={models.filter((m) => m.providerId === p.id).length}
        />
      ))}
    </ul>
  );
}
