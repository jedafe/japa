package server

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/publicapi"
)

// intelDigestReader satisfies publicapi.IntelReader over the intel domain
// queries — the same promoted corpus the radar aggregates, reduced to the
// two highest-signal domains for a new-tab card: visa routes and jobs.
// Null keyset cursors yield the latest-N rows per domain; the merged list
// is ordered by created_at desc and capped.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest).
type intelDigestReader struct {
	q *gen.Queries
}

func (r *intelDigestReader) ListRecentForSpace(ctx context.Context, spaceID pgtype.UUID, limit int32) ([]publicapi.IntelDigestItem, error) {
	if limit <= 0 {
		limit = 3
	}
	ids := []pgtype.UUID{spaceID}

	routes, err := r.q.ListVisaRoutesBySpaces(ctx, gen.ListVisaRoutesBySpacesParams{
		Column1: ids, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	jobs, err := r.q.ListIntelJobsBySpaces(ctx, gen.ListIntelJobsBySpacesParams{
		Column1: ids, Limit: limit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]publicapi.IntelDigestItem, 0, len(routes)+len(jobs))
	for _, v := range routes {
		item := publicapi.IntelDigestItem{
			ID:        v.ID.String(),
			Kind:      "visa_route",
			Title:     v.Name,
			CreatedAt: v.CreatedAt.Time.UTC().Format(time.RFC3339),
		}
		if v.EligibilitySummary != nil {
			item.Summary = *v.EligibilitySummary
		}
		out = append(out, item)
	}
	for _, j := range jobs {
		item := publicapi.IntelDigestItem{
			ID:        j.ID.String(),
			Kind:      "job",
			Title:     j.Title,
			CreatedAt: j.CreatedAt.Time.UTC().Format(time.RFC3339),
		}
		if j.Employer != nil {
			item.Summary = *j.Employer
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if int32(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}
