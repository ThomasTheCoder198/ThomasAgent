package model

import (
	"time"

	"github.com/google/uuid"
)

type ProviderKind string

const (
	KindOpenRouter       ProviderKind = "openrouter"
	KindOpenAI           ProviderKind = "openai"
	KindAnthropic        ProviderKind = "anthropic"
	KindGoogle           ProviderKind = "google"
	KindAzure            ProviderKind = "azure"
	KindOllama           ProviderKind = "ollama"
	KindOpenAICompatible ProviderKind = "openai_compatible"
	KindCohere           ProviderKind = "cohere"
	KindVoyage           ProviderKind = "voyage"
	KindJina             ProviderKind = "jina"
	KindTypeSafe         ProviderKind = "typesafe"
)

// DefaultBaseURL returns "" for kinds the owner must give a base URL for.
func DefaultBaseURL(kind ProviderKind) string {
	switch kind {
	case KindOpenRouter:
		return "https://openrouter.ai/api/v1"
	case KindOpenAI:
		return "https://api.openai.com/v1"
	case KindAnthropic:
		return "https://api.anthropic.com/v1"
	case KindGoogle:
		return "https://generativelanguage.googleapis.com/v1beta"
	case KindCohere:
		return "https://api.cohere.com/v2"
	case KindVoyage:
		return "https://api.voyageai.com/v1"
	case KindJina:
		return "https://api.jina.ai/v1"
	case KindTypeSafe:
		return "https://api.typesafe.ai/v1"
	default:
		return ""
	}
}

func IsValidProviderKind(kind ProviderKind) bool {
	switch kind {
	case KindOpenRouter, KindOpenAI, KindAnthropic, KindGoogle, KindAzure, KindOllama, KindOpenAICompatible, KindCohere, KindVoyage, KindJina, KindTypeSafe:
		return true
	}
	return false
}

// Provider never carries the API key; HasAPIKey only says one is stored in the vault.
type Provider struct {
	ID        uuid.UUID
	Kind      ProviderKind
	Name      string
	BaseURL   string
	HasAPIKey bool
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProviderInput uses pointers where "absent" and "empty" mean different things on update:
// a nil APIKey keeps the stored key, an empty one deletes it.
type ProviderInput struct {
	Kind    ProviderKind
	Name    string
	BaseURL string
	APIKey  *string
	Enabled *bool
}

// StoredProvider is a provider row with its vault reference; it never leaves the service layer.
type StoredProvider struct {
	Provider
	SecretID *uuid.UUID
}

// ProviderAccess is a provider with its decrypted key and the key's version; it never leaves the service layer.
type ProviderAccess struct {
	StoredProvider
	APIKey     string
	KeyVersion string
}
