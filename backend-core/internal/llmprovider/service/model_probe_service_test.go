package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/genkitx/genkitxtest"
)

type fakeProbeResolver struct{ m model.ResolvedModel }

func (f fakeProbeResolver) ResolveModel(context.Context, uuid.UUID) (model.ResolvedModel, error) {
	return f.m, nil
}

type recordedCalls struct{ calls []calllog.LLMCall }

func (r *recordedCalls) LLM(_ context.Context, call calllog.LLMCall) { r.calls = append(r.calls, call) }

func TestProbe_ChatModelAnswersAndLogsOneCall(t *testing.T) {
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "OK", InputTokens: 4, OutputTokens: 1})
	calls := &recordedCalls{}
	probe := service.NewModelProbeService(fakeProbeResolver{model.ResolvedModel{
		ProviderKind: model.KindOpenRouter, BaseURL: server.URL + "/api/v1", APIKey: "k", ModelRef: "m",
		Capabilities: []model.Capability{model.CapabilityChat},
	}}, genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1}), calls, time.Second)

	result, err := probe.Probe(context.Background(), uuid.New())

	require.NoError(t, err)
	require.GreaterOrEqual(t, result.LatencyMS, int64(0))
	require.Len(t, calls.calls, 1)
	require.Equal(t, int64(4), calls.calls[0].InputTokens)
	require.Empty(t, calls.calls[0].ErrorCode)
}

func TestProbe_CompatibleProvidersSendOutputTokenLimit(t *testing.T) {
	for _, kind := range []model.ProviderKind{model.KindOpenAI, model.KindOpenAICompatible, model.KindOllama} {
		t.Run(string(kind), func(t *testing.T) {
			server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Text: "OK"})
			probe := service.NewModelProbeService(fakeProbeResolver{model.ResolvedModel{
				ProviderKind: kind, BaseURL: server.URL, APIKey: "k", ModelRef: "m",
				Capabilities: []model.Capability{model.CapabilityChat},
			}}, genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1}), &recordedCalls{}, time.Second)

			_, err := probe.Probe(context.Background(), uuid.New())

			require.NoError(t, err)
			require.Len(t, server.Requests(), 1)
			require.Equal(t, float64(16), server.Requests()[0].Body["max_tokens"])
		})
	}
}

func TestProbe_RejectedKeyIsProviderRejected(t *testing.T) {
	server := genkitxtest.NewOpenAIServer(t, genkitxtest.Turn{Status: 401})
	calls := &recordedCalls{}
	probe := service.NewModelProbeService(fakeProbeResolver{model.ResolvedModel{
		ProviderKind: model.KindOpenRouter, BaseURL: server.URL + "/api/v1", APIKey: "bad", ModelRef: "m",
		Capabilities: []model.Capability{model.CapabilityChat},
	}}, genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1}), calls, time.Second)

	_, err := probe.Probe(context.Background(), uuid.New())

	require.ErrorIs(t, err, errors.ErrLLMProviderRejected)
	require.Equal(t, string(errors.CodeLLMProviderRejected), calls.calls[0].ErrorCode)
}

func TestProbe_EmbeddingOnlyModelIsNotTestableInCore(t *testing.T) {
	probe := service.NewModelProbeService(fakeProbeResolver{model.ResolvedModel{
		ProviderKind: model.KindOpenAI, Capabilities: []model.Capability{model.CapabilityEmbedding},
	}}, genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1}), &recordedCalls{}, time.Second)

	_, err := probe.Probe(context.Background(), uuid.New())

	require.ErrorIs(t, err, errors.ErrLLMModelCapabilityMismatch)
}

func TestProbe_DecisionModelAnswersOverSystemOne(t *testing.T) {
	server := genkitxtest.NewDecisionsServer(t, map[string]any{"greeting": map[string]any{"type": "noul", "noul": 0.97}})
	probe := service.NewModelProbeService(fakeProbeResolver{model.ResolvedModel{
		ProviderKind: model.KindOpenRouter, BaseURL: server.URL + "/api/v1", APIKey: "k", ModelRef: "typesafe/jev-1.14",
		Capabilities: []model.Capability{model.CapabilityDecision},
	}}, genkitx.NewModelRegistry(genkitx.Config{CacheSize: 1}), &recordedCalls{}, time.Second)

	_, err := probe.Probe(context.Background(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, "/api/alpha/decisions", server.Requests()[0].Path)
}
