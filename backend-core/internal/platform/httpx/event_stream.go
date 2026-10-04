package httpx

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
)

const (
	contentTypeEventStream  = "text/event-stream"
	headerCacheControl      = "Cache-Control"
	cacheControlNoTransform = "no-cache, no-transform"
	headerProxyBuffering    = "X-Accel-Buffering"
	proxyBufferingOff       = "no"
	eventDataPrefix         = "data: "
	eventTerminator         = "\n\n"
	heartbeatFrame          = ": ping\n\n"
	newline                 = '\n'
	carriageReturn          = '\r'
)

var (
	ErrEventStreamClosed = stderrors.New("event stream closed")
	errMultiLinePayload  = stderrors.New("event payload must be a single line")
	errHeartbeatInterval = stderrors.New("event stream heartbeat must be positive")
)

// EventStream writes Server-Sent Events. no-transform and X-Accel-Buffering stop intermediaries (Next.js rewrites,
// nginx-style proxies) from compressing or buffering frames; the heartbeat comment keeps idle connections open
// through proxy idle timeouts. The payload encoding (UI Message Stream) belongs to the caller.
type EventStream struct {
	request    *http.Request
	writer     http.ResponseWriter
	controller *http.ResponseController
	mu         sync.Mutex
	closed     bool
	stop       chan struct{}
	stopOnce   sync.Once
	beatDone   chan struct{}
}

// NewEventStream commits the 200 status and headers immediately so the client sees the stream open.
// The caller must `defer stream.Close()`.
func NewEventStream(w http.ResponseWriter, r *http.Request, heartbeat time.Duration) (*EventStream, error) {
	if heartbeat <= 0 {
		return nil, errors.ErrInternalError.WithCause(errHeartbeatInterval)
	}
	header := w.Header()
	header.Set(headerContentType, contentTypeEventStream)
	header.Set(headerCacheControl, cacheControlNoTransform)
	header.Set(headerProxyBuffering, proxyBufferingOff)
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	if err := controller.Flush(); err != nil {
		return nil, errors.ErrInternalError.WithCause(fmt.Errorf("flush event stream headers: %w", err))
	}
	s := &EventStream{
		request: r, writer: w, controller: controller,
		stop: make(chan struct{}), beatDone: make(chan struct{}),
	}
	go s.beat(heartbeat)
	return s, nil
}

func (s *EventStream) Send(payload []byte) error {
	if bytes.IndexByte(payload, newline) >= 0 || bytes.IndexByte(payload, carriageReturn) >= 0 {
		return errMultiLinePayload
	}
	frame := make([]byte, 0, len(eventDataPrefix)+len(payload)+len(eventTerminator))
	frame = append(frame, eventDataPrefix...)
	frame = append(frame, payload...)
	frame = append(frame, eventTerminator...)
	return s.write(frame)
}

func (s *EventStream) Done() <-chan struct{} { return s.request.Context().Done() }

func (s *EventStream) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.beatDone
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

// write serializes heartbeat and data writes: http.ResponseWriter is not safe for concurrent use.
func (s *EventStream) write(frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.request.Context().Err() != nil {
		return ErrEventStreamClosed
	}
	if _, err := s.writer.Write(frame); err != nil {
		return fmt.Errorf("write event frame: %w", err)
	}
	if err := s.controller.Flush(); err != nil {
		return fmt.Errorf("flush event frame: %w", err)
	}
	return nil
}

func (s *EventStream) beat(interval time.Duration) {
	defer close(s.beatDone)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-s.request.Context().Done():
			return
		case <-ticker.C:
			if err := s.write([]byte(heartbeatFrame)); err != nil {
				return
			}
		}
	}
}
