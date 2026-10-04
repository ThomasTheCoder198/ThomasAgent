package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres"
)

var errNotFound = errors.New("auth: not found")

type Repository struct{ db postgres.DBTX }

func NewRepository(db postgres.DBTX) *Repository { return &Repository{db: db} }

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	u := User{Email: email}
	err := r.db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, display_name`,
		email, passwordHash).Scan(&u.ID, &u.DisplayName)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.db.QueryRow(ctx, `SELECT id, email, display_name, password_hash FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", errNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("find user: %w", err)
	}
	return u, hash, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrf string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tokenHash, csrf, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *Repository) FindSession(ctx context.Context, tokenHash []byte) (User, Session, error) {
	var u User
	var s Session
	err := r.db.QueryRow(ctx, `SELECT s.id, s.user_id, s.csrf_token, s.expires_at, u.id, u.email, u.display_name
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`, tokenHash).
		Scan(&s.ID, &s.UserID, &s.CSRFToken, &s.ExpiresAt, &u.ID, &u.Email, &u.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, errNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find session: %w", err)
	}
	return u, s, nil
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
