package s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

const presignTTL = 1 * time.Hour

// Store implementa output.MediaStore sobre MinIO, vía su API compatible con S3
// (por eso el cliente es el AWS SDK).
//
// Los objetos se suben sin ACL: quién puede leerlos lo decide la política del
// bucket (lectura anónima solo para media.PublicPrefix).
type Store struct {
	client  *awss3.Client
	presign *awss3.PresignClient
	bucket  string
	baseURL string
}

// New recibe dos clientes: client habla con S3 desde el servicio y presignClient
// firma las URLs que abre el navegador. En local difieren en el endpoint
// (minio:9000 dentro de Docker, localhost:9000 desde el host); la firma incluye
// el host, así que tiene que ser el que va a usar el navegador.
func New(client, presignClient *awss3.Client, bucket, baseURL string) *Store {
	return &Store{
		client:  client,
		presign: awss3.NewPresignClient(presignClient),
		bucket:  bucket,
		baseURL: baseURL,
	}
}

// Upload sube un archivo al bucket y retorna su URL pública.
func (s *Store) Upload(ctx context.Context, key string, r io.Reader, mimeType string, sizeBytes int64) (string, error) {
	_, err := s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(sizeBytes),
	})
	if err != nil {
		return "", fmt.Errorf("s3.Upload: %w", err)
	}
	return s.baseURL + "/" + key, nil
}

// Delete elimina un objeto del bucket. Acepta la key o la URL pública que
// devolvió Upload (los posts solo guardan la URL).
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.keyOf(key)),
	})
	if err != nil {
		return fmt.Errorf("s3.Delete: %w", err)
	}
	return nil
}

// PresignedURL genera una URL firmada temporal para acceso privado.
// Acepta la key o la URL pública que devolvió Upload.
func (s *Store) PresignedURL(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.keyOf(key)),
	}, awss3.WithPresignExpires(presignTTL))
	if err != nil {
		return "", fmt.Errorf("s3.PresignedURL: %w", err)
	}
	return req.URL, nil
}

// keyOf le quita a una URL pública el prefijo baseURL; una key queda igual.
func (s *Store) keyOf(keyOrURL string) string {
	return strings.TrimPrefix(keyOrURL, s.baseURL+"/")
}
