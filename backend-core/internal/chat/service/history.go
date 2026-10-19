package service

import (
	"encoding/json"
	"strings"

	"github.com/firebase/genkit/go/ai"
)

const (
	uiPartText  = "text"
	uiPartSteer = "data-steer"
)

type uiPart struct {
	Type string          `json:"type"`
	Text string          `json:"text"`
	Data json.RawMessage `json:"data"`
}

type steerPartData struct {
	Text string `json:"text"`
}

// initialMessages rebuilds the model's view of the conversation from stored UI parts (index: Go rebuilds history
// from the DB). Turns already folded into the summary are left out; the summary rides in the system prompt.
func (r *run) initialMessages() []*ai.Message {
	var out []*ai.Message
	for _, m := range r.history.Messages {
		if r.history.Conversation.Summary != "" && m.Seq <= r.history.Conversation.SummaryThroughSeq {
			continue
		}
		text, steers := textAndSteers(m.Parts)
		for _, steer := range steers {
			out = append(out, ai.NewUserTextMessage(steer))
		}
		if text == "" {
			continue
		}
		if m.Role == roleUser {
			out = append(out, ai.NewUserTextMessage(text))
		} else {
			out = append(out, ai.NewModelTextMessage(text))
		}
	}
	return out
}

// textAndSteers reads a stored message: its text parts joined, and the texts of its steer parts. A message whose parts
// cannot be read contributes nothing.
func textAndSteers(raw json.RawMessage) (string, []string) {
	var parts []uiPart
	if json.Unmarshal(raw, &parts) != nil {
		return "", nil
	}
	var text strings.Builder
	var steers []string
	for _, part := range parts {
		switch part.Type {
		case uiPartText:
			text.WriteString(part.Text)
		case uiPartSteer:
			var steer steerPartData
			if json.Unmarshal(part.Data, &steer) == nil && steer.Text != "" {
				steers = append(steers, steer.Text)
			}
		}
	}
	return text.String(), steers
}
