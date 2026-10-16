# features/chat

## Purpose

The chat surface in the Editorial Trust system (`DESIGN.md`): the agent's run drawn as a line inside each message
(stations, Thinking Orbs, approvals), the answer with numbered citation roundels, the composer, and the evidence
panel (page with highlighted block, sources, Run Inspector). An empty thread opens on a hero. It does not persist
anything; core owns history.

## Entry points

- `ChatPage.tsx` — server half for `/chat` and `/chat/[id]`; loads conversation, scope and model choice.
- `ChatSurface.tsx` — client orchestrator; `useChatSession.ts` wires AI SDK `useChat` to `transport.ts` and reports failures with `useNotify().error`.
- `run-view.ts` — the only module that knows the UI Message Stream parts; maps a message to stations, text, citations, usage.
- `contract.ts` — PROVISIONAL M2 shapes and endpoints shared with `mock-core/`.
- `components/` — ChatHeader, ChatHero (`HeroIntro`, `HeroSources`), Composer, AssistantMessage (+ `PendingReply`, `UserMessage`), RunLine, StationMarker, StationContent, ApprovalSign, SummaryRow + MiniLine, Answer (+ `remark-citations.ts`), CitationRoundel, MessageFoot, SourceBadge.
- `components/AgentOrb.tsx` + `OrbNote.tsx` — the only places a Thinking Orb is mounted; `useCssColor.ts` resolves the theme token the orb needs as a hex.
- `evidence/` — EvidencePanel (docked column from xl, right Sheet md–xl, bottom Sheet 85dvh on phones), DocumentPageView, SourceList, RunInspector.

## Layout

- One centred `max-w-3xl` column (16px gutters, 24px from md) holds hero, thread and composer.
- Empty thread: from md the hero (64px `working` orb — `breathing` reads as still at that size — Newsreader title, one muted line), the composer and the tool-source chips sit in the vertical middle; below md the hero takes the top third and the composer stays docked at the bottom. The composer keeps one place in the tree, so its focus survives the first send.
- Before the first part of a reply: a 32px `solving` orb with "Thinking…" (`role="status"`). Live station: a 20px orb tinted with its line colour (KB searching, other tools connecting, model solving); while the answer text streams a 20px `composing` orb sits under it. Under `prefers-reduced-motion` orbs keep running at 0.6x speed (owner decision 2026-10-16).
- Evidence: Escape/overlay close apply to the Sheets (modal Radix); the docked column closes with its button.

## Dependencies

- Uses: `ai` 7 / `@ai-sdk/react` 4, `react-markdown` + `remark-gfm`, `thinking-orbs`, `lib/api/client` (CSRF headers), `lib/useNotify`, `components/ui` (shadcn), `components/lines`, `features/shell` (menu button, `pageOwnsHeader`).
- Used by: `app/(workspace)/chat/*`.

## Run & test

```bash
npx vitest run features/chat
npm run e2e -- e2e/chat.spec.ts
```

## Conventions

- Tool line colour and name plate come from `toolMetadata` (`{ line, source, durationMs }`), reasoning timing from `providerMetadata.thomas.durationMs`, citation locators from `source-document.providerMetadata.thomas`. Never infer a line from a tool name.
- The transport sends only the newest message (`ChatRequestBody`); the chosen model rides in the request `body`.
- `[n]` markers become roundels only when a source numbered `n` exists in the same message.
- Tool colours come only from `components/lines.ts`; an approval card uses its own station's line colour and icon.
- `ChatHeader` renders the shell's menu button only when `pageOwnsHeader(pathname)` says chat owns the mobile bar, so phones never get two bars.
- Orbs are mounted only through `AgentOrb`/`OrbNote` (tint from a CSS token) over `components/Orb.tsx`, which is never paused for reduced motion; tests mock `@/components/Orb` because jsdom has no canvas.
- `/chat` creates a fresh draft conversation; `/chat/[id]` renders the workspace 404 state when that conversation is missing.

## Common failures

- Stations appear all at once → the stream is buffered by compression → core must send `Cache-Control: no-transform` (S8).
- Approval click does nothing → `sendAutomaticallyWhen` needs `lastAssistantMessageIsCompleteWithApprovalResponses` and core must continue the same assistant message.
