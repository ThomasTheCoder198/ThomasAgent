package ragx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/logging"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	logEvent         = "ragx call"
	fieldTarget      = "target"
	fieldOperation   = "operation"
	fieldStatus      = "status"
	fieldOutcome     = "outcome"
	fieldErrorCode   = "error_code"
	fieldRagxTraceID = "ragx_trace_id"
	fieldMethod      = "method"
	fieldRoute       = "route"
	fieldJTI         = "jti"

	opProvisionOrg = "provision_org"
	opForward      = "app_proxy"
	apiPrefix      = "v1"
)

// TokenSigner is satisfied by *apptoken.Signer.
type TokenSigner interface {
	Issue(apptoken.Claims) (apptoken.Issued, error)
}

type Caller struct {
	api    *Client
	http   *http.Client
	base   *url.URL
	signer TokenSigner
	log    *slog.Logger
	cfg    config.RagxConfig
}

// NewRagxCaller does not use outbound.NewClient: ragx is a configured first-party service on a private address, and
// that client's whole-request timeout would cut the app proxy's forwards. Deadlines come from the context.
func NewRagxCaller(cfg config.RagxConfig, signer TokenSigner, recorder outbound.Recorder, log *slog.Logger) (*Caller, error) {
	client := &http.Client{
		Transport:     outbound.Instrument(newRagxTransport(cfg), outbound.TargetRagx, recorder, true),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	api, err := NewClient(cfg.URL, WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("ragx client: %w", err)
	}
	baseURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("ragx url: %w", err)
	}
	return &Caller{api: api, http: client, base: baseURL, signer: signer, log: log, cfg: cfg}, nil
}

func newRagxTransport(cfg config.RagxConfig) *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: cfg.ConnectTimeout}).DialContext,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
	}
}

type call struct {
	ctx       context.Context
	cancel    context.CancelFunc
	editors   []RequestEditorFn
	operation string
	method    string
	route     string
	jti       string
	start     time.Time
}

// begin signs with the same request id the transport will send: ragx rejects a token whose rid differs.
func (c *Caller) begin(ctx context.Context, operation string, scope apptoken.Scope, bounded bool) (call, error) {
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return call{}, err
	}
	if !requestid.IsValid(requestid.FromContext(ctx)) {
		ctx = requestid.NewContext(ctx, requestid.NewJob())
	}
	issued, err := c.signer.Issue(apptoken.Claims{OrgID: orgID, Scope: scope, RequestID: requestid.FromContext(ctx)})
	if err != nil {
		return call{}, err
	}
	cancel := context.CancelFunc(func() {})
	if bounded {
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
	}
	authorize := func(_ context.Context, req *http.Request) error {
		req.Header.Set(httpx.HeaderAuthorization, httpx.AuthSchemeBearer+" "+issued.Token)
		return nil
	}
	return call{ctx: ctx, cancel: cancel, editors: []RequestEditorFn{authorize}, operation: operation, jti: issued.ID, start: time.Now()}, nil
}

// check maps the round trip to a catalog error and writes the one outbound log line. On success the body stays open.
func (c *Caller) check(cl call, res *http.Response, err error) error {
	status := 0
	var traceID string
	if res != nil {
		status = res.StatusCode
		traceID = res.Header.Get(outbound.HeaderRagxTraceID)
	}
	var failure error
	switch {
	case err != nil:
		failure = errors.ErrRagxUnavailable.WithCause(err)
	case status >= http.StatusMultipleChoices:
		failure = responseError(res, c.cfg.MaxErrorBodyBytes)
		_ = res.Body.Close()
	}
	c.logCall(cl, status, traceID, failure)
	return failure
}

func (c *Caller) logCall(cl call, status int, traceID string, failure error) {
	attrs := []slog.Attr{
		slog.String(fieldTarget, outbound.TargetRagx), slog.String(fieldOperation, cl.operation),
		slog.Int(fieldStatus, status), slog.Int64(logging.FieldDurationMS, time.Since(cl.start).Milliseconds()),
		slog.String(fieldJTI, cl.jti),
	}
	if cl.method != "" {
		attrs = append(attrs, slog.String(fieldMethod, cl.method), slog.String(fieldRoute, cl.route))
	}
	if outbound.IsRagxTraceID(traceID) {
		attrs = append(attrs, slog.String(fieldRagxTraceID, traceID))
	}
	level, outcome := slog.LevelInfo, metrics.OutcomeSuccess
	if failure != nil {
		// Warn, not error: the HTTP or job boundary logs the failure once at its own level.
		level, outcome = slog.LevelWarn, metrics.OutcomeError
		attrs = append(attrs, slog.String(fieldErrorCode, string(errors.CodeOf(failure))))
	}
	attrs = append(attrs, slog.String(fieldOutcome, outcome))
	c.log.LogAttrs(cl.ctx, level, logEvent, attrs...)
}

type sendFunc func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error)

type dataBody[T any] struct {
	Data T `json:"data"`
}

// readJSON maps the round trip and decodes a JSON body into B, closing the body on every path.
func readJSON[B any](c *Caller, cl call, send sendFunc) (B, error) {
	var body B
	res, err := send(cl.ctx, cl.editors)
	if err := c.check(cl, res, err); err != nil {
		return body, err
	}
	defer func() { _ = res.Body.Close() }()
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return body, errors.ErrRagxUnavailable.WithCause(fmt.Errorf("decode ragx %s: %w", cl.operation, err))
	}
	return body, nil
}

// doJSON runs one bounded JSON call; the one place that pairs begin with its cancel.
func doJSON[B any](c *Caller, ctx context.Context, operation string, scope apptoken.Scope, send sendFunc) (B, error) {
	cl, err := c.begin(ctx, operation, scope, true)
	if err != nil {
		var zero B
		return zero, err
	}
	defer cl.cancel()
	return readJSON[B](c, cl, send)
}

func doData[T any](c *Caller, ctx context.Context, operation string, scope apptoken.Scope, send sendFunc) (T, error) {
	body, err := doJSON[dataBody[T]](c, ctx, operation, scope, send)
	return body.Data, err
}

func (c *Caller) ProvisionOrg(ctx context.Context, name string, externalParsing bool, settingsVersion int64) (Org, error) {
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return Org{}, err
	}
	return doData[Org](c, ctx, opProvisionOrg, apptoken.ScopeOrgProvision, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.ProvisionOrg(ctx, orgID, &ProvisionOrgParams{}, ProvisionOrgJSONRequestBody{Name: name, ExternalParsing: externalParsing, SettingsVersion: settingsVersion}, editors...)
	})
}
