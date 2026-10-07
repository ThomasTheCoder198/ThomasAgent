package dto

import (
	"time"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type LoginResponse struct {
	User      UserResponse `json:"user"`
	CSRFToken string       `json:"csrfToken"`
	ExpiresAt string       `json:"expiresAt"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}

type LogoutResponse struct {
	LoggedOut bool `json:"loggedOut"`
}

func FromUser(user model.User) UserResponse {
	return UserResponse{ID: user.ID.String(), Email: user.Email, DisplayName: user.DisplayName}
}

func FromIssuedSession(user model.User, session model.IssuedSession) LoginResponse {
	return LoginResponse{User: FromUser(user), CSRFToken: session.CSRFToken, ExpiresAt: session.ExpiresAt.Format(time.RFC3339)}
}
