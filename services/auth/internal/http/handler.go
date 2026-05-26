package http

import (
	"encoding/json"
	"net/http"

	"feed-backend/services/auth/internal/usecase"

	"github.com/go-chi/chi/v5"
)

type AuthHandler struct {
	uc *usecase.AuthUseCase
}

func NewAuthHandler(uc *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{uc: uc}
}

// ProblemDetail sigue RFC 7807 (alineado con tu OpenAPI spec)
type ProblemDetail struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req usecase.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON", "Request body malformed")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Validation Error", "username, email and password are required")
		return
	}

	resp, err := h.uc.Register(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusConflict, "Registration Failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req usecase.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON", "Request body malformed")
		return
	}

	resp, err := h.uc.Login(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication Failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// NewRouter configura el mux de chi con los endpoints de auth
func NewRouter(handler *AuthHandler) *chi.Mux {
	r := chi.NewRouter()

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Endpoints alineados con OpenAPI spec
	r.Post("/auth/register", handler.Register)
	r.Post("/auth/login", handler.Login)

	return r
}

func writeError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ProblemDetail{
		Type:   "https://api.feed.local/errors/auth",
		Title:  title,
		Status: status,
		Detail: detail,
	})
}
