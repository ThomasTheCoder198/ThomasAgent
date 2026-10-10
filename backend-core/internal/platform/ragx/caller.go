package ragx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

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

	opProvisionOrg = "provision_org"
	opListKBs      = "list_knowledge_bases"
	opCreateKB     = "create_knowledge_base"
	opGetKB        = "get_knowledge_base"
	opRenameKB     = "rename_knowledge_base"
	opDeleteKB     = "delete_knowledge_base"
	opListFiles    = "list_files"
	opUploadFile   = "upload_file"
	opDeleteFile   = "delete_file"
	opRetryFile    = "retry_file"
	opRawFile      = "download_file"
	opPreviewFile  = "get_file_preview"
)

type TokenSigner interface {
	Sign(apptoken.Claims) (string, error)
}

type Caller struct {
	api    *Client
	signer TokenSigner
	log    *slog.Logger
	cfg    config.RagxConfig
}

type FilePage struct {
	Files  []KbFile
	Cursor *string
	Limit  int
}

type Upload struct {
	KnowledgeBaseID uuid.UUID
	ContentType     string
	ContentLength   int64
	Body            io.Reader
	IdempotencyKey  string
}

// NewRagxCaller does not use outbound.NewClient: ragx is a configured first-party service on a private address, and
// that client's whole-request timeout would cut streamed uploads and downloads. Deadlines come from the context.
func NewRagxCaller(cfg config.RagxConfig, signer TokenSigner, recorder outbound.Recorder, log *slog.Logger) (*Caller, error) {
	base := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: cfg.ConnectTimeout}).DialContext,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
	}
	client := &http.Client{
		Transport:     outbound.Instrument(base, outbound.TargetRagx, recorder, true),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	api, err := NewClient(cfg.URL, WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("ragx client: %w", err)
	}
	return &Caller{api: api, signer: signer, log: log, cfg: cfg}, nil
}

type call struct {
	ctx       context.Context
	cancel    context.CancelFunc
	editors   []RequestEditorFn
	operation string
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
	token, err := c.signer.Sign(apptoken.Claims{OrgID: orgID, Scope: scope, RequestID: requestid.FromContext(ctx)})
	if err != nil {
		return call{}, err
	}
	cancel := context.CancelFunc(func() {})
	if bounded {
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
	}
	authorize := func(_ context.Context, req *http.Request) error {
		req.Header.Set(httpx.HeaderAuthorization, httpx.AuthSchemeBearer+" "+token)
		return nil
	}
	return call{ctx: ctx, cancel: cancel, editors: []RequestEditorFn{authorize}, operation: operation, start: time.Now()}, nil
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

func (c *Caller) doEmpty(ctx context.Context, operation string, scope apptoken.Scope, send sendFunc) error {
	cl, err := c.begin(ctx, operation, scope, true)
	if err != nil {
		return err
	}
	defer cl.cancel()
	res, err := send(cl.ctx, cl.editors)
	if err := c.check(cl, res, err); err != nil {
		return err
	}
	return res.Body.Close()
}

func (c *Caller) ProvisionOrg(ctx context.Context, name string, externalParsing bool) (Org, error) {
	orgID, err := tenant.ID(ctx)
	if err != nil {
		return Org{}, err
	}
	return doData[Org](c, ctx, opProvisionOrg, apptoken.ScopeOrgProvision, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.ProvisionOrg(ctx, orgID, &ProvisionOrgParams{}, ProvisionOrgJSONRequestBody{Name: name, ExternalParsing: externalParsing}, editors...)
	})
}

func (c *Caller) ListKnowledgeBases(ctx context.Context) ([]KnowledgeBase, error) {
	return doData[[]KnowledgeBase](c, ctx, opListKBs, apptoken.ScopeKBRead, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.ListKnowledgeBases(ctx, &ListKnowledgeBasesParams{}, editors...)
	})
}

func (c *Caller) CreateKnowledgeBase(ctx context.Context, name string) (KnowledgeBase, error) {
	return doData[KnowledgeBase](c, ctx, opCreateKB, apptoken.ScopeKBWrite, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.CreateKnowledgeBase(ctx, &CreateKnowledgeBaseParams{}, CreateKnowledgeBaseJSONRequestBody{Name: name}, editors...)
	})
}

func (c *Caller) GetKnowledgeBase(ctx context.Context, kbID uuid.UUID) (KnowledgeBase, error) {
	return doData[KnowledgeBase](c, ctx, opGetKB, apptoken.ScopeKBRead, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.GetKnowledgeBase(ctx, kbID, &GetKnowledgeBaseParams{}, editors...)
	})
}

