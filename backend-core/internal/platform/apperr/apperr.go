package apperr

import (
	"errors"
	"net/http"
	"strings"
)

type spec struct {
	status    int
	retryable bool
	messageVI string
	messageEN string
}

type Lang string

const (
	LangVI Lang = "vi"
	LangEN Lang = "en"
)

type Error struct {
	Code    Code
	Message string
	Details map[string]any
	Cause   error
}

type Option func(*Error)

func WithMessage(msg string) Option             { return func(e *Error) { e.Message = msg } }
func WithDetails(details map[string]any) Option { return func(e *Error) { e.Details = details } }
func WithCause(err error) Option                { return func(e *Error) { e.Cause = err } }

func New(code Code, opts ...Option) *Error {
	e := &Error{Code: code}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Cause.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) spec() spec {
	if s, ok := catalog[e.Code]; ok {
		return s
	}
	return catalog[CodeInternalError]
}

func (e *Error) Status() int {
	if s := e.spec(); s.status != 0 {
		return s.status
	}
	return http.StatusInternalServerError
}

func (e *Error) Retryable() bool { return e.spec().retryable }

func (e *Error) LocalizedMessage(lang Lang) string {
	if e.Message != "" {
		return e.Message
	}
	if lang == LangEN {
		return e.spec().messageEN
	}
	return e.spec().messageVI
}

func From(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return New(CodeInternalError, WithCause(err))
}

func LangFromHeader(acceptLanguage string) Lang {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(acceptLanguage)), string(LangEN)) {
		return LangEN
	}
	return LangVI
}
