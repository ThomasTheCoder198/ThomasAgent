package auth

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

var (
	errNotFound      = stderrors.New("auth: not found")
	errAlreadyExists = stderrors.New("auth: already exists")
)

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
	var n int
	if err = r.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, err
	}
	u := User{Email: email}
	err = r.db.QueryRow(ctx, `INSERT INTO users (tenant_id, email, password_hash) VALUES ($3, $1, $2)
		ON CONFLICT (tenant_id, email) DO NOTHING RETURNING id, display_name`,
		email, passwordHash, tenantID).Scan(&u.ID, &u.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, errAlreadyExists
	}
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (User, string, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, "", err
	}
	var u User
	var hash string
	err = r.db.QueryRow(ctx, `SELECT id, email, display_name, password_hash FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID).
		Scan(&u.ID, &u.Email, &u.DisplayName, &hash)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, "", errNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("find user: %w", err)
	}
	return u, hash, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrf string, expiresAt time.Time) error {
	return r.createSession(ctx, r.db, userID, tokenHash, csrf, expiresAt)
}

func (r *Repository) createSession(ctx context.Context, db postgres.DBTX, userID uuid.UUID, tokenHash []byte, csrf string, expiresAt time.Time) error {
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

func (r *Repository) FindSession(ctx context.Context, tokenHash []byte) (User, Session, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return User{}, Session{}, err
	}
	var u User
	var s Session
	err = r.db.QueryRow(ctx, `WITH touched_session AS (
  UPDATE sessions SET last_seen_at=now()
  WHERE token_hash=$1 AND tenant_id=$2 AND expires_at>now()
   AND last_seen_at<now()-($3 * interval '1 second')
 ) SELECT s.id, s.tenant_id, s.user_id, s.csrf_token, s.expires_at, s.expires_at<=now(), u.id, u.email, u.display_name
  FROM sessions s JOIN users u ON u.id=s.user_id AND u.tenant_id=s.tenant_id
  WHERE s.token_hash=$1 AND s.tenant_id=$2`, tokenHash, tenantID, r.sessionTouchInterval.Seconds()).
		Scan(&s.ID, &s.TenantID, &s.UserID, &s.CSRFToken, &s.ExpiresAt, &s.Expired, &u.ID, &u.Email, &u.DisplayName)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, errNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find session: %w", err)
	}
	return u, s, nil
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

// PurgeAllExpiredSessions is reserved for the system job: request paths must
// continue using tenant-scoped repository methods.
func (r *Repository) PurgeAllExpiredSessions(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`)
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
	if _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND tenant_id = $2`, tokenHash, tenantID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// FindSessionByToken is the authentication boundary: only a verified token hash
// may establish the tenant before any business query or expired-session cleanup.
func (r *Repository) FindSessionByToken(ctx context.Context, tokenHash []byte) (User, Session, error) {
	var tenantID string
	err := r.db.QueryRow(ctx, `SELECT tenant_id FROM sessions WHERE token_hash=$1`, tokenHash).Scan(&tenantID)
	if stderrors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, errNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("lookup session tenant: %w", err)
	}
	return r.FindSession(tenant.WithID(ctx, tenantID), tokenHash)
}
