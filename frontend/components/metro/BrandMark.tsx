import { cn } from "@/lib/cn";

type Props = { label?: string; className?: string; labelClassName?: string };

/** The ThomasAgent roundel: an Apps-red ring crossed by an MCP-blue bar, set like a station sign. */
export function BrandMark({ label, className, labelClassName }: Props) {
  return (
    <span className={cn("flex items-center gap-2.5", className)}>
      <span
        aria-hidden
        className="border-app relative ml-[9px] size-6.5 shrink-0 rounded-full border-[5px] after:absolute after:inset-x-[-9px] after:top-1/2 after:h-1.5 after:-translate-y-1/2 after:bg-[var(--mcp)]"
      />
      {label && (
        <span className={cn("text-[17px] font-bold tracking-[-0.01em] whitespace-nowrap", labelClassName)}>
          {label}
        </span>
      )}
    </span>
  );
}
