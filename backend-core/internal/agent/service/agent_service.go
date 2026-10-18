package service

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/agent/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	llmmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
)

// KnowledgeBaseLookup is the part of ragx.Caller the agent service needs (ragx spec §14.5).
type KnowledgeBaseLookup interface {
	GetKnowledgeBase(ctx context.Context, kbID uuid.UUID) (ragx.KnowledgeBase, error)
}

// ModelResolver checks an override against a platform model; llmprovider's ResolveService implements it.
type ModelResolver interface {
	ResolveModel(ctx context.Context, modelID uuid.UUID) (llmmodel.ResolvedModel, error)
}

type AgentService interface {
	List(ctx context.Context) ([]model.Agent, error)
	Get(ctx context.Context, id uuid.UUID) (model.Agent, error)
	Create(ctx context.Context, in model.AgentInput) (model.Agent, error)
	Update(ctx context.Context, id uuid.UUID, patch model.AgentPatch) (model.Agent, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AgentServiceImpl struct {
	pool   *pgxpool.Pool
	agents repository.AgentRepo
	kbs    KnowledgeBaseLookup
	models ModelResolver
	log    *slog.Logger
}

func NewAgentService(pool *pgxpool.Pool, agents repository.AgentRepo, kbs KnowledgeBaseLookup, models ModelResolver, log *slog.Logger) AgentService {
	return &AgentServiceImpl{pool: pool, agents: agents, kbs: kbs, models: models, log: log}
}

func (s *AgentServiceImpl) List(ctx context.Context) ([]model.Agent, error) {
	agents, err := s.agents.ListAgents(ctx, s.pool)
	if err != nil {
		return nil, errors.Normalize(err)
	}
	names := map[uuid.UUID]kbView{}
	for i := range agents {
		view, seen := names[agents[i].KBID]
		if !seen {
			view = s.lookupKB(ctx, agents[i].KBID)
			names[agents[i].KBID] = view
		}
		agents[i].KBName, agents[i].KBMissing = view.name, view.missing
	}
	return agents, nil
}

func (s *AgentServiceImpl) Get(ctx context.Context, id uuid.UUID) (model.Agent, error) {
	agent, err := s.agents.GetAgent(ctx, s.pool, id)
	if err != nil {
		return model.Agent{}, errors.Normalize(err)
	}
	return s.withKB(ctx, agent), nil
}

func (s *AgentServiceImpl) Create(ctx context.Context, in model.AgentInput) (model.Agent, error) {
	if err := model.NormalizeInput(&in); err != nil {
		return model.Agent{}, err
	}
	kbName, err := s.requireKB(ctx, in.KBID)
	if err != nil {
		return model.Agent{}, err
	}
	if err := s.checkOverrides(ctx, in.ModelOverrides); err != nil {
		return model.Agent{}, err
	}
	var created model.Agent
	err = postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		created, err = s.agents.InsertAgent(ctx, tx, model.Agent{ID: uuid.New(), Name: in.Name, Description: in.Description,
			Instructions: in.Instructions, KBID: in.KBID, ModelOverrides: in.ModelOverrides})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionCreate, TargetType: auditTargetAgent, TargetID: created.ID.String()})
	})
	created.KBName = kbName
	return created, err
}

func (s *AgentServiceImpl) Update(ctx context.Context, id uuid.UUID, patch model.AgentPatch) (model.Agent, error) {
	// The KB and model checks call outside services, so they run before the row lock (ruling A2-9).
	var kbName string
	if patch.KBID != nil {
		if *patch.KBID == uuid.Nil {
			return model.Agent{}, errors.FieldError(model.FieldKBID, errors.FieldCodeRequired)
		}
		name, err := s.requireKB(ctx, *patch.KBID)
		if err != nil {
			return model.Agent{}, err
		}
		kbName = name
	}
	if err := s.checkPatchOverrides(ctx, patch.ModelOverrides); err != nil {
		return model.Agent{}, err
	}
	var updated model.Agent
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := s.agents.GetAgentForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		in := current.Apply(patch)
		if err := model.NormalizeInput(&in); err != nil {
			return err
		}
		updated, err = s.agents.UpdateAgent(ctx, tx, model.Agent{ID: id, Name: in.Name, Description: in.Description,
			Instructions: in.Instructions, KBID: in.KBID, ModelOverrides: in.ModelOverrides})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionUpdate, TargetType: auditTargetAgent, TargetID: id.String()})
	})
	if err != nil {
		return model.Agent{}, err
	}
	if patch.KBID != nil {
		updated.KBName = kbName
		return updated, nil
	}
	return s.withKB(ctx, updated), nil
}

