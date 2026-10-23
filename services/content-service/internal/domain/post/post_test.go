package post

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewVisibility(t *testing.T) {
	tests := []struct {
		in      Visibility
		want    Visibility
		wantErr error
	}{
		{"", VisibilityPublic, nil},
		{VisibilityPublic, VisibilityPublic, nil},
		{VisibilityFollowersOnly, VisibilityFollowersOnly, nil},
		{VisibilityPrivate, VisibilityPrivate, nil},
		{"friends", "", ErrInvalidVisibility},
		{"PUBLIC", "", ErrInvalidVisibility},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			p, err := New(uuid.New(), "caption", tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && p.Visibility != tt.want {
				t.Errorf("Visibility = %q, want %q", p.Visibility, tt.want)
			}
		})
	}
}
