import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

/**
 * The sticker (ink outline + hard offset shadow) is the primary action's mark and belongs to `default` only:
 * hover lifts it, press sinks it onto its own shadow. Every size has a 44px floor below md for touch.
 */
export const buttonVariants = cva(
  "ease-out-expo inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-semibold whitespace-nowrap transition-[color,background-color,border-color,box-shadow,transform] duration-150 disabled:pointer-events-none disabled:opacity-60 aria-invalid:border-destructive max-md:min-h-11 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default:
          "border-outline bg-primary text-primary-foreground shadow-sticker hover:bg-primary-hover hover:shadow-sticker-lg active:shadow-none border-[1.5px] hover:-translate-0.5 active:translate-0.5",
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
  const shared = {
    "data-slot": "button",
    "data-variant": variant,
    "data-size": size,
    className: cn(buttonVariants({ variant, size }), className),
  };
  if (asChild) {
    return (
      <Slot.Root {...shared} {...props}>
        {children}
      </Slot.Root>
    );
  }
  return (
    <button
      {...shared}
      type={type}
      aria-busy={pending || undefined}
      disabled={disabled || pending}
      {...props}
    >
      <ButtonContent pending={pending} size={size}>
        {children}
      </ButtonContent>
    </button>
  );
}
