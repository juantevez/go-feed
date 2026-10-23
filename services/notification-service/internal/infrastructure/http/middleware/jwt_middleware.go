package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	Sub  uuid.UUID
	Role string
	JTI  string
}

type contextKey string

const claimsKey contextKey = "jwt_claims"

func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if !strings.HasPrefix(raw, "Bearer ") {
				respondError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			claims, err := parseToken(strings.TrimPrefix(raw, "Bearer "), secret)
			if err != nil {
				if errors.Is(err, gojwt.ErrTokenExpired) {
					respondError(w, http.StatusUnauthorized, "token expired")
				} else {
					respondError(w, http.StatusUnauthorized, "invalid token")
				}
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

type rawClaims struct {
	gojwt.RegisteredClaims
	Role string `json:"role"`
	Kind string `json:"kind"`
}

func parseToken(tokenStr, secret string) (*Claims, error) {
	t, err := gojwt.ParseWithClaims(tokenStr, &rawClaims{}, func(t *gojwt.Token) (any, error) {
		if _, ok := t.Method.(*gojwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	rc, ok := t.Claims.(*rawClaims)
	if !ok || !t.Valid || rc.Kind != "access" {
		return nil, errors.New("invalid token")
	}
	if rc.ExpiresAt != nil && rc.ExpiresAt.Before(time.Now()) {
		return nil, gojwt.ErrTokenExpired
	}
	sub, err := uuid.Parse(rc.Subject)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	return &Claims{Sub: sub, Role: rc.Role, JTI: rc.ID}, nil
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
