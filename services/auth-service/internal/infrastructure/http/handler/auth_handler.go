package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
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
	reqID := chimiddleware.GetReqID(r.Context())

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "register: invalid body", "request_id", reqID, "err", err)
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

	slog.InfoContext(r.Context(), "register: attempt", "request_id", reqID, "email", req.Email, "username", req.Username)

	result, err := h.auth.Register(r.Context(), input.RegisterCommand{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		var invalid *user.ValidationError
		switch {
		case errors.As(err, &invalid):
			slog.WarnContext(r.Context(), "register: invalid input", "request_id", reqID, "field", invalid.Field, "err", invalid.Msg)
			respondError(w, http.StatusBadRequest, invalid.Msg)
		case errors.Is(err, user.ErrEmailAlreadyTaken):
			slog.WarnContext(r.Context(), "register: email already taken", "request_id", reqID, "email", req.Email)
			respondError(w, http.StatusConflict, "email already registered")
		default:
			slog.ErrorContext(r.Context(), "register: unexpected error", "request_id", reqID, "email", req.Email, "err", err)
			respondError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	slog.InfoContext(r.Context(), "register: ok", "request_id", reqID, "user_id", result.UserID)
	respondJSON(w, http.StatusCreated, map[string]string{"user_id": result.UserID.String()})
}

// ── Login ─────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login handles POST /auth/login → 200 {access_token, refresh_token}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	reqID := chimiddleware.GetReqID(r.Context())

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "login: invalid body", "request_id", reqID, "err", err)
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	slog.InfoContext(r.Context(), "login: attempt", "request_id", reqID, "email", req.Email)

	result, err := h.auth.Login(r.Context(), input.LoginCommand{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, user.ErrInvalidCredentials):
			slog.WarnContext(r.Context(), "login: invalid credentials", "request_id", reqID, "email", req.Email)
			respondError(w, http.StatusUnauthorized, "invalid credentials")
		default:
			slog.ErrorContext(r.Context(), "login: unexpected error", "request_id", reqID, "email", req.Email, "err", err)
			respondError(w, http.StatusInternalServerError, "login failed")
		}
		return
	}

	slog.InfoContext(r.Context(), "login: ok", "request_id", reqID, "email", req.Email)
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
	reqID := chimiddleware.GetReqID(r.Context())

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.WarnContext(r.Context(), "refresh: invalid body", "request_id", reqID, "err", err)
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
			slog.WarnContext(r.Context(), "refresh: invalid or expired token", "request_id", reqID)
			respondError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		default:
			slog.ErrorContext(r.Context(), "refresh: unexpected error", "request_id", reqID, "err", err)
			respondError(w, http.StatusInternalServerError, "refresh failed")
		}
		return
	}

	slog.InfoContext(r.Context(), "refresh: ok", "request_id", reqID)
	respondJSON(w, http.StatusOK, map[string]string{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
	})
}

// ── Logout ────────────────────────────────────────────────────────────────────

// Logout handles POST /auth/logout → 204
// Expects: Authorization: Bearer <access_token>
// The claims (jti, exp) are extracted by the JWT middleware. After logout the
// access token is rejected by every service and its refresh token can't be used.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	reqID := chimiddleware.GetReqID(r.Context())

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims.JTI == "" {
		slog.WarnContext(r.Context(), "logout: missing or invalid token", "request_id", reqID)
		respondError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	jti := claims.JTI

	if err := h.auth.Logout(r.Context(), input.LogoutCommand{JTI: jti, ExpiresAt: claims.ExpiresAt}); err != nil {
		slog.ErrorContext(r.Context(), "logout: failed", "request_id", reqID, "jti", jti, "err", err)
		respondError(w, http.StatusInternalServerError, "logout failed")
		return
	}

	slog.InfoContext(r.Context(), "logout: ok", "request_id", reqID, "jti", jti)
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
