"use client";

import { useTranslations } from "next-intl";

import { RouteStrip } from "@/components/metro/RouteStrip";

/** The empty thread: the question that starts a new line, and the lines it can travel. */
export function NewChatIntro() {
  const t = useTranslations("chat");
  return (
    <section className="flex flex-col gap-6 pt-[14vh] pb-10">
      <div>
        <h2 className="text-[clamp(1.6rem,2.4vw,2.1rem)] leading-tight font-bold tracking-[-0.02em]">
          {t("newTitle")}
        </h2>
        <p className="text-ink-2 mt-2 max-w-[56ch] leading-relaxed">{t("newBody")}</p>
      </div>
      <div className="bg-sign max-w-md rounded-xl px-5 py-4">
        <RouteStrip
          stops={[
            { line: "model", label: "Model" },
            { line: "kb", label: "KB" },
            { line: "mcp", label: "MCP" },
            { line: "cmp", label: "Composio" },
            { line: "app", label: "Apps" },
          ]}
        />
      </div>
    </section>
  );
}
