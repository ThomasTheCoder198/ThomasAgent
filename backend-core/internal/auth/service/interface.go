package service

import (
	"context"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
)

// SessionAuthenticator is the authentication boundary consumed by HTTP middleware.
type SessionAuthenticator interface {
	Authenticate(context.Context, string) (model.User, model.Session, error)
}

// AuthService is the behavior supplied to the auth HTTP controller and session middleware.
type AuthService interface {
	SessionAuthenticator
	Login(context.Context, string, string) (model.User, model.IssuedSession, error)
	Logout(context.Context, string) error
	RecordFailedLogin(context.Context) error
}
