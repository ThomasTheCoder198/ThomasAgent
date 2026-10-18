package client

import (
	"math"
	"slices"
	"strconv"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

const (
	tokensPerMillion     = 1_000_000
	openRouterParamTool  = "tools"
	openRouterParamThink = "reasoning"
	modalityImage        = "image"
	modalityText         = "text"
)

// openRouterModels is the GET /models body; other providers share only the "data[].id" field.
type openRouterModels struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Architecture  struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters []string `json:"supported_parameters"`
}

// toPlainModels keeps only the ids for providers whose list carries no capabilities or prices.
func toPlainModels(body openRouterModels) []model.RemoteModel {
	out := make([]model.RemoteModel, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, model.RemoteModel{ModelRef: m.ID, DisplayName: m.ID})
	}
	return out
}

func toOpenRouterModels(body openRouterModels) []model.RemoteModel {
	out := make([]model.RemoteModel, 0, len(body.Data))
	for _, m := range body.Data {
		capabilities := []model.Capability{}
		if slices.Contains(m.Architecture.OutputModalities, modalityText) {
			capabilities = append(capabilities, model.CapabilityChat)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamTool) {
			capabilities = append(capabilities, model.CapabilityTools)
		}
		if slices.Contains(m.SupportedParameters, openRouterParamThink) {
			capabilities = append(capabilities, model.CapabilityReasoning)
		}
		if slices.Contains(m.Architecture.InputModalities, modalityImage) {
			capabilities = append(capabilities, model.CapabilityVision)
		}
		var contextWindow *int
		if m.ContextLength > 0 {
			length := m.ContextLength
			contextWindow = &length
		}
		out = append(out, model.RemoteModel{
			ModelRef: m.ID, DisplayName: m.Name, Capabilities: capabilities, ContextWindow: contextWindow,
			InputPricePerMTok: perMillion(m.Pricing.Prompt), OutputPricePerMTok: perMillion(m.Pricing.Completion),
		})
	}
	return out
}

// toDecisionModels maps OpenRouter's decision list: a decision model only decides (O8), so it carries exactly
// the decision capability, whatever its parameters say.
func toDecisionModels(body openRouterModels) []model.RemoteModel {
	out := make([]model.RemoteModel, 0, len(body.Data))
	for _, m := range body.Data {
		if m.ID == "" {
			continue
		}
		var contextWindow *int
		if m.ContextLength > 0 {
			length := m.ContextLength
			contextWindow = &length
		}
		name := m.Name
		if name == "" {
			name = m.ID
		}
		out = append(out, model.RemoteModel{ModelRef: m.ID, DisplayName: name, Capabilities: []model.Capability{model.CapabilityDecision},
			ContextWindow: contextWindow, InputPricePerMTok: perMillion(m.Pricing.Prompt), OutputPricePerMTok: perMillion(m.Pricing.Completion)})
	}
	return out
}

// mergeDecisionModels replaces a chat row that is also a decision model and appends the others.
func mergeDecisionModels(models, decisions []model.RemoteModel) []model.RemoteModel {
	index := make(map[string]int, len(models))
	for i, m := range models {
		index[m.ModelRef] = i
	}
	for _, decision := range decisions {
		if i, ok := index[decision.ModelRef]; ok {
			models[i] = decision
			continue
		}
		models = append(models, decision)
	}
	return models
}

// perMillion converts OpenRouter's per-token price; unknown or negative prices stay nil.
func perMillion(perToken string) *float64 {
	pricePerToken, err := strconv.ParseFloat(perToken, 64)
	if err != nil || pricePerToken < 0 || math.IsNaN(pricePerToken) || math.IsInf(pricePerToken, 0) {
		return nil
	}
	price := pricePerToken * tokensPerMillion
	if math.IsInf(price, 0) {
		return nil
	}
	return &price
}
