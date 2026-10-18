package ragx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/apptoken"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/calllog"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/outbound"
)

type ToolName string

const (
	ToolKBSearch        ToolName = "kb_search"
	ToolKBOutline       ToolName = "kb_outline"
	ToolKBReadSection   ToolName = "kb_read_section"
	ToolKBListDocuments ToolName = "kb_list_documents"
	appRagx                      = "ragx"
	opToolPrefix                 = "tool."
	toolsPath                    = "tools"
	fieldInput                   = "input"
)

var scopeKeys = []string{"kbid", "kb_id", "orgid", "org_id", "tenantid", "tenant_id"}

func IsScopeKey(key string) bool { return slices.Contains(scopeKeys, strings.ToLower(key)) }

// CallTool signs the Agent's KB into the token and returns only ragx's data member.
func (c *Caller) CallTool(ctx context.Context, kbID uuid.UUID, tool ToolName, input json.RawMessage) (json.RawMessage, error) {
	if kbID == uuid.Nil {
		return nil, errors.ErrInternalError.WithCause(errors.ErrToolWithoutKB)
	}
	body, err := stripScopeKeys(input)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	cl, err := c.begin(ctx, opToolPrefix+string(tool), apptoken.ToolScope(string(tool)), kbID.String(), c.cfg.ToolTimeout)
	if err != nil {
		return nil, err
	}
	defer cl.cancel()
	req, err := http.NewRequestWithContext(cl.ctx, http.MethodPost, c.base.JoinPath(apiPrefix, toolsPath, string(tool)).String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.ErrInternalError.WithCause(fmt.Errorf("build ragx tool request: %w", err))
	}
	req.Header.Set(httpx.HeaderContentType, httpx.MediaTypeJSON)
	markRepeatable(req.Header)
	for _, edit := range cl.editors {
		if err := edit(cl.ctx, req); err != nil {
			return nil, errors.ErrInternalError.WithCause(err)
		}
	}
	res, err := c.http.Do(req) //nolint:bodyclose // check and readToolData close the body
	traceID := ""
	if res != nil {
		traceID = res.Header.Get(outbound.HeaderRagxTraceID)
	}
	failure := c.check(cl, res, err)
	var data json.RawMessage
	if failure == nil {
		data, failure = c.readToolData(res)
	}
	errorCode := ""
	if failure != nil {
		errorCode = string(errors.CodeOf(failure))
	}
	c.toolLog.Tool(cl.ctx, calllog.ToolCall{App: appRagx, Tool: string(tool), Duration: time.Since(start), ErrorCode: errorCode, RagxTraceID: traceID})
	return data, failure
}

func (c *Caller) readToolData(res *http.Response) (json.RawMessage, error) {
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, c.cfg.MaxResponseBytes+1))
	if err != nil {
		return nil, errors.ErrRagxUnavailable.WithCause(fmt.Errorf("read ragx tool response: %w", err))
	}
	if int64(len(raw)) > c.cfg.MaxResponseBytes {
		return nil, errors.ErrRagxUnavailable.WithCause(errors.ErrRagxResponseTooLarge)
	}
	var body dataBody[json.RawMessage]
	if err := json.Unmarshal(raw, &body); err != nil || len(body.Data) == 0 {
		return nil, errors.ErrRagxUnavailable.WithCause(fmt.Errorf("decode ragx tool response: %w", err))
	}
	return body.Data, nil
}

func stripScopeKeys(input json.RawMessage) ([]byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(input, &object); err != nil || object == nil {
		return nil, errors.FieldError(fieldInput, errors.FieldCodeInvalid).WithCause(errors.ErrToolInputNotObject)
	}
	for key := range object {
		if IsScopeKey(key) {
			delete(object, key)
		}
	}
	body, err := json.Marshal(object)
	if err != nil {
		return nil, errors.ErrInternalError.WithCause(err)
	}
	return body, nil
}
