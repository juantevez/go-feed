package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/juantevez/my-ig/content-service/internal/application/port/input"
	"github.com/juantevez/my-ig/content-service/internal/domain/post"
	"github.com/juantevez/my-ig/content-service/internal/infrastructure/http/middleware"
)

const maxUploadSize = 50 << 20 // 50 MB por request (multipart total)

// PostHandler es el adaptador HTTP para PostUseCase.
type PostHandler struct {
	posts input.PostUseCase
}

func NewPostHandler(posts input.PostUseCase) *PostHandler {
	return &PostHandler{posts: posts}
}

// ── POST /posts ───────────────────────────────────────────────────────────────

// CreatePost maneja POST /posts (multipart/form-data)
// Fields: caption, visibility
// Files:  media[] (imágenes o videos)
func (h *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		respondError(w, http.StatusBadRequest, "request too large or invalid multipart form")
		return
	}

	visibility := post.Visibility(r.FormValue("visibility"))
	if visibility == "" {
		visibility = post.VisibilityPublic
	}

	files := r.MultipartForm.File["media"]
	if len(files) == 0 {
		respondError(w, http.StatusBadRequest, "at least one media file is required")
		return
	}

	result, err := h.posts.CreatePost(r.Context(), input.CreatePostCommand{
		AuthorID:   claims.Sub,
		Caption:    r.FormValue("caption"),
		Visibility: visibility,
		Files:      files,
		TraceID:    r.Header.Get("X-Request-ID"),
	})
	if err != nil {
		switch {
		case errors.Is(err, post.ErrInvalidCaption):
			respondError(w, http.StatusBadRequest, err.Error())
		default:
			respondError(w, http.StatusInternalServerError, "failed to create post")
		}
		return
	}

	respondJSON(w, http.StatusCreated, toPostResponse(result.Post))
}

// ── GET /posts/{id} ───────────────────────────────────────────────────────────

func (h *PostHandler) GetPost(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}

	var requestingUser uuid.UUID
	if claims, ok := middleware.ClaimsFromContext(r.Context()); ok {
		requestingUser = claims.Sub
	}

	result, err := h.posts.GetPost(r.Context(), input.GetPostQuery{
		PostID:         postID,
		RequestingUser: requestingUser,
	})
	if err != nil {
		switch {
		case errors.Is(err, post.ErrPostNotFound):
			respondError(w, http.StatusNotFound, "post not found")
		default:
			respondError(w, http.StatusInternalServerError, "failed to get post")
		}
		return
	}

	respondJSON(w, http.StatusOK, toPostResponse(result.Post))
}

// ── DELETE /posts/{id} ────────────────────────────────────────────────────────

func (h *PostHandler) DeletePost(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	postID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}

	err = h.posts.DeletePost(r.Context(), input.DeletePostCommand{
		PostID:   postID,
		AuthorID: claims.Sub,
		Reason:   "user_request",
		TraceID:  r.Header.Get("X-Request-ID"),
	})
	if err != nil {
		switch {
		case errors.Is(err, post.ErrPostNotFound):
			respondError(w, http.StatusNotFound, "post not found")
		case errors.Is(err, post.ErrNotAuthor):
			respondError(w, http.StatusForbidden, "not authorized to delete this post")
		case errors.Is(err, post.ErrAlreadyDeleted):
			respondError(w, http.StatusGone, "post already deleted")
		default:
			respondError(w, http.StatusInternalServerError, "failed to delete post")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── response DTOs ─────────────────────────────────────────────────────────────

type postResponse struct {
	ID         string   `json:"id"`
	AuthorID   string   `json:"author_id"`
	Caption    string   `json:"caption"`
	Status     string   `json:"status"`
	Visibility string   `json:"visibility"`
	MediaURLs  []string `json:"media_urls"`
	Tags       []string `json:"tags"`
	CreatedAt  string   `json:"created_at"`
}

func toPostResponse(p *post.Post) postResponse {
	return postResponse{
		ID:         p.ID.String(),
		AuthorID:   p.AuthorID.String(),
		Caption:    p.Caption,
		Status:     string(p.Status),
		Visibility: string(p.Visibility),
		MediaURLs:  p.MediaURLs,
		Tags:       p.Tags,
		CreatedAt:  p.CreatedAt.Format(time.RFC3339),
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
