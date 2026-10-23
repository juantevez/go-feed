package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const testSecret = "test-secret"

type noRevocations struct{}

func (noRevocations) IsRevoked(string) bool { return false }

func accessToken(t *testing.T, sub uuid.UUID) string {
	t.Helper()
	claims := gojwt.MapClaims{
		"sub":  sub.String(),
		"jti":  uuid.NewString(),
		"exp":  time.Now().Add(time.Minute).Unix(),
		"kind": "access",
	}
	tok, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// wsServer simula /ws: el middleware y un upgrade que, como el Hub, ofrece el
// subprotocolo del token. Responde con el sub autenticado como primer mensaje.
func wsServer(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{Subprotocols: []string{"access_token"}}
	h := JWTWebSocketMiddleware(testSecret, noRevocations{}, "access_token")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, _ := ClaimsFromContext(r.Context())
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteMessage(websocket.TextMessage, []byte(claims.Sub.String()))
		}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestWebSocketTokenAsSubprotocol(t *testing.T) {
	sub := uuid.New()
	// Lo que hace el navegador: new WebSocket(url, ["access_token", token]).
	dialer := websocket.Dialer{Subprotocols: []string{"access_token", accessToken(t, sub)}}
	conn, resp, err := dialer.Dial(wsServer(t), nil)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, resp)
	}
	defer conn.Close()

	// El navegador corta la conexión si el servidor no elige uno de sus subprotocolos.
	if got := conn.Subprotocol(); got != "access_token" {
		t.Errorf("selected subprotocol = %q, want access_token", got)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil || string(msg) != sub.String() {
		t.Errorf("authenticated sub = %q, %v; want %s", msg, err, sub)
	}
}

func TestWebSocketTokenInHeader(t *testing.T) {
	sub := uuid.New()
	h := http.Header{"Authorization": {"Bearer " + accessToken(t, sub)}}
	conn, _, err := websocket.DefaultDialer.Dial(wsServer(t), h)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_, msg, _ := conn.ReadMessage()
	if string(msg) != sub.String() {
		t.Errorf("authenticated sub = %q, want %s", msg, sub)
	}
}

func TestWebSocketRejectsMissingOrInvalidToken(t *testing.T) {
	url := wsServer(t)
	cases := map[string]websocket.Dialer{
		"sin token":            {},
		"solo el subprotocolo": {Subprotocols: []string{"access_token"}},
		"token inválido":       {Subprotocols: []string{"access_token", "not-a-jwt"}},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			_, resp, err := d.Dial(url, nil)
			if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("err = %v, resp = %v; want 401", err, resp)
			}
		})
	}

	// Por query string no se acepta: la URL queda en los logs.
	_, resp, err := websocket.DefaultDialer.Dial(url+"?token="+accessToken(t, uuid.New()), nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("query token: err = %v, resp = %v; want 401", err, resp)
	}
}
