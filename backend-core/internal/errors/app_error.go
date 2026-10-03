package errors

import (
	stderrors "errors"
	"maps"
	"strings"
)

type ErrorDefinition struct {
	HTTPStatus int
	Retryable  bool
	MessageVI  string
	MessageEN  string
}

type Lang string

const (
	LangVI Lang = "vi"
	LangEN Lang = "en"
)

type Sentinel Code

func (s Sentinel) Error() string { return string(s) }

func (s Sentinel) Is(target error) bool { return matchesCode(Code(s), target) }

// Normalization creates request-local fields instead of attaching state to the shared definition.
func (s Sentinel) As(target any) bool {
	appErr, ok := target.(**AppError)
	if !ok {
		return false
	}
	*appErr = New(Code(s))
	return true
}

func (s Sentinel) WithCause(cause error) *AppError { return New(Code(s), WithCause(cause)) }

func (s Sentinel) WithMessage(message string) *AppError { return New(Code(s), WithMessage(message)) }

func (s Sentinel) WithDetails(details map[string]any) *AppError {
	return New(Code(s), WithDetails(details))
}

type AppError struct {
	Code    Code
	Message string
	Details map[string]any
	Cause   error
}

type Error = AppError

type Option func(*AppError)

func WithMessage(message string) Option { return func(e *AppError) { e.Message = message } }
func WithDetails(details map[string]any) Option {
	return func(e *AppError) { e.Details = maps.Clone(details) }
}
func WithCause(cause error) Option { return func(e *AppError) { e.Cause = cause } }

func New(code Code, options ...Option) *AppError {
	e := &AppError{Code: code}
	for _, option := range options {
		option(e)
	}
	return e
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Cause.Error()
	}
	return string(e.Code)
}

func (e *AppError) Unwrap() error { return e.Cause }

func (e *AppError) Is(target error) bool { return matchesCode(e.Code, target) }

func matchesCode(code Code, target error) bool {
	if code == "" {
		return false
	}
	var candidate *AppError
	return stderrors.As(target, &candidate) && candidate != nil && code == candidate.Code
}

func (e *AppError) Status() int { return LookupDefinition(e.Code).HTTPStatus }

func (e *AppError) Retryable() bool { return LookupDefinition(e.Code).Retryable }

func (e *AppError) LocalizedMessage(lang Lang) string {
	if e.Message != "" {
		return e.Message
	}
	definition := LookupDefinition(e.Code)
	if lang == LangEN {
		return definition.MessageEN
	}
	return definition.MessageVI
}

func (e *AppError) WithCause(cause error) *AppError {
	return New(e.Code, WithMessage(e.Message), WithDetails(e.Details), WithCause(cause))
}

func (e *AppError) WithMessage(message string) *AppError {
	return New(e.Code, WithMessage(message), WithDetails(e.Details), WithCause(e.Cause))
}

func (e *AppError) WithDetails(details map[string]any) *AppError {
	return New(e.Code, WithMessage(e.Message), WithDetails(details), WithCause(e.Cause))
}

func From(err error) *AppError {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr
	}
	return ErrInternalError.WithCause(err)
}

func LangFromHeader(acceptLanguage string) Lang {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(acceptLanguage)), string(LangEN)) {
		return LangEN
	}
	return LangVI
}
