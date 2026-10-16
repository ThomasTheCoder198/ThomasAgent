"use client";

import { Switch as SwitchPrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

import { pressFocus } from "./focus";

/** Pair it with a Label (and a hint) that wraps the row, so the whole row is the touch target. */
export function Switch({ className, ...props }: ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        pressFocus,
        "data-[state=checked]:bg-primary data-[state=unchecked]:bg-input inline-flex h-6 w-10 shrink-0 items-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-60",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="bg-card ease-out-expo pointer-events-none block size-5 translate-x-0.5 rounded-full transition-transform duration-150 data-[state=checked]:translate-x-[1.125rem]"
      />
    </SwitchPrimitive.Root>
  );
}
