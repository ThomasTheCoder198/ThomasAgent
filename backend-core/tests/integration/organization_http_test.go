package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	orgcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/controller"
	orgmodel "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/model"
	orgrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/repository"
	orgservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/organization/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/retry"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// mountOrganizationsForTest registers the organization routes exactly as cmd/core/routes.go does.
func mountOrganizationsForTest(routes gin.IRoutes, controller *orgcontroller.OrganizationController) {
	routes.GET("/api/v1/orgs", controller.HandleListOrganizations)
	routes.POST("/api/v1/orgs", controller.HandleCreateOrganization)
	routes.GET("/api/v1/orgs/:orgId", controller.HandleGetOrganization)
	routes.PATCH("/api/v1/orgs/:orgId", controller.HandleUpdateOrganization)
	routes.POST("/api/v1/orgs/:orgId/provision", controller.HandleProvisionOrganization)
}

// newOrganizationRouter serves the organization routes in the platform tenant without a session, against the
// real repository and service and the given fake ragx.
func newOrganizationRouter(t *testing.T, fake *fakeRagx) (*gin.Engine, *pgxpool.Pool, *bytes.Buffer) {
	t.Helper()
	pool := pgtest.Start(t)
	logs := &bytes.Buffer{}
	log := logger(t, logs)
	policy := retry.NewPolicy(config.RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	service := orgservice.NewOrganizationService(pool, orgrepository.NewOrganizationRepo(), fake.caller(t, logs), policy, uuid.NewString)
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(log), httpx.LogAccess(log))
	router.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(platformContext(ctx.Request.Context()))
		ctx.Next()
	})
	mountOrganizationsForTest(router, orgcontroller.NewOrganizationController(service, testMaxBodyBytes))
	return router, pool, logs
}

// dataOf decodes the "data" object of a success response.
func dataOf(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body), res.Body.String())
	return body.Data
}

func errorOf(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body), res.Body.String())
	return body.Error
}

func createOrg(t *testing.T, router http.Handler, name string) (int, map[string]any) {
	t.Helper()
	res := send(router, http.MethodPost, "/api/v1/orgs", `{"name":"`+name+`","externalParsing":true}`,
		map[string]string{"X-Request-Id": "req_org-create-0001"})
	return res.Code, dataOf(t, res)
}

