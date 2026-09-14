package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/worker"
)

// sourceLinkPreviewAdapter adapts *linkpreview.Fetcher to
// worker.SourceLinkPreviewer without importing linkpreview into the worker
// package (that would create an import cycle because linkpreview has HTTP
// handlers that import auth, and auth imports worker).
type sourceLinkPreviewAdapter struct {
	fetcher *linkpreview.Fetcher
}

func (a *sourceLinkPreviewAdapter) Fetch(ctx context.Context, rawURL string) (*worker.Preview, error) {
	p, err := a.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	return &worker.Preview{
		Description: p.Description,
		Favicon:     p.Favicon,
		Image:       p.Image,
		Keywords:    p.Keywords,
	}, nil
}
