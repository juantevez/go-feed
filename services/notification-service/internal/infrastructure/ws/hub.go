package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
	sendBuffer     = 64
)

// Códigos de cierre propios (rango 4000-4999, reservado para aplicaciones), para
// que el cliente sepa si tiene que refrescar el token y reconectar o no.
const (
	// CloseTokenExpired: venció el access token con el que se abrió la conexión.
	// El cliente refresca el token y reconecta.
	CloseTokenExpired = 4001
	// CloseTokenRevoked: se hizo logout con ese token. El cliente no reconecta.
	CloseTokenRevoked = 4003
)

// Session es lo que se sabe del access token con el que se abrió una conexión.
type Session struct {
	UserID    uuid.UUID
	TokenID   string    // jti: para cerrar la conexión si el token se revoca
	ExpiresAt time.Time // la conexión se cierra cuando vence el token
}

// TokenSubprotocol es el subprotocolo con el que un navegador manda el access
// token en el upgrade: new WebSocket(url, ["access_token", token]). El servidor
// lo selecciona en la respuesta (el navegador corta la conexión si no lo hace).
const TokenSubprotocol = "access_token"

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{TokenSubprotocol},
	// Sin chequeo de origen: la auth no va por cookie, así que otra página no
	// puede abrir un WebSocket en nombre del usuario sin tener su token.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub gestiona todas las conexiones WebSocket activas.
// Implementa output.NotificationPusher.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID][]*Client // userID → conexiones activas (puede haber varias)
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[uuid.UUID][]*Client),
	}
}

// Push envía una notificación a todas las conexiones activas del usuario.
// Si el usuario no está conectado, no hace nada (no es un error).
func (h *Hub) Push(_ context.Context, userID uuid.UUID, n *notification.Notification) error {
	// Copia bajo lock: remove reescribe el slice y no se puede recorrer sin él.
	h.mu.RLock()
	clients := append([]*Client(nil), h.clients[userID]...)
	h.mu.RUnlock()

	if len(clients) == 0 {
		return nil // usuario no conectado — se leerá desde la DB
	}

	msg, err := json.Marshal(toWsMessage(n))
	if err != nil {
		return err
	}

	for _, c := range clients {
		select {
		case c.send <- msg:
		case <-c.done:
			// Se desconectó mientras tanto: su writePump ya no lee send.
		default:
			// Buffer lleno — cliente lento, desconectar
			slog.Warn("ws: slow client, disconnecting", "user_id", userID)
			h.remove(c, websocket.CloseTryAgainLater, "slow client")
		}
	}
	return nil
}

// ServeWS upgradea la conexión HTTP a WebSocket y registra el cliente.
// La sesión viene del JWT validado por el middleware. El token se valida solo en
// el upgrade, así que la conexión se cierra sola cuando vence (CloseTokenExpired)
// o cuando se revoca (RevokeToken, CloseTokenRevoked).
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, s Session) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "user_id", s.UserID, "err", err)
		return
	}

	client := &Client{
		hub:     h,
		userID:  s.UserID,
		tokenID: s.TokenID,
		conn:    conn,
		send:    make(chan []byte, sendBuffer),
		done:    make(chan struct{}),
	}

	// El timer se crea antes de registrar: una vez en el hub, otros goroutines
	// pueden cerrar el cliente y leer expiry. Si vence antes del register, el
	// cliente queda cerrado y readPump lo saca del hub en cuanto corta.
	if !s.ExpiresAt.IsZero() {
		client.expiry = time.AfterFunc(time.Until(s.ExpiresAt), func() {
			slog.Info("ws: token expired, closing", "user_id", s.UserID)
			h.remove(client, CloseTokenExpired, "token expired")
		})
	}
	h.register(s.UserID, client)

	go client.writePump()
	go client.readPump()
}

// RevokeToken cierra las conexiones abiertas con el access token jti (logout).
// Las otras sesiones del mismo usuario siguen abiertas, igual que en la API HTTP.
// Se registra como callback de revocation.Denylist.OnRevoke.
func (h *Hub) RevokeToken(jti string) {
	var revoked []*Client
	h.mu.RLock()
	for _, clients := range h.clients {
		for _, c := range clients {
			if c.tokenID == jti {
				revoked = append(revoked, c)
			}
		}
	}
	h.mu.RUnlock()

	for _, c := range revoked {
		slog.Info("ws: token revoked, closing", "user_id", c.userID)
		h.remove(c, CloseTokenRevoked, "token revoked")
	}
}

// ── gestión de clientes ───────────────────────────────────────────────────────

func (h *Hub) register(userID uuid.UUID, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[userID] = append(h.clients[userID], c)
	slog.Info("ws: client connected", "user_id", userID, "total", len(h.clients[userID]))
}

// remove saca al cliente del hub y lo cierra con code/reason (el primer cierre
// gana). Es idempotente: lo llaman Push (cliente lento), readPump (desconexión),
// el vencimiento y la revocación del token, a veces más de uno para la misma
// conexión.
func (h *Hub) remove(c *Client, code int, reason string) {
	userID := c.userID
	h.mu.Lock()
	clients := h.clients[userID]
	kept := make([]*Client, 0, len(clients))
	for _, cl := range clients {
		if cl != c {
			kept = append(kept, cl)
		}
	}
	removed := len(kept) < len(clients)
	if len(kept) == 0 {
		delete(h.clients, userID)
	} else {
		h.clients[userID] = kept
	}
	h.mu.Unlock()

	c.close(code, reason)
	if removed {
		slog.Info("ws: client disconnected", "user_id", userID)
	}
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client representa una conexión WebSocket individual.
//
// send nunca se cierra: varios goroutines le escriben (Push) y cerrarlo desde
// remove hacía que un Push concurrente o un segundo remove entraran en panic. El
// fin de la conexión se señala cerrando done, una sola vez.
type Client struct {
	hub       *Hub
	userID    uuid.UUID
	tokenID   string
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	expiry    *time.Timer

	// Se escriben antes de cerrar done y se leen después: no necesitan lock.
	closeCode   int
	closeReason string
}

func (c *Client) close(code int, reason string) {
	c.closeOnce.Do(func() {
		if c.expiry != nil {
			c.expiry.Stop()
		}
		c.closeCode, c.closeReason = code, reason
		close(c.done)
	})
}

// writePump escribe mensajes del canal send a la conexión WebSocket.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close() // readPump lo detecta y hace remove
	}()

	for {
		select {
		case msg := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-c.done:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(c.closeCode, c.closeReason))
			return
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump lee mensajes del cliente (pong, close) y detecta desconexiones.
func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c, websocket.CloseNormalClosure, "")
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("ws: unexpected close", "user_id", c.userID, "err", err)
			}
			return
		}
	}
}

// ── mensaje WebSocket ─────────────────────────────────────────────────────────

type wsMessage struct {
	Type      string            `json:"type"`
	ID        string            `json:"id"`
	Payload   map[string]string `json:"payload"`
	CreatedAt string            `json:"created_at"`
}

func toWsMessage(n *notification.Notification) wsMessage {
	return wsMessage{
		Type:      string(n.Type),
		ID:        n.ID.String(),
		Payload:   n.Payload,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
}
