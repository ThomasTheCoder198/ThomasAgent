package audit

import (
	"context"
	"testing"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"

	"github.com/stretchr/testify/require"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

func TestRecordStoresEvent(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(context.Background(), tenant.PlatformID)
	require.NoError(t, Record(ctx, pool, Event{Actor: "owner", Action: "provider.create", TargetType: "provider", TargetID: "p1", Metadata: map[string]any{"kind": "openrouter"}}))
	var action string
	require.NoError(t, pool.QueryRow(ctx, "SELECT action FROM audit_events").Scan(&action))
	require.Equal(t, "provider.create", action)
}

func TestRecordStoresRequestIDAndPlatformScope(t *testing.T) {
	pool := pgtest.Start(t)
	const origin = "req_3f2b8c1e-9d4a-4b7e-8f21-0c6d5e4a3b21"
	ctx := logging.ContextWithRequestID(tenant.WithID(context.Background(), tenant.PlatformID), origin)
	require.NoError(t, Record(ctx, pool, Event{Actor: "owner", Action: "auth.login", TargetType: "user", TargetID: "u1", Metadata: map[string]any{"kind": "login"}}))
	var storedRequestID, tenant, kind string
	require.NoError(t, pool.QueryRow(ctx, "SELECT request_id, tenant_id, metadata->>'kind' FROM audit_events").Scan(&storedRequestID, &tenant, &kind))
	require.Equal(t, origin, storedRequestID)
	require.Equal(t, "default", tenant)
	require.Equal(t, "login", kind)
}

func TestRecordUsesEmptyObjectForNilMetadata(t *testing.T) {
	pool := pgtest.Start(t)
	require.NoError(t, Record(tenant.WithID(context.Background(), tenant.PlatformID), pool, Event{Actor: "owner", Action: "auth.login.failed", TargetType: "auth"}))
	var metadata string
	require.NoError(t, pool.QueryRow(tenant.WithID(context.Background(), tenant.PlatformID), `SELECT metadata::text FROM audit_events`).Scan(&metadata))
	require.Equal(t, "{}", metadata)
}

func TestRecordUsesTrustedTenantContext(t *testing.T) {
	pool := pgtest.Start(t)
	ctx := tenant.WithID(t.Context(), "other")
	require.NoError(t, Record(ctx, pool, Event{Actor: "owner", Action: "auth.login", TargetType: "user"}))
	var scope string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tenant_id FROM audit_events`).Scan(&scope))
	require.Equal(t, "other", scope)
}

func TestRecordRejectsMissingTenantAndIgnoresEventTenant(t *testing.T) {
	pool := pgtest.Start(t)
	require.ErrorIs(t, Record(t.Context(), pool, Event{TenantID: "default", Actor: "owner", Action: "auth.login", TargetType: "user"}), errors.ErrMissingTenant)
	ctx := tenant.WithID(t.Context(), "other")
	require.NoError(t, Record(ctx, pool, Event{TenantID: "default", Actor: "owner", Action: "auth.login", TargetType: "user"}))
	var scope string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tenant_id FROM audit_events`).Scan(&scope))
	require.Equal(t, "other", scope)
}
