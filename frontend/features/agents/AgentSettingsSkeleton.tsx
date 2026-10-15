import { Skeleton } from "@/components/ui/Skeleton";

const TABS = 5;
const ROWS = 3;

/** Header, tab rail and settings rows in their final positions, so the page does not jump when data lands. */
export function AgentSettingsSkeleton() {
  return (
    <div aria-hidden>
      <div className="mx-auto w-full max-w-272 px-4 pt-4 pb-6 md:px-6 md:pt-6 lg:px-8">
        <Skeleton className="h-8 w-28" />
        <Skeleton className="mt-3 h-9 w-72 max-w-full" />
        <div className="mt-4 flex gap-4">
          <Skeleton className="h-6 w-28 rounded-full" />
          <Skeleton className="h-6 w-24" />
          <Skeleton className="h-6 w-32" />
        </div>
      </div>
      <div className="mx-auto w-full max-w-272 px-4 md:px-6 lg:px-8">
        <div className="border-border flex h-10 items-center gap-4 border-b max-md:h-11">
          {Array.from({ length: TABS }, (_, tab) => (
            <Skeleton key={tab} className="h-4 w-20" />
          ))}
        </div>
        {Array.from({ length: ROWS }, (_, row) => (
          <div
            key={row}
            className="border-border grid gap-4 border-b py-6 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-8"
          >
            <div className="flex flex-col gap-2">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-48 max-w-full" />
            </div>
            <Skeleton className="h-10 w-full max-md:h-11" />
          </div>
        ))}
      </div>
    </div>
  );
}
