"use client";

import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import { motion, type HTMLMotionProps } from "motion/react";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

import { pressFocus } from "./focus";

/**
 * The sticker (ink outline + hard offset shadow) is the primary action's mark and belongs to `default` only:
 * hover lifts it, press sinks it onto its own shadow. Every size has a 44px floor below md for touch.
 */
export const buttonVariants = cva(
  [
    pressFocus,
    "ease-out-expo inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-semibold whitespace-nowrap transition-[color,background-color,border-color,box-shadow] duration-150 disabled:pointer-events-none disabled:opacity-60 aria-invalid:border-destructive max-md:min-h-11 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  ],
  {
    variants: {
      variant: {
        default:
          "border-outline bg-primary text-primary-foreground shadow-sticker hover:bg-primary-hover hover:shadow-sticker-lg active:shadow-none border-[1.5px]",
        outline: "border-outline text-foreground hover:bg-accent border bg-transparent",
        secondary: "bg-secondary text-secondary-foreground hover:bg-muted",
        ghost: "text-foreground hover:bg-accent",
        destructive: "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        sm: "h-8 px-3",
        default: "h-9 px-4",
        lg: "h-11 px-5",
        hero: "h-13 px-6 text-base",
        icon: "size-9 max-md:size-11",
        "icon-sm": "size-8 max-md:size-11",
      },
    },
    compoundVariants: [{ variant: "link", class: "h-auto px-0 py-0.5" }],
    defaultVariants: { variant: "default", size: "default" },
  },
);

export type ButtonProps = ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
    /** A request is in flight: the button is disabled, marked busy and shows a spinner before its label. */
    pending?: boolean;
  };

const isIconSize = (size: ButtonProps["size"]) => size === "icon" || size === "icon-sm";

/** An icon-only button has no room for a spinner beside its glyph, so the spinner replaces it. */
function ButtonContent({ pending, size, children }: Pick<ButtonProps, "pending" | "size" | "children">) {
  if (pending && isIconSize(size)) return <Loader2 aria-hidden className="animate-spin" />;
  return (
    <>
      {pending && <Loader2 aria-hidden className="animate-spin" />}
      {children}
    </>
  );
}

type Variant = NonNullable<ButtonProps["variant"]>;

/**
 * Hover and press are springs from MotionProvider. The sticker button lifts toward its shadow's origin and sinks
 * onto it; other filled variants rise 1px; ghost and link only squeeze on press.
 */
const HOVER_BY_VARIANT: Partial<Record<Variant, { x?: number; y: number }>> = {
  default: { x: -1, y: -1 },
  outline: { y: -1 },
  secondary: { y: -1 },
  destructive: { y: -1 },
};
const PRESS_SQUEEZE = { scale: 0.97 };
const PRESS_SINK = { scale: 0.97, x: 1, y: 1 };
/** `asChild` renders someone else's element (a link), which Motion cannot wrap, so CSS carries the same feel. */
const CSS_FEEL =
  "transition-[color,background-color,border-color,box-shadow,transform,translate,scale] active:scale-[0.97]";
const CSS_LIFT_STICKER = "hover:-translate-0.5 active:translate-0.5";
const CSS_LIFT = "hover:-translate-y-px";

/** The Motion springs for a variant; an inert (disabled or pending) button has none. */
function springs(variant: Variant, inert: boolean) {
  if (inert) return {};
  return {
    whileHover: HOVER_BY_VARIANT[variant],
    whileTap: variant === "default" ? PRESS_SINK : PRESS_SQUEEZE,
  };
}

/** The CSS stand-in for `asChild`, where Motion cannot wrap the child. */
function cssFeel(variant: Variant) {
  if (!HOVER_BY_VARIANT[variant]) return CSS_FEEL;
  return cn(CSS_FEEL, variant === "default" ? CSS_LIFT_STICKER : CSS_LIFT);
}

export function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  pending = false,
  type = "button",
  disabled,
  children,
  ...props
}: ButtonProps) {
  const kind: Variant = variant ?? "default";
  const shared = {
    "data-slot": "button",
    "data-variant": variant,
    "data-size": size,
    className: cn(buttonVariants({ variant, size }), className),
  };
  if (asChild) {
    return (
      <Slot.Root {...shared} className={cn(shared.className, cssFeel(kind))} {...props}>
        {children}
      </Slot.Root>
    );
  }
  const inert = disabled || pending;
  return (
    <motion.button
      {...shared}
      type={type}
      aria-busy={pending || undefined}
      disabled={inert}
      {...springs(kind, inert)}
      // Motion types the drag and animation handlers differently from React's; none of them are used on buttons.
      {...(props as HTMLMotionProps<"button">)}
    >
      <ButtonContent pending={pending} size={size}>
        {children}
      </ButtonContent>
    </motion.button>
  );
}
