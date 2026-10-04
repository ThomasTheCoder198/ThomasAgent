import type { ButtonHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type Variant = "primary" | "line" | "outline" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-ink text-ground hover:bg-ink-2",
  line: "bg-mcp text-on-mcp hover:brightness-110",
  outline: "border-mcp-rule bg-paper text-mcp-ink hover:bg-mcp-tint border",
  ghost: "text-mcp-ink-2 hover:bg-mcp-tint",
};

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant };

export function Button({ variant = "primary", className, type = "button", ...props }: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        "inline-flex items-center justify-center gap-2 rounded-lg px-3.5 py-2 text-[13.5px] font-semibold transition-[background-color,filter] duration-150 disabled:cursor-not-allowed disabled:opacity-60",
        variants[variant],
        className,
      )}
      {...props}
    />
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
        "grid size-8.5 shrink-0 place-items-center rounded-lg transition-colors disabled:cursor-not-allowed disabled:opacity-50",
        tone === "ground"
          ? "text-ink-2 hover:bg-hover hover:text-ink"
          : "text-sign-ink-2 hover:bg-sign-active hover:text-sign-ink",
        className,
      )}
      {...props}
    />
  );
}
