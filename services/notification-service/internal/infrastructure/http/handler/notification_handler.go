package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/juantevez/my-ig/notification-service/internal/application/port/input"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/http/middleware"
	"github.com/juantevez/my-ig/notification-service/internal/infrastructure/ws"
)

// NotificationHandler es el adaptador HTTP para NotificationUseCase y WebSocket.
type NotificationHandler struct {
	svc input.NotificationUseCase
	hub *ws.Hub
}

func NewNotificationHandler(svc input.NotificationUseCase, hub *ws.Hub) *NotificationHandler {
	return &NotificationHandler{svc: svc, hub: hub}
}

// GET /notifications
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	unreadOnly := r.URL.Query().Get("unread") == "true"

	result, err := h.svc.GetNotifications(r.Context(), input.GetNotificationsQuery{
		UserID:     claims.Sub,
		Limit:      limit,
		Offset:     offset,
		UnreadOnly: unreadOnly,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get notifications")
		return
	}

	respondJSON(w, http.StatusOK, toListResponse(result))
}

// PATCH /notifications/{id}/read
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	notifID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	if err := h.svc.MarkRead(r.Context(), notifID, claims.Sub); err != nil {
		switch {
		case errors.Is(err, notification.ErrNotificationNotFound):
			respondError(w, http.StatusNotFound, "notification not found")
		default:
			respondError(w, http.StatusInternalServerError, "failed to mark as read")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PATCH /notifications/read-all
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.svc.MarkAllRead(r.Context(), claims.Sub); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to mark all as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /ws — upgrade a WebSocket para notificaciones en tiempo real
func (h *NotificationHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.hub.ServeWS(w, r, claims.Sub)
}

// ── response DTOs ─────────────────────────────────────────────────────────────

type notificationResponse struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	SourceUserID string            `json:"source_user_id"`
	Payload      map[string]string `json:"payload"`
	Read         bool              `json:"read"`
	CreatedAt    string            `json:"created_at"`
}

type listResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	UnreadCount   int                    `json:"unread_count"`
}

func toListResponse(result *input.GetNotificationsResult) listResponse {
	items := make([]notificationResponse, len(result.Notifications))
	for i, n := range result.Notifications {
		items[i] = notificationResponse{
			ID:           n.ID.String(),
			Type:         string(n.Type),
			SourceUserID: n.SourceUserID.String(),
			Payload:      n.Payload,
			Read:         n.Read,
			CreatedAt:    n.CreatedAt.Format(time.RFC3339),
		}
	}
	return listResponse{
		Notifications: items,
		UnreadCount:   result.UnreadCount,
	}
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
