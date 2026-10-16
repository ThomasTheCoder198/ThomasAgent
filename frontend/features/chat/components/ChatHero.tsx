"use client";

import { Brain } from "lucide-react";
import { useTranslations } from "next-intl";

import { lineIcons } from "@/components/lineIcons";
import { lineText, toolLines, type Line } from "@/components/lines";
import { Badge } from "@/components/ui/Badge";
import { cn } from "@/lib/cn";
import { staggerStyle } from "@/lib/stagger";

import { AgentOrb } from "./AgentOrb";

/** Product names of the sources the agent can reach; the model's own label is translated. */
const SOURCE_NAMES = { kb: "KB", mcp: "MCP", cmp: "Composio", app: "Apps" } as const;

const HERO_ORB_SIZE = 64;

/** Hero pieces arrive one after another: orb, title, body, composer, chips. */
export const HERO_STAGGER_MS = 60;
export const heroStep = { orb: 0, title: 1, body: 2, composer: 3, chips: 4 } as const;
export const heroStyle = (step: number) => staggerStyle(step, HERO_STAGGER_MS);

/** The empty thread's opening: a living orb, the question, and one muted line about how answers are made. */
export function HeroIntro({ className }: { className?: string }) {
  const t = useTranslations("chat");
  return (
    <section className={cn("flex flex-col items-center gap-4 px-4 text-center md:px-6", className)}>
      {/* `breathing` is a slow face-on ring that reads as still at 64px; the orbiting particles of `working` read as alive. */}
      <div className="animate-rise" style={heroStyle(heroStep.orb)}>
        <AgentOrb state="working" size={HERO_ORB_SIZE} />
      </div>
      <h2
        style={heroStyle(heroStep.title)}
        className="animate-rise max-w-[24ch] font-serif text-[clamp(2rem,4vw,2.75rem)] leading-[1.15] font-medium tracking-[-0.01em] text-balance"
      >
        {t("newTitle")}
      </h2>
      <p
        style={heroStyle(heroStep.body)}
        className="text-muted-foreground animate-rise max-w-[56ch] text-[0.9375rem] text-pretty"
      >
        {t("newBody")}
      </p>
    </section>
  );
}

function SourceChip({ line, name }: { line: Line; name: string }) {
  const Icon = line === "model" ? Brain : lineIcons[line];
  return (
    <li>
      <Badge className="h-7 px-3 text-[0.8125rem] [&>svg]:size-4">
        <Icon aria-hidden className={lineText[line]} />
        {name}
      </Badge>
    </li>
  );
}

/** What the agent can reach, as small chips in each source's line colour. */
export function HeroSources({ className }: { className?: string }) {
  const t = useTranslations("chat");
  return (
    <ul
      aria-label={t("sourcesLabel")}
      style={heroStyle(heroStep.chips)}
      className={cn("animate-rise flex flex-wrap justify-center gap-2 px-4 md:px-6", className)}
    >
      <SourceChip line="model" name={t("model")} />
      {toolLines.map((line) => (
        <SourceChip key={line} line={line} name={SOURCE_NAMES[line]} />
      ))}
    </ul>
  );
}
