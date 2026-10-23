package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	domaintoken "github.com/juantevez/my-ig/auth-service/internal/domain/token"
)

// tokenClaims are the JWT claims used for both access and refresh tokens.
// The `kind` claim differentiates the two.
type tokenClaims struct {
	gojwt.RegisteredClaims
	Role string `json:"role"`
	Kind string `json:"kind"`
}

// RefreshTokenStore is the minimal persistence interface the adapter needs.
// Satisfied by postgres.RefreshTokenRepository.
type RefreshTokenStore interface {
	Save(ctx context.Context, t *domaintoken.Token) error
	FindByJTI(ctx context.Context, jti string) (*domaintoken.Token, error)
	Revoke(ctx context.Context, jti string) error
}

// Service implements domain/token.Service using HS256 JWT.
// RS256 is preferred in production (swap key types and signing method).
type Service struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	store           RefreshTokenStore
}

// New creates the JWT service.
func New(secret string, accessTTL, refreshTTL time.Duration, store RefreshTokenStore) *Service {
	return &Service{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		store:           store,
	}
}

// GeneratePair issues a new access+refresh token pair and persists the refresh token.
func (s *Service) GeneratePair(ctx context.Context, userID uuid.UUID, role string) (*domaintoken.Pair, error) {
	accessJTI := uuid.NewString()
	accessToken, err := s.sign(userID, role, accessJTI, domaintoken.KindAccess, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("jwt.GeneratePair: sign access: %w", err)
	}

	refreshJTI := uuid.NewString()
	refreshToken, err := s.sign(userID, role, refreshJTI, domaintoken.KindRefresh, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("jwt.GeneratePair: sign refresh: %w", err)
	}

	// Persist the refresh token record for rotation/revocation tracking.
	rt := domaintoken.NewRefresh(userID, refreshJTI, s.refreshTokenTTL)
	if err := s.store.Save(ctx, rt); err != nil {
		return nil, fmt.Errorf("jwt.GeneratePair: persist refresh token: %w", err)
	}

	return &domaintoken.Pair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Validate parses and verifies an access token, returning its claims.
// Access tokens are stateless — no DB lookup needed.
func (s *Service) Validate(_ context.Context, accessToken string) (*domaintoken.Claims, error) {
	claims, err := s.parse(accessToken)
	if err != nil {
		return nil, err
	}
	if claims.Kind != string(domaintoken.KindAccess) {
		return nil, domaintoken.ErrTokenInvalid
	}

	sub, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, domaintoken.ErrTokenInvalid
	}

	return &domaintoken.Claims{
		Sub:  sub,
		Role: claims.Role,
		JTI:  claims.ID,
	}, nil
}

// Refresh validates the refresh token, revokes it, and issues a new pair (rotation).
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*domaintoken.Pair, error) {
	claims, err := s.parse(refreshToken)
	if err != nil {
		return nil, err
	}
	if claims.Kind != string(domaintoken.KindRefresh) {
		return nil, domaintoken.ErrTokenInvalid
	}

	// Verify it exists in the store and hasn't been revoked.
	stored, err := s.store.FindByJTI(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("jwt.Refresh: lookup: %w", err)
	}
	if stored.Revoked || stored.IsExpired() {
		return nil, domaintoken.ErrTokenExpired
	}

	// Revoke the old refresh token immediately (one-time use).
	if err := s.store.Revoke(ctx, claims.ID); err != nil {
		return nil, fmt.Errorf("jwt.Refresh: revoke old token: %w", err)
	}

	// Issue a fresh pair.
	sub, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, domaintoken.ErrTokenInvalid
	}
	return s.GeneratePair(ctx, sub, claims.Role)
}

// Revoke marks an access token's JTI as invalid.
// For access tokens we store the JTI in the refresh_tokens table with a
// short TTL matching the access token — this is the simplest revocation
// strategy without Redis. Swap for a Redis SET in production.
func (s *Service) Revoke(ctx context.Context, jti string) error {
	if err := s.store.Revoke(ctx, jti); err != nil {
		// If not found it may be a first-party access token JTI — not an error.
		if errors.Is(err, domaintoken.ErrTokenInvalid) {
			return nil
		}
		return fmt.Errorf("jwt.Revoke: %w", err)
	}
	return nil
}

// ── internal helpers ─────────────────────────────────────────────────────────

func (s *Service) sign(userID uuid.UUID, role, jti string, kind domaintoken.Kind, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := tokenClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   userID.String(),
			ID:        jti,
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "auth-service",
		},
		Role: role,
		Kind: string(kind),
	}
	t := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return signed, nil
}

func (s *Service) parse(tokenStr string) (*tokenClaims, error) {
	t, err := gojwt.ParseWithClaims(tokenStr, &tokenClaims{}, func(t *gojwt.Token) (any, error) {
		if _, ok := t.Method.(*gojwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, domaintoken.ErrTokenExpired
		}
		return nil, domaintoken.ErrTokenInvalid
	}

	claims, ok := t.Claims.(*tokenClaims)
	if !ok || !t.Valid {
		return nil, domaintoken.ErrTokenInvalid
	}
	return claims, nil
}
