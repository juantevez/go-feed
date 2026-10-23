package media

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Type distingue imágenes de videos.
type Type string

const (
	TypeImage Type = "image"
	TypeVideo Type = "video"
)

// MaxFileSizeBytes es el límite de tamaño de archivo (50 MB).
const MaxFileSizeBytes = 50 * 1024 * 1024

// allowedMIMETypes lista los MIME types aceptados.
// La validación es sobre el contenido real, no solo la extensión.
var allowedMIMETypes = map[string]Type{
	"image/jpeg": TypeImage,
	"image/png":  TypeImage,
	"image/webp": TypeImage,
	"video/mp4":  TypeVideo,
	"video/webm": TypeVideo,
}

// Media representa un archivo adjunto a un post.
type Media struct {
	ID        uuid.UUID
	PostID    uuid.UUID
	Type      Type
	MIMEType  string
	SizeBytes int64
	Filename  string
	S3Key     string // key dentro del bucket, e.g. "posts/{postID}/{mediaID}.jpg"
	URL       string // URL pública o pre-signed
}

// New valida y construye un Media listo para subir.
func New(postID uuid.UUID, filename, mimeType string, sizeBytes int64) (*Media, error) {
	if postID == uuid.Nil {
		return nil, errors.New("postID is required")
	}
	if sizeBytes > MaxFileSizeBytes {
		return nil, errors.New("file exceeds maximum size of 50MB")
	}

	mediaType, ok := allowedMIMETypes[strings.ToLower(mimeType)]
	if !ok {
		return nil, errors.New("unsupported media type: " + mimeType)
	}

	id := uuid.New()
	ext := filepath.Ext(filename)
	s3Key := "posts/" + postID.String() + "/" + id.String() + ext

	return &Media{
		ID:        id,
		PostID:    postID,
		Type:      mediaType,
		MIMEType:  mimeType,
		SizeBytes: sizeBytes,
		Filename:  filename,
		S3Key:     s3Key,
	}, nil
}
