package uistream

import (
	"encoding/json"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
)

// Facts from the ai 7.0.127 golden: reasoning retains its id and provider metadata; text has no id;
// both end in state done. Dynamic tools retain their input and the latest tool metadata (including output
// duration); data parts retain ids and replace prior data with the same type and id.

const (
	partText            = "text"
	partReasoning       = "reasoning"
	partDynamicTool     = "dynamic-tool"
	stateStreaming      = "streaming"
	stateInputStreaming = "input-streaming"
	stateDone           = "done"
	stateInput          = "input-available"
	stateOutput         = "output-available"
	stateOutputError    = "output-error"
	keyType             = "type"
	keyText             = "text"
	keyState            = "state"
	keyProviderMeta     = "providerMetadata"
	keyToolName         = "toolName"
	keyToolCallID       = "toolCallId"
	keyInput            = "input"
	keyOutput           = "output"
	keyErrorText        = "errorText"
	keyToolMetadata     = "toolMetadata"
	keyID               = "id"
	keyData             = "data"
	keySourceID         = "sourceId"
	keyMediaType        = "mediaType"
	keyTitle            = "title"
	keyFilename         = "filename"

	// dataKeySeparator joins a data part's type and id into one map key; NUL cannot appear in either.
	dataKeySeparator = "\x00"
	emptyParts       = "[]"
)

// MessageBuilder applies run events the way the AI SDK applies chunks, so the stored message is what the browser
// showed (B2-6). It is not safe for concurrent use; the run loop owns it.
type MessageBuilder struct {
	runID    string
	parts    []map[string]any
	byText   map[string]int
	byReason map[string]int
	byTool   map[string]int
	byData   map[string]int
	encoder  *Encoder // reuses the encoder's data ids and tool metadata
}

func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{
		byText: map[string]int{}, byReason: map[string]int{}, byTool: map[string]int{},
		byData: map[string]int{}, encoder: NewEncoder(),
	}
}

// Apply folds one event in. Events the browser does not store (start, finish, error) only update state.
func (b *MessageBuilder) Apply(event model.Event) {
	chunks, err := b.encoder.chunks(event)
	if err != nil {
		return // an event the encoder refuses never reached the browser either
	}
	if start, ok := event.(model.Start); ok {
		b.runID = start.RunID.String()
	}
	for _, chunk := range chunks {
		b.applyChunk(chunk)
	}
}

func (b *MessageBuilder) applyChunk(chunk any) {
	switch c := chunk.(type) {
	case idChunk:
		b.applyID(c)
	case deltaChunk:
		index := b.byText
		if c.Type == chunkReasoningDelta {
			index = b.byReason
		}
		if i, ok := index[c.ID]; ok {
			b.parts[i][keyText] = b.parts[i][keyText].(string) + c.Delta
		}
	case reasoningEndChunk:
		if i, ok := b.byReason[c.ID]; ok {
			b.parts[i][keyState] = stateDone
			b.parts[i][keyProviderMeta] = c.ProviderMetadata
		}
	case toolInputChunk:
		b.applyToolInput(c)
	default:
		b.applyResultChunk(chunk)
	}
}

func (b *MessageBuilder) applyResultChunk(chunk any) {
	switch c := chunk.(type) {
	case toolOutputChunk:
		if i, ok := b.byTool[c.ToolCallID]; ok {
			b.parts[i][keyState] = stateOutput
			b.parts[i][keyOutput] = c.Output
			b.parts[i][keyToolMetadata] = c.ToolMetadata
		}
	case toolErrorChunk:
		if i, ok := b.byTool[c.ToolCallID]; ok {
			b.parts[i][keyState] = stateOutputError
			b.parts[i][keyErrorText] = c.ErrorText
			b.parts[i][keyToolMetadata] = c.ToolMetadata
		}
	case sourceChunk:
		part := map[string]any{keyType: c.Type, keySourceID: c.SourceID, keyMediaType: c.MediaType, keyTitle: c.Title,
			keyProviderMeta: c.ProviderMetadata}
		if c.Filename != "" {
			part[keyFilename] = c.Filename
		}
		b.add(part)
	case dataChunk:
		part := map[string]any{keyType: c.Type, keyID: c.ID, keyData: c.Data}
		key := c.Type + dataKeySeparator + c.ID
		if i, ok := b.byData[key]; ok { // same type and id replaces, as the SDK does
			b.parts[i] = part
			return
		}
		b.byData[key] = b.add(part)
	}
}

func (b *MessageBuilder) applyID(c idChunk) {
	switch c.Type {
	case chunkTextStart:
		b.byText[c.ID] = b.add(map[string]any{keyType: partText, keyText: "", keyState: stateStreaming})
	case chunkReasoningStart:
		b.byReason[c.ID] = b.add(map[string]any{keyType: partReasoning, keyID: c.ID, keyText: "", keyState: stateStreaming})
	case chunkTextEnd:
		if i, ok := b.byText[c.ID]; ok {
			b.parts[i][keyState] = stateDone
		}
	}
}

func (b *MessageBuilder) applyToolInput(c toolInputChunk) {
	i, ok := b.byTool[c.ToolCallID]
	if !ok {
		i = b.add(map[string]any{keyType: partDynamicTool, keyToolName: c.ToolName, keyToolCallID: c.ToolCallID})
		b.byTool[c.ToolCallID] = i
	}
	b.parts[i][keyToolMetadata] = c.ToolMetadata
	if c.Type == chunkToolInputAvailable {
		b.parts[i][keyState] = stateInput
		b.parts[i][keyInput] = c.Input
	} else {
		b.parts[i][keyState] = stateInputStreaming
	}
}

func (b *MessageBuilder) add(part map[string]any) int {
	b.parts = append(b.parts, part)
	return len(b.parts) - 1
}

// Parts closes any text or reasoning still streaming (a stopped or failed run) and returns the parts JSON.
func (b *MessageBuilder) Parts() (json.RawMessage, error) {
	for _, part := range b.parts {
		if part[keyState] == stateStreaming {
			part[keyState] = stateDone
		}
	}
	if b.parts == nil {
		return json.RawMessage(emptyParts), nil
	}
	return json.Marshal(b.parts)
}

// Empty reports whether no part was built yet, so a run that streamed nothing stores no assistant message.
func (b *MessageBuilder) Empty() bool { return len(b.parts) == 0 }

func (b *MessageBuilder) Metadata() json.RawMessage {
	raw, _ := json.Marshal(runMetadata{RunID: b.runID})
	return raw
}