func (c *Caller) RenameKnowledgeBase(ctx context.Context, kbID uuid.UUID, name string) (KnowledgeBase, error) {
	return doData[KnowledgeBase](c, ctx, opRenameKB, apptoken.ScopeKBWrite, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.RenameKnowledgeBase(ctx, kbID, &RenameKnowledgeBaseParams{}, RenameKnowledgeBaseJSONRequestBody{Name: name}, editors...)
	})
}

func (c *Caller) DeleteKnowledgeBase(ctx context.Context, kbID uuid.UUID) error {
	return c.doEmpty(ctx, opDeleteKB, apptoken.ScopeKBWrite, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.DeleteKnowledgeBase(ctx, kbID, &DeleteKnowledgeBaseParams{}, editors...)
	})
}

type filePageBody struct {
	Data []KbFile `json:"data"`
	Meta struct {
		Page struct {
			Cursor *string `json:"cursor"`
			Limit  int     `json:"limit"`
		} `json:"page"`
	} `json:"meta"`
}

func (c *Caller) ListFiles(ctx context.Context, kbID uuid.UUID, cursor *string, limit *int) (FilePage, error) {
	body, err := doJSON[filePageBody](c, ctx, opListFiles, apptoken.ScopeKBRead, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.ListFiles(ctx, kbID, &ListFilesParams{Cursor: cursor, Limit: limit}, editors...)
	})
	if err != nil {
		return FilePage{}, err
	}
	return FilePage{Files: body.Data, Cursor: body.Meta.Page.Cursor, Limit: body.Meta.Page.Limit}, nil
}

// UploadFile streams the caller's body. ragx requires Content-Length, so the editor restores the length that
// http.NewRequest cannot see through an io.Reader; a body shorter than the length fails the request.
func (c *Caller) UploadFile(ctx context.Context, in Upload) (KbFile, bool, error) {
	cl, err := c.begin(ctx, opUploadFile, apptoken.ScopeKBWrite, false)
	if err != nil {
		return KbFile{}, false, err
	}
	params := &UploadFileParams{}
	if in.IdempotencyKey != "" {
		params.IdempotencyKey = &in.IdempotencyKey
	}
	withLength := func(_ context.Context, req *http.Request) error {
		req.ContentLength = in.ContentLength
		return nil
	}
	created := false
	body, err := readJSON[dataBody[KbFile]](c, cl, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		res, err := c.api.UploadFileWithBody(ctx, in.KnowledgeBaseID, params, in.ContentType, in.Body, append(editors, withLength)...)
		created = err == nil && res.StatusCode == http.StatusCreated
		return res, err
	})
	return body.Data, created, err
}

func (c *Caller) DeleteFile(ctx context.Context, kbID, fileID uuid.UUID) error {
	return c.doEmpty(ctx, opDeleteFile, apptoken.ScopeKBWrite, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.DeleteFile(ctx, kbID, fileID, &DeleteFileParams{}, editors...)
	})
}

func (c *Caller) RetryFile(ctx context.Context, kbID, fileID uuid.UUID) (KbFile, error) {
	return doData[KbFile](c, ctx, opRetryFile, apptoken.ScopeKBWrite, func(ctx context.Context, editors []RequestEditorFn) (*http.Response, error) {
		return c.api.RetryFile(ctx, kbID, fileID, &RetryFileParams{}, editors...)
	})
}

// OpenFileRaw and OpenFilePreview return the open response; the caller copies and closes the body. Their deadline
// is the HTTP route's transfer timeout, not RequestTimeout.
func (c *Caller) OpenFileRaw(ctx context.Context, kbID, fileID uuid.UUID) (*http.Response, error) {
	cl, err := c.begin(ctx, opRawFile, apptoken.ScopeKBRead, false)
	if err != nil {
		return nil, err
	}
	res, err := c.api.DownloadFile(cl.ctx, kbID, fileID, &DownloadFileParams{}, cl.editors...)
	if err := c.check(cl, res, err); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Caller) OpenFilePreview(ctx context.Context, kbID, fileID uuid.UUID) (*http.Response, error) {
	cl, err := c.begin(ctx, opPreviewFile, apptoken.ScopeKBRead, false)
	if err != nil {
		return nil, err
	}
	res, err := c.api.GetFilePreview(cl.ctx, kbID, fileID, &GetFilePreviewParams{}, cl.editors...)
	if err := c.check(cl, res, err); err != nil {
		return nil, err
	}
	return res, nil
}
