package notification

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrNotOwner             = errors.New("not the notification owner")
)

// Type clasifica el origen de la notificación.
type Type string

const (
	TypeNewFollower  Type = "new_follower"
	TypeNewPost      Type = "new_post"
	TypePostLiked    Type = "post_liked"
	TypeCommentAdded Type = "comment_added"
)

// Notification representa una alerta generada para un usuario.
// Es inmutable una vez creada — solo cambia el campo Read.
type Notification struct {
	ID           uuid.UUID
	TargetUserID uuid.UUID // usuario que recibe la notificación
	SourceUserID uuid.UUID // usuario que la genera
	Type         Type
	Payload      map[string]string // datos extra: post_id, comment_id, etc.
	Read         bool
	CreatedAt    time.Time
	ReadAt       *time.Time
}

// New crea una Notification válida.
func New(targetUserID, sourceUserID uuid.UUID, t Type, payload map[string]string) (*Notification, error) {
	if targetUserID == uuid.Nil {
		return nil, errors.New("targetUserID is required")
	}
	if sourceUserID == uuid.Nil {
		return nil, errors.New("sourceUserID is required")
	}
	if t == "" {
		return nil, errors.New("type is required")
	}
	if targetUserID == sourceUserID {
		return nil, errors.New("cannot notify yourself")
	}

	return &Notification{
		ID:           uuid.New(),
		TargetUserID: targetUserID,
		SourceUserID: sourceUserID,
		Type:         t,
		Payload:      payload,
		Read:         false,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// MarkRead marca la notificación como leída.
func (n *Notification) MarkRead() {
	if n.Read {
		return
	}
	n.Read = true
	now := time.Now().UTC()
	n.ReadAt = &now
}
