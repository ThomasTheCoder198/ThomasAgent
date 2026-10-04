import type { ButtonHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type Variant = "primary" | "line" | "outline" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-ink text-ground hover:bg-ink-2",
  line: "bg-mcp text-on-mcp hover:brightness-110",
  outline: "border-mcp-rule bg-paper text-mcp-ink hover:bg-mcp-tint border",
  ghost: "text-mcp-ink-2 hover:bg-mcp-tint",
};

/** Every control answers a press the same way: a short sink, released with the world's ease-out. */
const PRESS = "ease-out-expo active:scale-[0.97] active:duration-75";

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  /** Request in flight: a train-leaving bar runs along the bottom edge. */
  pending?: boolean;
};

export function Button({
  variant = "primary",
  pending,
  className,
  type = "button",
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      type={type}
      aria-busy={pending || undefined}
      className={cn(
        "relative inline-flex items-center justify-center gap-2 overflow-hidden rounded-lg px-3.5 py-2 text-[13.5px] font-semibold transition-[background-color,filter,transform] duration-200 disabled:cursor-not-allowed disabled:opacity-60",
        PRESS,
        variants[variant],
        className,
      )}
      {...props}
    >
      {children}
      {pending && (
        <span
          aria-hidden
          className="animate-depart absolute bottom-0 left-0 h-0.5 w-2/5 rounded-full bg-current opacity-70"
        />
      )}
    </button>
  );
}

type IconButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { label: string; tone?: "ground" | "sign" };

/** Square icon control; the accessible name is mandatory because the glyph alone says nothing. */
export function IconButton({
  label,
  tone = "ground",
  className,
  type = "button",
  ...props
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      className={cn(
        "grid size-8.5 shrink-0 place-items-center rounded-lg transition-[background-color,color,transform] duration-200 disabled:cursor-not-allowed disabled:opacity-50",
        PRESS,
        tone === "ground"
          ? "text-ink-2 hover:bg-hover hover:text-ink"
          : "text-sign-ink-2 hover:bg-sign-active hover:text-sign-ink",
        className,
      )}
      {...props}
    />
  );
}
