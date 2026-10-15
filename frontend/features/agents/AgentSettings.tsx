"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import type { Model, RoleAssignment } from "@/features/registry/types";
import { loadedOr, type Loaded } from "@/lib/api/loaded";

import type { AgentDetail, KnowledgeBaseSummary } from "./contract";
import { AgentHeader } from "./components/AgentHeader";
import { ModelsTab } from "./components/ModelsTab";
import { SaveBar } from "./components/SaveBar";
import { agentTabs, isAgentTab, TAB_QUERY_KEY, type AgentTab } from "./tab-ids";
import { KnowledgeTab } from "./components/KnowledgeTab";
import { InstructionsTab, OverviewTab, type TabProps } from "./components/tabs";
import type { AgentDraft } from "./draft";
import { ToolsTab } from "./components/ToolsTab";
import { useAgentEditor } from "./useAgentEditor";

const tabKey = {
  overview: "tabOverview",
  instructions: "tabInstructions",
  knowledge: "tabKnowledge",
  tools: "tabTools",
  models: "tabModels",
} as const;

type Props = {
  agent: AgentDetail;
  knowledgeBases: Loaded<KnowledgeBaseSummary[]>;
  models: Loaded<Model[]>;
  roles: Loaded<RoleAssignment[]>;
  initialTab: AgentTab;
};

/** What the header shows, computed from the draft so it updates live while the owner edits. */
function headerFacts({
  agent,
  draft,
  knowledgeBases,
  models,
  roles,
}: Omit<Props, "initialTab"> & { draft: AgentDraft }) {
  const chatModelId =
    draft.overrides["chat.default"] ?? loadedOr(roles, []).find((r) => r.role === "chat.default")?.modelId;
  return {
    // A blank draft name keeps the stored name in the header; the field itself shows the error.
    name: draft.name.trim() || agent.name,
    description: draft.description,
    // An unloaded or unknown KB shows its raw id rather than pretending there is no binding.
    kbName: loadedOr(knowledgeBases, []).find((k) => k.id === draft.kbId)?.name ?? draft.kbId,
    lines: agent.toolSources.map((s) => s.line),
    toolCount: Object.values(draft.policies).filter((p) => p !== "off").length,
    model: loadedOr(models, []).find((m) => m.id === chatModelId)?.displayName ?? "",
  };
}

type BodyProps = TabProps & Pick<Props, "knowledgeBases" | "models" | "roles"> & { tab: AgentTab };

function TabBody({ tab, knowledgeBases, models, roles, ...tabProps }: BodyProps) {
  if (tab === "instructions") return <InstructionsTab {...tabProps} />;
  if (tab === "knowledge") return <KnowledgeTab {...tabProps} knowledgeBases={knowledgeBases} />;
  if (tab === "tools") return <ToolsTab {...tabProps} />;
  if (tab === "models") return <ModelsTab {...tabProps} models={models} roles={roles} />;
  return <OverviewTab {...tabProps} />;
}

/** The Agent's header, its settings tabs, and the save bar; edits stay local until one PATCH saves them. */
export function AgentSettings({ agent: initial, knowledgeBases, models, roles, initialTab }: Props) {
  const t = useTranslations("agents");
  const editor = useAgentEditor(initial);
  const { agent, draft, update } = editor;
  const [tab, setTab] = useState<AgentTab>(initialTab);

  function selectTab(next: string) {
    if (!isAgentTab(next)) return;
    setTab(next);
    // Keep the tab in the URL so a reload or a shared link lands on the same section.
    window.history.replaceState(null, "", `?${new URLSearchParams({ [TAB_QUERY_KEY]: next })}`);
  }

  return (
    <Tabs value={tab} onValueChange={selectTab} className="min-h-full gap-0">
      <AgentHeader {...headerFacts({ agent, draft, knowledgeBases, models, roles })} />
      <div className="bg-background sticky top-0 z-10">
        <div className="mx-auto w-full max-w-5xl px-4 md:px-6 lg:px-8">
          <TabsList aria-label={t("tabsLabel")}>
            {agentTabs.map((id) => (
              <TabsTrigger key={id} value={id}>
                {t(tabKey[id])}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </div>
      <TabsContent
        key={tab}
        value={tab}
        className="animate-rise mx-auto w-full max-w-5xl flex-1 px-4 pt-6 pb-28 outline-none md:px-6 lg:px-8"
      >
        {/* Disabling the whole fieldset while saving keeps the draft identical to what was sent. */}
        <fieldset disabled={editor.saving} className="contents">
          <TabBody
            tab={tab}
            agent={agent}
            draft={draft}
            update={update}
            errors={editor.errors}
            knowledgeBases={knowledgeBases}
            models={models}
            roles={roles}
          />
        </fieldset>
      </TabsContent>
      <SaveBar
        dirty={editor.dirty}
        canSave={editor.canSave}
        errors={editor.errors}
        saving={editor.saving}
        onSave={() => void editor.persist()}
        onDiscard={editor.discard}
      />
    </Tabs>
  );
}
