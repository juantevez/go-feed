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
			h.remove(userID, c)
		}
	}
	return nil
}

// ServeWS upgradea la conexión HTTP a WebSocket y registra el cliente.
// El userID viene del JWT validado por el middleware.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws: upgrade failed", "user_id", userID, "err", err)
		return
	}

	client := &Client{
		hub:    h,
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
	}

	h.register(userID, client)

	go client.writePump()
	go client.readPump()
}

// ── gestión de clientes ───────────────────────────────────────────────────────

func (h *Hub) register(userID uuid.UUID, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[userID] = append(h.clients[userID], c)
	slog.Info("ws: client connected", "user_id", userID, "total", len(h.clients[userID]))
}

// remove saca al cliente del hub y lo cierra. Es idempotente: lo llaman tanto
// Push (cliente lento) como readPump (desconexión), a veces los dos para la
// misma conexión.
func (h *Hub) remove(userID uuid.UUID, c *Client) {
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

	c.close()
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
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func (c *Client) close() {
	c.closeOnce.Do(func() { close(c.done) })
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
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
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
		c.hub.remove(c.userID, c)
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
