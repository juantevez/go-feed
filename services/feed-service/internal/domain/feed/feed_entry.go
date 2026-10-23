package feed

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrFeedEntryNotFound se retorna cuando no existe una entrada en el feed.
	ErrFeedEntryNotFound = errors.New("feed entry not found")

	// ErrFeedEmpty se retorna cuando el timeline de un usuario está vacío.
	ErrFeedEmpty = errors.New("feed is empty")
)

// Strategy indica cómo se generó el feed para este usuario.
type Strategy string

const (
	StrategyFanOut Strategy = "fan_out" // push: pre-materializado en Redis/DB
	StrategyFanIn  Strategy = "fan_in"  // pull: generado on-demand al leer
)

// FeedEntry representa una entrada materializada en el timeline de un usuario.
// Es la proyección de lectura — no modifica posts ni usuarios.
//
// El score permite ordenamiento cronológico puro (unix timestamp)
// o ranking algorítmico (fase 2: recency + engagement_rate + relationship_weight).
type FeedEntry struct {
	ID         uuid.UUID
	UserID     uuid.UUID // dueño del feed (el seguidor)
	PostID     uuid.UUID // post que aparece en su feed
	AuthorID   uuid.UUID // quien publicó el post
	Score      float64   // unix timestamp para cronológico; ranking para algorítmico
	InsertedAt time.Time
}

// New crea una FeedEntry válida con score cronológico puro.
func New(userID, postID, authorID uuid.UUID, publishedAt time.Time) (*FeedEntry, error) {
	if userID == uuid.Nil {
		return nil, errors.New("userID is required")
	}
	if postID == uuid.Nil {
		return nil, errors.New("postID is required")
	}
	if authorID == uuid.Nil {
		return nil, errors.New("authorID is required")
	}

	return &FeedEntry{
		ID:         uuid.New(),
		UserID:     userID,
		PostID:     postID,
		AuthorID:   authorID,
		Score:      float64(publishedAt.UTC().UnixMilli()),
		InsertedAt: time.Now().UTC(),
	}, nil
}

// Cursor es la posición de paginación cursor-based.
// Formato: score_postID (ej. "1716739200000_abc123")
// Evita el problema de offset con feeds que se actualizan en tiempo real.
type Cursor struct {
	Score  float64
	PostID uuid.UUID
}
