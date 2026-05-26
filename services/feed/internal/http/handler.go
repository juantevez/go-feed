package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"feed-backend/services/feed/internal/usecase"

	"github.com/go-chi/chi/v5"
)

type FeedHandler struct {
	uc *usecase.FeedUseCase
}

func NewFeedHandler(uc *usecase.FeedUseCase) *FeedHandler {
	return &FeedHandler{uc: uc}
}

func (h *FeedHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	// En prod: extraer userID de claims JWT vía middleware
	userID := r.Header.Get("X-User-ID") // Placeholder para integración auth
	if userID == "" {
		http.Error(w, `{"title":"Unauthorized","status":401}`, http.StatusUnauthorized)
		return
	}

	req := usecase.FeedRequest{
		UserID: userID,
		Cursor: r.URL.Query().Get("cursor"),
	}
	if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 {
		req.Limit = l
	}

	resp, err := h.uc.GetFeed(r.Context(), req)
	if err != nil {
		http.Error(w, `{"title":"Internal Server Error","status":500}`, http.StatusInternalServerError)
		return
	}

	if resp.NextCursor != nil {
		w.Header().Set("X-Next-Cursor", *resp.NextCursor)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func NewRouter(handler *FeedHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/feed", handler.GetFeed)
	return r
}