func (s *AgentServiceImpl) checkPatchOverrides(ctx context.Context, overrides map[llmmodel.Role]*uuid.UUID) error {
	for _, role := range slices.Sorted(maps.Keys(overrides)) { // a stable first error, like checkOverrides
		modelID := overrides[role]
		if modelID == nil {
			if !model.IsOverridable(role) {
				return overrideInvalid(role)
			}
			continue
		}
		if err := s.checkOverrides(ctx, map[llmmodel.Role]uuid.UUID{role: *modelID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *AgentServiceImpl) Delete(ctx context.Context, id uuid.UUID) error {
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.agents.DeleteAgent(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionDelete, TargetType: auditTargetAgent, TargetID: id.String()})
	})
}

type kbView struct {
	name    string
	missing bool
}

// lookupKB never fails a read (ruling A2-9). Only KB_NOT_FOUND marks the KB missing; an outage leaves the name
// empty and the KB present, so the chat runtime keeps its KB tools and reports the outage per tool call.
func (s *AgentServiceImpl) lookupKB(ctx context.Context, kbID uuid.UUID) kbView {
	kb, err := s.kbs.GetKnowledgeBase(ctx, kbID)
	if err == nil {
		return kbView{name: kb.Name}
	}
	if errors.CodeOf(err) == errors.CodeKbNotFound {
		return kbView{missing: true}
	}
	s.log.WarnContext(ctx, logKBLookupFailed, slog.String(fieldKBID, kbID.String()), slog.String(logging.FieldErrorCode, string(errors.CodeOf(err))))
	return kbView{}
}

func (s *AgentServiceImpl) withKB(ctx context.Context, agent model.Agent) model.Agent {
	view := s.lookupKB(ctx, agent.KBID)
	agent.KBName, agent.KBMissing = view.name, view.missing
	return agent
}

// requireKB answers AGENT_KB_INVALID for a KB ragx does not know in this org; other ragx failures pass through.
func (s *AgentServiceImpl) requireKB(ctx context.Context, kbID uuid.UUID) (string, error) {
	kb, err := s.kbs.GetKnowledgeBase(ctx, kbID)
	if err != nil {
		if errors.CodeOf(err) == errors.CodeKbNotFound {
			return "", errors.ErrAgentKbInvalid
		}
		return "", err
	}
	return kb.Name, nil
}

// checkOverrides accepts only overridable roles pinned to an enabled platform model that fits the role (A2-10).
func (s *AgentServiceImpl) checkOverrides(ctx context.Context, overrides map[llmmodel.Role]uuid.UUID) error {
	for _, role := range slices.Sorted(maps.Keys(overrides)) { // a stable first error for the same input
		if !model.IsOverridable(role) {
			return overrideInvalid(role)
		}
		resolved, err := s.models.ResolveModel(ctx, overrides[role])
		if err != nil {
			if code := errors.CodeOf(err); code == errors.CodeLLMModelNotFound || code == errors.CodeProviderUnavailable {
				return overrideInvalid(role)
			}
			return err
		}
		if !(llmmodel.LLMModel{Capabilities: resolved.Capabilities}).FitsRole(role) {
			return overrideInvalid(role)
		}
	}
	return nil
}

func overrideInvalid(role llmmodel.Role) error {
	return errors.FieldError(model.FieldModelOverrides+overrideFieldSeparator+string(role), errors.FieldCodeInvalid)
}
