package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

type UserRepository interface {
	CountUsers(context.Context) (int, error)
	CreateUser(context.Context, string, string) (model.User, error)
	FindUserByEmail(context.Context, string) (model.User, string, error)
}

type SessionRepository interface {
	CreateSession(context.Context, uuid.UUID, []byte, string, time.Time) error
	CreateSessionIn(context.Context, postgres.DBTX, uuid.UUID, []byte, string, time.Time) error
	FindSession(context.Context, []byte) (model.User, model.Session, error)
	FindSessionByToken(context.Context, []byte) (model.User, model.Session, error)
	DeleteSession(context.Context, []byte) error
	PurgeExpiredSessions(context.Context) (int64, error)
	PurgeAllExpiredSessions(context.Context) (int64, error)
}

type AuthRepository interface {
	UserRepository
	SessionRepository
}

var _ AuthRepository = (*Repository)(nil)
