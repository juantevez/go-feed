package handler

import (
	"net/http"

	"github.com/tuusuario/my-ig/auth-service/internal/application/port/input"
)

// AuthHandler is the HTTP adapter for the AuthUseCase driving port.
type AuthHandler struct {
	auth input.AuthUseCase
}

func NewAuthHandler(auth input.AuthUseCase) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Register handles POST /auth/register
// TODO: decode body → RegisterCommand → call use case → respond 201
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Login handles POST /auth/login
// TODO: decode body → LoginCommand → call use case → respond 200 with token pair
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Refresh handles POST /auth/refresh
// TODO: decode body → RefreshCommand → call use case → respond 200 with new pair
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Logout handles POST /auth/logout
// TODO: extract JTI from bearer → LogoutCommand → call use case → respond 204
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
