package dto

import "github.com/thomasthecoder198/thomastheragx/backend-core/internal/auth/model"

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

func NewUserResponse(user model.User) UserResponse {
	return UserResponse{ID: user.ID.String(), Email: user.Email, DisplayName: user.DisplayName}
}

type MeResponse struct {
	User UserResponse `json:"user"`
}
