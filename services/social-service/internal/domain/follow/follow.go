package follow

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAlreadyFollowing = errors.New("already following this user")
	ErrNotFollowing     = errors.New("not following this user")
	ErrSelfFollow       = errors.New("cannot follow yourself")
	ErrFollowNotFound   = errors.New("follow relationship not found")
)

// Follow representa la relación de seguimiento entre dos usuarios.
// Es el agregado raíz del bounded context social.
type Follow struct {
	ID          uuid.UUID
	FollowerID  uuid.UUID // quien sigue
	FollowingID uuid.UUID // a quien sigue
	Mutual      bool      // true si FollowingID también sigue a FollowerID
	CreatedAt   time.Time
}

// New crea y valida una relación de Follow.
func New(followerID, followingID uuid.UUID) (*Follow, error) {
	if followerID == uuid.Nil || followingID == uuid.Nil {
		return nil, errors.New("followerID and followingID are required")
	}
	if followerID == followingID {
		return nil, ErrSelfFollow
	}

	return &Follow{
		ID:          uuid.New(),
		FollowerID:  followerID,
		FollowingID: followingID,
		Mutual:      false,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// Stats agrupa contadores del grafo social para un usuario.
type Stats struct {
	UserID         uuid.UUID
	FollowerCount  int
	FollowingCount int
}
