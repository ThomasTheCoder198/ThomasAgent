package service

import (
	"context"
	"math"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/vault"
)

const (
	actorOwner         = "owner"
	targetProvider     = "provider"
	actionProviderNew  = "provider.create"
	actionProviderEdit = "provider.update"
	actionProviderDrop = "provider.delete"
	validationRequired = errors.FieldCodeRequired
	validationInvalid  = errors.FieldCodeInvalid
)

type Service struct {
	pool            *pgxpool.Pool
	secrets         *vault.Store
	catalog         Catalog
	maxRemoteModels int
	repo            repository.LLMProviderRepository
}

var _ LLMProviderService = (*Service)(nil)
var _ Resolver = (*Service)(nil)

func NewLLMProviderService(pool *pgxpool.Pool, secrets *vault.Store, catalog Catalog, maxRemoteModels int) *Service {
	return NewLLMProviderServiceWithRepository(pool, secrets, catalog, maxRemoteModels, repository.NewLLMProviderRepository())
}

func NewLLMProviderServiceWithRepository(pool *pgxpool.Pool, secrets *vault.Store, catalog Catalog, maxRemoteModels int, repo repository.LLMProviderRepository) *Service {
	return &Service{pool: pool, secrets: secrets, catalog: catalog, maxRemoteModels: maxRemoteModels, repo: repo}
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return errors.Normalize(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}

func normalizeProvider(in *model.ProviderInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if !model.ValidKind(in.Kind) {
		return errors.FieldError("kind", validationInvalid)
	}
	if in.Name == "" {
		return errors.FieldError("name", validationRequired)
	}
	if in.BaseURL == "" {
		in.BaseURL = model.DefaultBaseURL(in.Kind)
	}
	if in.BaseURL == "" {
		return errors.FieldError("baseUrl", validationRequired)
	}
	return validateBaseURL(in.BaseURL)
}

func validateBaseURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || !outbound.IsSupportedScheme(u.Scheme) {
		return errors.FieldError("baseUrl", validationInvalid)
	}
	return nil
}

func (s *Service) CreateProvider(ctx context.Context, in model.ProviderInput) (model.Provider, error) {
	if err := normalizeProvider(&in); err != nil {
		return model.Provider{}, err
	}
	var out model.Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var secretID *uuid.UUID
		if in.APIKey != nil && *in.APIKey != "" {
			id, err := s.secrets.Put(ctx, tx, *in.APIKey)
			if err != nil {
				return err
			}
			secretID = &id
		}
		p, err := s.repo.InsertProvider(ctx, tx, in, secretID)
		if err != nil {
			return err
		}
		out = p
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionProviderNew, TargetType: targetProvider,
			TargetID: p.ID.String(), Metadata: map[string]any{"kind": string(p.Kind), "name": p.Name}})
	})
	return out, err
}

func (s *Service) UpdateProvider(ctx context.Context, id uuid.UUID, in model.ProviderInput) (model.Provider, error) {
	var out model.Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := s.repo.GetProviderForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.Kind != "" && in.Kind != cur.Kind {
			return errors.FieldError("kind", validationInvalid)
		}
		merged := model.ProviderInput{Kind: cur.Kind, Name: firstNonEmpty(in.Name, cur.Name), BaseURL: firstNonEmpty(in.BaseURL, cur.BaseURL)}
		if err := normalizeProvider(&merged); err != nil {
			return err
		}
		enabled := cur.Enabled
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		secretID, err := s.providerUpdateSecret(ctx, tx, cur, merged.BaseURL, in.APIKey)
		if err != nil {
			return err
		}
		p, err := s.repo.UpdateProvider(ctx, tx, id, merged.Name, merged.BaseURL, enabled, secretID)
		if err != nil {
			return err
		}
		out = p
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionProviderEdit, TargetType: targetProvider,
			TargetID: id.String(), Metadata: map[string]any{"keyChanged": in.APIKey != nil}})
	})
	return out, err
}

func (s *Service) providerUpdateSecret(ctx context.Context, tx pgx.Tx, current repository.ProviderRow, nextBaseURL string, nextKey *string) (*uuid.UUID, error) {
	if nextKey == nil && nextBaseURL != current.BaseURL && current.SecretID != nil {
		if err := s.secrets.Delete(ctx, tx, *current.SecretID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return s.applyKey(ctx, tx, current.SecretID, nextKey)
}

func (s *Service) applyKey(ctx context.Context, tx pgx.Tx, current *uuid.UUID, next *string) (*uuid.UUID, error) {
	switch {
	case next == nil:
		return current, nil
	case *next == "" && current != nil:
		err := s.secrets.Delete(ctx, tx, *current)
		return nil, errors.Normalize(err)
	case *next == "":
		return nil, nil
	case current != nil:
		err := s.secrets.Replace(ctx, tx, *current, *next)
		return current, errors.Normalize(err)
	default:
		id, err := s.secrets.Put(ctx, tx, *next)
		return &id, errors.Normalize(err)
	}
}

func (s *Service) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		secretID, err := s.repo.DeleteProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		if secretID != nil {
			if err := s.secrets.Delete(ctx, tx, *secretID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionProviderDrop, TargetType: targetProvider, TargetID: id.String()})
	})
}

