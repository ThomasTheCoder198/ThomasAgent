import { cn } from "@/lib/cn";

type Props = { label?: string; className?: string; labelClassName?: string };

/** The ThomasAgent wordmark: a small violet square and the name set in Newsreader. */
export function BrandMark({ label, className, labelClassName }: Props) {
  return (
    <span className={cn("flex items-center gap-2", className)}>
      <span aria-hidden className="bg-primary size-4 shrink-0 rounded-sm" />
      {label && (
        <span
          className={cn(
            "font-serif text-xl leading-none font-medium tracking-tight whitespace-nowrap",
            labelClassName,
          )}
        >
          {label}
        </span>
      )}
    </span>
  );
}
