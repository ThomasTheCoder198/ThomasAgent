package service

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/llmprovider/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

// LLMModelService manages models the owner adds by hand; catalog sync lives in ModelSyncService.
type LLMModelService interface {
	CreateModel(ctx context.Context, input model.LLMModelInput) (model.LLMModel, error)
	DeleteModel(ctx context.Context, id uuid.UUID) error
	UpdateCapabilities(ctx context.Context, id uuid.UUID, capabilities []model.Capability) (model.LLMModel, error)
	ListModels(ctx context.Context, providerID *uuid.UUID, capability *model.Capability) ([]model.LLMModel, error)
}

type LLMModelServiceImpl struct {
	pool   *pgxpool.Pool
	models repository.LLMModelRepo
	roles  repository.RoleRepo
}

func NewLLMModelService(pool *pgxpool.Pool, models repository.LLMModelRepo, roles repository.RoleRepo) LLMModelService {
	return &LLMModelServiceImpl{pool: pool, models: models, roles: roles}
}

func (s *LLMModelServiceImpl) CreateModel(ctx context.Context, input model.LLMModelInput) (model.LLMModel, error) {
	if err := validateLLMModel(&input); err != nil {
		return model.LLMModel{}, err
	}
	var created model.LLMModel
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		llmModel, _, err := s.models.UpsertModel(ctx, tx, input, model.ModelSourceManual)
		if err != nil {
			return err
		}
		created = llmModel
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelCreate, TargetType: auditTargetModel, TargetID: llmModel.ID.String()})
	})
	return created, err
}

func (s *LLMModelServiceImpl) DeleteModel(ctx context.Context, id uuid.UUID) error {
	return postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.models.DeleteModel(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelDelete, TargetType: auditTargetModel, TargetID: id.String()})
	})
}

func (s *LLMModelServiceImpl) UpdateCapabilities(ctx context.Context, id uuid.UUID, capabilities []model.Capability) (model.LLMModel, error) {
	capabilities, err := normalizeCapabilities(capabilities)
	if err != nil {
		return model.LLMModel{}, err
	}
	var updated model.LLMModel
	err = postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.checkCapabilityEdit(ctx, tx, id, capabilities); err != nil {
			return err
		}
		var err error
		updated, err = s.models.UpdateCapabilities(ctx, tx, id, capabilities)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionModelUpdate, TargetType: auditTargetModel,
			TargetID: id.String(), Metadata: map[string]any{"capabilities": capabilities}})
	})
	return updated, err
}

// checkCapabilityEdit locks the model row FOR UPDATE before it reads the roles. Role assignment only takes FOR SHARE, which
// conflicts with FOR UPDATE, so an assignment either committed before this read (the edit sees its role) or waits for
// this transaction and then sees the new capabilities. A share lock here would not conflict with the assignment's.
func (s *LLMModelServiceImpl) checkCapabilityEdit(ctx context.Context, tx pgx.Tx, id uuid.UUID, capabilities []model.Capability) error {
	current, err := s.models.GetModelForUpdate(ctx, tx, id)
	if err != nil {
		return err
	}
	if slices.Contains(capabilities, model.CapabilityEmbedding) && (current.EmbeddingDims == nil || *current.EmbeddingDims <= 0) {
		return errors.FieldError("embeddingDims", errors.FieldCodeRequired)
	}
	roles, err := s.roles.RolesUsingModel(ctx, tx, id)
	if err != nil {
		return err
	}
	next := model.LLMModel{Capabilities: capabilities}
	for _, role := range roles {
		if !next.FitsRole(role) {
			return capabilityMismatch(role)
		}
	}
	return nil
}

// normalizeCapabilities sorts, deduplicates and validates an operator's capability list.
func normalizeCapabilities(capabilities []model.Capability) ([]model.Capability, error) {
	capabilities = slices.Compact(slices.Sorted(slices.Values(capabilities)))
	if len(capabilities) == 0 {
		return nil, errors.FieldError("capabilities", errors.FieldCodeRequired)
	}
	for _, capability := range capabilities {
		if !model.IsValidCapability(capability) {
			return nil, errors.FieldError("capabilities", errors.FieldCodeInvalid)
		}
	}
	return capabilities, nil
}

func (s *LLMModelServiceImpl) ListModels(ctx context.Context, providerID *uuid.UUID, capability *model.Capability) ([]model.LLMModel, error) {
	llmModels, err := s.models.ListModels(ctx, s.pool, providerID, capability)
	return llmModels, errors.Normalize(err)
}

// validateLLMModel trims and checks a model before it is stored, by hand or by sync.
func validateLLMModel(input *model.LLMModelInput) error {
	if err := validateLLMModelNumbers(input); err != nil {
		return err
	}
	input.ModelRef = strings.TrimSpace(input.ModelRef)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.ModelRef == "" {
		return errors.FieldError("modelRef", errors.FieldCodeRequired)
	}
	if input.DisplayName == "" {
		input.DisplayName = input.ModelRef
	}
	for _, capability := range input.Capabilities {
		if !model.IsValidCapability(capability) {
			return errors.FieldError("capabilities", errors.FieldCodeInvalid)
		}
		if capability == model.CapabilityEmbedding && (input.EmbeddingDims == nil || *input.EmbeddingDims <= 0) {
			return errors.FieldError("embeddingDims", errors.FieldCodeRequired)
		}
	}
	return nil
}

func validateLLMModelNumbers(input *model.LLMModelInput) error {
	for field, value := range map[string]*int{"contextWindow": input.ContextWindow, "embeddingDims": input.EmbeddingDims} {
		if value != nil && *value <= 0 {
			return errors.FieldError(field, errors.FieldCodeInvalid)
		}
	}
	for field, value := range map[string]*float64{"inputPricePerMTok": input.InputPricePerMTok, "outputPricePerMTok": input.OutputPricePerMTok} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return errors.FieldError(field, errors.FieldCodeInvalid)
		}
	}
	return nil
}
