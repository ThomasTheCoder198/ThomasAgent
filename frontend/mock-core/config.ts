/** Tunables for the local fake core. Every value can be overridden through MOCK_CORE_* env vars. */
const DEFAULT_PORT = 8787;
const SESSION_TTL_SECONDS = 43_200;

export const mockConfig = {
  // Not 8080: the real core and `task api-test` own that port.
  port: Number(process.env.MOCK_CORE_PORT ?? DEFAULT_PORT),
  ownerEmail: process.env.MOCK_CORE_OWNER_EMAIL ?? "owner@thomas.local",
  ownerPassword: process.env.MOCK_CORE_OWNER_PASSWORD ?? "metro-wayfinding",
  // The suffix keeps synthetic data visibly synthetic in the UI (owner plate), with no mock code in the app.
  // Leading tag so the word survives truncation in the 248px owner plate.
  ownerName: process.env.MOCK_CORE_OWNER_NAME ?? "Mẫu · Thomas",
  sessionTtlSeconds: Number(process.env.MOCK_CORE_SESSION_TTL ?? SESSION_TTL_SECONDS),
  /** Multiplies every scripted delay; 0 makes runs instant (handy for e2e). */
  speed: Number(process.env.MOCK_CORE_SPEED ?? 1),
} as const;

/** Scripted pacing of a run, in milliseconds before `speed` is applied. */
export const pacing = {
  firstChunk: 450,
  reasoningWord: 38,
  textWord: 26,
  kbSearch: 620,
  kbRead: 240,
  toolCall: 900,
  betweenStations: 180,
} as const;
