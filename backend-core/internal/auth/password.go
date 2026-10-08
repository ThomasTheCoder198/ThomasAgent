package auth

import (
	"context"
	"fmt"

	"github.com/alexedwards/argon2id"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
)

// A real hash of a throwaway password: comparing against it when the email is unknown
// keeps the response time of "unknown email" and "wrong password" the same.
const timingDecoyPassword = "timing-decoy-password"

// passwordHasher runs Argon2id under a concurrency budget so login bursts cannot exhaust memory.
type passwordHasher struct {
	params   *argon2id.Params
	slots    chan struct{}
	decoy    string
	decoyErr error
}

func newPasswordHasher(settings config.PasswordHashConfig) *passwordHasher {
	settings = withPasswordHashDefaults(settings)
	hasher := &passwordHasher{
		params: &argon2id.Params{
			Iterations:  settings.Iterations,
			Memory:      settings.MemoryKiB,
			Parallelism: settings.Parallelism,
			SaltLength:  settings.SaltLength,
			KeyLength:   settings.KeyLength,
		},
		slots: make(chan struct{}, settings.MaxConcurrency),
	}
	hasher.decoy, hasher.decoyErr = hasher.hash(context.Background(), timingDecoyPassword)
	return hasher
}

func withPasswordHashDefaults(settings config.PasswordHashConfig) config.PasswordHashConfig {
	if settings.Iterations < config.MinimumArgonIterations {
		settings.Iterations = config.MinimumArgonIterations
	}
	if settings.MemoryKiB != config.ArgonMemoryKiB {
		settings.MemoryKiB = config.ArgonMemoryKiB
	}
	if settings.MaxConcurrency < 1 {
		settings.MaxConcurrency = config.DefaultArgonConcurrency
	}
	if settings.Parallelism == 0 {
		settings.Parallelism = config.DefaultArgonParallelism
	}
	if settings.SaltLength == 0 {
		settings.SaltLength = config.DefaultArgonSaltLength
	}
	if settings.KeyLength == 0 {
		settings.KeyLength = config.DefaultArgonKeyLength
	}
	return settings
}

// ready reports whether the timing decoy exists; without it unknown emails would answer faster.
func (h *passwordHasher) ready() error {
	if h.decoyErr != nil {
		return fmt.Errorf("initialize auth timing hash: %w", h.decoyErr)
	}
	return nil
}

func (h *passwordHasher) hash(ctx context.Context, password string) (string, error) {
	if err := h.acquireSlot(ctx); err != nil {
		return "", err
	}
	defer h.releaseSlot()
	return argon2id.CreateHash(password, h.params)
}

func (h *passwordHasher) compare(ctx context.Context, password, encodedHash string) (bool, error) {
	if err := h.acquireSlot(ctx); err != nil {
		return false, err
	}
	defer h.releaseSlot()
	return argon2id.ComparePasswordAndHash(password, encodedHash)
}

func (h *passwordHasher) compareWithDecoy(ctx context.Context, password string) error {
	_, err := h.compare(ctx, password, h.decoy)
	return err
}

func (h *passwordHasher) acquireSlot(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.ErrDependencyTimeout.WithCause(err)
	}
	select {
	case <-ctx.Done():
		return errors.ErrDependencyTimeout.WithCause(ctx.Err())
	case h.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			h.releaseSlot()
			return errors.ErrDependencyTimeout.WithCause(err)
		}
		return nil
	}
}

func (h *passwordHasher) releaseSlot() { <-h.slots }
