package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juantevez/my-ig/feed-service/internal/application/port/input"
	"github.com/juantevez/my-ig/feed-service/internal/domain/feed"
)

// FeedHandler es el adaptador HTTP para FeedUseCase.
type FeedHandler struct {
	feed input.FeedUseCase
}

func NewFeedHandler(feed input.FeedUseCase) *FeedHandler {
	return &FeedHandler{feed: feed}
}

// feedEntryResponse es la representación JSON de una FeedEntry.
type feedEntryResponse struct {
	PostID     string  `json:"post_id"`
	AuthorID   string  `json:"author_id"`
	Score      float64 `json:"score"`
	InsertedAt string  `json:"inserted_at"`
}

// feedResponse es la respuesta paginada del timeline.
type feedResponse struct {
	Entries    []feedEntryResponse `json:"entries"`
	NextCursor string              `json:"next_cursor,omitempty"`
	HasMore    bool                `json:"has_more"`
}

// GetFeed maneja GET /feed
// Query params:
//   - cursor: opaco, retornado en la respuesta anterior
//   - limit:  número de entradas (1-50, default 20)
//
// Header requerido: X-User-ID (inyectado por el API Gateway desde el JWT)
func (h *FeedHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	// En producción el API Gateway valida el JWT y propaga el user_id como header.
	// En desarrollo el middleware local puede inyectarlo.
	userIDStr := r.Header.Get("X-User-ID")
	if userIDStr == "" {
		respondError(w, http.StatusUnauthorized, "missing X-User-ID header")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid X-User-ID")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	var cursor *feed.Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err = parseCursor(raw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
	}

	result, err := h.feed.GetFeed(r.Context(), input.GetFeedQuery{
		UserID: userID,
		Cursor: cursor,
		Limit:  limit,
	})
	if err != nil {
		if errors.Is(err, feed.ErrFeedEmpty) {
			respondJSON(w, http.StatusOK, feedResponse{Entries: []feedEntryResponse{}, HasMore: false})
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get feed")
		return
	}

	resp := feedResponse{
		Entries: make([]feedEntryResponse, len(result.Entries)),
		HasMore: result.HasMore,
	}
	for i, e := range result.Entries {
		resp.Entries[i] = feedEntryResponse{
			PostID:     e.PostID.String(),
			AuthorID:   e.AuthorID.String(),
			Score:      e.Score,
			InsertedAt: e.InsertedAt.Format(time.RFC3339),
		}
	}
	if result.NextCursor != nil {
		resp.NextCursor = encodeCursor(result.NextCursor)
	}

	respondJSON(w, http.StatusOK, resp)
}

// ── cursor encoding ───────────────────────────────────────────────────────────

// encodeCursor serializa el cursor como "score_postID" para el cliente.
func encodeCursor(c *feed.Cursor) string {
	return fmt.Sprintf("%.0f_%s", c.Score, c.PostID.String())
}

// parseCursor deserializa el cursor del query param.
func parseCursor(raw string) (*feed.Cursor, error) {
	parts := strings.SplitN(raw, "_", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid cursor format")
	}
	score, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, errors.New("invalid cursor score")
	}
	postID, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, errors.New("invalid cursor post_id")
	}
	return &feed.Cursor{Score: score, PostID: postID}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}
