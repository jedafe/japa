package server

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
	intel_stores "github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/publicapi"
)

// intelDigestReader satisfies publicapi.IntelReader over the intel domain
// queries — the same promoted corpus the radar aggregates, reduced to the
// two highest-signal domains for a new-tab card: visa routes and jobs.
// Null keyset cursors yield the latest-N rows per domain; the merged list
// is ordered by created_at desc and capped. Two radar rules apply here
// too: inactive visa routes never surface, and items whose intel source
// the viewer blocked are filtered out (radar.filterBlockedRadarItems).
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest).
type intelDigestReader struct {
	q            *gen.Queries
	intelStores  *intel_stores.Store
	intelSources *sources.Service
}

func (r *intelDigestReader) ListRecentForSpace(ctx context.Context, viewerID, spaceID pgtype.UUID, limit int32) ([]publicapi.IntelDigestItem, error) {
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
	// recordIDByIndex aligns each output item with its intel_record_id so
	// the blocked-source filter can resolve attribution.
	recordIDs := make([]pgtype.UUID, 0, len(routes)+len(jobs))
	for _, v := range routes {
		if !v.IsActive {
			continue
		}
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
		recordIDs = append(recordIDs, v.IntelRecordID)
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
		recordIDs = append(recordIDs, j.IntelRecordID)
	}

	out = r.filterBlockedSources(ctx, viewerID, out, recordIDs)

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if int32(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}

// filterBlockedSources drops items whose intel source the viewer has
// blocked — mirrors radar.filterBlockedRadarItems. Failures degrade to
// the unfiltered list (a preference, not a privacy boundary).
func (r *intelDigestReader) filterBlockedSources(ctx context.Context, viewerID pgtype.UUID, items []publicapi.IntelDigestItem, recordIDs []pgtype.UUID) []publicapi.IntelDigestItem {
	if !viewerID.Valid || r.intelSources == nil || r.intelStores == nil || len(items) == 0 {
		return items
	}
	blocked, err := r.intelSources.ListBlockedSourceIDs(ctx, viewerID)
	if err != nil || len(blocked) == 0 {
		return items
	}
	blockedSet := make(map[string]struct{}, len(blocked))
	for _, id := range blocked {
		if id.Valid {
			blockedSet[id.String()] = struct{}{}
		}
	}

	valid := make([]pgtype.UUID, 0, len(recordIDs))
	for _, rid := range recordIDs {
		if rid.Valid {
			valid = append(valid, rid)
		}
	}
	if len(valid) == 0 {
		return items
	}
	sourceMap, err := r.intelStores.MapSourcesByRecordIDs(ctx, valid)
	if err != nil {
		return items
	}

	filtered := make([]publicapi.IntelDigestItem, 0, len(items))
	for i, it := range items {
		if attr, ok := sourceMap[recordIDs[i].String()]; ok {
			if _, isBlocked := blockedSet[attr.SourceID]; isBlocked {
				continue
			}
		}
		filtered = append(filtered, it)
	}
	return filtered
}
