package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const presignTTL = 1 * time.Hour

// Store implementa output.MediaStore sobre AWS S3 (o compatible: MinIO, R2).
type Store struct {
	client  *awss3.Client
	presign *awss3.PresignClient
	bucket  string
	baseURL string
}

func New(client *awss3.Client, bucket, baseURL string) *Store {
	return &Store{
		client:  client,
		presign: awss3.NewPresignClient(client),
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
		ACL:           types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return "", fmt.Errorf("s3.Upload: %w", err)
	}
	return s.baseURL + "/" + key, nil
}

// Delete elimina un objeto del bucket.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3.Delete: %w", err)
	}
	return nil
}

// PresignedURL genera una URL firmada temporal para acceso privado.
func (s *Store) PresignedURL(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, awss3.WithPresignExpires(presignTTL))
	if err != nil {
		return "", fmt.Errorf("s3.PresignedURL: %w", err)
	}
	return req.URL, nil
}
