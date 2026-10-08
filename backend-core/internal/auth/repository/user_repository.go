package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// UserRepo stores users of the tenant in ctx.
type UserRepo interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, email, passwordHash string) (model.User, error)
	// FindUserByEmail returns the user and the stored Argon2id password hash.
	FindUserByEmail(ctx context.Context, email string) (model.User, string, error)
}

type userRepo struct {
	db postgres.DBTX
}

// NewUserRepo works on a pool or on a transaction.
func NewUserRepo(db postgres.DBTX) UserRepo {
	return &userRepo{db: db}
}

func (r *userRepo) CountUsers(ctx context.Context) (int, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	if err = r.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (r *userRepo) CreateUser(ctx context.Context, email, passwordHash string) (model.User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, err
	}
	user := model.User{Email: email}
	err = r.db.QueryRow(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, email) DO NOTHING RETURNING id, display_name`,
		email, passwordHash, tenantID).Scan(&user.ID, &user.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, errors.ErrConflict
	}
	if err != nil {
		return model.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *userRepo) FindUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, "", err
	}
	var user model.User
	var passwordHash string
	err = r.db.QueryRow(ctx, `SELECT id, email, display_name, password_hash FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID).
		Scan(&user.ID, &user.Email, &user.DisplayName, &passwordHash)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("find user: %w", err)
	}
	return user, passwordHash, nil
}
