package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

func platform() context.Context { return tenant.WithID(context.Background(), tenant.PlatformID) }

func TestOrganizationRepo_StaleSyncResultsNeverOverwriteNewerSettings(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewOrganizationRepo()
	ctx := platform()
	id := uuid.NewString()
	sent := model.Settings{Name: "Acme", ExternalParsing: true}
	_, err := repo.InsertOrganization(ctx, pool, id, sent)
	require.NoError(t, err)
	kb := uuid.New()
	org, err := repo.MarkSynced(ctx, pool, id, sent, kb)
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.True(t, org.SettingsSynced)

	newer := model.Settings{Name: "Acme 2", ExternalParsing: false}
	_, err = repo.UpdateSettings(ctx, pool, id, newer)
	require.NoError(t, err)
	// A late failure for the old settings must not touch the row the newer edit owns.
	org, err = repo.MarkSyncFailed(ctx, pool, id, sent, errors.CodeRagxUnavailable)
	require.NoError(t, err)
	require.Equal(t, newer, org.Settings)
	require.Empty(t, org.RagxErrorCode)
	// A late success for the old settings keeps the org ready but cannot claim the newer settings are in ragx.
	org, err = repo.MarkSynced(ctx, pool, id, sent, kb)
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.False(t, org.SettingsSynced)
	// The newer settings' own failure keeps a ready org ready.
	org, err = repo.MarkSyncFailed(ctx, pool, id, newer, errors.CodeRagxUnavailable)
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.Equal(t, string(errors.CodeRagxUnavailable), org.RagxErrorCode)
	require.Equal(t, &kb, org.DefaultKnowledgeBaseID)
}

func TestOrganizationRepo_FirstFailureMarksPendingFailed(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewOrganizationRepo()
	ctx := platform()
	id := uuid.NewString()
	sent := model.Settings{Name: "Acme", ExternalParsing: true}
	_, err := repo.InsertOrganization(ctx, pool, id, sent)
	require.NoError(t, err)
	org, err := repo.MarkSyncFailed(ctx, pool, id, sent, errors.CodeLLMRoleNotAssigned)
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusFailed, org.RagxStatus)
	require.Equal(t, "LLM_ROLE_NOT_ASSIGNED", org.RagxErrorCode)
	org, err = repo.MarkSynced(ctx, pool, id, sent, uuid.New())
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.Empty(t, org.RagxErrorCode)
	require.True(t, org.SettingsSynced)
}

func TestOrganizationRepo_IsPlatformScopedAndRejectsThePlatformID(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewOrganizationRepo()
	_, err := repo.InsertOrganization(platform(), pool, tenant.PlatformID, model.Settings{Name: "x", ExternalParsing: true})
	require.Error(t, err, "the CHECK constraint refuses 'default'")
	id := uuid.NewString()
	_, err = repo.InsertOrganization(platform(), pool, id, model.Settings{Name: "Acme", ExternalParsing: true})
	require.NoError(t, err)
	_, err = repo.InsertOrganization(platform(), pool, uuid.NewString(), model.Settings{Name: "ACME", ExternalParsing: false})
	require.ErrorIs(t, err, errors.ErrConflict)
	other := tenant.WithID(context.Background(), id)
	orgs, err := repo.ListOrganizations(other, pool)
	require.NoError(t, err)
	require.Empty(t, orgs, "an org tenant never sees the platform's organization list")
	_, err = repo.GetOrganization(other, pool, id)
	require.ErrorIs(t, err, errors.ErrNotFound)
}

func TestOrganizationRepo_LateSuccessForOldSettingsClearsAStaleErrorCode(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewOrganizationRepo()
	ctx := platform()
	id := uuid.NewString()
	sent := model.Settings{Name: "Acme", ExternalParsing: true}
	_, err := repo.InsertOrganization(ctx, pool, id, sent)
	require.NoError(t, err)
	_, err = repo.MarkSyncFailed(ctx, pool, id, sent, errors.CodeLLMRoleNotAssigned)
	require.NoError(t, err)
	_, err = repo.UpdateSettings(ctx, pool, id, model.Settings{Name: "Acme 2", ExternalParsing: false})
	require.NoError(t, err)
	org, err := repo.MarkSynced(ctx, pool, id, sent, uuid.New())
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.False(t, org.SettingsSynced)
	require.Empty(t, org.RagxErrorCode, "a ready org carries no error code")
}

func TestOrganizationRepo_LateSuccessKeepsTheNewerSettingsFailureCodeOfAReadyOrg(t *testing.T) {
	pool := pgtest.Start(t)
	repo := NewOrganizationRepo()
	ctx := platform()
	id := uuid.NewString()
	s1 := model.Settings{Name: "Acme", ExternalParsing: true}
	kb := uuid.New()
	_, err := repo.InsertOrganization(ctx, pool, id, s1)
	require.NoError(t, err)
	_, err = repo.MarkSynced(ctx, pool, id, s1, kb)
	require.NoError(t, err)
	s2 := model.Settings{Name: "Acme 2", ExternalParsing: false}
	_, err = repo.UpdateSettings(ctx, pool, id, s2)
	require.NoError(t, err)
	_, err = repo.MarkSyncFailed(ctx, pool, id, s2, errors.CodeRagxUnavailable)
	require.NoError(t, err)
	org, err := repo.MarkSynced(ctx, pool, id, s1, kb) // the older call finishes last
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.False(t, org.SettingsSynced)
	require.Equal(t, string(errors.CodeRagxUnavailable), org.RagxErrorCode, "S2 is still unsynced and the code says why")
}
