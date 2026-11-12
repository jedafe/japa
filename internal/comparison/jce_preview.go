package comparison

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/media"
	"golang.org/x/sync/errgroup"
)

// loadPreviewItems hydrates ≤3 items per content id for directory cards.
// Batched: one items query + one media query + one geo query per page of
// results, plus deduped, concurrency-bounded resolver calls for refs that
// still lack image/geo. Best-effort throughout — a failed lookup drops that
// field, never the list row.
func (s *Service) loadPreviewItems(ctx context.Context, contentIDs []pgtype.UUID) map[pgtype.UUID][]JCEPreviewItem {
	out := map[pgtype.UUID][]JCEPreviewItem{}
	if len(contentIDs) == 0 {
		return out
	}

	rows, err := s.store.listComparisonItemPreviews(ctx, contentIDs)
	if err != nil {
		return out
	}

	// previews[i] stays positionally aligned with rows[i] throughout.
	previews := make([]*JCEPreviewItem, len(rows))
	for i, r := range rows {
		previews[i] = &JCEPreviewItem{Title: r.Title}
	}

	// Media batch: image_id → proxied URL.
	{
		var imageIDs []pgtype.UUID
		for _, r := range rows {
			if r.ImageID.Valid {
				imageIDs = append(imageIDs, r.ImageID)
			}
		}
		if len(imageIDs) > 0 {
			if mediaRows, merr := s.store.getMediaByIDs(ctx, imageIDs); merr == nil {
				urlByID := map[string]string{}
				for _, m := range mediaRows {
					urlByID[m.ID.String()] = media.MediaURL(m.StorageBucket, m.StoragePath, media.ImageProxyOpts{})
				}
				for i, r := range rows {
					if r.ImageID.Valid {
						mergePreviewItem(previews[i], previewItemParts{imageURL: urlByID[r.ImageID.String()]})
					}
				}
			}
		}
	}

	// Geo batch: geo_space_id → geo code + parent (country) code.
	{
		var spaceIDs []pgtype.UUID
		for _, r := range rows {
			if r.GeoSpaceID.Valid {
				spaceIDs = append(spaceIDs, r.GeoSpaceID)
			}
		}
		if len(spaceIDs) > 0 {
			if geoRows, gerr := s.store.listSpaceGeoByIDs(ctx, spaceIDs); gerr == nil {
				type geoParts struct{ code, country string }
				geoByID := map[string]geoParts{}
				for _, g := range geoRows {
					code, country := "", ""
					if g.GeoCode != nil {
						code = *g.GeoCode
					}
					if g.Level == "country" {
						country = code
					} else if g.ParentGeoCode != nil {
						country = *g.ParentGeoCode
					}
					geoByID[g.ID.String()] = geoParts{code: code, country: country}
				}
				for i, r := range rows {
					if r.GeoSpaceID.Valid {
						gp := geoByID[r.GeoSpaceID.String()]
						mergePreviewItem(previews[i], previewItemParts{geoCode: gp.code, countryCode: gp.country})
					}
				}
			}
		}
	}

	// item_ref resolution fills remaining gaps (university logos, wiki geo).
	// Deduped per ref, bounded concurrency — resolution is service-level, not
	// a SQL loop; failures leave fields empty.
	if s.resolver != nil {
		type refJob struct {
			idx int
			key string
			ref map[string]any
		}
		seen := map[string]*resolver.ResolvedItem{}
		var jobs []refJob
		for i, r := range rows {
			if len(r.ItemRef) == 0 {
				continue
			}
			if previews[i].ImageURL != "" && previews[i].CountryCode != "" {
				continue // explicit image + geo already cover the card
			}
			ref, err := resolver.CoerceMap(r.ItemRef)
			if err != nil {
				continue
			}
			jobs = append(jobs, refJob{idx: i, key: itemRefDedupKey(r.ItemRef), ref: ref})
		}

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(8)
		var mu sync.Mutex
		for _, j := range jobs {
			g.Go(func() error {
				mu.Lock()
				cached, ok := seen[j.key]
				mu.Unlock()
				if !ok {
					resolved, rerr := s.resolver.Resolve(gctx, j.ref)
					if rerr != nil {
						mu.Lock()
						seen[j.key] = nil
						mu.Unlock()
						return nil // best-effort
					}
					resolvedCopy := resolved
					cached = &resolvedCopy
					mu.Lock()
					seen[j.key] = cached
					mu.Unlock()
				}
				if cached == nil {
					return nil
				}

				parts := previewItemParts{imageURL: cached.ImageURL}
				if tp, _ := j.ref["type"].(string); tp != "" {
					parts.itemType = tp
				}
				if c, _ := cached.Metadata["geo_code"].(string); c != "" {
					parts.geoCode = c
				}
				if c, _ := cached.Metadata["country"].(string); c != "" {
					parts.countryCode = c
				} else if c, _ := cached.Metadata["country_code"].(string); c != "" {
					parts.countryCode = c
				}
				mu.Lock()
				mergePreviewItem(previews[j.idx], parts)
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()
	}

	for i, r := range rows {
		out[r.ContentID] = append(out[r.ContentID], *previews[i])
	}

	return out
}
