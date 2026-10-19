package genkitx

import "github.com/firebase/genkit/go/ai"

// usageCostKey is where compat_oai puts OpenRouter's reported cost (generate.go of the pinned Genkit).
const usageCostKey = "cost"

type Usage struct {
	InputTokens, OutputTokens, CachedTokens, ReasoningTokens int64
	CostUSD                                                  *float64
}

func UsageOf(resp *ai.ModelResponse) Usage {
	if resp == nil || resp.Usage == nil {
		return Usage{}
	}
	u := resp.Usage
	out := Usage{
		InputTokens: int64(u.InputTokens), OutputTokens: int64(u.OutputTokens),
		CachedTokens: int64(u.CachedContentTokens), ReasoningTokens: int64(u.ThoughtsTokens),
	}
	if cost, ok := u.Custom[usageCostKey]; ok {
		out.CostUSD = &cost
	}
	return out
}
