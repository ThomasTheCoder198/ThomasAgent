package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

// Audit vocabulary of this domain; the values are stored in audit_events and must not change.
const (
	auditActorOwner      = "owner"
	auditActionLogin     = "auth.login"
	AuditActionLoginFail = "auth.login.failed"
	auditTargetUser      = "user"
	auditTargetAuth      = "auth"
)

const ownerBootstrapLockKey = "core.auth.owner"

// AuthService owns the owner account, password login and the session lifecycle.
type AuthService interface {
	// EnsureOwner creates the configured owner once, even when several processes start together.
	EnsureOwner(ctx context.Context) error
	Login(ctx context.Context, email, password string) (model.User, model.IssuedSession, error)
	// Authenticate resolves the session cookie value; the session row decides the tenant.
	Authenticate(ctx context.Context, sessionToken string) (model.User, model.Session, error)
	Logout(ctx context.Context, sessionToken string) error
	RecordFailedLogin(ctx context.Context) error
	PurgeExpiredSessions(ctx context.Context) (int64, error)
	// PurgeExpiredSessionsAcrossTenants is for the periodic system job only.
	PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error)
}

// TransactionDB is the pool: it runs queries and opens transactions.
type TransactionDB interface {
	postgres.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type AuthServiceImpl struct {
	users     repository.UserRepo
	sessions  repository.SessionRepo
	db        TransactionDB
	config    config.AuthConfig
	now       func() time.Time
	passwords *passwordHasher
}

func NewAuthService(users repository.UserRepo, sessions repository.SessionRepo, db TransactionDB, authConfig config.AuthConfig, now func() time.Time) AuthService {
	return &AuthServiceImpl{
		users:     users,
		sessions:  sessions,
		db:        db,
		config:    authConfig,
		now:       now,
		passwords: newPasswordHasher(authConfig.PasswordHash),
	}
}

func (s *AuthServiceImpl) EnsureOwner(ctx context.Context) error {
	if err := s.passwords.ready(); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize bootstrap across processes even when their configured owner emails differ.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, ownerBootstrapLockKey); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	users := repository.NewUserRepo(tx)
	userCount, err := users.CountUsers(ctx)
	if err != nil || userCount > 0 {
		return err
	}
	if err := s.createOwner(ctx, users); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AuthServiceImpl) createOwner(ctx context.Context, users repository.UserRepo) error {
	email := model.NormalizeEmail(s.config.OwnerEmail)
	if email == "" || len(s.config.OwnerPassword) < s.config.MinPasswordLength {
		return fmt.Errorf("no owner exists: set CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD (min %d chars)", s.config.MinPasswordLength)
	}
	passwordHash, err := s.passwords.hash(ctx, s.config.OwnerPassword)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
	}
	_, err = users.CreateUser(ctx, email, passwordHash)
	if stderrors.Is(err, errors.ErrConflict) {
		return nil
	}
	return err
}

func (s *AuthServiceImpl) Login(ctx context.Context, email, password string) (model.User, model.IssuedSession, error) {
	if err := s.passwords.ready(); err != nil {
		return model.User{}, model.IssuedSession{}, err
	}
	user, passwordHash, err := s.users.FindUserByEmail(ctx, model.NormalizeEmail(email))
	if stderrors.Is(err, errors.ErrNotFound) {
		if err := s.passwords.compareWithDecoy(ctx, password); err != nil {
			return model.User{}, model.IssuedSession{}, fmt.Errorf("compare timing hash: %w", err)
		}
		return model.User{}, model.IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	if err != nil {
		return model.User{}, model.IssuedSession{}, err
	}
	matches, err := s.passwords.compare(ctx, password, passwordHash)
	if err != nil {
		return model.User{}, model.IssuedSession{}, fmt.Errorf("compare password: %w", err)
	}
	if !matches {
		return model.User{}, model.IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	issued, err := s.issueSession(ctx, user)
	if err != nil {
		return model.User{}, model.IssuedSession{}, err
	}
	return user, issued, nil
}

func (s *AuthServiceImpl) RecordFailedLogin(ctx context.Context) error {
	return audit.Record(ctx, s.db, audit.Event{Actor: auditActorOwner, Action: AuditActionLoginFail, TargetType: auditTargetAuth})
}

// issueSession stores the session and its login audit row in one transaction; a failed rollback
// keeps both causes.
func (s *AuthServiceImpl) issueSession(ctx context.Context, user model.User) (session model.IssuedSession, resultErr error) {
	sessionToken, err := newRandomToken()
	if err != nil {
		return model.IssuedSession{}, err
	}
	csrfToken, err := newRandomToken()
	if err != nil {
		return model.IssuedSession{}, err
	}
	expiresAt := s.now().Add(s.config.SessionTTL)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !stderrors.Is(err, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.ErrInternalError.WithCause(stderrors.Join(resultErr, err))
		}
	}()
	if err := s.sessions.CreateSessionInTx(ctx, tx, user.ID, model.HashSessionToken(sessionToken), csrfToken, expiresAt); err != nil {
		return model.IssuedSession{}, err
	}
	loginEvent := audit.Event{Actor: auditActorOwner, Action: auditActionLogin, TargetType: auditTargetUser, TargetID: user.ID.String()}
	if err := audit.Record(ctx, tx, loginEvent); err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	return model.IssuedSession{Token: sessionToken, CSRFToken: csrfToken, ExpiresAt: expiresAt}, nil
}

func (s *AuthServiceImpl) Authenticate(ctx context.Context, sessionToken string) (model.User, model.Session, error) {
	if sessionToken == "" {
		return model.User{}, model.Session{}, errors.ErrUnauthenticated
	}
	tokenHash := model.HashSessionToken(sessionToken)
	user, session, err := s.sessions.FindSessionAcrossTenants(ctx, tokenHash)
	if stderrors.Is(err, errors.ErrNotFound) {
		return model.User{}, model.Session{}, errors.ErrUnauthenticated
	}
	if err != nil {
		return model.User{}, model.Session{}, err
	}
	if session.Expired {
		if err := s.sessions.DeleteSession(tenant.WithID(ctx, session.TenantID), tokenHash); err != nil {
			return model.User{}, model.Session{}, errors.ErrInternalError.WithCause(err)
		}
		return model.User{}, model.Session{}, errors.ErrAuthSessionExpired
	}
	return user, session, nil
}

func (s *AuthServiceImpl) Logout(ctx context.Context, sessionToken string) error {
	return s.sessions.DeleteSession(ctx, model.HashSessionToken(sessionToken))
}

func (s *AuthServiceImpl) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.PurgeExpiredSessions(ctx)
}

func (s *AuthServiceImpl) PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error) {
	return s.sessions.PurgeExpiredSessionsAcrossTenants(ctx)
}
