package server

import (
	"context"

	"github.com/jedafe/japa/internal/honcho"
)

// honchoFetcherAdapter adapts *honcho.Client to gdpr.HonchoFetcher. It
// converts honcho.ContextEntry values into the plain []map[string]interface{}
// shape the GDPR service expects, so the gdpr package does not depend on
// internal/honcho (BUG-012).
type honchoFetcherAdapter struct {
	client *honcho.Client
}

func (a *honchoFetcherAdapter) GetContext(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	entries, err := a.client.GetContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]interface{}{
			"id":         e.ID,
			"content":    e.Content,
			"metadata":   e.Metadata,
			"created_at": e.CreatedAt,
		})
	}

	return out, nil
}
