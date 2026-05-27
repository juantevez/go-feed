package post

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPostNotFound   = errors.New("post not found")
	ErrNotAuthor      = errors.New("only the author can modify this post")
	ErrAlreadyDeleted = errors.New("post already deleted")
	ErrInvalidCaption = errors.New("caption exceeds maximum length")
)

// Status representa el ciclo de vida de un post.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusDeleted   Status = "deleted"
)

// Visibility controla quién puede ver el post.
type Visibility string

const (
	VisibilityPublic        Visibility = "public"
	VisibilityFollowersOnly Visibility = "followers_only"
	VisibilityPrivate       Visibility = "private"
)

const maxCaptionLength = 2200 // límite estilo Instagram

// Post es el agregado raíz del bounded context de contenido.
type Post struct {
	ID         uuid.UUID
	AuthorID   uuid.UUID
	Caption    string
	Status     Status
	Visibility Visibility
	MediaURLs  []string // URLs públicas de S3 (post-upload)
	Tags       []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

// New crea un Post en estado draft.
// El caller sube el media a S3 y luego llama a Publish.
func New(authorID uuid.UUID, caption string, visibility Visibility) (*Post, error) {
	if authorID == uuid.Nil {
		return nil, errors.New("authorID is required")
	}
	caption = strings.TrimSpace(caption)
	if len([]rune(caption)) > maxCaptionLength {
		return nil, ErrInvalidCaption
	}
	if visibility == "" {
		visibility = VisibilityPublic
	}

	now := time.Now().UTC()
	return &Post{
		ID:         uuid.New(),
		AuthorID:   authorID,
		Caption:    caption,
		Status:     StatusDraft,
		Visibility: visibility,
		MediaURLs:  []string{},
		Tags:       extractTags(caption),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// Publish transiciona el post de draft a published.
// Requiere al menos un archivo de media adjunto.
func (p *Post) Publish(mediaURLs []string) error {
	if p.Status == StatusDeleted {
		return ErrAlreadyDeleted
	}
	if len(mediaURLs) == 0 {
		return errors.New("at least one media file is required to publish")
	}
	p.MediaURLs = mediaURLs
	p.Status = StatusPublished
	p.UpdatedAt = time.Now().UTC()
	return nil
}

// Delete marca el post como eliminado (soft delete).
// Solo el autor puede eliminarlo.
func (p *Post) Delete(requestingUserID uuid.UUID) error {
	if p.AuthorID != requestingUserID {
		return ErrNotAuthor
	}
	if p.Status == StatusDeleted {
		return ErrAlreadyDeleted
	}
	now := time.Now().UTC()
	p.Status = StatusDeleted
	p.DeletedAt = &now
	p.UpdatedAt = now
	return nil
}

// IsVisible retorna true si el post es visible públicamente.
func (p *Post) IsVisible() bool {
	return p.Status == StatusPublished && p.Visibility == VisibilityPublic
}

// extractTags parsea hashtags del caption (#golang, #go, etc.)
func extractTags(caption string) []string {
	var tags []string
	seen := make(map[string]bool)
	for _, word := range strings.Fields(caption) {
		if strings.HasPrefix(word, "#") {
			tag := strings.ToLower(strings.TrimPrefix(word, "#"))
			tag = strings.Trim(tag, ".,!?;:")
			if tag != "" && !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
			}
		}
	}
	return tags
}
