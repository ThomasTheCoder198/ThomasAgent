package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/ragx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type fakeProvisioner struct {
	mu      sync.Mutex
	tenants []string
	errs    []error // returned in order; when exhausted, the call succeeds
	onCall  func()
	kb      uuid.UUID
}

func (f *fakeProvisioner) ProvisionOrg(ctx context.Context, name string, externalParsing bool, settingsVersion int64) (ragx.Org, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, _ := tenant.ID(ctx)
	f.tenants = append(f.tenants, id)
	if f.onCall != nil {
		f.onCall()
	}
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return ragx.Org{}, err
	}
	return ragx.Org{OrgId: id, Name: name, ExternalParsing: externalParsing, SettingsVersion: settingsVersion, DefaultKbId: f.kb}, nil
}

func newService(t *testing.T, provisioner *fakeProvisioner, attempts int) OrganizationService {
	t.Helper()
	pool := pgtest.Start(t)
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: attempts, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	policy.Sleep = func(context.Context, time.Duration) error { return nil }
	return NewOrganizationService(pool, repository.NewOrganizationRepo(), provisioner, policy, uuid.NewString)
}

func platformRequest() context.Context {
	return requestid.NewContext(tenant.WithID(context.Background(), tenant.PlatformID), "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21")
}

func yes() *bool { v := true; return &v }

func TestOrganizationService_ProvisionsWithTheOrgAsTenant(t *testing.T) {
	provisioner := &fakeProvisioner{kb: uuid.New()}
	svc := newService(t, provisioner, 1)
	org, err := svc.CreateOrganization(platformRequest(), model.OrganizationInput{Name: " Acme ", ExternalParsing: yes()})
	require.NoError(t, err)
	require.Equal(t, []string{org.ID}, provisioner.tenants, "oid is the org row's id, never the platform tenant")
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	require.Equal(t, &provisioner.kb, org.DefaultKnowledgeBaseID)
	require.True(t, org.SettingsSynced)
	require.Equal(t, int64(1), org.SettingsVersion)
	require.Equal(t, "Acme", org.Name)
}

func TestOrganizationService_RetriesOnlyRetryableErrorsUpToTheConfiguredAttempts(t *testing.T) {
	retryable := &fakeProvisioner{errs: []error{errors.ErrRagxUnavailable, errors.ErrRagxUnavailable, errors.ErrRagxUnavailable}}
	org, err := newService(t, retryable, 3).CreateOrganization(platformRequest(), model.OrganizationInput{Name: "A", ExternalParsing: yes()})
	require.NoError(t, err)
	require.Len(t, retryable.tenants, 3)
	require.Equal(t, model.RagxStatusFailed, org.RagxStatus)
	require.Equal(t, "RAGX_UNAVAILABLE", org.RagxErrorCode)

	terminal := &fakeProvisioner{errs: []error{errors.ErrLLMRoleNotAssigned}}
	org, err = newService(t, terminal, 3).CreateOrganization(platformRequest(), model.OrganizationInput{Name: "B", ExternalParsing: yes()})
	require.NoError(t, err)
	require.Len(t, terminal.tenants, 1, "a non-retryable ragx answer is not retried")
	require.Equal(t, "LLM_ROLE_NOT_ASSIGNED", org.RagxErrorCode)
}

func TestOrganizationService_CanceledRequestStillRecordsTheProvisionResult(t *testing.T) {
	ctx, cancel := context.WithCancel(platformRequest())
	provisioner := &fakeProvisioner{kb: uuid.New(), onCall: cancel} // the browser goes away mid-call
	svc := newService(t, provisioner, 1)
	org, err := svc.CreateOrganization(ctx, model.OrganizationInput{Name: "Acme", ExternalParsing: yes()})
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, org.RagxStatus)
	stored, err := svc.GetOrganization(platformRequest(), org.ID)
	require.NoError(t, err)
	require.Equal(t, model.RagxStatusReady, stored.RagxStatus, "the outcome is recorded, not left pending")
}

func TestOrganizationService_UnchangedPatchKeepsSettingsVersion(t *testing.T) {
	provisioner := &fakeProvisioner{kb: uuid.New()}
	svc := newService(t, provisioner, 1)
	created, err := svc.CreateOrganization(platformRequest(), model.OrganizationInput{Name: "Acme", ExternalParsing: yes()})
	require.NoError(t, err)
	unchanged, err := svc.UpdateOrganization(platformRequest(), created.ID, model.OrganizationPatch{Name: ptr(" Acme ")})
	require.NoError(t, err)
	require.Equal(t, int64(1), unchanged.SettingsVersion)
	require.Len(t, provisioner.tenants, 1)
}

func TestOrganizationService_GetRejectsPlatformAndMalformedIDsBeforeTheDatabase(t *testing.T) {
	svc := newService(t, &fakeProvisioner{}, 1)
	for _, id := range []string{tenant.PlatformID, "not-a-uuid", "", uuid.NewString()} {
		_, err := svc.GetOrganization(platformRequest(), id)
		require.ErrorIs(t, err, errors.ErrNotFound, id)
	}
}

type orderedProvisioner struct {
	mu      sync.Mutex
	kb      uuid.UUID
	current ragx.Org
	started chan int64
	release map[int64]chan struct{}
}

func (f *orderedProvisioner) ProvisionOrg(ctx context.Context, name string, externalParsing bool, version int64) (ragx.Org, error) {
	if gate := f.release[version]; gate != nil {
		f.started <- version
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if version > f.current.SettingsVersion {
		f.current = ragx.Org{OrgId: f.current.OrgId, Name: name, ExternalParsing: externalParsing, SettingsVersion: version, DefaultKbId: f.kb}
	}
	return f.current, nil
}

func TestOrganizationService_ConcurrentSettingsSyncInBothCompletionOrders(t *testing.T) {
	for _, order := range [][2]int64{{2, 3}, {3, 2}} {
		t.Run(fmt.Sprintf("completion_%d_%d", order[0], order[1]), func(t *testing.T) {
			pool := pgtest.Start(t)
			provisioner := &orderedProvisioner{kb: uuid.New(), started: make(chan int64, 2), release: map[int64]chan struct{}{2: make(chan struct{}), 3: make(chan struct{})}}
			policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
			svc := NewOrganizationService(pool, repository.NewOrganizationRepo(), provisioner, policy, uuid.NewString)
			ctx := platformRequest()
			created, err := svc.CreateOrganization(ctx, model.OrganizationInput{Name: "Initial", ExternalParsing: yes()})
			require.NoError(t, err)
			results := make(chan model.Organization, 2)
			errors := make(chan error, 2)
			go func() {
				org, err := svc.UpdateOrganization(ctx, created.ID, model.OrganizationPatch{Name: ptr("A")})
				results <- org
				errors <- err
			}()
			require.Equal(t, int64(2), <-provisioner.started)
			go func() {
				org, err := svc.UpdateOrganization(ctx, created.ID, model.OrganizationPatch{Name: ptr("B")})
				results <- org
				errors <- err
			}()
			require.Equal(t, int64(3), <-provisioner.started)
			for _, version := range order {
				close(provisioner.release[version])
				<-results
				require.NoError(t, <-errors)
			}
			stored, err := svc.GetOrganization(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, "B", stored.Name)
			require.Equal(t, int64(3), stored.SettingsVersion)
			require.True(t, stored.SettingsSynced)
			provisioner.mu.Lock()
			require.Equal(t, "B", provisioner.current.Name)
			provisioner.mu.Unlock()
		})
	}
}

func ptr(value string) *string { return &value }
