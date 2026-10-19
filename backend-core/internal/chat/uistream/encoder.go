// Package uistream encodes chat run events as AI SDK UI Message Stream v1 chunks, one JSON object per SSE data
// line, and rebuilds the assistant message the browser builds from them. Goldens in testdata come from ai 7.0.127.
package uistream

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	HeaderName        = "x-vercel-ai-ui-message-stream"
	HeaderValue       = "v1"
	metadataNamespace = "thomas"

	chunkStart               = "start"
	chunkFinish              = "finish"
	chunkError               = "error"
	chunkReasoningStart      = "reasoning-start"
	chunkReasoningDelta      = "reasoning-delta"
	chunkReasoningEnd        = "reasoning-end"
	chunkTextStart           = "text-start"
	chunkTextDelta           = "text-delta"
	chunkTextEnd             = "text-end"
	chunkToolInputStart      = "tool-input-start"
	chunkToolInputAvailable  = "tool-input-available"
	chunkToolOutputAvailable = "tool-output-available"
	chunkToolOutputError     = "tool-output-error"
	chunkSourceDocument      = "source-document"
	chunkDataStatus          = "data-status"
	chunkDataUsage           = "data-usage"
	chunkDataCitations       = "data-citations"
	chunkDataCompaction      = "data-compaction"
	chunkDataSteer           = "data-steer"

	idStatusInfix     = "-s"
	idSteerInfix      = "-steer-"
	idUsageSuffix     = "-usage"
	idCitationsSuffix = "-citations"
	idCompactSuffix   = "-compaction"
)

// DoneFrame ends the stream, as the AI SDK's own SSE transform does.
var DoneFrame = []byte("[DONE]")

type runMetadata struct {
	RunID string `json:"runId"`
}

type durationMeta struct {
	DurationMS int64 `json:"durationMs"`
}

type toolMetadata struct {
	Line       string `json:"line"`
	Source     string `json:"source"`
	DurationMS *int64 `json:"durationMs,omitempty"`
}

type startChunk struct {
	Type            string      `json:"type"`
	MessageID       string      `json:"messageId"`
	MessageMetadata runMetadata `json:"messageMetadata"`
}

type typeChunk struct {
	Type string `json:"type"`
}

type idChunk struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type deltaChunk struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Delta string `json:"delta"`
}

type reasoningEndChunk struct {
	Type             string                  `json:"type"`
	ID               string                  `json:"id"`
	ProviderMetadata map[string]durationMeta `json:"providerMetadata"`
}

type toolInputChunk struct {
	Type         string          `json:"type"`
	ToolCallID   string          `json:"toolCallId"`
	ToolName     string          `json:"toolName"`
	Dynamic      bool            `json:"dynamic"`
	ToolMetadata toolMetadata    `json:"toolMetadata"`
	Input        json.RawMessage `json:"input,omitempty"`
}

type toolOutputChunk struct {
	Type         string          `json:"type"`
	ToolCallID   string          `json:"toolCallId"`
	Output       json.RawMessage `json:"output"`
	Dynamic      bool            `json:"dynamic"`
	ToolMetadata toolMetadata    `json:"toolMetadata"`
}

type toolErrorChunk struct {
	Type         string       `json:"type"`
	ToolCallID   string       `json:"toolCallId"`
	ErrorText    string       `json:"errorText"`
	Dynamic      bool         `json:"dynamic"`
	ToolMetadata toolMetadata `json:"toolMetadata"`
}

type sourceChunk struct {
	Type             string                           `json:"type"`
	SourceID         string                           `json:"sourceId"`
	MediaType        string                           `json:"mediaType"`
	Title            string                           `json:"title"`
	Filename         string                           `json:"filename,omitempty"`
	ProviderMetadata map[string]model.CitationLocator `json:"providerMetadata"`
}

type dataChunk struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Data any    `json:"data"`
}

type errorChunk struct {
	Type      string `json:"type"`
	ErrorText string `json:"errorText"`
}

type labelData struct {
	Label string `json:"label"`
}

type flagsData struct {
	Flags []model.CitationFlag `json:"flags"`
}

type compactionData struct {
	SummarizedTurns int `json:"summarizedTurns"`
}

type steerData struct {
	Text string `json:"text"`
}

// Encoder is per stream: it remembers the run id (for data part ids) and each tool call's metadata (the output
// chunks repeat it, as the mock does).
type Encoder struct {
	runID    string
	tools    map[string]toolMetadata
	statuses int
	steers   int
}

func NewEncoder() *Encoder {
	return &Encoder{tools: map[string]toolMetadata{}}
}

