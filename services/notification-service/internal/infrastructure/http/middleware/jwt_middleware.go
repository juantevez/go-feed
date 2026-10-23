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

// RevocationChecker consulta la lista de access tokens revocados por logout
// (revocation.Denylist, compartida vía NATS KV con auth-service).
type RevocationChecker interface {
	IsRevoked(jti string) bool
}

func JWTMiddleware(secret string, revoked RevocationChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if !strings.HasPrefix(raw, "Bearer ") {
				respondError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}
			authenticate(w, r, next, strings.TrimPrefix(raw, "Bearer "), secret, revoked)
		})
	}
}

// JWTWebSocketMiddleware autentica el upgrade a WebSocket. Acepta el token en el
// header Authorization (clientes que pueden mandar headers) o como subprotocolo:
// la API WebSocket del navegador no permite headers, solo subprotocolos, así que
// el front abre la conexión con new WebSocket(url, [tokenProtocol, token]).
// No se acepta por query string: la URL completa queda en los logs.
func JWTWebSocketMiddleware(secret string, revoked RevocationChecker, tokenProtocol string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if raw := r.Header.Get("Authorization"); strings.HasPrefix(raw, "Bearer ") {
				authenticate(w, r, next, strings.TrimPrefix(raw, "Bearer "), secret, revoked)
				return
			}
			token, ok := tokenFromSubprotocols(r, tokenProtocol)
			if !ok {
				respondError(w, http.StatusUnauthorized, "missing access token")
				return
			}
			authenticate(w, r, next, token, secret, revoked)
		})
	}
}

// tokenFromSubprotocols busca, en Sec-WebSocket-Protocol, el valor que sigue a
// tokenProtocol: el navegador manda "access_token, <jwt>".
func tokenFromSubprotocols(r *http.Request, tokenProtocol string) (string, bool) {
	var protocols []string
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			protocols = append(protocols, strings.TrimSpace(p))
		}
	}
	for i := 0; i+1 < len(protocols); i++ {
		if protocols[i] == tokenProtocol && protocols[i+1] != "" {
			return protocols[i+1], true
		}
	}
	return "", false
}

// authenticate valida el token y, si es válido, sigue con los claims en el contexto.
func authenticate(w http.ResponseWriter, r *http.Request, next http.Handler, tokenStr, secret string, revoked RevocationChecker) {
	claims, err := parseToken(tokenStr, secret)
	if err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			respondError(w, http.StatusUnauthorized, "token expired")
		} else {
			respondError(w, http.StatusUnauthorized, "invalid token")
		}
		return
	}

	if revoked.IsRevoked(claims.JTI) {
		respondError(w, http.StatusUnauthorized, "token revoked")
		return
	}

	ctx := context.WithValue(r.Context(), claimsKey, claims)
	next.ServeHTTP(w, r.WithContext(ctx))
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
