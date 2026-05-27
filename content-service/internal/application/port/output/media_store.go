package output

import (
	"context"
	"io"
)

// MediaStore es el puerto driven para almacenamiento de archivos de media.
// El adaptador S3 implementa esta interfaz.
type MediaStore interface {
	// Upload sube un archivo al store y retorna la URL pública.
	Upload(ctx context.Context, key string, r io.Reader, mimeType string, sizeBytes int64) (url string, err error)

	// Delete elimina un archivo del store por su key.
	Delete(ctx context.Context, key string) error

	// PresignedURL genera una URL firmada con TTL para acceso temporal.
	PresignedURL(ctx context.Context, key string) (string, error)
}
