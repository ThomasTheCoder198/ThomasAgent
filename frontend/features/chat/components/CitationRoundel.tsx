import { lineBg, onLine, type Line } from "@/components/lines";
import { cn } from "@/lib/cn";

type Props = {
  n: number;
  label: string;
  line?: Line;
  selected?: boolean;
  onSelect?: (n: number) => void;
  className?: string;
};

/** A 20px numbered roundel in its source line's colour; selecting it opens the exact cited block. */
export function CitationRoundel({ n, label, line = "kb", selected = false, onSelect, className }: Props) {
  const look = cn(
    "tabular inline-grid size-5 place-items-center rounded-full text-[0.6875rem] leading-none font-semibold",
    lineBg[line],
    onLine[line],
    className,
  );
  if (!onSelect) {
    return (
      <span className={look} aria-label={label}>
        {n}
      </span>
    );
  }
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={selected}
      onClick={() => onSelect(n)}
      className={cn(
        look,
        // The glyph stays 20px; below md an invisible halo brings the hit area to the 44px touch floor.
        "ease-out-expo aria-pressed:ring-primary/35 focus-visible:ring-ring/40 relative mx-0.5 -translate-y-px align-middle transition-transform duration-200 hover:scale-110 focus-visible:scale-110 focus-visible:ring-[3px] focus-visible:outline-none aria-pressed:ring-[3px] max-md:after:absolute max-md:after:-inset-3 max-md:after:content-['']",
      )}
    >
      {n}
    </button>
  );
}
