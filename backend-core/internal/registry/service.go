package registry

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
	pool    *pgxpool.Pool
	secrets *vault.Store
	catalog Catalog
}

func NewService(pool *pgxpool.Pool, secrets *vault.Store, catalog Catalog) *Service {
	return &Service{pool: pool, secrets: secrets, catalog: catalog}
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return appError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	return nil
}

func normalizeProvider(in *ProviderInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if !validKind(in.Kind) {
		return fieldError("kind", validationInvalid)
	}
	if in.Name == "" {
		return fieldError("name", validationRequired)
	}
	if in.BaseURL == "" {
		in.BaseURL = DefaultBaseURL(in.Kind)
	}
	if in.BaseURL == "" {
		return fieldError("baseUrl", validationRequired)
	}
	return validateBaseURL(in.BaseURL)
}

func validateBaseURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fieldError("baseUrl", validationInvalid)
	}
	return nil
}

func (s *Service) CreateProvider(ctx context.Context, in ProviderInput) (Provider, error) {
	if err := normalizeProvider(&in); err != nil {
		return Provider{}, err
	}
	var out Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var secretID *uuid.UUID
		if in.APIKey != nil && *in.APIKey != "" {
			id, err := s.secrets.Put(ctx, tx, *in.APIKey)
			if err != nil {
				return err
			}
			secretID = &id
		}
		p, err := insertProvider(ctx, tx, in, secretID)
		if err != nil {
			return err
		}
		out = p
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionProviderNew, TargetType: targetProvider,
			TargetID: p.ID.String(), Metadata: map[string]any{"kind": string(p.Kind), "name": p.Name}})
	})
	return out, err
}

func (s *Service) UpdateProvider(ctx context.Context, id uuid.UUID, in ProviderInput) (Provider, error) {
	var out Provider
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := getProviderForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.Kind != "" && in.Kind != cur.Kind {
			return fieldError("kind", validationInvalid)
		}
		merged := ProviderInput{Kind: cur.Kind, Name: firstNonEmpty(in.Name, cur.Name), BaseURL: firstNonEmpty(in.BaseURL, cur.BaseURL)}
		if err := normalizeProvider(&merged); err != nil {
			return err
		}
		enabled := cur.Enabled
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		secretID, err := s.applyKey(ctx, tx, cur.secretID, in.APIKey)
		if err != nil {
			return err
		}
		p, err := updateProvider(ctx, tx, id, merged.Name, merged.BaseURL, enabled, secretID)
		if err != nil {
			return err
		}
		out = p
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionProviderEdit, TargetType: targetProvider,
			TargetID: id.String(), Metadata: map[string]any{"keyChanged": in.APIKey != nil}})
	})
	return out, err
}

func (s *Service) applyKey(ctx context.Context, tx pgx.Tx, current *uuid.UUID, next *string) (*uuid.UUID, error) {
	switch {
	case next == nil:
		return current, nil
	case *next == "" && current != nil:
		err := s.secrets.Delete(ctx, tx, *current)
		return nil, appError(err)
	case *next == "":
		return nil, nil
	case current != nil:
		err := s.secrets.Replace(ctx, tx, *current, *next)
		return current, appError(err)
	default:
		id, err := s.secrets.Put(ctx, tx, *next)
		return &id, appError(err)
	}
}

func (s *Service) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		secretID, err := deleteProvider(ctx, tx, id)
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

func (s *Service) ListProviders(ctx context.Context) ([]Provider, error) {
	out, err := listProviders(ctx, s.pool)
	return out, appError(err)
}

func (s *Service) TestProvider(ctx context.Context, id uuid.UUID) (int, error) {
	p, key, err := s.providerWithKey(ctx, id)
	if err != nil {
		return 0, err
	}
	models, err := s.catalog.ListModels(ctx, p.Kind, p.BaseURL, key)
	if err != nil {
		return 0, err
	}
	return len(models), nil
}

func (s *Service) providerWithKey(ctx context.Context, id uuid.UUID) (providerRow, string, error) {
	p, err := getProvider(ctx, s.pool, id)
	if err != nil {
		return providerRow{}, "", appError(err)
	}
	if p.secretID == nil {
		return p, "", nil
	}
	key, err := s.secrets.Get(ctx, s.pool, *p.secretID)
	return p, key, appError(err)
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

type SyncResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
}

func validateModel(in *ModelInput) error {
	if err := validateModelNumbers(in); err != nil {
		return err
	}
	in.ModelRef = strings.TrimSpace(in.ModelRef)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.ModelRef == "" {
		return fieldError("modelRef", validationRequired)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.ModelRef
	}
	for _, c := range in.Capabilities {
		if !validCapability(c) {
			return fieldError("capabilities", validationInvalid)
		}
		if c == CapEmbedding && (in.EmbeddingDims == nil || *in.EmbeddingDims <= 0) {
			return fieldError("embeddingDims", validationRequired)
		}
	}
	return nil
}

