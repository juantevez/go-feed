package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	domaintoken "github.com/juantevez/my-ig/auth-service/internal/domain/token"
)

type contextKey string

const (
	claimsKey contextKey = "jwt_claims"
	jtiKey    contextKey = "jwt_jti"
)

// JWTMiddleware valida el access token del header Authorization: Bearer <token>.
// Inyecta los claims y el JTI en el contexto para que los handlers los lean.
func JWTMiddleware(svc domaintoken.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if !strings.HasPrefix(raw, "Bearer ") {
				respondError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			tokenStr := strings.TrimPrefix(raw, "Bearer ")
			claims, err := svc.Validate(r.Context(), tokenStr)
			if err != nil {
				switch {
				case errors.Is(err, domaintoken.ErrTokenExpired):
					respondError(w, http.StatusUnauthorized, "token expired")
				default:
					respondError(w, http.StatusUnauthorized, "invalid token")
				}
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			ctx = context.WithValue(ctx, jtiKey, claims.JTI)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext extrae los claims del contexto inyectados por JWTMiddleware.
func ClaimsFromContext(ctx context.Context) (*domaintoken.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*domaintoken.Claims)
	return c, ok
}

// JTIFromContext extrae el JTI del contexto inyectado por JWTMiddleware.
func JTIFromContext(ctx context.Context) (string, bool) {
	jti, ok := ctx.Value(jtiKey).(string)
	return jti, ok
}

// ── helpers locales ───────────────────────────────────────────────────────────

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
