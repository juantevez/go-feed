package usecase

import (
	"context"
	"fmt"

	"github.com/juantevez/my-ig/content-service/internal/domain/post"
)

// presignPrivateMedia reemplaza las URLs de media de un post no público por URLs
// prefirmadas: esa media no tiene lectura anónima en el bucket. Se llama solo
// después de autorizar a quien pide, y el post resultante no debe persistirse.
func (s *PostService) presignPrivateMedia(ctx context.Context, p *post.Post) error {
	if p.IsPublic() {
		return nil
	}
	urls := make([]string, len(p.MediaURLs))
	for i, u := range p.MediaURLs {
		signed, err := s.store.PresignedURL(ctx, u)
		if err != nil {
			return fmt.Errorf("presign media: %w", err)
		}
		urls[i] = signed
	}
	p.MediaURLs = urls
	return nil
}