func (s *Service) ListProviders(ctx context.Context) ([]model.Provider, error) {
	out, err := s.repo.ListProviders(ctx, s.pool)
	return out, errors.Normalize(err)
}

func (s *Service) TestProvider(ctx context.Context, id uuid.UUID) (int, error) {
	p, key, err := s.providerWithKey(ctx, id)
	if err != nil {
		return 0, err
	}
	models, err := s.catalog.ListModels(ctx, p.ID, p.Kind, p.BaseURL, key)
	if err != nil {
		return 0, err
	}
	return len(models), nil
}

func (s *Service) providerWithKey(ctx context.Context, id uuid.UUID) (repository.ProviderRow, string, error) {
	p, err := s.repo.GetProvider(ctx, s.pool, id)
	if err != nil {
		return repository.ProviderRow{}, "", errors.Normalize(err)
	}
	if p.SecretID == nil {
		return p, "", nil
	}
	key, err := s.secrets.Get(ctx, s.pool, *p.SecretID)
	return p, key, errors.Normalize(err)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

const (
	sourceManual    = "manual"
	sourceSync      = "sync"
	targetModel     = "model"
	targetRole      = "model_role"
	actionModelNew  = "model.create"
	actionModelDrop = "model.delete"
	actionModelSync = "model.sync"
	actionRoleSet   = "model_role.assign"
)

func validateModel(in *model.ModelInput) error {
	if err := validateModelNumbers(in); err != nil {
		return err
	}
	in.ModelRef = strings.TrimSpace(in.ModelRef)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.ModelRef == "" {
		return errors.FieldError("modelRef", validationRequired)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.ModelRef
	}
	for _, c := range in.Capabilities {
		if !model.ValidCapability(c) {
			return errors.FieldError("capabilities", validationInvalid)
		}
		if c == model.CapEmbedding && (in.EmbeddingDims == nil || *in.EmbeddingDims <= 0) {
			return errors.FieldError("embeddingDims", validationRequired)
		}
	}
	return nil
}

func validateModelNumbers(in *model.ModelInput) error {
	for field, value := range map[string]*int{"contextWindow": in.ContextWindow, "embeddingDims": in.EmbeddingDims} {
		if value != nil && *value <= 0 {
			return errors.FieldError(field, validationInvalid)
		}
	}
	for field, value := range map[string]*float64{"inputPricePerMTok": in.InputPricePerMTok, "outputPricePerMTok": in.OutputPricePerMTok} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return errors.FieldError(field, validationInvalid)
		}
	}
	return nil
}

func (s *Service) CreateModel(ctx context.Context, in model.ModelInput) (model.Model, error) {
	if err := validateModel(&in); err != nil {
		return model.Model{}, err
	}
	var out model.Model
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		m, _, err := s.repo.UpsertModel(ctx, tx, in, sourceManual)
		if err != nil {
			return err
		}
		out = m
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionModelNew, TargetType: targetModel, TargetID: m.ID.String()})
	})
	return out, err
}

func (s *Service) DeleteModel(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.DeleteModel(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionModelDrop, TargetType: targetModel, TargetID: id.String()})
	})
}

func (s *Service) ListModels(ctx context.Context, providerID *uuid.UUID, capability *model.Capability) ([]model.Model, error) {
	out, err := s.repo.ListModels(ctx, s.pool, providerID, capability)
	return out, errors.Normalize(err)
}

func (s *Service) SyncModels(ctx context.Context, providerID uuid.UUID) (model.SyncResult, error) {
	p, key, err := s.providerWithKey(ctx, providerID)
	if err != nil {
		return model.SyncResult{}, err
	}
	if p.Kind != model.KindOpenRouter {
		return model.SyncResult{}, errors.ErrLLMProviderSyncUnsupported
	}
	remote, err := s.catalog.ListModels(ctx, p.ID, p.Kind, p.BaseURL, key)
	if err != nil {
		return model.SyncResult{}, err
	}
	var res model.SyncResult
	if s.maxRemoteModels <= 0 {
		return model.SyncResult{}, errors.ErrInternalError
	}
	snapshotComplete := len(remote) > 0 && len(remote) <= s.maxRemoteModels
	if len(remote) > s.maxRemoteModels {
		res.Skipped = len(remote) - s.maxRemoteModels
		remote = remote[:s.maxRemoteModels]
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		kept, err := s.syncRemoteModels(ctx, tx, providerID, remote, &res)
		if err != nil {
			return err
		}
		// A truncated catalog cannot establish that an omitted row is stale.
		if snapshotComplete {
			if err := s.repo.DeleteStaleSyncModels(ctx, tx, providerID, kept); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionModelSync, TargetType: targetProvider,
			TargetID: providerID.String(), Metadata: map[string]any{"added": res.Added, "updated": res.Updated, "skipped": res.Skipped}})
	})
	return res, err
}

