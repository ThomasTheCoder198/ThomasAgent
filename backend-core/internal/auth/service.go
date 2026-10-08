package auth

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	auditActorOwner       = "owner"
	auditActionLogin      = "auth.login"
	auditActionLoginFail  = "auth.login.failed"
	auditTargetUser       = "user"
	auditTargetAuth       = "auth"
	ownerBootstrapLockKey = "core.auth.owner"
)

type transactionDB interface {
	postgres.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

// Service owns the owner account, password login and the session lifecycle.
type Service struct {
	repository *Repository
	db         transactionDB
	config     config.AuthConfig
	now        func() time.Time
	passwords  *passwordHasher
}

func NewService(repository *Repository, db transactionDB, authConfig config.AuthConfig, now func() time.Time) *Service {
	return &Service{
		repository: repository,
		db:         db,
		config:     authConfig,
		now:        now,
		passwords:  newPasswordHasher(authConfig.PasswordHash),
	}
}

func (s *Service) EnsureOwner(ctx context.Context) error {
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
	repository := NewRepository(tx)
	userCount, err := repository.CountUsers(ctx)
	if err != nil || userCount > 0 {
		return err
	}
	if err := s.createOwner(ctx, repository); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) createOwner(ctx context.Context, repository *Repository) error {
	email := NormalizeEmail(s.config.OwnerEmail)
	if email == "" || len(s.config.OwnerPassword) < s.config.MinPasswordLength {
		return fmt.Errorf("no owner exists: set CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD (min %d chars)", s.config.MinPasswordLength)
	}
	passwordHash, err := s.passwords.hash(ctx, s.config.OwnerPassword)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
	}
	_, err = repository.CreateUser(ctx, email, passwordHash)
	if stderrors.Is(err, errors.ErrConflict) {
		return nil
	}
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (User, IssuedSession, error) {
	if err := s.passwords.ready(); err != nil {
		return User{}, IssuedSession{}, err
	}
	user, passwordHash, err := s.repository.FindUserByEmail(ctx, NormalizeEmail(email))
	if stderrors.Is(err, errors.ErrNotFound) {
		if err := s.passwords.compareWithDecoy(ctx, password); err != nil {
			return User{}, IssuedSession{}, fmt.Errorf("compare timing hash: %w", err)
		}
		return User{}, IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	if err != nil {
		return User{}, IssuedSession{}, err
	}
	matches, err := s.passwords.compare(ctx, password, passwordHash)
	if err != nil {
		return User{}, IssuedSession{}, fmt.Errorf("compare password: %w", err)
	}
	if !matches {
		return User{}, IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	issued, err := s.issueSession(ctx, user)
	if err != nil {
		return User{}, IssuedSession{}, err
	}
	return user, issued, nil
}

func (s *Service) RecordFailedLogin(ctx context.Context) error {
	return audit.Record(ctx, s.db, audit.Event{Actor: auditActorOwner, Action: auditActionLoginFail, TargetType: auditTargetAuth})
}

// issueSession stores the session and its login audit row in one transaction.
func (s *Service) issueSession(ctx context.Context, user User) (session IssuedSession, resultErr error) {
	sessionToken, err := newRandomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	csrfToken, err := newRandomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	expiresAt := s.now().Add(s.config.SessionTTL)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !stderrors.Is(err, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.ErrInternalError.WithCause(stderrors.Join(resultErr, err))
		}
	}()
	if err := s.repository.CreateSessionInTx(ctx, tx, user.ID, hashSessionToken(sessionToken), csrfToken, expiresAt); err != nil {
		return IssuedSession{}, err
	}
	loginEvent := audit.Event{Actor: auditActorOwner, Action: auditActionLogin, TargetType: auditTargetUser, TargetID: user.ID.String()}
	if err := audit.Record(ctx, tx, loginEvent); err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	return IssuedSession{Token: sessionToken, CSRFToken: csrfToken, ExpiresAt: expiresAt}, nil
}

// Authenticate resolves the session cookie value to its user and session; the session row decides the tenant.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (User, Session, error) {
	if sessionToken == "" {
		return User{}, Session{}, errors.ErrUnauthenticated
	}
	tokenHash := hashSessionToken(sessionToken)
	user, session, err := s.repository.FindSessionAcrossTenants(ctx, tokenHash)
	if stderrors.Is(err, errors.ErrNotFound) {
		return User{}, Session{}, errors.ErrUnauthenticated
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if session.Expired {
		if err := s.repository.DeleteSession(tenant.WithID(ctx, session.TenantID), tokenHash); err != nil {
			return User{}, Session{}, errors.ErrInternalError.WithCause(err)
		}
		return User{}, Session{}, errors.ErrAuthSessionExpired
	}
	return user, session, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	return s.repository.DeleteSession(ctx, hashSessionToken(sessionToken))
}

func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.repository.PurgeExpiredSessions(ctx)
}

// PurgeExpiredSessionsAcrossTenants is for the periodic system job only.
func (s *Service) PurgeExpiredSessionsAcrossTenants(ctx context.Context) (int64, error) {
	return s.repository.PurgeExpiredSessionsAcrossTenants(ctx)
}
