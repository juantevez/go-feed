package token

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrTokenExpired is returned when a token's TTL has elapsed.
var ErrTokenExpired = errors.New("token expired")

// ErrTokenInvalid is returned when a token cannot be verified.
var ErrTokenInvalid = errors.New("token invalid")

// ErrTokenRevoked is returned when a valid access token was revoked by logout.
var ErrTokenRevoked = errors.New("token revoked")

// Kind distinguishes access tokens from refresh tokens.
type Kind string

const (
	KindAccess  Kind = "access"
	KindRefresh Kind = "refresh"
)

// Token represents a JWT pair entry tracked for rotation and revocation.
type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Kind      Kind
	JTI       string // JWT ID claim — used for revocation lookup
	AccessJTI string // refresh only: jti of the access token issued in the same pair
	ExpiresAt time.Time
	CreatedAt time.Time
	Revoked   bool
}

// IsExpired returns true when the token's validity window has passed.
func (t *Token) IsExpired() bool {
	return time.Now().UTC().After(t.ExpiresAt)
}

// NewRefresh creates a new refresh token record (long-lived, rotative).
// accessJTI links it to the access token of the same pair, so logout can find it.
func NewRefresh(userID uuid.UUID, jti, accessJTI string, ttl time.Duration) *Token {
	now := time.Now().UTC()
	return &Token{
		ID:        uuid.New(),
		UserID:    userID,
		Kind:      KindRefresh,
		JTI:       jti,
		AccessJTI: accessJTI,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
}
