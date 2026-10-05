package httpx

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

const (
	contentTypeEventStream  = "text/event-stream"
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
	errWriteTimeout      = stderrors.New("event stream write timeout must be positive")
)

// EventStream writes Server-Sent Events. no-transform and X-Accel-Buffering stop intermediaries (Next.js rewrites,
// nginx-style proxies) from compressing or buffering frames; the heartbeat comment keeps idle connections open
// through proxy idle timeouts. The payload encoding (UI Message Stream) belongs to the caller.
type EventStream struct {
	request      *http.Request
	writer       http.ResponseWriter
	controller   *http.ResponseController
	mu           sync.Mutex
	closed       bool
	stop         chan struct{}
	stopOnce     sync.Once
	beatDone     chan struct{}
	writeTimeout time.Duration
	stopContext  func() bool
}

// NewEventStream commits the 200 status and headers immediately so the client sees the stream open.
// The caller must `defer stream.Close()`.
func NewEventStream(w http.ResponseWriter, r *http.Request, cfg config.HTTPConfig) (*EventStream, error) {
	if cfg.EventStreamWriteTimeout <= 0 {
		return nil, errors.ErrInternalError.WithCause(errWriteTimeout)
	}
	return newEventStream(w, r, cfg.EventStreamHeartbeat, cfg.EventStreamWriteTimeout)
}

func newEventStream(w http.ResponseWriter, r *http.Request, heartbeat, writeTimeout time.Duration) (*EventStream, error) {
	if heartbeat <= 0 {
		return nil, errors.ErrInternalError.WithCause(errHeartbeatInterval)
	}
	if writeTimeout <= 0 {
		return nil, errors.ErrInternalError.WithCause(errWriteTimeout)
	}
	header := w.Header()
	header.Set(headerContentType, contentTypeEventStream)
	header.Set(HeaderCacheControl, cacheControlNoTransform)
	header.Set(headerProxyBuffering, proxyBufferingOff)
	controller := http.NewResponseController(w)
	if err := setEventStreamDeadline(controller, time.Now().Add(writeTimeout)); err != nil {
		return nil, errors.ErrInternalError.WithCause(fmt.Errorf("set event stream header deadline: %w", err))
	}
	w.WriteHeader(http.StatusOK)
	flushErr := controller.Flush()
	deadlineErr := setEventStreamDeadline(controller, time.Time{})
	if err := stderrors.Join(flushErr, deadlineErr); err != nil {
		return nil, errors.ErrInternalError.WithCause(fmt.Errorf("flush event stream headers: %w", err))
	}
	s := &EventStream{
		request: r, writer: w, controller: controller, writeTimeout: writeTimeout,
		stop: make(chan struct{}), beatDone: make(chan struct{}),
	}
	// Cancellation must reach producers even while a heartbeat is blocked on network I/O.
	s.stopContext = context.AfterFunc(r.Context(), func() { s.stopOnce.Do(func() { close(s.stop) }) })
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

func (s *EventStream) Done() <-chan struct{} { return s.stop }

func (s *EventStream) Close() {
	s.stopContext()
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
	if err := setEventStreamDeadline(s.controller, time.Now().Add(s.writeTimeout)); err != nil {
		s.stopOnce.Do(func() { close(s.stop) })
		s.closed = true
		return fmt.Errorf("set event stream write deadline: %w", err)
	}
	defer func() { _ = setEventStreamDeadline(s.controller, time.Time{}) }()
	if _, err := s.writer.Write(frame); err != nil {
		s.stopOnce.Do(func() { close(s.stop) })
		s.closed = true
		return fmt.Errorf("write event frame: %w", err)
	}
	if err := s.controller.Flush(); err != nil {
		s.stopOnce.Do(func() { close(s.stop) })
		s.closed = true
		return fmt.Errorf("flush event frame: %w", err)
	}
	return nil
}

func setEventStreamDeadline(controller *http.ResponseController, deadline time.Time) error {
	err := controller.SetWriteDeadline(deadline)
	// Legacy middleware may expose Flush without forwarding optional deadline support.
	if stderrors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

func (s *EventStream) beat(interval time.Duration) {
	defer close(s.beatDone)
	defer s.stopOnce.Do(func() { close(s.stop) })
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
