package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/juantevez/my-ig/social-service/internal/application/port/input"
	"github.com/juantevez/my-ig/social-service/internal/domain/follow"
	"github.com/juantevez/my-ig/social-service/internal/infrastructure/http/middleware"
)

// FollowHandler es el adaptador HTTP para FollowUseCase.
type FollowHandler struct {
	follows input.FollowUseCase
}

func NewFollowHandler(follows input.FollowUseCase) *FollowHandler {
	return &FollowHandler{follows: follows}
}

// POST /follow/{target_id}
func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "target_id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid target_id")
		return
	}

	err = h.follows.Follow(r.Context(), input.FollowCommand{
		FollowerID:  claims.Sub,
		FollowingID: targetID,
		TraceID:     r.Header.Get("X-Request-ID"),
	})
	if err != nil {
		switch {
		case errors.Is(err, follow.ErrAlreadyFollowing):
			respondError(w, http.StatusConflict, "already following this user")
		case errors.Is(err, follow.ErrSelfFollow):
			respondError(w, http.StatusBadRequest, "cannot follow yourself")
		default:
			respondError(w, http.StatusInternalServerError, "follow failed")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "following"})
}

// DELETE /follow/{target_id}
func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "target_id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid target_id")
		return
	}

	err = h.follows.Unfollow(r.Context(), input.UnfollowCommand{
		FollowerID:  claims.Sub,
		FollowingID: targetID,
		TraceID:     r.Header.Get("X-Request-ID"),
	})
	if err != nil {
		switch {
		case errors.Is(err, follow.ErrNotFollowing):
			respondError(w, http.StatusNotFound, "not following this user")
		default:
			respondError(w, http.StatusInternalServerError, "unfollow failed")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /users/{user_id}/followers
func (h *FollowHandler) GetFollowers(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	result, err := h.follows.GetFollowers(r.Context(), input.GetFollowersQuery{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get followers")
		return
	}

	respondJSON(w, http.StatusOK, toFollowListResponse(result.Follows))
}

// GET /users/{user_id}/following
func (h *FollowHandler) GetFollowing(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	result, err := h.follows.GetFollowing(r.Context(), input.GetFollowingQuery{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get following")
		return
	}

	respondJSON(w, http.StatusOK, toFollowListResponse(result.Follows))
}

// GET /users/{user_id}/stats
func (h *FollowHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	stats, err := h.follows.GetStats(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"user_id":         stats.UserID.String(),
		"follower_count":  stats.FollowerCount,
		"following_count": stats.FollowingCount,
	})
}

// ── response helpers ──────────────────────────────────────────────────────────

type followResponse struct {
	FollowerID  string `json:"follower_id"`
	FollowingID string `json:"following_id"`
	Mutual      bool   `json:"mutual"`
}

func toFollowListResponse(follows []*follow.Follow) []followResponse {
	resp := make([]followResponse, len(follows))
	for i, f := range follows {
		resp[i] = followResponse{
			FollowerID:  f.FollowerID.String(),
			FollowingID: f.FollowingID.String(),
			Mutual:      f.Mutual,
		}
	}
	return resp
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}
