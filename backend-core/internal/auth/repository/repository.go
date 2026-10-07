package repository

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

type Repository struct {
	DB                   postgres.DBTX
	sessionTouchInterval time.Duration
}

func NewAuthRepository(db postgres.DBTX) *Repository {
	return NewAuthRepositoryWithSessionTouchInterval(db, config.DefaultSessionTouchInterval)
}

func NewAuthRepositoryWithSessionTouchInterval(db postgres.DBTX, interval time.Duration) *Repository {
	if interval <= 0 {
		interval = config.DefaultSessionTouchInterval
	}
	return &Repository{DB: db, sessionTouchInterval: interval}
}

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	if err = r.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (model.User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, err
	}
	u := model.User{Email: email}
	err = r.DB.QueryRow(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, email) DO NOTHING RETURNING id, display_name`,
		email, passwordHash, tenantID).Scan(&u.ID, &u.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, errors.ErrConflict
	}
	if err != nil {
		return model.User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, "", err
	}
	var u model.User
	var hash string
	err = r.DB.QueryRow(ctx, `SELECT id, email, display_name, password_hash FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID).
		Scan(&u.ID, &u.Email, &u.DisplayName, &hash)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, "", errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, "", fmt.Errorf("find user: %w", err)
	}
	return u, hash, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrf string, expiresAt time.Time) error {
	return r.CreateSessionIn(ctx, r.DB, userID, tokenHash, csrf, expiresAt)
}

func (r *Repository) CreateSessionIn(ctx context.Context, db postgres.DBTX, userID uuid.UUID, tokenHash []byte, csrf string, expiresAt time.Time) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO sessions (tenant_id, user_id, token_hash, csrf_token, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, userID, tokenHash, csrf, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *Repository) FindSession(ctx context.Context, tokenHash []byte) (model.User, model.Session, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, model.Session{}, err
	}
	var u model.User
	var s model.Session
	err = r.DB.QueryRow(ctx, `WITH touched_session AS (
  UPDATE sessions SET last_seen_at=now()
  WHERE token_hash=$1 AND tenant_id=$2 AND expires_at>now()
   AND last_seen_at<now()-($3 * interval '1 second')
 ) SELECT s.id, s.tenant_id, s.user_id, s.csrf_token, s.expires_at, s.expires_at<=now(), u.id, u.email, u.display_name
  FROM sessions s JOIN users u ON u.id=s.user_id AND u.tenant_id=s.tenant_id
  WHERE s.token_hash=$1 AND s.tenant_id=$2`, tokenHash, tenantID, r.sessionTouchInterval.Seconds()).
		Scan(&s.ID, &s.TenantID, &s.UserID, &s.CSRFToken, &s.ExpiresAt, &s.Expired, &u.ID, &u.Email, &u.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.Session{}, errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, model.Session{}, fmt.Errorf("find session: %w", err)
	}
	return u, s, nil
}

func (r *Repository) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	result, err := r.DB.Exec(ctx, `DELETE FROM sessions WHERE tenant_id=$1 AND expires_at<=now()`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}
	return result.RowsAffected(), nil
}

// PurgeAllExpiredSessions is reserved for the system job: request paths must
// continue using tenant-scoped repository methods.
func (r *Repository) PurgeAllExpiredSessions(ctx context.Context) (int64, error) {
	result, err := r.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`)
	if err != nil {
		return 0, fmt.Errorf("purge all expired sessions: %w", err)
	}
	return result.RowsAffected(), nil
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	if _, err := r.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND tenant_id = $2`, tokenHash, tenantID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// FindSessionByToken is the authentication boundary: only a verified token hash
// may establish the tenant before any business query or expired-session cleanup.
func (r *Repository) FindSessionByToken(ctx context.Context, tokenHash []byte) (model.User, model.Session, error) {
	var tenantID string
	err := r.DB.QueryRow(ctx, `SELECT tenant_id FROM sessions WHERE token_hash=$1`, tokenHash).Scan(&tenantID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.Session{}, errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, model.Session{}, fmt.Errorf("lookup session tenant: %w", err)
	}
	return r.FindSession(tenant.WithID(ctx, tenantID), tokenHash)
}
