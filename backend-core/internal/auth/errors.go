package auth

import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/errors"

func errInvalidCredentials() error { return errors.New(errors.CodeAuthInvalidCredentials) }
func errSessionExpired() error     { return errors.New(errors.CodeAuthSessionExpired) }
func errUnauthenticated() error    { return errors.New(errors.CodeUnauthenticated) }
