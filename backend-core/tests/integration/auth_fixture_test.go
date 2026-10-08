package integration

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	authcontroller "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/controller"
	authlimiter "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/limiter"
	authmiddleware "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/middleware"
	authrepository "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	ownerEmail    = "thomas@example.com"
	ownerPassword = "correct horse battery"
)

var testLimiterIdentityKey = []byte("0123456789abcdef0123456789abcdef")

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// swappableDB lets a test inject a failing database after the owner is bootstrapped.
type swappableDB struct{ inner authservice.TransactionDB }

func (db *swappableDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.inner.Exec(ctx, sql, args...)
}
func (db *swappableDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.inner.Query(ctx, sql, args...)
}
func (db *swappableDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.inner.QueryRow(ctx, sql, args...)
}
func (db *swappableDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.inner.Begin(ctx)
}

// failingExecDB fails statements with the given prefix outside transactions.
type failingExecDB struct {
	postgres.DBTX
	statement string
	failure   error
}

func (db failingExecDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if strings.HasPrefix(query, db.statement) {
		return pgconn.CommandTag{}, db.failure
	}
	return db.DBTX.Exec(ctx, query, args...)
}

func (db failingExecDB) Begin(ctx context.Context) (pgx.Tx, error) {
	base, ok := db.DBTX.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return nil, stderrors.New("underlying database cannot begin transaction")
	}
	return base.Begin(ctx)
}

// authFixture is a bootstrapped AuthService plus the handles a test needs to reach the database.
type authFixture struct {
	authservice.AuthService
	pool     *pgxpool.Pool
	db       *swappableDB
	users    authrepository.UserRepo
	sessions authrepository.SessionRepo
}

func newAuthFixture(t *testing.T) (*authFixture, *clock) {
	t.Helper()
	pool := pgtest.Start(t)
	testClock := &clock{t: time.Now()}
	authConfig := config.AuthConfig{OwnerEmail: ownerEmail, OwnerPassword: ownerPassword, SessionTTL: time.Hour, MinPasswordLength: 12}
	users := authrepository.NewUserRepo(pool)
	sessions := authrepository.NewSessionRepo(pool, 0)
	db := &swappableDB{inner: pool}
	fixture := &authFixture{
		AuthService: authservice.NewAuthService(users, sessions, db, authConfig, testClock.now),
		pool:        pool,
		db:          db,
		users:       users,
		sessions:    sessions,
	}
	require.NoError(t, fixture.EnsureOwner(tenant.WithID(t.Context(), tenant.PlatformID)))
	return fixture, testClock
}

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	container, err := tcredis.Run(context.Background(), "redis:8.10.2")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	redisURL, err := container.ConnectionString(context.Background())
	require.NoError(t, err)
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

// newTestLimiter gives the email+IP and email budgets the same limit and keeps the default IP budget.
func newTestLimiter(redisClient *redis.Client, maxAttempts int) *authlimiter.LoginLimiter {
	return authlimiter.NewLoginLimiter(redisClient, config.AuthConfig{
		LoginMaxAttempts:      maxAttempts,
		LoginEmailMaxAttempts: maxAttempts,
		LoginIPMaxAttempts:    config.DefaultLoginIPMaxAttempts,
		LoginWindow:           time.Minute,
	}, testLimiterIdentityKey)
}

// mountAuthForTest registers the auth routes exactly as cmd/core/routes.go does.
func mountAuthForTest(router *gin.Engine, params authcontroller.AuthControllerParams) {
	controller := authcontroller.NewAuthController(params)
	router.POST("/api/v1/auth/login", controller.HandleLogin)
	protected := router.Group("", authmiddleware.RequireSession(params.AuthService, params.CookieSecure))
	protected.POST("/api/v1/auth/logout", controller.HandleLogout)
	protected.GET("/api/v1/auth/me", controller.HandleMe)
}

func wrapAuthHTTP(handler http.HandlerFunc) gin.HandlerFunc { return gin.WrapF(handler) }

// newAuthServer mounts auth plus two protected probes: a mutation and an event stream.
func newAuthServer(t *testing.T, maxAttempts int) (http.Handler, *clock) {
	t.Helper()
	fixture, testClock := newAuthFixture(t)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := httpx.NewRouter(httpx.NewSlogErrorLogger(logger), httpx.LogAccess(logger))
	mountAuthForTest(router, authcontroller.AuthControllerParams{AuthService: fixture, Limiter: newTestLimiter(startRedis(t), maxAttempts)})
	protected := router.Group("", authmiddleware.RequireSession(fixture, false))
	protected.POST("/api/v1/things", wrapAuthHTTP(func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteSuccess(w, req, http.StatusCreated, map[string]bool{"ok": true})
	}))
	protected.POST("/api/v1/stream-probe", wrapAuthHTTP(streamProbe))
	return router, testClock
}

func login(t *testing.T, handler http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","password":"` + password + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "10.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func cookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, candidate := range response.Result().Cookies() {
		if candidate.Name == name {
			return candidate
		}
	}
	return nil
}

func errCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct{ Error struct{ Code string } }
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	return envelope.Error.Code
}
