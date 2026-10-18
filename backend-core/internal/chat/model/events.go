// Package model holds the chat run's types: the request, the events a run emits and the citation shapes the UI
// reads. Events are the only contract between the run service and the stream encoder (index R5).
package model

import (
	"encoding/json"

	"github.com/google/uuid"
)

// LineKB and SourceKB label KB tool stations (frontend components/lines.ts and run-view.ts).
const (
	LineKB   = "kb"
	SourceKB = "Knowledge Base"
)

type Event interface{ isEvent() }

type Start struct {
	MessageID string
	RunID     uuid.UUID
}

type ReasoningStart struct{ ID string }

type ReasoningDelta struct{ ID, Delta string }

type ReasoningEnd struct {
	ID         string
	DurationMS int64
}

// ToolInputAvailable becomes both tool-input-start and tool-input-available, as the mock emits them.
type ToolInputAvailable struct {
	CallID, Tool string
	Input        json.RawMessage
	Line, Source string
}

// ToolOutputAvailable carries the UI summary (B2-2), not what the model saw.
type ToolOutputAvailable struct {
	CallID     string
	Output     json.RawMessage
	DurationMS int64
}

type ToolOutputError struct {
	CallID, ErrorText string
	DurationMS        int64
}

type Status struct{ Label string }

type SourceDocument struct {
	SourceID, Title, FileName string
	Locator                   CitationLocator
}

type TextStart struct{ ID string }

type TextDelta struct{ ID, Delta string }

type TextEnd struct{ ID string }

type CitationFlags struct{ Flags []CitationFlag }

type Compaction struct{ SummarizedTurns int }

type Steer struct{ Text string }

type Usage struct{ RunUsage }

// Error is sent after the stream has started; failures before it are plain HTTP errors (R5).
type Error struct{ Code, Message string }

type Finish struct{}

func (Start) isEvent()               {}
func (ReasoningStart) isEvent()      {}
func (ReasoningDelta) isEvent()      {}
func (ReasoningEnd) isEvent()        {}
func (ToolInputAvailable) isEvent()  {}
func (ToolOutputAvailable) isEvent() {}
func (ToolOutputError) isEvent()     {}
func (Status) isEvent()              {}
func (SourceDocument) isEvent()      {}
func (TextStart) isEvent()           {}
func (TextDelta) isEvent()           {}
func (TextEnd) isEvent()             {}
func (CitationFlags) isEvent()       {}
func (Compaction) isEvent()          {}
func (Steer) isEvent()               {}
func (Usage) isEvent()               {}
func (Error) isEvent()               {}
func (Finish) isEvent()              {}
