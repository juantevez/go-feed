package follow

import (
	"context"

	"github.com/google/uuid"
)

// Repository es el puerto driven para persistencia del grafo social.
type Repository interface {
	// Save persiste una nueva relación de follow.
	// Retorna ErrAlreadyFollowing si ya existe.
	Save(ctx context.Context, f *Follow) error

	// Delete elimina la relación de follow.
	// Retorna ErrNotFollowing si no existe.
	Delete(ctx context.Context, followerID, followingID uuid.UUID) error

	// Exists verifica si followerID sigue a followingID.
	Exists(ctx context.Context, followerID, followingID uuid.UUID) (bool, error)

	// GetFollowers retorna los seguidores de un usuario (paginado).
	GetFollowers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Follow, error)

	// GetFollowing retorna los usuarios que sigue un usuario (paginado).
	GetFollowing(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Follow, error)

	// GetStats retorna follower_count y following_count de un usuario.
	GetStats(ctx context.Context, userID uuid.UUID) (*Stats, error)

	// UpdateMutual actualiza el flag mutual en ambas direcciones.
	UpdateMutual(ctx context.Context, followerID, followingID uuid.UUID, mutual bool) error
}
