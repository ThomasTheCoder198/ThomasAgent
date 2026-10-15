import type { ReactNode } from "react";

import { MenuButton } from "./MenuButton";

/**
 * Title block for workspace pages: Newsreader title, muted description, actions on the right (under the title
 * below sm). On phones it also carries the menu button, so these pages have one bar.
 */
export function PageHeader({ title, body, actions }: { title: string; body?: string; actions?: ReactNode }) {
  return (
    <header className="mx-auto flex w-full max-w-5xl items-start gap-3 px-4 pt-6 pb-6 md:px-6 md:pt-10 md:pb-8 lg:px-8">
      <MenuButton />
      <div className="flex min-w-0 flex-1 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h1 className="font-serif text-[clamp(1.75rem,3vw,2rem)] leading-tight font-medium tracking-tight">
            {title}
          </h1>
          {body && <p className="text-muted-foreground mt-2 max-w-[60ch]">{body}</p>}
        </div>
        {actions && <div className="shrink-0">{actions}</div>}
      </div>
    </header>
  );
}