func TestOrganizationHTTP_CreateProvisionsSynchronously(t *testing.T) {
	fake := newFakeRagx(t)
	router, pool, _ := newOrganizationRouter(t, fake)
	status, org := createOrg(t, router, "Acme")
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "ready", org["ragxStatus"])
	require.Equal(t, true, org["settingsSynced"])
	require.Nil(t, org["ragxErrorCode"])
	require.Equal(t, fixedDefaultKB.String(), org["defaultKnowledgeBaseId"])
	calls := fake.calls()
	require.Len(t, calls, 1)
	require.Equal(t, http.MethodPut, calls[0].method)
	require.Equal(t, "/v1/orgs/"+org["id"].(string), calls[0].path)
	require.Equal(t, org["id"], calls[0].claims["oid"])
	require.Equal(t, "org:provision", calls[0].claims["scp"])
	require.Equal(t, "req_org-create-0001", calls[0].claims["rid"])
	require.JSONEq(t, `{"name":"Acme","externalParsing":true,"settingsVersion":1}`, string(calls[0].body))
	var action string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT action FROM audit_events WHERE target_id = $1`, org["id"]).Scan(&action))
	require.Equal(t, "organization.create", action)
}

func TestOrganizationHTTP_RagxFailureLeavesAFailedOrgThatRetryRecovers(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, logs := newOrganizationRouter(t, fake)
	fake.setResponder(ragxError(http.StatusNotFound, "LLM_ROLE_NOT_ASSIGNED"))
	status, org := createOrg(t, router, "Acme")
	require.Equal(t, http.StatusCreated, status, "the row exists, so create answers 201 with its real state")
	require.Equal(t, "failed", org["ragxStatus"])
	require.Equal(t, "LLM_ROLE_NOT_ASSIGNED", org["ragxErrorCode"])
	id := org["id"].(string)

	fake.setResponder(ragxError(http.StatusInternalServerError, "INTERNAL_ERROR"))
	res := send(router, http.MethodPost, "/api/v1/orgs/"+id+"/provision", "", nil)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "RAGX_UNAVAILABLE", dataOf(t, res)["ragxErrorCode"])
	require.Contains(t, logs.String(), `"msg":"ragx call"`)

	fake.setResponder(provisionOK)
	res = send(router, http.MethodPost, "/api/v1/orgs/"+id+"/provision", "", nil)
	require.Equal(t, http.StatusOK, res.Code)
	recovered := dataOf(t, res)
	require.Equal(t, "ready", recovered["ragxStatus"])
	require.Nil(t, recovered["ragxErrorCode"])
	require.Equal(t, true, recovered["settingsSynced"])
}

func TestOrganizationHTTP_RetryRecoversAnOrgStuckPending(t *testing.T) {
	fake := newFakeRagx(t)
	router, pool, _ := newOrganizationRouter(t, fake)
	id := uuid.NewString() // the process died between commit and the ragx call
	_, err := orgrepository.NewOrganizationRepo().InsertOrganization(tenant.WithID(context.Background(), tenant.PlatformID), pool, id,
		orgmodel.Settings{Name: "Stuck", ExternalParsing: false})
	require.NoError(t, err)
	res := send(router, http.MethodPost, "/api/v1/orgs/"+id+"/provision", "", nil)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "ready", dataOf(t, res)["ragxStatus"])
	require.JSONEq(t, `{"name":"Stuck","externalParsing":false,"settingsVersion":1}`, string(fake.calls()[0].body))
}

func TestOrganizationHTTP_ReadyOrgStaysReadyWhenASettingsSyncFails(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, _ := newOrganizationRouter(t, fake)
	_, org := createOrg(t, router, "Acme")
	id := org["id"].(string)
	fake.setResponder(ragxError(http.StatusServiceUnavailable, "INTERNAL_ERROR"))
	res := send(router, http.MethodPatch, "/api/v1/orgs/"+id, `{"name":"Acme Vietnam"}`, nil)
	require.Equal(t, http.StatusOK, res.Code)
	edited := dataOf(t, res)
	require.Equal(t, "ready", edited["ragxStatus"], "KB work continues after a failed settings sync")
	require.Equal(t, false, edited["settingsSynced"])
	require.Equal(t, "RAGX_UNAVAILABLE", edited["ragxErrorCode"])
	require.Equal(t, "Acme Vietnam", edited["name"])
	fake.setResponder(provisionOK)
	res = send(router, http.MethodPost, "/api/v1/orgs/"+id+"/provision", "", nil)
	require.Equal(t, true, dataOf(t, res)["settingsSynced"])
	require.JSONEq(t, `{"name":"Acme Vietnam","externalParsing":true,"settingsVersion":2}`, string(fake.calls()[2].body))
}

func TestOrganizationHTTP_PatchOfANonReadyOrgOnlyWritesTheRow(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, _ := newOrganizationRouter(t, fake)
	fake.setResponder(ragxError(http.StatusNotFound, "LLM_ROLE_NOT_ASSIGNED"))
	_, org := createOrg(t, router, "Acme")
	require.Equal(t, "failed", org["ragxStatus"])
	require.Len(t, fake.calls(), 1)
	res := send(router, http.MethodPatch, "/api/v1/orgs/"+org["id"].(string), `{"externalParsing":false}`, nil)
	require.Equal(t, http.StatusOK, res.Code)
	patched := dataOf(t, res)
	require.Equal(t, false, patched["externalParsing"])
	require.Equal(t, "failed", patched["ragxStatus"])
	require.Len(t, fake.calls(), 1, "a non-ready org is only saved; provision sends it later")
}

func TestOrganizationHTTP_UnchangedPatchDoesNotCallRagx(t *testing.T) {
	fake := newFakeRagx(t)
	router, pool, _ := newOrganizationRouter(t, fake)
	_, org := createOrg(t, router, "Acme")
	id := org["id"].(string)
	var before int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE target_id = $1`, id).Scan(&before))
	res := send(router, http.MethodPatch, "/api/v1/orgs/"+id, `{"name":"Acme","externalParsing":true}`, nil)
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "ready", dataOf(t, res)["ragxStatus"])
	require.Len(t, fake.calls(), 1, "nothing changed, so ragx is not called")
	var after int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE target_id = $1`, id).Scan(&after))
	require.Equal(t, before, after, "nothing changed, so nothing is audited")
}

func TestOrganizationHTTP_ValidatesAndRejectsClientChosenIDs(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, _ := newOrganizationRouter(t, fake)
	_, org := createOrg(t, router, "Acme") // baseline: one org and one ragx call
	require.Len(t, fake.calls(), 1)

	fieldsOf := func(res *httptest.ResponseRecorder) any { return errorOf(t, res)["details"].(map[string]any)["fields"] }
	for _, tc := range []struct {
		body, field, code string
	}{
		{`{"name":"   ","externalParsing":true}`, "name", "REQUIRED"},
		{`{"name":"` + string(bytes.Repeat([]byte("ư"), orgmodel.NameMaxLength+1)) + `","externalParsing":true}`, "name", "INVALID"},
		{`{"name":"Beta"}`, "externalParsing", "REQUIRED"},
	} {
		res := send(router, http.MethodPost, "/api/v1/orgs", tc.body, nil)
		require.Equal(t, http.StatusBadRequest, res.Code, tc.body)
		require.Equal(t, []any{map[string]any{"field": tc.field, "code": tc.code}}, fieldsOf(res))
	}
	res := send(router, http.MethodPost, "/api/v1/orgs", `{"id":"default","name":"Beta","externalParsing":true}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code, "a client cannot choose the id")

	res = send(router, http.MethodPost, "/api/v1/orgs", `{"name":"acme ","externalParsing":true}`, nil)
	require.Equal(t, http.StatusConflict, res.Code)
	require.Equal(t, "CONFLICT", errorOf(t, res)["code"])
	require.Equal(t, []any{map[string]any{"field": "name", "code": "TAKEN"}}, fieldsOf(res))

	res = send(router, http.MethodPatch, "/api/v1/orgs/"+org["id"].(string), `{}`, nil)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Equal(t, []any{map[string]any{"field": "body", "code": "REQUIRED"}}, fieldsOf(res))

	for _, id := range []string{tenant.PlatformID, "not-a-uuid", uuid.NewString()} {
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/api/v1/orgs/" + id, ""},
			{http.MethodPatch, "/api/v1/orgs/" + id, `{"name":"X"}`},
			{http.MethodPost, "/api/v1/orgs/" + id + "/provision", ""},
		} {
			res := send(router, tc.method, tc.path, tc.body, nil)
			require.Equal(t, http.StatusNotFound, res.Code, tc.method+" "+tc.path)
			require.Equal(t, string(errors.CodeNotFound), errorOf(t, res)["code"])
		}
	}
	require.Len(t, fake.calls(), 1, "none of the rejected requests reached ragx")
}

func TestOrganizationHTTP_ListIsOrderedByName(t *testing.T) {
	fake := newFakeRagx(t)
	router, _, _ := newOrganizationRouter(t, fake)
	createOrg(t, router, "b")
	createOrg(t, router, "A")
	res := send(router, http.MethodGet, "/api/v1/orgs", "", nil)
	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	require.Equal(t, "A", body.Data[0]["name"])
	require.Equal(t, "b", body.Data[1]["name"])
}
