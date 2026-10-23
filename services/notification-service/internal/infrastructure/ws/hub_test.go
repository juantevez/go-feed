package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/juantevez/my-ig/notification-service/internal/domain/notification"
)

func newTestServer(t *testing.T, hub *Hub, userID uuid.UUID) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, userID)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func connected(h *Hub, userID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID])
}

func waitConnected(t *testing.T, h *Hub, userID uuid.UUID, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for connected(h, userID) != want {
		if time.Now().After(deadline) {
			t.Fatalf("connected = %d, want %d", connected(h, userID), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func bigNotification() *notification.Notification {
	return &notification.Notification{
		ID:      uuid.New(),
		Type:    notification.TypeNewFollower,
		Payload: map[string]string{"x": strings.Repeat("x", 64*1024)},
	}
}

// Un cliente que no lee llena su buffer: Push lo desconecta y después readPump
// también llama a remove. Antes eso cerraba send dos veces (panic).
func TestSlowClientIsDisconnectedWithoutPanic(t *testing.T) {
	hub, userID := NewHub(), uuid.New()
	dial(t, newTestServer(t, hub, userID)) // nunca lee
	waitConnected(t, hub, userID, 1)

	n := bigNotification()
	for i := 0; i < 5000 && connected(hub, userID) > 0; i++ {
		_ = hub.Push(context.Background(), userID, n)
	}
	if connected(hub, userID) != 0 {
		t.Fatal("slow client was never disconnected")
	}

	// Dar tiempo a writePump/readPump a cerrar y a readPump a llamar a remove.
	time.Sleep(500 * time.Millisecond)
	_ = hub.Push(context.Background(), userID, n) // ya sin clientes: no-op
}

// Pushes concurrentes mientras los clientes se conectan y desconectan. Correr
// con -race: antes Push recorría el slice que remove modificaba, y podía mandar
// a un canal ya cerrado.
func TestConcurrentPushAndDisconnect(t *testing.T) {
	hub, userID := NewHub(), uuid.New()
	url := newTestServer(t, hub, userID)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := &notification.Notification{ID: uuid.New(), Type: notification.TypeNewFollower}
			for {
				select {
				case <-stop:
					return
				default:
					_ = hub.Push(context.Background(), userID, n)
				}
			}
		}()
	}

	for i := 0; i < 30; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
		conn.Close()
	}
	close(stop)
	wg.Wait()
	waitConnected(t, hub, userID, 0)
}

func TestPushReachesEveryConnectionOfTheUser(t *testing.T) {
	hub, userID := NewHub(), uuid.New()
	url := newTestServer(t, hub, userID)
	a, b := dial(t, url), dial(t, url)
	waitConnected(t, hub, userID, 2)

	n := &notification.Notification{ID: uuid.New(), Type: notification.TypeNewFollower, Payload: map[string]string{"follower_id": "x"}}
	if err := hub.Push(context.Background(), userID, n); err != nil {
		t.Fatalf("push: %v", err)
	}
	for _, c := range []*websocket.Conn{a, b} {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil || !strings.Contains(string(msg), n.ID.String()) {
			t.Errorf("read = %q, %v; want the notification", msg, err)
		}
	}
}
