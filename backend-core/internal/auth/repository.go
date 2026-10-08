package auth

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// Repository stores users and sessions. Every method is scoped to the tenant in ctx,
// except the two "AcrossTenants" methods, which exist for authentication and the purge job.
type Repository struct {
	db                   postgres.DBTX
	sessionTouchInterval time.Duration
}

func NewRepository(db postgres.DBTX) *Repository {
	return NewRepositoryWithSessionTouchInterval(db, config.DefaultSessionTouchInterval)
}

func NewRepositoryWithSessionTouchInterval(db postgres.DBTX, interval time.Duration) *Repository {
	if interval <= 0 {
		interval = config.DefaultSessionTouchInterval
	}
	return &Repository{db: db, sessionTouchInterval: interval}
}

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
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

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, err
	}
	user := User{Email: email}
	err = r.db.QueryRow(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, email) DO NOTHING RETURNING id, display_name`,
		email, passwordHash, tenantID).Scan(&user.ID, &user.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, errors.ErrConflict
	}
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// FindUserByEmail returns the user and the stored Argon2id password hash.
func (r *Repository) FindUserByEmail(ctx context.Context, email string) (User, string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, "", err
	}
	var user User
	var passwordHash string
	err = r.db.QueryRow(ctx, `SELECT id, email, display_name, password_hash FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID).
		Scan(&user.ID, &user.Email, &user.DisplayName, &passwordHash)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, "", errors.ErrNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("find user: %w", err)
	}
	return user, passwordHash, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error {
	return r.CreateSessionInTx(ctx, r.db, userID, tokenHash, csrfToken, expiresAt)
}

// CreateSessionInTx lets login write the session and its audit row in the caller's transaction.
func (r *Repository) CreateSessionInTx(ctx context.Context, tx postgres.DBTX, userID uuid.UUID, tokenHash []byte, csrfToken string, expiresAt time.Time) error {
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
func (r *Repository) FindSession(ctx context.Context, tokenHash []byte) (User, Session, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, Session{}, err
	}
	var user User
	var session Session
	err = r.db.QueryRow(ctx, `WITH touched_session AS (
  UPDATE sessions SET last_seen_at=now()
  WHERE token_hash=$1 AND tenant_id=$2 AND expires_at>now()
   AND last_seen_at<now()-($3 * interval '1 second')
 ) SELECT s.id, s.tenant_id, s.user_id, s.csrf_token, s.expires_at, s.expires_at<=now(), u.id, u.email, u.display_name
  FROM sessions s JOIN users u ON u.id=s.user_id AND u.tenant_id=s.tenant_id
  WHERE s.token_hash=$1 AND s.tenant_id=$2`, tokenHash, tenantID, r.sessionTouchInterval.Seconds()).
		Scan(&session.ID, &session.TenantID, &session.UserID, &session.CSRFToken, &session.ExpiresAt, &session.Expired, &user.ID, &user.Email, &user.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, errors.ErrNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find session: %w", err)
	}
	return user, session, nil
}

// FindSessionAcrossTenants is the authentication boundary: only a verified token hash
// may establish the tenant before any business query or expired-session cleanup.
func (r *Repository) FindSessionAcrossTenants(ctx context.Context, tokenHash []byte) (User, Session, error) {
	var tenantID string
	err := r.db.QueryRow(ctx, `SELECT tenant_id FROM sessions WHERE token_hash=$1`, tokenHash).Scan(&tenantID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, errors.ErrNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("lookup session tenant: %w", err)
	}
	return r.FindSession(tenant.WithID(ctx, tenantID), tokenHash)
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND tenant_id = $2`, tokenHash, tenantID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *Repository) PurgeExpiredSessions(ctx context.Context) (int64, error) {
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
func (r *Repository) PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`)
	if err != nil {
		return 0, fmt.Errorf("purge all expired sessions: %w", err)
	}
	return result.RowsAffected(), nil
}
