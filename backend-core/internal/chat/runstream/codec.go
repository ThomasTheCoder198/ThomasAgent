package runstream

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/chat/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

// envelope tags the JSON of an event with its type, because model.Event is a closed interface with no type field and
// the reader must rebuild the concrete value for the UI stream encoder.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// eventKind ties one event type to its wire name and its decoder. The names are part of the stream's wire format
// between core serve and core worker.
type eventKind struct {
	name   string
	typ    reflect.Type
	decode func(json.RawMessage) (model.Event, error)
}

func kind[T model.Event](name string) eventKind {
	return eventKind{name: name, typ: reflect.TypeFor[T](), decode: func(data json.RawMessage) (model.Event, error) {
		var event T
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("%w: %w", errors.ErrRunEventInvalid, err)
		}
		return event, nil
	}}
}

// eventKinds lists every model.Event; the round-trip test fails when an event is added to model and not here.
func eventKinds() []eventKind {
	return []eventKind{
		kind[model.Start]("start"), kind[model.ReasoningStart]("reasoning-start"), kind[model.ReasoningDelta]("reasoning-delta"),
		kind[model.ReasoningEnd]("reasoning-end"), kind[model.ToolInputAvailable]("tool-input-available"),
		kind[model.ToolOutputAvailable]("tool-output-available"), kind[model.ToolOutputError]("tool-output-error"),
		kind[model.Status]("status"), kind[model.SourceDocument]("source-document"), kind[model.TextStart]("text-start"),
		kind[model.TextDelta]("text-delta"), kind[model.TextEnd]("text-end"), kind[model.CitationFlags]("citation-flags"),
		kind[model.Compaction]("compaction"), kind[model.Steer]("steer"), kind[model.Usage]("usage"),
		kind[model.Error]("error"), kind[model.Finish]("finish"),
	}
}

// IsTerminal reports whether the event ends a run's stream: the worker's finish or error.
func IsTerminal(event model.Event) bool {
	switch event.(type) {
	case model.Finish, model.Error:
		return true
	default:
		return false
	}
}

// EncodeEvent is the JSON the worker writes into a run's stream entry.
func EncodeEvent(event model.Event) ([]byte, error) {
	typ := reflect.TypeOf(event)
	for _, candidate := range eventKinds() {
		if candidate.typ != typ {
			continue
		}
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errors.ErrRunEventInvalid, err)
		}
		raw, err := json.Marshal(envelope{Type: candidate.name, Data: data})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errors.ErrRunEventInvalid, err)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("%w: %T", errors.ErrRunEventUnknown, event)
}

// DecodeEvent rebuilds the concrete event from an entry written by EncodeEvent.
func DecodeEvent(raw []byte) (model.Event, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: %w", errors.ErrRunEventInvalid, err)
	}
	for _, candidate := range eventKinds() {
		if candidate.name == env.Type {
			return candidate.decode(env.Data)
		}
	}
	return nil, fmt.Errorf("%w: type %q", errors.ErrRunEventUnknown, env.Type)
}
