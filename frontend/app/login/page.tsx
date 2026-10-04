import { getTranslations } from "next-intl/server";

import { BrandMark } from "@/components/metro/BrandMark";
import { RouteStrip, type RouteStop } from "@/components/metro/RouteStrip";
import { LoginForm } from "@/features/auth/LoginForm";
import { safeNext } from "@/features/auth/paths";

/** Line names are product nouns, kept identical across locales like the sidebar's. */
const stops: RouteStop[] = [
  { line: "model", label: "Model" },
  { line: "kb", label: "Knowledge Base", short: "KB" },
  { line: "mcp", label: "MCP" },
  { line: "cmp", label: "Composio" },
  { line: "app", label: "Apps" },
];

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  const t = await getTranslations("auth");
  const { next } = await searchParams;
  return (
    <main className="grid min-h-dvh grid-rows-[auto_1fr] md:grid-cols-[minmax(340px,2fr)_3fr] md:grid-rows-1">
      <section className="bg-sign text-sign-ink flex flex-col gap-8 px-6 py-7 md:justify-between md:gap-10 md:px-10 md:py-10">
        <BrandMark label="ThomasAgent" className="animate-fade" />
        <p className="animate-rise max-w-[18ch] text-[clamp(1.35rem,2.6vw,2.5rem)] leading-[1.15] font-semibold tracking-[-0.02em]">
          {t("signLine")}
        </p>
        {/* The page's one authored moment: the owner's line is drawn across the sign as the page opens. */}
        <div className="animate-draw-x">
          <RouteStrip stops={stops} />
        </div>
      </section>
      <section className="flex items-start justify-center px-6 py-10 md:items-center md:p-8">
        {/* The form is the first station of the owner's line: a roundel heading a stem, as on every empty page. */}
        <div className="relative w-full max-w-sm pl-9">
          <span
            aria-hidden
            className="border-model bg-ground animate-pop absolute top-2 left-0 size-5 rounded-full border-[5px] [animation-delay:0.5s]"
          />
          <span
            aria-hidden
            className="bg-model animate-stem absolute top-7 bottom-1 left-[8.5px] w-[3px] origin-top rounded-b-sm [animation-delay:0.65s]"
          />
          <div className="animate-rise">
            <h1 className="text-[28px] font-bold tracking-[-0.02em]">{t("title")}</h1>
            <p className="text-ink-2 mt-1 mb-9">{t("subtitle")}</p>
            <LoginForm next={safeNext(next)} />
          </div>
        </div>
      </section>
    </main>
  );
}
