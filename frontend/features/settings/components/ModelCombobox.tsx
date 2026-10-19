"use client";

import { Check, ChevronDown, RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";
import { useId, useState } from "react";

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/Command";
import { fieldFocus } from "@/components/ui/focus";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/Popover";
import type { FieldControlProps } from "@/components/ui/Field";
import type { Model } from "@/features/registry/types";
import { cn } from "@/lib/cn";

import { modelFacts } from "../model-facts";
import { fitsRole, missingCapabilities, type SettingsRole } from "../role-fit";

type Props = {
  control: FieldControlProps;
  role: SettingsRole;
  /** The chosen provider's models. */
  models: Model[];
  value: string | undefined;
  onChange: (id: string) => void;
  disabled?: boolean;
  placeholder: string;
  /** "Not capable for {role}" group heading. */
  roleLabel: string;
  /** Re-syncs the provider's list; omitted when the page cannot (no provider, or no handler). */
  onSync?: () => Promise<void>;
};

function SyncButton({ onSync }: { onSync: () => Promise<void> }) {
  const t = useTranslations("settings");
  const [pending, setPending] = useState(false);
  async function run() {
    setPending(true);
    try {
      await onSync();
    } finally {
      setPending(false);
    }
  }
  return (
    <button
      type="button"
      onClick={() => void run()}
      disabled={pending}
      className="hover:text-foreground inline-flex min-h-8 items-center gap-1.5 underline-offset-4 hover:underline disabled:opacity-60 max-md:min-h-11"
    >
      <RefreshCw aria-hidden className={cn("size-3.5", pending && "animate-spin")} />
      {t("syncList")}
    </button>
  );
}

/**
 * Searchable model list for one provider (O6, shadcn Combobox: Popover + Command): models fit for the role first;
 * the rest grouped below, dimmed and not selectable, so the operator sees why a model is missing. OpenRouter
 * alone lists about 500; the search is client-side over the loaded list (owner 2026-10-18).
 */
export function ModelCombobox({
  control,
  role,
  models,
  value,
  onChange,
  disabled,
  placeholder,
  roleLabel,
  onSync,
}: Props) {
  const t = useTranslations("settings");
  const [open, setOpen] = useState(false);
  const listId = useId();
  const fit = models.filter((m) => fitsRole(m, role));
  const unfit = models.filter((m) => !fitsRole(m, role));
  const selected = models.find((m) => m.id === value);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          {...control}
          type="button"
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          disabled={disabled}
          className={cn(
            fieldFocus,
            "border-input bg-card text-foreground aria-invalid:border-destructive flex h-10 w-full min-w-0 items-center justify-between gap-2 rounded-sm border px-3 text-start text-sm transition-[color,border-color,box-shadow] duration-150 disabled:cursor-not-allowed disabled:opacity-60 max-md:min-h-11",
          )}
        >
          <span className={cn("truncate", !selected && "text-muted-foreground")}>
            {selected ? selected.displayName : placeholder}
          </span>
          <ChevronDown aria-hidden className="text-muted-foreground size-4 shrink-0" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[min(26rem,var(--radix-popover-content-available-width))] min-w-(--radix-popover-trigger-width) p-0">
        <Command>
          <CommandInput placeholder={t("searchModel")} aria-label={t("searchModel")} />
          <CommandList id={listId}>
            <CommandEmpty>{t("noModel")}</CommandEmpty>
            <CommandGroup>
              {fit.map((m) => (
                <CommandItem
                  key={m.id}
                  value={`${m.displayName} ${m.modelRef}`}
                  onSelect={() => {
                    onChange(m.id);
                    setOpen(false);
                  }}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{m.displayName}</span>
                    <span className="tabular text-muted-foreground block truncate text-xs">{m.modelRef}</span>
                  </span>
                  <span className="tabular text-muted-foreground shrink-0 text-xs">{modelFacts(m)}</span>
                  <Check aria-hidden className={cn("text-primary size-4", m.id !== value && "invisible")} />
                </CommandItem>
              ))}
            </CommandGroup>
            {unfit.length > 0 && (
              <CommandGroup heading={t("unfitGroup", { role: roleLabel })}>
                {unfit.map((m) => (
                  <CommandItem key={m.id} value={`${m.displayName} ${m.modelRef}`} disabled>
                    <span className="min-w-0 flex-1 truncate">
                      {m.displayName}
                      <span className="ml-2 text-xs">
                        ·{" "}
                        {m.capabilities.length === 0
                          ? t("unknownCapabilities")
                          : t("missingCaps", { caps: missingCapabilities(m, role).join(", ") })}
                      </span>
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
        <div className="border-border text-muted-foreground flex items-center justify-between gap-3 border-t px-3 py-2 text-[0.8125rem]">
          <span className="tabular">{t("modelCount", { count: models.length })}</span>
          {onSync && <SyncButton onSync={onSync} />}
        </div>
      </PopoverContent>
    </Popover>
  );
}
