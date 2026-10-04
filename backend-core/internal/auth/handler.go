package auth

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/httpx"
)

const maxLoginBodyBytes = 4 << 10

type Handler struct {
	svc          *Service
	lim          *Limiter
	cookieSecure bool
}

func NewHandler(svc *Service, lim *Limiter, cookieSecure bool) *Handler {
	return &Handler{svc: svc, lim: lim, cookieSecure: cookieSecure}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type loginResponse struct {
	User      userDTO `json:"user"`
	CSRFToken string  `json:"csrfToken"`
	ExpiresAt string  `json:"expiresAt"`
}

func toDTO(u User) userDTO {
	return userDTO{ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName}
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/v1/auth/login", h.login)
	r.Group(func(pr chi.Router) {
		pr.Use(RequireSession(h.svc))
		pr.Post("/api/v1/auth/logout", h.logout)
		pr.Get("/api/v1/auth/me", h.me)
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err := h.lim.Allow(r.Context(), host); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req, maxLoginBodyBytes); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	user, issued, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.setCookies(w, issued)
	httpx.WriteSuccess(w, r, http.StatusOK, loginResponse{User: toDTO(user), CSRFToken: issued.CSRFToken, ExpiresAt: issued.ExpiresAt.Format(time.RFC3339)})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(CookieSession)
	if err := h.svc.Logout(r.Context(), c.Value); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.clearCookies(w)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]bool{"loggedOut": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFrom(r.Context())
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]userDTO{"user": toDTO(user)})
}

func (h *Handler) setCookies(w http.ResponseWriter, s IssuedSession) {
	http.SetCookie(w, &http.Cookie{Name: CookieSession, Value: s.Token, Path: "/", Expires: s.ExpiresAt, HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: CookieCSRF, Value: s.CSRFToken, Path: "/", Expires: s.ExpiresAt, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{CookieSession, CookieCSRF} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == CookieSession, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
	}
}