// Encode returns the JSON payload of each chunk the event becomes, in order.
func (e *Encoder) Encode(event model.Event) ([][]byte, error) {
	chunks, err := e.chunks(event)
	if err != nil {
		return nil, err
	}
	frames := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		frame, err := json.Marshal(chunk)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func (e *Encoder) chunks(event model.Event) ([]any, error) {
	switch ev := event.(type) {
	case model.Start:
		e.runID = ev.RunID.String()
		return []any{startChunk{Type: chunkStart, MessageID: ev.MessageID, MessageMetadata: runMetadata{RunID: e.runID}}}, nil
	case model.ReasoningStart:
		return []any{idChunk{Type: chunkReasoningStart, ID: ev.ID}}, nil
	case model.ReasoningDelta:
		return []any{deltaChunk{Type: chunkReasoningDelta, ID: ev.ID, Delta: ev.Delta}}, nil
	case model.ReasoningEnd:
		return []any{reasoningEndChunk{Type: chunkReasoningEnd, ID: ev.ID,
			ProviderMetadata: map[string]durationMeta{metadataNamespace: {DurationMS: ev.DurationMS}}}}, nil
	case model.ToolInputAvailable:
		meta := toolMetadata{Line: ev.Line, Source: ev.Source}
		e.tools[ev.CallID] = meta
		return []any{
			toolInputChunk{Type: chunkToolInputStart, ToolCallID: ev.CallID, ToolName: ev.Tool, Dynamic: true, ToolMetadata: meta},
			toolInputChunk{Type: chunkToolInputAvailable, ToolCallID: ev.CallID, ToolName: ev.Tool, Dynamic: true,
				ToolMetadata: meta, Input: ev.Input},
		}, nil
	case model.ToolOutputAvailable:
		meta, ok := e.tools[ev.CallID]
		if !ok {
			return nil, fmt.Errorf("%w: output of %q", errors.ErrUIStreamUnknownToolCall, ev.CallID)
		}
		return []any{toolOutputChunk{Type: chunkToolOutputAvailable, ToolCallID: ev.CallID, Output: ev.Output, Dynamic: true,
			ToolMetadata: withDuration(meta, ev.DurationMS)}}, nil
	case model.ToolOutputError:
		meta, ok := e.tools[ev.CallID]
		if !ok {
			return nil, fmt.Errorf("%w: error of %q", errors.ErrUIStreamUnknownToolCall, ev.CallID)
		}
		return []any{toolErrorChunk{Type: chunkToolOutputError, ToolCallID: ev.CallID, ErrorText: ev.ErrorText, Dynamic: true,
			ToolMetadata: withDuration(meta, ev.DurationMS)}}, nil
	default:
		return e.remainingChunks(event)
	}
}

func (e *Encoder) remainingChunks(event model.Event) ([]any, error) {
	switch ev := event.(type) {
	case model.Status:
		e.statuses++
		return []any{dataChunk{Type: chunkDataStatus, ID: e.runID + idStatusInfix + strconv.Itoa(e.statuses), Data: labelData{Label: ev.Label}}}, nil
	case model.SourceDocument:
		return []any{sourceChunk{Type: chunkSourceDocument, SourceID: ev.SourceID, MediaType: mediaTypeOf(ev.FileName),
			Title: ev.Title, Filename: ev.FileName,
			ProviderMetadata: map[string]model.CitationLocator{metadataNamespace: ev.Locator}}}, nil
	case model.TextStart:
		return []any{idChunk{Type: chunkTextStart, ID: ev.ID}}, nil
	case model.TextDelta:
		return []any{deltaChunk{Type: chunkTextDelta, ID: ev.ID, Delta: ev.Delta}}, nil
	case model.TextEnd:
		return []any{idChunk{Type: chunkTextEnd, ID: ev.ID}}, nil
	case model.CitationFlags:
		return []any{dataChunk{Type: chunkDataCitations, ID: e.runID + idCitationsSuffix, Data: flagsData{Flags: ev.Flags}}}, nil
	case model.Compaction:
		return []any{dataChunk{Type: chunkDataCompaction, ID: e.runID + idCompactSuffix,
			Data: compactionData{SummarizedTurns: ev.SummarizedTurns}}}, nil
	default:
		return e.finalChunks(event)
	}
}

func (e *Encoder) finalChunks(event model.Event) ([]any, error) {
	switch ev := event.(type) {
	case model.Steer:
		e.steers++
		return []any{dataChunk{Type: chunkDataSteer, ID: e.runID + idSteerInfix + strconv.Itoa(e.steers), Data: steerData{Text: ev.Text}}}, nil
	case model.Usage:
		return []any{dataChunk{Type: chunkDataUsage, ID: e.runID + idUsageSuffix, Data: ev.RunUsage}}, nil
	case model.Error: // B2-7: the code only; Message stays server-side for logs
		return []any{errorChunk{Type: chunkError, ErrorText: ev.Code}}, nil
	case model.Finish:
		return []any{typeChunk{Type: chunkFinish}}, nil
	default:
		return nil, fmt.Errorf("%w: %T", errors.ErrUIStreamUnknownEvent, event)
	}
}

func withDuration(meta toolMetadata, durationMS int64) toolMetadata {
	meta.DurationMS = &durationMS
	return meta
}
