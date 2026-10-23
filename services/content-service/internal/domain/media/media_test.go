package media

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	postID := uuid.New()

	tests := []struct {
		name     string
		filename string
		mimeType string
		size     int64
		wantErr  error
		wantExt  string
	}{
		{"jpeg", "foto.jpeg", "image/jpeg", 1024, nil, ".jpg"},
		{"png", "foto.png", "image/png", 1024, nil, ".png"},
		{"webm", "clip.webm", "video/webm", 1024, nil, ".webm"},
		{"extensión del cliente ignorada", "foto.txt", "image/png", 1024, nil, ".png"},
		{"texto", "nota.txt", "text/plain; charset=utf-8", 10, ErrUnsupportedType, ""},
		{"octet-stream", "raro.bin", "application/octet-stream", 10, ErrUnsupportedType, ""},
		{"imagen en el límite", "foto.webp", "image/webp", MaxImageSizeBytes, nil, ".webp"},
		{"imagen excede el límite", "foto.webp", "image/webp", MaxImageSizeBytes + 1, ErrFileTooLarge, ""},
		{"video de más de 8MB", "clip.mp4", "video/mp4", MaxImageSizeBytes + 1, nil, ".mp4"},
		{"video en el límite", "big.mp4", "video/mp4", MaxVideoSizeBytes, nil, ".mp4"},
		{"video excede el límite", "big.mp4", "video/mp4", MaxVideoSizeBytes + 1, ErrFileTooLarge, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(postID, false, tt.filename, tt.mimeType, tt.size)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				var fe *FileError
				if !errors.As(err, &fe) || fe.Filename != tt.filename {
					t.Fatalf("err = %#v, want *FileError for %q", err, tt.filename)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !strings.HasPrefix(m.S3Key, PublicPrefix+postID.String()+"/") || !strings.HasSuffix(m.S3Key, tt.wantExt) {
				t.Errorf("S3Key = %q, want %s%s/<id>%s", m.S3Key, PublicPrefix, postID, tt.wantExt)
			}
		})
	}
}

func TestNewPrivateKey(t *testing.T) {
	postID := uuid.New()
	m, err := New(postID, true, "foto.png", "image/png", 1024)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(m.S3Key, PrivatePrefix+postID.String()+"/") {
		t.Errorf("S3Key = %q, want prefix %s", m.S3Key, PrivatePrefix)
	}
}

func TestFileTooLargeMessage(t *testing.T) {
	_, err := New(uuid.New(), false, "foto.png", "image/png", MaxImageSizeBytes+1)
	if want := "foto.png: file too large (max 8MB for images)"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}
