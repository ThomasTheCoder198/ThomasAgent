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

// SessionRepo stores login sessions. Every method is scoped to the tenant in ctx, except the
// two "AcrossTenants" methods, which exist for authentication and the purge job.
type SessionRepo interface {
	CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error
	// CreateSessionInTx lets login write the session and its audit row in the caller's transaction.
	CreateSessionInTx(ctx context.Context, tx postgres.DBTX, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error
	FindSession(ctx context.Context, tokenHash []byte) (model.User, model.Session, error)
	FindSessionAcrossTenants(ctx context.Context, tokenHash []byte) (model.User, model.Session, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	PurgeExpiredSessions(ctx context.Context) (int64, error)
	PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error)
}

type sessionRepo struct {
	db            postgres.DBTX
	touchInterval time.Duration
}

// NewSessionRepo refreshes last_seen_at at most once per touchInterval; zero uses the default.
func NewSessionRepo(db postgres.DBTX, touchInterval time.Duration) SessionRepo {
	if touchInterval <= 0 {
		touchInterval = config.DefaultSessionTouchInterval
	}
	return &sessionRepo{db: db, touchInterval: touchInterval}
}

func (r *sessionRepo) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error {
	return r.CreateSessionInTx(ctx, r.db, userID, tokenHash, csrfToken, expiresAt)
}

func (r *sessionRepo) CreateSessionInTx(ctx context.Context, tx postgres.DBTX, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions (tenant_id, user_id, token_hash, csrf_token, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, userID, tokenHash, csrfToken, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindSession reads a session in the ctx tenant and refreshes last_seen_at at most once per touch interval.
func (r *sessionRepo) FindSession(ctx context.Context, tokenHash []byte) (model.User, model.Session, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return model.User{}, model.Session{}, err
	}
	var user model.User
	var session model.Session
	err = r.db.QueryRow(ctx, `WITH touched_session AS (
  UPDATE sessions SET last_seen_at=now()
  WHERE token_hash=$1 AND tenant_id=$2 AND expires_at>now()
   AND last_seen_at<now()-($3 * interval '1 second')
 ) SELECT s.id, s.tenant_id, s.user_id, s.csrf_token, s.expires_at, s.expires_at<=now(), u.id, u.email, u.display_name
  FROM sessions s JOIN users u ON u.id=s.user_id AND u.tenant_id=s.tenant_id
  WHERE s.token_hash=$1 AND s.tenant_id=$2`, tokenHash, tenantID, r.touchInterval.Seconds()).
		Scan(&session.ID, &session.TenantID, &session.UserID, &session.CSRFToken, &session.ExpiresAt, &session.Expired, &user.ID, &user.Email, &user.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.Session{}, errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, model.Session{}, fmt.Errorf("find session: %w", err)
	}
	return user, session, nil
}

// FindSessionAcrossTenants is the authentication boundary: only a verified token hash
// may establish the tenant before any business query or expired-session cleanup.
func (r *sessionRepo) FindSessionAcrossTenants(ctx context.Context, tokenHash []byte) (model.User, model.Session, error) {
	var tenantID string
	err := r.db.QueryRow(ctx, `SELECT tenant_id FROM sessions WHERE token_hash=$1`, tokenHash).Scan(&tenantID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.Session{}, errors.ErrNotFound
	}
	if err != nil {
		return model.User{}, model.Session{}, fmt.Errorf("lookup session tenant: %w", err)
	}
	return r.FindSession(tenant.WithID(ctx, tenantID), tokenHash)
}

func (r *sessionRepo) DeleteSession(ctx context.Context, tokenHash []byte) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND tenant_id = $2`, tokenHash, tenantID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *sessionRepo) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return 0, err
	}
	result, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE tenant_id=$1 AND expires_at<=now()`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}
	return result.RowsAffected(), nil
}

// PurgeExpiredSessionsAcrossTenants is reserved for the system job: request paths must
// continue using tenant-scoped repository methods.
func (r *sessionRepo) PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`)
	if err != nil {
		return 0, fmt.Errorf("purge all expired sessions: %w", err)
	}
	return result.RowsAffected(), nil
}
