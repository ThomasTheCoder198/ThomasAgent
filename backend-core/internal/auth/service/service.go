package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/repository"
	authtoken "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/token"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	actorOwner     = "owner"
	actionLogin    = "auth.login"
	targetTypeUser = "user"
	// A real hash of a throwaway password: comparing against it when the email is unknown
	// keeps the response time of "unknown email" and "wrong password" the same.
	timingDecoyPassword = "timing-decoy-password"
	ownerBootstrapLock  = "core.auth.owner"
)

type transactionDB interface {
	postgres.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

type Service struct {
	repo        repository.AuthRepository
	db          transactionDB
	cfg         config.AuthConfig
	now         func() time.Time
	decoy       string
	decoyErr    error
	argonParams *argon2id.Params
	hashSlots   chan struct{}
}

var _ AuthService = (*Service)(nil)

func NewAuthService(repo repository.AuthRepository, db transactionDB, cfg config.AuthConfig, now func() time.Time) *Service {
	if cfg.PasswordHash.Iterations < config.MinimumArgonIterations {
		cfg.PasswordHash.Iterations = config.MinimumArgonIterations
	}
	if cfg.PasswordHash.MemoryKiB != config.ArgonMemoryKiB {
		cfg.PasswordHash.MemoryKiB = config.ArgonMemoryKiB
	}
	if cfg.PasswordHash.MaxConcurrency < 1 {
		cfg.PasswordHash.MaxConcurrency = config.DefaultArgonConcurrency
	}
	if cfg.PasswordHash.Parallelism == 0 {
		cfg.PasswordHash.Parallelism = config.DefaultArgonParallelism
	}
	if cfg.PasswordHash.SaltLength == 0 {
		cfg.PasswordHash.SaltLength = config.DefaultArgonSaltLength
	}
	if cfg.PasswordHash.KeyLength == 0 {
		cfg.PasswordHash.KeyLength = config.DefaultArgonKeyLength
	}
	s := &Service{repo: repo, db: db, cfg: cfg, now: now,
		argonParams: &argon2id.Params{Iterations: cfg.PasswordHash.Iterations, Memory: cfg.PasswordHash.MemoryKiB, Parallelism: cfg.PasswordHash.Parallelism, SaltLength: cfg.PasswordHash.SaltLength, KeyLength: cfg.PasswordHash.KeyLength},
		hashSlots:   make(chan struct{}, cfg.PasswordHash.MaxConcurrency)}
	decoy, err := s.createHash(context.Background(), timingDecoyPassword)
	s.decoy, s.decoyErr = decoy, err
	return s
}

func (s *Service) acquireHashSlot(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.ErrDependencyTimeout.WithCause(err)
	}
	select {
	case <-ctx.Done():
		return errors.ErrDependencyTimeout.WithCause(ctx.Err())
	case s.hashSlots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-s.hashSlots
			return errors.ErrDependencyTimeout.WithCause(err)
		}
		return nil
	}
}

func (s *Service) EnsureOwner(ctx context.Context) error {
	if s.decoyErr != nil {
		return fmt.Errorf("initialize auth timing hash: %w", s.decoyErr)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize bootstrap across processes even when their configured owner emails differ.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, ownerBootstrapLock); err != nil {
		return errors.ErrInternalError.WithCause(err)
	}
	repo := repository.NewAuthRepository(tx)
	n, err := repo.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if err := s.createOwner(ctx, repo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) createOwner(ctx context.Context, repo repository.AuthRepository) error {
	email := model.NormalizeEmail(s.cfg.OwnerEmail)
	if email == "" || len(s.cfg.OwnerPassword) < s.cfg.MinPasswordLength {
		return fmt.Errorf("no owner exists: set CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD (min %d chars)", s.cfg.MinPasswordLength)
	}
	hash, err := s.createHash(ctx, s.cfg.OwnerPassword)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
	}
	_, err = repo.CreateUser(ctx, email, hash)
	if stderrors.Is(err, errors.ErrConflict) {
		return nil
	}
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (model.User, model.IssuedSession, error) {
	if s.decoyErr != nil {
		return model.User{}, model.IssuedSession{}, fmt.Errorf("initialize auth timing hash: %w", s.decoyErr)
	}
	user, hash, err := s.repo.FindUserByEmail(ctx, model.NormalizeEmail(email))
	if stderrors.Is(err, errors.ErrNotFound) {
		if _, err := s.compareHash(ctx, password, s.decoy); err != nil {
			return model.User{}, model.IssuedSession{}, fmt.Errorf("compare timing hash: %w", err)
		}
		return model.User{}, model.IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	if err != nil {
		return model.User{}, model.IssuedSession{}, err
	}
	ok, err := s.compareHash(ctx, password, hash)
	if err != nil {
		return model.User{}, model.IssuedSession{}, fmt.Errorf("compare password: %w", err)
	}
	if !ok {
		return model.User{}, model.IssuedSession{}, errors.ErrAuthInvalidCredentials
	}
	issued, err := s.issue(ctx, user)
	if err != nil {
		return model.User{}, model.IssuedSession{}, err
	}
	return user, issued, nil
}

func (s *Service) RecordFailedLogin(ctx context.Context) error {
	return audit.Record(ctx, s.db, audit.Event{Actor: actorOwner, Action: model.ActionLoginFailed, TargetType: "auth"})
}

func (s *Service) issue(ctx context.Context, user model.User) (session model.IssuedSession, resultErr error) {
	token, err := randomToken()
	if err != nil {
		return model.IssuedSession{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return model.IssuedSession{}, err
	}
	expires := s.now().Add(s.cfg.SessionTTL)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !stderrors.Is(err, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.ErrInternalError.WithCause(stderrors.Join(resultErr, err))
		}
	}()
	if err := s.repo.CreateSessionIn(ctx, tx, user.ID, authtoken.Hash(token), csrf, expires); err != nil {
		return model.IssuedSession{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionLogin, TargetType: targetTypeUser, TargetID: user.ID.String()}); err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	return model.IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (model.User, model.Session, error) {
	if rawToken == "" {
		return model.User{}, model.Session{}, errors.ErrUnauthenticated
	}
	user, sess, err := s.repo.FindSessionByToken(ctx, authtoken.Hash(rawToken))
	if stderrors.Is(err, errors.ErrNotFound) {
		return model.User{}, model.Session{}, errors.ErrUnauthenticated
	}
	if err != nil {
		return model.User{}, model.Session{}, err
	}
	if sess.Expired {
		if err := s.repo.DeleteSession(tenant.WithID(ctx, sess.TenantID), authtoken.Hash(rawToken)); err != nil {
			return model.User{}, model.Session{}, errors.ErrInternalError.WithCause(err)
		}
		return model.User{}, model.Session{}, errors.ErrAuthSessionExpired
	}
	return user, sess, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.repo.DeleteSession(ctx, authtoken.Hash(rawToken))
}

func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.PurgeExpiredSessions(ctx)
}

func (s *Service) PurgeAllExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.PurgeAllExpiredSessions(ctx)
}

func randomToken() (string, error) {
	b := make([]byte, authtoken.Bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Service) createHash(ctx context.Context, password string) (string, error) {
	if err := s.acquireHashSlot(ctx); err != nil {
		return "", err
	}
	defer func() { <-s.hashSlots }()
	return argon2id.CreateHash(password, s.argonParams)
}

func (s *Service) compareHash(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquireHashSlot(ctx); err != nil {
		return false, err
	}
	defer func() { <-s.hashSlots }()
	return argon2id.ComparePasswordAndHash(password, encoded)
}
