package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/juantevez/my-ig/auth-service/internal/application/port/input"
	"github.com/juantevez/my-ig/auth-service/internal/domain/token"
	"github.com/juantevez/my-ig/auth-service/internal/domain/user"
	"github.com/juantevez/my-ig/auth-service/internal/infrastructure/http/middleware"
)

// AuthHandler is the HTTP adapter for the AuthUseCase driving port.
type AuthHandler struct {
	auth input.AuthUseCase
}

func NewAuthHandler(auth input.AuthUseCase) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// ── Register ─────────────────────────────────────────────────────────────────

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register handles POST /auth/register → 201 {user_id}
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username, email and password are required")
		return
	}
	if len(req.Password) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	result, err := h.auth.Register(r.Context(), input.RegisterCommand{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, user.ErrEmailAlreadyTaken):
			respondError(w, http.StatusConflict, "email already registered")
		default:
			respondError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"user_id": result.UserID.String()})
}

// ── Login ─────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login handles POST /auth/login → 200 {access_token, refresh_token}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	result, err := h.auth.Login(r.Context(), input.LoginCommand{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, user.ErrInvalidCredentials):
			respondError(w, http.StatusUnauthorized, "invalid credentials")
		default:
			respondError(w, http.StatusInternalServerError, "login failed")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
	})
}

// ── Refresh ───────────────────────────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh handles POST /auth/refresh → 200 {access_token, refresh_token}
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	result, err := h.auth.Refresh(r.Context(), input.RefreshCommand{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		switch {
		case errors.Is(err, token.ErrTokenExpired), errors.Is(err, token.ErrTokenInvalid):
			respondError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		default:
			respondError(w, http.StatusInternalServerError, "refresh failed")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
	})
}

// ── Logout ────────────────────────────────────────────────────────────────────

// Logout handles POST /auth/logout → 204
// Expects: Authorization: Bearer <access_token>
// The JTI is extracted from the Bearer token by the JWT middleware and
// stored in the request context. Here we just read it.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	jti, ok := middleware.JTIFromContext(r.Context())
	if !ok || jti == "" {
		respondError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	if err := h.auth.Logout(r.Context(), input.LogoutCommand{JTI: jti}); err != nil {
		respondError(w, http.StatusInternalServerError, "logout failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

type errorResponse struct {
	Error string `json:"error"`
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, errorResponse{Error: msg})
}
