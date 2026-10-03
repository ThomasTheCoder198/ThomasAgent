package apperr

import catalogerrors "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"

type Lang = catalogerrors.Lang

const (
	LangVI = catalogerrors.LangVI
	LangEN = catalogerrors.LangEN
)

type Error = catalogerrors.AppError
type Option = catalogerrors.Option

func WithMessage(message string) Option         { return catalogerrors.WithMessage(message) }
func WithDetails(details map[string]any) Option { return catalogerrors.WithDetails(details) }
func WithCause(cause error) Option              { return catalogerrors.WithCause(cause) }

func New(code Code, opts ...Option) *Error {
	return catalogerrors.New(code, opts...)
}

func From(err error) *Error {
	return catalogerrors.From(err)
}

func LangFromHeader(acceptLanguage string) Lang {
	return catalogerrors.LangFromHeader(acceptLanguage)
}
