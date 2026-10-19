// Package genkitx turns a resolved model (provider, endpoint, key) into a callable Genkit model. Instances are
// cached (spike S1, approach A): one per endpoint where the key travels per request (OpenRouter chat), one per key
// version where the plugin holds the key (OpenAI-compatible chat, System One decisions).
package genkitx

import (
	"cmp"
	"container/list"
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/firebase/genkit/go/plugins/compat_oai/openrouter"
	systemonex "github.com/firebase/genkit/go/plugins/systemone/exp"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
)

const (
	// pluginPlaceholderKey satisfies plugin Init where the real key travels per request (S1: Init needs a non-empty key).
	pluginPlaceholderKey  = "per-request-key"
	openRouterProvider    = "openrouter"
	compatProvider        = "compat"
	decisionProvider      = "openrouter-decisions"
	openRouterAPIPath     = "/api/v1"
	cacheKeySeparator     = "|"
	keyVersionSeparator   = "@"
	reasonKindUnsupported = "kind_not_supported"
	reasonMissingKey      = "missing_key"
	detailReason          = "reason"
	detailKind            = "kind"
)

type Config struct {
	CacheSize int
	// HTTPClient carries the outbound policy (SSRF allowlist, LLM metrics). Nil uses Go's default client (tests).
	HTTPClient *http.Client
	// MaxRetries is the provider SDK's retry budget: it retries 429/5xx with backoff and honours Retry-After.
	MaxRetries int
}

type ChatModel struct {
	G    *genkit.Genkit
	Name string
	// Config carries the per-request key for OpenRouter or the SDK config for compatible providers.
	Config any
}

// Options names the model and adds its config with an output cap.
func (m ChatModel) Options(maxOutputTokens int) []ai.GenerateOption {
	options := []ai.GenerateOption{ai.WithModelName(m.Name)}
	switch cfg := m.Config.(type) {
	case openrouter.ChatConfig:
		cfg.MaxOutputTokens = maxOutputTokens
		options = append(options, ai.WithConfig(cfg))
	case openai.ChatCompletionNewParams:
		cfg.MaxTokens = openai.Int(int64(maxOutputTokens))
		options = append(options, ai.WithConfig(cfg))
	}
	return options
}

type DecisionModel struct {
	G    *genkit.Genkit
	Name string
}

type ModelRegistry interface {
	Chat(ctx context.Context, m llmmodel.ResolvedModel) (ChatModel, error)
	Decision(ctx context.Context, m llmmodel.ResolvedModel) (DecisionModel, error)
}

type Registry struct {
	cfg   Config
	mu    sync.Mutex
	order *list.List
	items map[string]*list.Element
}

type cacheEntry struct {
	key string
	g   *genkit.Genkit
}

func NewModelRegistry(cfg Config) *Registry {
	return &Registry{cfg: cfg, order: list.New(), items: map[string]*list.Element{}}
}

func (r *Registry) Chat(ctx context.Context, m llmmodel.ResolvedModel) (ChatModel, error) {
	base := baseURL(m)
	switch m.ProviderKind {
	case llmmodel.KindOpenRouter:
		if m.APIKey == "" { // compat_oai sends the plugin's placeholder key when the per-request key is empty
			return ChatModel{}, errors.ErrLLMProviderRejected.WithDetails(map[string]any{detailReason: reasonMissingKey})
		}
		g := r.instance(cacheKey(string(m.ProviderKind), base), func() *genkit.Genkit {
			return genkit.Init(context.WithoutCancel(ctx), genkit.WithPlugins(&openrouter.OpenRouter{
				APIKey: pluginPlaceholderKey, Opts: r.options(base),
			}))
		})
		config := openrouter.ChatConfig{RequestConfig: compat_oai.RequestConfig{APIKey: m.APIKey}}
		return ChatModel{G: g, Name: openRouterProvider + "/" + m.ModelRef, Config: config}, nil
	case llmmodel.KindOpenAI, llmmodel.KindOpenAICompatible, llmmodel.KindOllama:
		g := r.instance(cacheKey(string(m.ProviderKind), base, keyScope(m)), func() *genkit.Genkit {
			return genkit.Init(context.WithoutCancel(ctx), genkit.WithPlugins(&compat_oai.OpenAICompatible{
				Provider: compatProvider, APIKey: cmp.Or(m.APIKey, pluginPlaceholderKey), BaseURL: base,
				Opts: r.options(""),
			}))
		})
		return ChatModel{G: g, Name: compatProvider + "/" + m.ModelRef, Config: openai.ChatCompletionNewParams{}}, nil
	default:
		return ChatModel{}, errors.ErrLLMProviderRejected.WithDetails(map[string]any{
			detailReason: reasonKindUnsupported, detailKind: string(m.ProviderKind),
		})
	}
}

func (r *Registry) Decision(ctx context.Context, m llmmodel.ResolvedModel) (DecisionModel, error) {
	if m.ProviderKind != llmmodel.KindOpenRouter {
		return DecisionModel{}, errors.ErrLLMProviderRejected.WithDetails(map[string]any{
			detailReason: reasonKindUnsupported, detailKind: string(m.ProviderKind),
		})
	}
	if m.APIKey == "" { // systemonex would fall back to OPENROUTER_API_KEY from the environment
		return DecisionModel{}, errors.ErrLLMProviderRejected.WithDetails(map[string]any{detailReason: reasonMissingKey})
	}
	root := strings.TrimSuffix(strings.TrimSuffix(baseURL(m), "/"), openRouterAPIPath)
	g := r.instance(cacheKey(decisionProvider, root, keyScope(m)), func() *genkit.Genkit {
		plugin := systemonex.OpenRouter()
		plugin.APIKey = m.APIKey
		plugin.BaseURL = root
		plugin.HTTPClient = r.cfg.HTTPClient
		return genkit.Init(context.WithoutCancel(ctx), genkit.WithPlugins(plugin))
	})
	return DecisionModel{G: g, Name: decisionProvider + "/" + m.ModelRef}, nil
}

func (r *Registry) options(base string) []option.RequestOption {
	opts := []option.RequestOption{option.WithMaxRetries(r.cfg.MaxRetries)}
	if base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	if r.cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(r.cfg.HTTPClient))
	}
	return opts
}

// instance returns the cached Genkit for key, building it with build on a miss, and evicts the least recently used.
func (r *Registry) instance(key string, build func() *genkit.Genkit) *genkit.Genkit {
	r.mu.Lock()
	defer r.mu.Unlock()
	if element, ok := r.items[key]; ok {
		r.order.MoveToFront(element)
		return element.Value.(*cacheEntry).g
	}
	g := build()
	r.items[key] = r.order.PushFront(&cacheEntry{key: key, g: g})
	for r.order.Len() > max(r.cfg.CacheSize, 1) {
		oldest := r.order.Back()
		r.order.Remove(oldest)
		delete(r.items, oldest.Value.(*cacheEntry).key)
	}
	return g
}

// baseURL always names the endpoint, so OPENROUTER_BASE_URL in the environment never redirects a request.
func baseURL(m llmmodel.ResolvedModel) string {
	return cmp.Or(m.BaseURL, llmmodel.DefaultBaseURL(m.ProviderKind))
}

// keyScope changes when the stored key changes, so a rotated key gets a new instance without a restart.
func keyScope(m llmmodel.ResolvedModel) string {
	return m.ProviderID.String() + keyVersionSeparator + m.KeyVersion
}

func cacheKey(parts ...string) string { return strings.Join(parts, cacheKeySeparator) }
