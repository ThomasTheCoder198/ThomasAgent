import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = 4;

/** Header, the two scope tabs and four 1fr : 2fr role rows in their final positions. */
export function SettingsSkeleton() {
  return (
    <div aria-hidden>
      <div className="mx-auto w-full max-w-5xl px-4 pt-6 pb-6 md:px-6 md:pt-10 md:pb-8 lg:px-8">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="mt-3 h-5 w-96 max-w-full" />
      </div>
      <div className="mx-auto w-full max-w-5xl px-4 md:px-6 lg:px-8">
        <div className="border-border flex h-10 items-center gap-6 border-b max-md:h-11">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-4 w-28" />
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
