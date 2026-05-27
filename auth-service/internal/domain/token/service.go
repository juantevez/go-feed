package token

import (
	"context"

	"github.com/google/uuid"
)

// Service is the driven port for JWT signing, verification and revocation.
// The infrastructure adapter (e.g. jwtadapter) implements this.
type Service interface {
	// GeneratePair issues a new access+refresh pair for the given user.
	GeneratePair(ctx context.Context, userID uuid.UUID, role string) (*Pair, error)

	// Refresh validates the refresh token and rotates both tokens.
	Refresh(ctx context.Context, refreshToken string) (*Pair, error)

	// Revoke marks the token identified by jti as revoked.
	Revoke(ctx context.Context, jti string) error

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
	Sub  uuid.UUID
	Role string
	JTI  string
}