func validateModelNumbers(in *ModelInput) error {
	for field, value := range map[string]*int{"contextWindow": in.ContextWindow, "embeddingDims": in.EmbeddingDims} {
		if value != nil && *value <= 0 {
			return fieldError(field, validationInvalid)
		}
	}
	for field, value := range map[string]*float64{"inputPricePerMTok": in.InputPricePerMTok, "outputPricePerMTok": in.OutputPricePerMTok} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fieldError(field, validationInvalid)
		}
	}
	return nil
}

func (s *Service) CreateModel(ctx context.Context, in ModelInput) (Model, error) {
	if err := validateModel(&in); err != nil {
		return Model{}, err
	}
	var out Model
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		m, _, err := upsertModel(ctx, tx, in, sourceManual)
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
		if err := deleteModel(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionModelDrop, TargetType: targetModel, TargetID: id.String()})
	})
}

func (s *Service) ListModels(ctx context.Context, providerID *uuid.UUID, capability *Capability) ([]Model, error) {
	out, err := listModels(ctx, s.pool, providerID, capability)
	return out, appError(err)
}

func (s *Service) SyncModels(ctx context.Context, providerID uuid.UUID) (SyncResult, error) {
	p, key, err := s.providerWithKey(ctx, providerID)
	if err != nil {
		return SyncResult{}, err
	}
	if p.Kind != KindOpenRouter {
		return SyncResult{}, errors.New(errors.CodeRegistrySyncUnsupported)
	}
	remote, err := s.catalog.ListModels(ctx, p.Kind, p.BaseURL, key)
	if err != nil {
		return SyncResult{}, err
	}
	var res SyncResult
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		for _, r := range remote {
			in := ModelInput{ProviderID: providerID, ModelRef: r.ModelRef, DisplayName: r.DisplayName, Capabilities: r.Capabilities,
				ContextWindow: r.ContextWindow, InputPricePerMTok: r.InputPricePerMTok, OutputPricePerMTok: r.OutputPricePerMTok}
			if err := validateModel(&in); err != nil {
				return err
			}
			_, inserted, err := upsertModel(ctx, tx, in, sourceSync)
			if err != nil {
				return err
			}
			if inserted {
				res.Added++
			} else {
				res.Updated++
			}
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionModelSync, TargetType: targetProvider,
			TargetID: providerID.String(), Metadata: map[string]any{"added": res.Added, "updated": res.Updated}})
	})
	return res, err
}

func satisfies(m Model, role Role) bool {
	have := make(map[Capability]bool, len(m.Capabilities))
	for _, c := range m.Capabilities {
		have[c] = true
	}
	for _, set := range requirementsForRole(role) {
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

func (s *Service) AssignRole(ctx context.Context, role Role, modelID uuid.UUID) error {
	if requirementsForRole(role) == nil {
		return fieldError("role", validationInvalid)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		m, err := getModelForShare(ctx, tx, modelID)
		if err != nil {
			return err
		}
		if !satisfies(m, role) {
			return errCapabilityMismatch(role, requirementsForRole(role))
		}
		if err := upsertRole(ctx, tx, role, modelID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionRoleSet, TargetType: targetRole,
			TargetID: string(role), Metadata: map[string]any{"modelId": modelID.String()}})
	})
}

func (s *Service) ListRoles(ctx context.Context) ([]RoleAssignment, error) {
	out, err := listRoles(ctx, s.pool)
	return out, appError(err)
}

func (s *Service) Resolve(ctx context.Context, role Role) (ResolvedModel, error) {
	if requirementsForRole(role) == nil {
		return ResolvedModel{}, fieldError("role", validationInvalid)
	}
	effective, fallbackFrom, id, err := s.resolveAssignment(ctx, role)
	if err != nil {
		return ResolvedModel{}, appError(err)
	}
	m, err := getModel(ctx, s.pool, id)
	if err != nil {
		return ResolvedModel{}, appError(err)
	}
	p, key, err := s.providerWithKey(ctx, m.ProviderID)
	if err != nil {
		return ResolvedModel{}, appError(err)
	}
	if !p.Enabled {
		return ResolvedModel{}, errors.ErrProviderUnavailable
	}
	if !satisfies(m, effective) {
		return ResolvedModel{}, errCapabilityMismatch(effective, requirementsForRole(effective))
	}
	return ResolvedModel{Role: effective, FallbackFrom: fallbackFrom, ProviderKind: p.Kind, BaseURL: p.BaseURL, APIKey: key,
		ModelRef: m.ModelRef, Capabilities: m.Capabilities, EmbeddingDims: m.EmbeddingDims}, nil
}

func (s *Service) resolveAssignment(ctx context.Context, role Role) (Role, Role, uuid.UUID, error) {
	effective, fallbackFrom := role, Role("")
	id, ok, err := roleModelID(ctx, s.pool, role)
	if err != nil {
		return effective, fallbackFrom, id, err
	}
	if !ok && role == RoleDecision {
		effective, fallbackFrom = RoleChatFast, RoleDecision
		id, ok, err = roleModelID(ctx, s.pool, RoleChatFast)
		if err != nil {
			return effective, fallbackFrom, id, err
		}
	}
	if !ok {
		return effective, fallbackFrom, id, errRoleNotAssigned(role)
	}
	return effective, fallbackFrom, id, nil
}
