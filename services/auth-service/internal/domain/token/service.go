package token

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service is the driven port for JWT signing, verification and revocation.
// The infrastructure adapter (e.g. jwtadapter) implements this.
type Service interface {
	// GeneratePair issues a new access+refresh pair for the given user.
	GeneratePair(ctx context.Context, userID uuid.UUID, role string) (*Pair, error)

	// Refresh validates the refresh token and rotates both tokens.
	Refresh(ctx context.Context, refreshToken string) (*Pair, error)

	// Revoke ends the session of an access token: revokes the refresh token of
	// its pair and denylists the access token (by jti) until it expires.
	Revoke(ctx context.Context, accessJTI string, expiresAt time.Time) error

	// Validate parses and verifies the access token, returning its claims.
	Validate(ctx context.Context, accessToken string) (*Claims, error)
}

// Pair holds the two tokens returned to the client.
type Pair struct {
	AccessToken  string
	RefreshToken string
}

// Claims are the verified data extracted from a valid access token.
type Claims struct {
	Sub       uuid.UUID
	Role      string
	JTI       string
	ExpiresAt time.Time
}
