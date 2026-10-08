package dto

import (
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse returns the CSRF token in the body too, for clients that cannot read the cookie.
type LoginResponse struct {
	User      UserResponse `json:"user"`
	CSRFToken string       `json:"csrfToken"`
	ExpiresAt string       `json:"expiresAt"`
}

func NewLoginResponse(user model.User, session model.IssuedSession) LoginResponse {
	return LoginResponse{User: NewUserResponse(user), CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt.Format(time.RFC3339)}
}

type LogoutResponse struct {
	LoggedOut bool `json:"loggedOut"`
}
