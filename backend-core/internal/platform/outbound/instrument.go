package outbound

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/metrics"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/requestid"
)

const (
	TargetRagx        = "ragx"
	HeaderRagxTraceID = "X-Ragx-Trace-Id"
)

// A ragx trace id comes from a response header; only the OpenTelemetry hex form reaches logs.
var ragxTraceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func IsRagxTraceID(id string) bool { return ragxTraceID.MatchString(id) }

// Recorder is the part of the metrics registry the transport needs.
type Recorder interface {
	ObserveOutbound(target, outcome string, elapsed time.Duration)
}

type instrumentedTransport struct {
	next               http.RoundTripper
	target             string
	recorder           Recorder
	propagateRequestID bool
}

// Instrument records every round trip under target. Only the ragx transport propagates X-Request-Id; external
// providers never receive internal ids.
func Instrument(next http.RoundTripper, target string, rec Recorder, propagateRequestID bool) http.RoundTripper {
	return instrumentedTransport{next: next, target: target, recorder: rec, propagateRequestID: propagateRequestID}
}

func (t instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	// Drop supplied IDs as well as context IDs at external provider boundaries.
	for key := range req.Header {
		if strings.EqualFold(key, requestid.Header) {
			delete(req.Header, key)
		}
	}
	if t.propagateRequestID && t.target == TargetRagx {
		if id := requestid.FromContext(req.Context()); requestid.IsValid(id) {
			req.Header.Set(requestid.Header, id)
		}
	}
	start := time.Now()
	res, err := t.next.RoundTrip(req)
	if t.recorder != nil {
		outcome := metrics.OutcomeSuccess
		if err != nil || res.StatusCode >= http.StatusBadRequest {
			outcome = metrics.OutcomeError
		}
		t.recorder.ObserveOutbound(t.target, outcome, time.Since(start))
	}
	return res, err
}
