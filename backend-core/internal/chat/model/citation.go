package model

import (
	"encoding/json"

	"github.com/google/uuid"
)

// CitationLocator is providerMetadata.thomas of a source-document part (index wire contracts). Locators are ragx's
// Locator objects passed through as they came.
type CitationLocator struct {
	FileID   uuid.UUID `json:"fileId"`
	KBID     uuid.UUID `json:"kbId"`
	FileName string    `json:"fileName"`
	// FileVersion is the ingested version the blocks belong to (Codex round 2 #4): the Evidence Viewer keys its
	// preview cache by it, because ragx block ids are positional and survive a reingest. 0 (omitted) = unknown.
	FileVersion int               `json:"fileVersion,omitempty"`
	KBName      string            `json:"kbName"`
	Section     []string          `json:"section"`
	PageStart   *int              `json:"pageStart"`
	PageEnd     *int              `json:"pageEnd"`
	BlockIDs    []string          `json:"blockIds"`
	Score       float64           `json:"score"`
	Locators    []json.RawMessage `json:"locators"`
}

type CitationFlag struct {
	SourceID    string  `json:"sourceId"`
	Claim       string  `json:"claim"`
	Probability float64 `json:"probability"`
}

// RunUsage is the data-usage part; costUsd is null when the provider reports no cost. ContextWindow is the
// effective window of the chat model (R18), so the web's context meter needs no second lookup (index amendment 7).
type RunUsage struct {
	RunID         string   `json:"runId"`
	InputTokens   int64    `json:"inputTokens"`
	OutputTokens  int64    `json:"outputTokens"`
	CostUSD       *float64 `json:"costUsd"`
	TTFTMS        int64    `json:"ttftMs"`
	DurationMS    int64    `json:"durationMs"`
	ContextWindow int      `json:"contextWindow"`
}
