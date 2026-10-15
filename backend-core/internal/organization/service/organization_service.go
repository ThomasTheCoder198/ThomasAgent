package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// RagxProvisioner is the one ragx call this service makes (satisfied by *ragx.Caller).
type RagxProvisioner interface {
	ProvisionOrg(ctx context.Context, name string, externalParsing bool, settingsVersion int64) (ragx.Org, error)
}

type OrganizationService interface {
	ListOrganizations(ctx context.Context) ([]model.Organization, error)
	GetOrganization(ctx context.Context, id string) (model.Organization, error)
	CreateOrganization(ctx context.Context, input model.OrganizationInput) (model.Organization, error)
	UpdateOrganization(ctx context.Context, id string, patch model.OrganizationPatch) (model.Organization, error)
	ProvisionOrganization(ctx context.Context, id string) (model.Organization, error)
}

type OrganizationServiceImpl struct {
	pool   *pgxpool.Pool
	repo   repository.OrganizationRepo
	ragx   RagxProvisioner
	policy retry.Policy
	newID  func() string
}

func NewOrganizationService(pool *pgxpool.Pool, repo repository.OrganizationRepo, provisioner RagxProvisioner, policy retry.Policy, newID func() string) OrganizationService {
	return &OrganizationServiceImpl{pool: pool, repo: repo, ragx: provisioner, policy: policy, newID: newID}
}

const (
	fieldExternalParsing = "externalParsing"
	fieldBody            = "body"
)

// isOrgID refuses anything core did not generate, including the platform tenant, before touching the database.
func isOrgID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

func (s *OrganizationServiceImpl) ListOrganizations(ctx context.Context) ([]model.Organization, error) {
	return s.repo.ListOrganizations(ctx, s.pool)
}

func (s *OrganizationServiceImpl) GetOrganization(ctx context.Context, id string) (model.Organization, error) {
	if !isOrgID(id) {
		return model.Organization{}, errors.ErrNotFound
	}
	return s.repo.GetOrganization(ctx, s.pool, id)
}

func (s *OrganizationServiceImpl) CreateOrganization(ctx context.Context, input model.OrganizationInput) (model.Organization, error) {
	name, err := model.NormalizeName(input.Name)
	if err != nil {
		return model.Organization{}, err
	}
	if input.ExternalParsing == nil {
		return model.Organization{}, errors.FieldError(fieldExternalParsing, errors.FieldCodeRequired)
	}
	var created model.Organization
	err = postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		org, err := s.repo.InsertOrganization(ctx, tx, s.newID(), model.Settings{Name: name, ExternalParsing: *input.ExternalParsing})
		if err != nil {
			return err
		}
		created = org
		return s.audit(ctx, tx, auditActionCreate, org.ID)
	})
	if err != nil {
		return model.Organization{}, err
	}
	return s.provision(ctx, created) // committed first: never hold a transaction across the ragx call
}

func (s *OrganizationServiceImpl) UpdateOrganization(ctx context.Context, id string, patch model.OrganizationPatch) (model.Organization, error) {
	if patch.Name == nil && patch.ExternalParsing == nil {
		return model.Organization{}, errors.FieldError(fieldBody, errors.FieldCodeRequired)
	}
	if !isOrgID(id) {
		return model.Organization{}, errors.ErrNotFound
	}
	org, changed, err := s.savePatch(ctx, id, patch)
	if err != nil {
		return model.Organization{}, err
	}
	if !changed || org.RagxStatus != model.RagxStatusReady {
		return org, nil // ruling P2: only a changed, ready org is synced; provision sends the rest later
	}
	return s.provision(ctx, org)
}

// savePatch writes the merged settings and their audit row in one transaction. When nothing changes it writes
// nothing and returns the current row.
func (s *OrganizationServiceImpl) savePatch(ctx context.Context, id string, patch model.OrganizationPatch) (model.Organization, bool, error) {
	var saved model.Organization
	changed := false
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := s.repo.GetOrganizationForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		saved = current
		settings, err := mergeSettings(current.Settings, patch)
		if err != nil || settings == current.Settings {
			return err
		}
		if saved, err = s.repo.UpdateSettings(ctx, tx, id, settings); err != nil {
			return err
		}
		changed = true
		return s.audit(ctx, tx, auditActionUpdate, id)
	})
	return saved, changed, err
}

// mergeSettings applies the fields the patch names; the name goes through the same rule as on create.
func mergeSettings(current model.Settings, patch model.OrganizationPatch) (model.Settings, error) {
	if patch.Name != nil {
		name, err := model.NormalizeName(*patch.Name)
		if err != nil {
			return model.Settings{}, err
		}
		current.Name = name
	}
	if patch.ExternalParsing != nil {
		current.ExternalParsing = *patch.ExternalParsing
	}
	return current, nil
}

func (s *OrganizationServiceImpl) ProvisionOrganization(ctx context.Context, id string) (model.Organization, error) {
	org, err := s.GetOrganization(ctx, id)
	if err != nil {
		return model.Organization{}, err
	}
	if err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error { return s.audit(ctx, tx, auditActionProvision, org.ID) }); err != nil {
		return model.Organization{}, err
	}
	return s.provision(ctx, org)
}

// provision sends the settings the row held when the caller committed and records ragx's answer. A ragx error
// becomes the org's state, never this call's error: the row exists, so the caller always gets its real state
// (spec §14.4). Only a failed state write is an error.
func (s *OrganizationServiceImpl) provision(ctx context.Context, org model.Organization) (model.Organization, error) {
	// Ruling P4: a closed browser tab must not leave the outcome unrecorded; each attempt has its own ragx timeout.
	detached := context.WithoutCancel(ctx)
	sentVersion := org.SettingsVersion
	var answer ragx.Org
	err := retry.Do(tenant.WithID(detached, org.ID), s.policy, func(callCtx context.Context) error {
		var callErr error
		answer, callErr = s.ragx.ProvisionOrg(callCtx, org.Name, org.ExternalParsing, sentVersion)
		return callErr
	})
	if err != nil {
		return s.repo.MarkSyncFailed(detached, s.pool, org.ID, sentVersion, errors.CodeOf(err))
	}
	return s.repo.MarkSynced(detached, s.pool, org.ID, answer.SettingsVersion, answer.DefaultKbId)
}

func (s *OrganizationServiceImpl) audit(ctx context.Context, tx pgx.Tx, action, id string) error {
	return audit.Record(ctx, tx, audit.Event{Actor: auditActorOwner, Action: action, TargetType: auditTargetOrganization, TargetID: id})
}
