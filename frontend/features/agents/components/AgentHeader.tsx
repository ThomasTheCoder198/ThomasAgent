import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useTranslations } from "next-intl";

import type { ToolLine } from "@/components/lines";
import { Button } from "@/components/ui/Button";
import { MenuButton } from "@/features/shell/MenuButton";

import { AgentMeta } from "./AgentMeta";

type Props = {
  name: string;
  description: string;
  kbName: string;
  lines: readonly ToolLine[];
  toolCount: number;
  model: string;
};

/** The Agent's page header: way back, its name as the page title, then what it is built from. */
export function AgentHeader({ name, description, ...meta }: Props) {
  const t = useTranslations("agents");
  return (
    <header className="mx-auto w-full max-w-272 px-4 pt-4 pb-6 md:px-6 md:pt-6 lg:px-8">
      <div className="flex items-center gap-2">
        <MenuButton />
        <Button asChild variant="ghost" size="sm" className="text-muted-foreground -ml-3">
          <Link href="/agents">
            <ArrowLeft aria-hidden />
            {t("back")}
          </Link>
        </Button>
      </div>
      <h1 className="mt-2 truncate font-serif text-[clamp(1.75rem,3vw,2rem)] leading-[1.2] font-medium tracking-[-0.01em]">
        {name}
      </h1>
      {description && <p className="text-muted-foreground mt-2 line-clamp-2 max-w-[70ch]">{description}</p>}
      <div className="mt-4">
        <AgentMeta {...meta} />
      </div>
    </header>
  );
}