func (s *Service) syncRemoteModels(ctx context.Context, tx pgx.Tx, providerID uuid.UUID, remote []model.RemoteModel, result *model.SyncResult) ([]uuid.UUID, error) {
	kept := make([]uuid.UUID, 0, len(remote))
	for _, remoteModel := range remote {
		input := model.ModelInput{ProviderID: providerID, ModelRef: remoteModel.ModelRef, DisplayName: remoteModel.DisplayName,
			Capabilities: remoteModel.Capabilities, ContextWindow: remoteModel.ContextWindow,
			InputPricePerMTok: remoteModel.InputPricePerMTok, OutputPricePerMTok: remoteModel.OutputPricePerMTok}
		if err := validateModel(&input); err != nil {
			return nil, errors.ErrLLMProviderRejected.WithCause(err)
		}
		model, inserted, err := s.repo.UpsertModel(ctx, tx, input, sourceSync)
		if err != nil {
			if errors.ToAppError(err).Code == errors.CodeLLMNameTaken {
				result.Skipped++
				continue
			}
			return nil, err
		}
		if inserted {
			result.Added++
		} else {
			result.Updated++
		}
		kept = append(kept, model.ID)
	}
	return kept, nil
}

func satisfies(m model.Model, role model.Role) bool {
	have := make(map[model.Capability]bool, len(m.Capabilities))
	for _, c := range m.Capabilities {
		have[c] = true
	}
	for _, set := range model.RequirementsForRole(role) {
		ok := true
		for _, need := range set {
			ok = ok && have[need]
		}
		if ok {
			return true
		}
	}
	return false
}

func (s *Service) AssignRole(ctx context.Context, role model.Role, modelID uuid.UUID) error {
	if model.RequirementsForRole(role) == nil {
		return errors.FieldError("role", validationInvalid)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		m, err := s.repo.GetModelForShare(ctx, tx, modelID)
		if err != nil {
			return err
		}
		if !satisfies(m, role) {
			return errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{"role": string(role), "requires": model.RequirementsForRole(role)})
		}
		if err := s.repo.UpsertRole(ctx, tx, role, modelID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionRoleSet, TargetType: targetRole,
			TargetID: string(role), Metadata: map[string]any{"modelId": modelID.String()}})
	})
}

func (s *Service) ListRoles(ctx context.Context) ([]model.RoleAssignment, error) {
	out, err := s.repo.ListRoles(ctx, s.pool)
	return out, errors.Normalize(err)
}

func (s *Service) Resolve(ctx context.Context, role model.Role) (model.ResolvedModel, error) {
	if model.RequirementsForRole(role) == nil {
		return model.ResolvedModel{}, errors.FieldError("role", validationInvalid)
	}
	effective, fallbackFrom, id, err := s.resolveAssignment(ctx, role)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	m, err := s.repo.GetModel(ctx, s.pool, id)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	p, key, err := s.providerWithKey(ctx, m.ProviderID)
	if err != nil {
		return model.ResolvedModel{}, errors.Normalize(err)
	}
	if !p.Enabled {
		return model.ResolvedModel{}, errors.ErrProviderUnavailable
	}
	if !satisfies(m, effective) {
		return model.ResolvedModel{}, errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{"role": string(effective), "requires": model.RequirementsForRole(effective)})
	}
	return model.ResolvedModel{Role: effective, FallbackFrom: fallbackFrom, ProviderKind: p.Kind, BaseURL: p.BaseURL, APIKey: key,
		ModelRef: m.ModelRef, Capabilities: m.Capabilities, EmbeddingDims: m.EmbeddingDims}, nil
}

func (s *Service) resolveAssignment(ctx context.Context, role model.Role) (model.Role, model.Role, uuid.UUID, error) {
	effective, fallbackFrom := role, model.Role("")
	id, ok, err := s.repo.RoleModelID(ctx, s.pool, role)
	if err != nil {
		return effective, fallbackFrom, id, err
	}
	if !ok && role == model.RoleDecision {
		effective, fallbackFrom = model.RoleChatFast, model.RoleDecision
		id, ok, err = s.repo.RoleModelID(ctx, s.pool, model.RoleChatFast)
		if err != nil {
			return effective, fallbackFrom, id, err
		}
	}
	if !ok {
		return effective, fallbackFrom, id, errors.ErrLLMRoleNotAssigned.WithDetails(map[string]any{"role": string(role)})
	}
	return effective, fallbackFrom, id, nil
}
