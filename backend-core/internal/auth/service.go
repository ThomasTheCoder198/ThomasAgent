package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/tenant"
)

const (
	tokenBytes        = 32
	actorOwner        = "owner"
	actionLogin       = "auth.login"
	actionLoginFailed = "auth.login.failed"
	targetTypeUser    = "user"
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
	repo        *Repository
	db          transactionDB
	cfg         config.AuthConfig
	now         func() time.Time
	decoy       string
	decoyErr    error
	argonParams *argon2id.Params
	hashSlots   chan struct{}
}

func NewService(repo *Repository, db transactionDB, cfg config.AuthConfig, now func() time.Time) *Service {
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

func (s *Service) createHash(ctx context.Context, password string) (string, error) {
	ctx, span := otel.Tracer(TracerName).Start(ctx, argonCreateSpan)
	defer span.End()
	if err := s.acquireHashSlot(ctx); err != nil {
		span.RecordError(err)
		return "", err
	}
	defer func() { <-s.hashSlots }()
	return argon2id.CreateHash(password, s.argonParams)
}

func (s *Service) compareHash(ctx context.Context, password, encoded string) (bool, error) {
	ctx, span := otel.Tracer(TracerName).Start(ctx, argonCompareSpan)
	defer span.End()
	if err := s.acquireHashSlot(ctx); err != nil {
		span.RecordError(err)
		return false, err
	}
	defer func() { <-s.hashSlots }()
	return argon2id.ComparePasswordAndHash(password, encoded)
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

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

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
	repo := NewRepository(tx)
	n, err := repo.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if err := s.createOwner(ctx, repo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) createOwner(ctx context.Context, repo *Repository) error {
	email := normalizeEmail(s.cfg.OwnerEmail)
	if email == "" || len(s.cfg.OwnerPassword) < s.cfg.MinPasswordLength {
		return fmt.Errorf("no owner exists: set CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD (min %d chars)", s.cfg.MinPasswordLength)
	}
	hash, err := s.createHash(ctx, s.cfg.OwnerPassword)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
	}
	_, err = repo.CreateUser(ctx, email, hash)
	if stderrors.Is(err, errAlreadyExists) {
		return nil
	}
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (User, IssuedSession, error) {
	if s.decoyErr != nil {
		return User{}, IssuedSession{}, fmt.Errorf("initialize auth timing hash: %w", s.decoyErr)
	}
	user, hash, err := s.repo.FindUserByEmail(ctx, normalizeEmail(email))
	if stderrors.Is(err, errNotFound) {
		if _, err := s.compareHash(ctx, password, s.decoy); err != nil {
			return User{}, IssuedSession{}, fmt.Errorf("compare timing hash: %w", err)
		}
		return User{}, IssuedSession{}, errInvalidCredentials()
	}
	if err != nil {
		return User{}, IssuedSession{}, err
	}
	ok, err := s.compareHash(ctx, password, hash)
	if err != nil {
		return User{}, IssuedSession{}, fmt.Errorf("compare password: %w", err)
	}
	if !ok {
		return User{}, IssuedSession{}, errInvalidCredentials()
	}
	issued, err := s.issue(ctx, user)
	if err != nil {
		return User{}, IssuedSession{}, err
	}
	return user, issued, nil
}

func (s *Service) recordFailedLogin(ctx context.Context) error {
	return audit.Record(ctx, s.db, audit.Event{Actor: actorOwner, Action: actionLoginFailed, TargetType: "auth"})
}

func (s *Service) issue(ctx context.Context, user User) (session IssuedSession, resultErr error) {
	token, err := randomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	expires := s.now().Add(s.cfg.SessionTTL)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !stderrors.Is(err, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.ErrInternalError.WithCause(stderrors.Join(resultErr, err))
		}
	}()
	if err := s.repo.createSession(ctx, tx, user.ID, hashToken(token), csrf, expires); err != nil {
		return IssuedSession{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{Actor: actorOwner, Action: actionLogin, TargetType: targetTypeUser, TargetID: user.ID.String()}); err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedSession{}, errors.ErrInternalError.WithCause(err)
	}
	return IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (User, Session, error) {
	if rawToken == "" {
		return User{}, Session{}, errUnauthenticated()
	}
	user, sess, err := s.repo.FindSessionByToken(ctx, hashToken(rawToken))
	if stderrors.Is(err, errNotFound) {
		return User{}, Session{}, errUnauthenticated()
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if sess.Expired {
		if err := s.repo.DeleteSession(tenant.WithID(ctx, sess.TenantID), hashToken(rawToken)); err != nil {
			return User{}, Session{}, errors.ErrInternalError.WithCause(err)
		}
		return User{}, Session{}, errSessionExpired()
	}
	return user, sess, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.repo.DeleteSession(ctx, hashToken(rawToken))
}

func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.PurgeExpiredSessions(ctx)
}

func (s *Service) PurgeAllExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.PurgeAllExpiredSessions(ctx)
}

func randomToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
