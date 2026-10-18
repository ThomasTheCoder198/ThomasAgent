package client

import (
	"slices"
	"strings"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

const (
	googleModelPrefix    = "models/"
	googleMethodGenerate = "generateContent"
	googleMethodEmbed    = "embedContent"
)

// googleModels is the native GET {base}/models body (ruling A2-6); nextPageToken is ignored on purpose.
type googleModels struct {
	Models []googleModel `json:"models"`
}

type googleModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

func toGoogleModels(body googleModels) []model.RemoteModel {
	out := make([]model.RemoteModel, 0, len(body.Models))
	for _, m := range body.Models {
		ref := strings.TrimPrefix(m.Name, googleModelPrefix)
		capabilities := []model.Capability{}
		if slices.Contains(m.SupportedGenerationMethods, googleMethodGenerate) {
			capabilities = append(capabilities, model.CapabilityChat)
		}
		if slices.Contains(m.SupportedGenerationMethods, googleMethodEmbed) {
			capabilities = append(capabilities, model.CapabilityEmbedding)
		}
		var contextWindow *int
		if m.InputTokenLimit > 0 {
			limit := m.InputTokenLimit
			contextWindow = &limit
		}
		name := m.DisplayName
		if name == "" {
			name = ref
		}
		out = append(out, model.RemoteModel{ModelRef: ref, DisplayName: name, Capabilities: capabilities, ContextWindow: contextWindow})
	}
	return out
}
