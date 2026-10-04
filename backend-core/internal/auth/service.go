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

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/audit"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

const (
	tokenBytes     = 32
	actorOwner     = "owner"
	actionLogin    = "auth.login"
	targetTypeUser = "user"
	// A real hash of a throwaway password: comparing against it when the email is unknown
	// keeps the response time of "unknown email" and "wrong password" the same.
	timingDecoyPassword = "timing-decoy-password"
)

type Service struct {
	repo     *Repository
	db       postgres.DBTX
	cfg      config.AuthConfig
	now      func() time.Time
	decoy    string
	decoyErr error
}

func NewService(repo *Repository, db postgres.DBTX, cfg config.AuthConfig, now func() time.Time) *Service {
	decoy, err := argon2id.CreateHash(timingDecoyPassword, argon2id.DefaultParams)
	return &Service{repo: repo, db: db, cfg: cfg, now: now, decoy: decoy, decoyErr: err}
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *Service) EnsureOwner(ctx context.Context) error {
	if s.decoyErr != nil {
		return fmt.Errorf("initialize auth timing hash: %w", s.decoyErr)
	}
	n, err := s.repo.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	email := normalizeEmail(s.cfg.OwnerEmail)
	if email == "" || len(s.cfg.OwnerPassword) < s.cfg.MinPasswordLength {
		return fmt.Errorf("no owner exists: set CORE_AUTH_OWNER_EMAIL and CORE_AUTH_OWNER_PASSWORD (min %d chars)", s.cfg.MinPasswordLength)
	}
	hash, err := argon2id.CreateHash(s.cfg.OwnerPassword, argon2id.DefaultParams)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
	}
	_, err = s.repo.CreateUser(ctx, email, hash)
	return err
}

func (s *Service) Login(ctx context.Context, email, password string) (User, IssuedSession, error) {
	if s.decoyErr != nil {
		return User{}, IssuedSession{}, fmt.Errorf("initialize auth timing hash: %w", s.decoyErr)
	}
	user, hash, err := s.repo.FindUserByEmail(ctx, normalizeEmail(email))
	if stderrors.Is(err, errNotFound) {
		if _, err := argon2id.ComparePasswordAndHash(password, s.decoy); err != nil {
			return User{}, IssuedSession{}, fmt.Errorf("compare timing hash: %w", err)
		}
		return User{}, IssuedSession{}, errInvalidCredentials()
	}
	if err != nil {
		return User{}, IssuedSession{}, err
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
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
	if err := audit.Record(ctx, s.db, audit.Event{Actor: actorOwner, Action: actionLogin, TargetType: targetTypeUser, TargetID: user.ID.String()}); err != nil {
		cleanupErr := s.repo.DeleteSession(ctx, hashToken(issued.Token))
		return User{}, IssuedSession{}, errors.ErrInternalError.WithCause(stderrors.Join(err, cleanupErr))
	}
	return user, issued, nil
}

func (s *Service) issue(ctx context.Context, user User) (IssuedSession, error) {
	token, err := randomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return IssuedSession{}, err
	}
	expires := s.now().Add(s.cfg.SessionTTL)
	if err := s.repo.CreateSession(ctx, user.ID, hashToken(token), csrf, expires); err != nil {
		return IssuedSession{}, err
	}
	return IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (User, Session, error) {
	if rawToken == "" {
		return User{}, Session{}, errUnauthenticated()
	}
	user, sess, err := s.repo.FindSession(ctx, hashToken(rawToken))
	if stderrors.Is(err, errNotFound) {
		return User{}, Session{}, errUnauthenticated()
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if !s.now().Before(sess.ExpiresAt) {
		if err := s.repo.DeleteSession(ctx, hashToken(rawToken)); err != nil {
			return User{}, Session{}, errors.ErrInternalError.WithCause(err)
		}
		return User{}, Session{}, errSessionExpired()
	}
	return user, sess, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.repo.DeleteSession(ctx, hashToken(rawToken))
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
