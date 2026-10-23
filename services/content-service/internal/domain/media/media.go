package media

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Type distingue imágenes de videos.
type Type string

const (
	TypeImage Type = "image"
	TypeVideo Type = "video"
)

// Límites de tamaño por archivo, según el tipo.
const (
	MaxImageSizeBytes = 8 * 1024 * 1024
	MaxVideoSizeBytes = 50 * 1024 * 1024
)

var maxSizeBytes = map[Type]int64{
	TypeImage: MaxImageSizeBytes,
	TypeVideo: MaxVideoSizeBytes,
}

// Prefijos de las keys en el bucket. Solo PublicPrefix tiene lectura anónima;
// la media de posts no públicos se sirve con URLs prefirmadas.
const (
	PublicPrefix  = "posts/"
	PrivatePrefix = "private/posts/"
)

var (
	ErrUnsupportedType = errors.New("unsupported media type (allowed: image/jpeg, image/png, image/webp, video/mp4, video/webm)")
	ErrFileTooLarge    = errors.New("file too large")
)

// FileError asocia un error de validación al archivo que lo produjo,
// para que el cliente sepa cuál de los archivos subidos rechazar.
type FileError struct {
	Filename string
	Err      error
}

func (e *FileError) Error() string { return e.Filename + ": " + e.Err.Error() }
func (e *FileError) Unwrap() error { return e.Err }

type format struct {
	mediaType Type
	ext       string
}

// allowedMIMETypes lista los MIME types aceptados y la extensión de su S3 key.
var allowedMIMETypes = map[string]format{
	"image/jpeg": {TypeImage, ".jpg"},
	"image/png":  {TypeImage, ".png"},
	"image/webp": {TypeImage, ".webp"},
	"video/mp4":  {TypeVideo, ".mp4"},
	"video/webm": {TypeVideo, ".webm"},
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
// mimeType debe ser el detectado a partir del contenido del archivo, no el que
// declara el cliente: la extensión de la key sale de ahí, no de filename.
// private indica que el post no es público: la key va bajo PrivatePrefix.
func New(postID uuid.UUID, private bool, filename, mimeType string, sizeBytes int64) (*Media, error) {
	if postID == uuid.Nil {
		return nil, errors.New("postID is required")
	}

	f, ok := allowedMIMETypes[mimeType]
	if !ok {
		return nil, &FileError{Filename: filename, Err: ErrUnsupportedType}
	}
	if limit := maxSizeBytes[f.mediaType]; sizeBytes > limit {
		return nil, &FileError{
			Filename: filename,
			Err:      fmt.Errorf("%w (max %dMB for %ss)", ErrFileTooLarge, limit>>20, f.mediaType),
		}
	}

	prefix := PublicPrefix
	if private {
		prefix = PrivatePrefix
	}
	id := uuid.New()
	s3Key := prefix + postID.String() + "/" + id.String() + f.ext

	return &Media{
		ID:        id,
		PostID:    postID,
		Type:      f.mediaType,
		MIMEType:  mimeType,
		SizeBytes: sizeBytes,
		Filename:  filename,
		S3Key:     s3Key,
	}, nil
}
