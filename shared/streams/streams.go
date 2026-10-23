// Package streams declara, en un solo lugar, los streams de JetStream donde
// viven los eventos de dominio. Antes cada servicio declaraba los suyos: los que
// publican (auth-service en AUTH, social-service en USERS) no creaban su stream,
// y si la configuración cambiaba en uno y no en otro, ganaba el primero que
// arrancaba.
//
// Todos los servicios llaman a Ensure al arrancar, así cualquiera alcanza para
// dejar el bus listo sobre un NATS vacío, sin importar el orden de arranque.
//
// Fuera de este paquete quedan el stream DLQ (shared/dlq) y el bucket KV de
// tokens revocados (shared/revocation): cada uno lo declara el paquete que lo usa.
package streams

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// MaxAge es cuánto retiene JetStream los eventos de dominio. Un consumer caído
// más que esto pierde eventos.
const MaxAge = 7 * 24 * time.Hour

// Stream es un stream de eventos de dominio y los subjects que captura.
type Stream struct {
	Name     string
	Subjects []string
}

var (
	// Auth: eventos de auth-service (auth.user.registered.v1, auth.user.logged_in.v1).
	Auth = Stream{Name: "AUTH", Subjects: []string{"auth.>"}}
	// Posts: eventos de content-service (posts.created.v1, posts.deleted.v1).
	Posts = Stream{Name: "POSTS", Subjects: []string{"posts.>"}}
	// Users: eventos de social-service (users.followed.v1, users.unfollowed.v1).
	Users = Stream{Name: "USERS", Subjects: []string{"users.>"}}
)

// All son todos los streams de eventos de dominio.
var All = []Stream{Auth, Posts, Users}

// Config es la configuración canónica del stream.
func (s Stream) Config() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:     s.Name,
		Subjects: s.Subjects,
		Storage:  nats.FileStorage,
		MaxAge:   MaxAge,
	}
}

// Ensure crea los streams que falten. Si uno ya existe con otra configuración,
// lo actualiza a la canónica; si NATS no lo permite (ej. cambiar el tipo de
// storage), lo deja como está y lo loguea, sin frenar el arranque.
func Ensure(js nats.JetStreamContext) error {
	for _, s := range All {
		_, err := js.AddStream(s.Config())
		if err == nil {
			continue
		}
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return fmt.Errorf("streams: ensure %s: %w", s.Name, err)
		}
		// Existe con otra configuración (AddStream con la misma no da error).
		if _, err := js.UpdateStream(s.Config()); err != nil {
			slog.Warn("streams: stream config differs and could not be updated",
				"stream", s.Name, "err", err)
			continue
		}
		slog.Info("streams: stream config updated", "stream", s.Name)
	}
	return nil
}
