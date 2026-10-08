package llm

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
)

const (
	auditTargetRole       = "model_role"
	auditActionRoleAssign = "model_role.assign"
)

func (s *Service) AssignRole(ctx context.Context, role Role, modelID uuid.UUID) error {
	if RequirementsForRole(role) == nil {
		return errors.FieldError("role", fieldInvalid)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		model, err := s.repository.GetModelForShare(ctx, tx, modelID)
		if err != nil {
			return err
		}
		if !modelFitsRole(model, role) {
			return capabilityMismatch(role)
		}
		if err := s.repository.UpsertRole(ctx, tx, role, modelID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: auditActionRoleAssign, TargetType: auditTargetRole,
			TargetID: string(role), Metadata: map[string]any{"modelId": modelID.String()}})
	})
}

func (s *Service) ListRoles(ctx context.Context) ([]RoleAssignment, error) {
	assignments, err := s.repository.ListRoles(ctx, s.pool)
	return assignments, errors.Normalize(err)
}

// Resolve returns the model behind a role with its decrypted key, for internal callers only.
func (s *Service) Resolve(ctx context.Context, role Role) (ResolvedModel, error) {
	if RequirementsForRole(role) == nil {
		return ResolvedModel{}, errors.FieldError("role", fieldInvalid)
	}
	effectiveRole, fallbackFrom, modelID, err := s.resolveAssignment(ctx, role)
	if err != nil {
		return ResolvedModel{}, errors.Normalize(err)
	}
	model, err := s.repository.GetModel(ctx, s.pool, modelID)
	if err != nil {
		return ResolvedModel{}, errors.Normalize(err)
	}
	provider, apiKey, err := s.providerWithKey(ctx, model.ProviderID)
	if err != nil {
		return ResolvedModel{}, errors.Normalize(err)
	}
	if !provider.Enabled {
		return ResolvedModel{}, errors.ErrProviderUnavailable
	}
	if !modelFitsRole(model, effectiveRole) {
		return ResolvedModel{}, capabilityMismatch(effectiveRole)
	}
	return ResolvedModel{Role: effectiveRole, FallbackFrom: fallbackFrom, ProviderKind: provider.Kind, BaseURL: provider.BaseURL, APIKey: apiKey,
		ModelRef: model.ModelRef, Capabilities: model.Capabilities, EmbeddingDims: model.EmbeddingDims}, nil
}

// resolveAssignment falls back from an unassigned decision role to chat.fast.
func (s *Service) resolveAssignment(ctx context.Context, role Role) (effectiveRole, fallbackFrom Role, modelID uuid.UUID, err error) {
	effectiveRole = role
	modelID, assigned, err := s.repository.RoleModelID(ctx, s.pool, role)
	if err != nil {
		return effectiveRole, fallbackFrom, modelID, err
	}
	if !assigned && role == RoleDecision {
		effectiveRole, fallbackFrom = RoleChatFast, RoleDecision
		modelID, assigned, err = s.repository.RoleModelID(ctx, s.pool, RoleChatFast)
		if err != nil {
			return effectiveRole, fallbackFrom, modelID, err
		}
	}
	if !assigned {
		return effectiveRole, fallbackFrom, modelID, errors.ErrLLMRoleNotAssigned.WithDetails(map[string]any{"role": string(role)})
	}
	return effectiveRole, fallbackFrom, modelID, nil
}

func capabilityMismatch(role Role) error {
	return errors.ErrLLMModelCapabilityMismatch.WithDetails(map[string]any{"role": string(role), "requires": RequirementsForRole(role)})
}
