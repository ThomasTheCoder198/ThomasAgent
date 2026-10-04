# features/chat

## Purpose

The chat surface: the agent's run drawn as a Metro line inside each message (stations, Thinking Orbs, approvals),
the answer with numbered citation roundels, the composer, and the evidence panel (page with highlighted block,
sources, Run Inspector). It does not persist anything; core owns history.

## Entry points

- `ChatPage.tsx` — server half for `/chat` and `/chat/[id]`; loads conversation, scope and model choice.
- `ChatSurface.tsx` — client orchestrator; `useChatSession.ts` wires AI SDK `useChat` to `transport.ts`.
- `run-view.ts` — the only module that knows the UI Message Stream parts; maps a message to stations, text, citations, usage.
- `contract.ts` — PROVISIONAL M2 shapes and endpoints shared with `mock-core/`.
- `components/` — RunLine, StationMarker, ApprovalSign, SummaryRow + MiniLine, Answer (+ `remark-citations.ts`), Composer, MessageFoot.
- `evidence/` — EvidencePanel (column ≥ xl, overlay md–xl, bottom sheet on phones), DocumentPageView, SourceList, RunInspector.

## Dependencies

- Uses: `ai` 7 / `@ai-sdk/react` 4, `react-markdown` + `remark-gfm`, `thinking-orbs`, `lib/api/client` (CSRF headers), `components/metro`, `components/ui`.
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

## Common failures

- Stations appear all at once → the stream is buffered by compression → core must send `Cache-Control: no-transform` (S8).
- Approval click does nothing → `sendAutomaticallyWhen` needs `lastAssistantMessageIsCompleteWithApprovalResponses` and core must continue the same assistant message.
