import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = 5;

/** Shaped like the rows it stands for: a name over a description, then the badge, tool count and model. */
export function AgentListSkeleton() {
  return (
    <ul className="-mx-3" aria-hidden>
      {Array.from({ length: ROWS }, (_, row) => (
        <li key={row} className="border-border flex min-h-11 items-center gap-4 border-b px-3 py-3 md:py-4">
          <div className="flex min-w-0 flex-1 flex-col gap-2 md:flex-row md:items-center md:gap-6">
            <div className="flex min-w-0 flex-col gap-2 md:flex-1">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-64 max-w-full" />
            </div>
            <div className="flex items-center gap-4">
              <Skeleton className="h-6 w-28 rounded-full" />
              <Skeleton className="h-4 w-20" />
              <Skeleton className="h-4 w-24" />
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}
