import type { ReactNode } from "react";

import { MenuButton } from "./MenuButton";

/** Title block for workspace pages; on phones it also carries the menu button (one bar, like chat). */
export function PageHeader({ title, body, actions }: { title: string; body?: string; actions?: ReactNode }) {
  return (
    <header className="mx-auto flex w-full max-w-272 items-start gap-3 px-4 pt-6 pb-6 md:px-8 md:pt-10 md:pb-8">
      <MenuButton />
      <div className="min-w-0 flex-1">
        <h1 className="text-[clamp(1.6rem,2.4vw,2.1rem)] leading-tight font-bold tracking-[-0.02em]">
          {title}
        </h1>
        {body && <p className="text-ink-2 mt-1.5 max-w-[60ch]">{body}</p>}
      </div>
      {actions}
    </header>
  );
}
