"use client";

import { motion } from "motion/react";
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";
import { createContext, useContext, useId, type ComponentProps } from "react";

import { cn } from "@/lib/cn";

import { pressFocus } from "./focus";

/** Which item is on, and the group the sliding pill belongs to. Only single-select groups draw a pill. */
const GroupState = createContext<{ value: string | undefined; groupId: string }>({
  value: undefined,
  groupId: "",
});

/** A segmented choice: one quiet track, and a card-coloured pill that slides to the selected segment. */
export function ToggleGroup({ className, ...props }: ComponentProps<typeof ToggleGroupPrimitive.Root>) {
  const groupId = useId();
  const value = props.type === "single" ? props.value : undefined;
  return (
    <GroupState.Provider value={{ value, groupId }}>
      <ToggleGroupPrimitive.Root
        data-slot="toggle-group"
        className={cn(
          "border-border bg-secondary flex w-fit items-center gap-0.5 rounded-md border p-0.5",
          className,
        )}
        {...props}
      />
    </GroupState.Provider>
  );
}

export function ToggleGroupItem({
  className,
  children,
  ...props
}: ComponentProps<typeof ToggleGroupPrimitive.Item>) {
  const { value, groupId } = useContext(GroupState);
  const selected = value !== undefined && value === props.value;
  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      className={cn(
        pressFocus,
        "text-muted-foreground hover:text-foreground data-[state=on]:text-foreground relative inline-flex h-8 min-w-0 flex-1 items-center justify-center rounded-sm px-3 text-sm font-medium whitespace-nowrap transition-colors duration-150 disabled:pointer-events-none disabled:opacity-60 data-[state=on]:font-semibold max-md:min-h-11",
        className,
      )}
      {...props}
    >
      {selected && (
        <motion.span
          aria-hidden
          layoutId={`toggle-pill-${groupId}`}
          className="bg-card border-border absolute inset-0 rounded-sm border"
        />
      )}
      <span className="relative inline-flex items-center gap-1.5 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4">
        {children}
      </span>
    </ToggleGroupPrimitive.Item>
  );
}
