"use client";

import { motion } from "motion/react";
import { Tabs as TabsPrimitive } from "radix-ui";
import { createContext, useContext, useId, useState, type ComponentProps } from "react";

import { cn } from "@/lib/cn";

import { pressFocus } from "./focus";

/** The root tells its triggers which tab is selected and names the group the sliding underline belongs to. */
const TabsState = createContext<{ value: string | undefined; groupId: string }>({
  value: undefined,
  groupId: "",
});

export function Tabs({
  className,
  value,
  defaultValue,
  onValueChange,
  ...props
}: ComponentProps<typeof TabsPrimitive.Root>) {
  const groupId = useId();
  const [inner, setInner] = useState(defaultValue);
  const selected = value ?? inner;
  return (
    <TabsState.Provider value={{ value: selected, groupId }}>
      <TabsPrimitive.Root
        data-slot="tabs"
        value={selected}
        onValueChange={(next) => {
          setInner(next);
          onValueChange?.(next);
        }}
        className={cn("flex flex-col gap-4", className)}
        {...props}
      />
    </TabsState.Provider>
  );
}

/** Underline tabs: a hairline under the whole rail, a 2px primary bar under the active one. On phones the rail scrolls. */
export function TabsList({ className, ...props }: ComponentProps<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      className={cn("border-border flex w-full items-center gap-1 overflow-x-auto border-b", className)}
      {...props}
    />
  );
}

export function TabsTrigger({ className, children, ...props }: ComponentProps<typeof TabsPrimitive.Trigger>) {
  const { value, groupId } = useContext(TabsState);
  const active = value !== undefined && value === props.value;
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        pressFocus,
        "text-muted-foreground hover:text-foreground data-[state=active]:text-foreground relative -mb-px inline-flex min-h-10 shrink-0 items-center justify-center gap-2 rounded-sm px-3 py-2 text-sm font-medium whitespace-nowrap transition-colors duration-150 disabled:pointer-events-none disabled:opacity-60 data-[state=active]:font-semibold max-md:min-h-11 [&_svg]:size-4 [&_svg]:shrink-0",
        className,
      )}
      {...props}
    >
      {children}
      {active && (
        <motion.span
          aria-hidden
          layoutId={`tabs-underline-${groupId}`}
          className="bg-primary absolute inset-x-3 bottom-0 h-0.5 rounded-full"
        />
      )}
    </TabsPrimitive.Trigger>
  );
}

export function TabsContent({ className, ...props }: ComponentProps<typeof TabsPrimitive.Content>) {
  return <TabsPrimitive.Content data-slot="tabs-content" className={cn("flex-1", className)} {...props} />;
}
