import { cn } from "@/lib/cn";

/** The ThomasAgent roundel: an Apps-red ring crossed by an MCP-blue bar, set like a station sign. */
export function BrandMark({ label, className }: { label?: string; className?: string }) {
  return (
    <span className={cn("flex items-center gap-2.5", className)}>
      <span
        aria-hidden
        className="border-app relative size-6.5 shrink-0 rounded-full border-[5px] after:absolute after:inset-x-[-9px] after:top-1/2 after:h-1.5 after:-translate-y-1/2 after:bg-[var(--mcp)]"
      />
      {label && <span className="text-[17px] font-bold tracking-[-0.01em]">{label}</span>}
    </span>
  );
}
