package comparison

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"golang.org/x/sync/errgroup"
)

// previewCountryCode picks the flag code for a space. geo_code holds the
// ISO country code at every level except state (region codes like NG-LA
// or 708) — so the parent's code only wins when the parent IS a country
// (city → state, university → city both keep their own ISO code).
func previewCountryCode(g gen.ListSpaceGeoByIDsRow, ownCode string) string {
	if g.ParentLevel != nil && *g.ParentLevel == "country" && g.ParentGeoCode != nil {
		return *g.ParentGeoCode
	}
	return ownCode
}

// previewMediaRole marks which card slot a media id feeds — the header
// banner (imageURL) or the vs medallion avatar (avatarURL).
type previewMediaRole int

const (
	previewMediaHeader previewMediaRole = iota
	previewMediaAvatar
)

// loadPreviewItems hydrates ≤3 items per content id for directory cards.
// Batched: one items query + one media query + one geo query per page of
// results, plus deduped, concurrency-bounded resolver calls for refs that
// still lack image/geo. Space-family refs contribute their logo_media_id
// (avatar medallion) and header_image_id (banner cover) — space avatar and
// cover reflect on the card per the directory design. Best-effort
// throughout — a failed lookup drops that field, never the list row.
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

	// pendingMedia[i] collects (mediaID, role) pairs per preview index so
	// one GetMediaByIDs batch resolves every candidate — the item's own
	// image_id, plus its geo space's logo (avatar) and header (cover).
	pendingMedia := make(map[int][]struct {
		id   pgtype.UUID
		role previewMediaRole
	})

	// Geo batch: geo_space_id → geo code + parent code + space media ids.
	{
		var spaceIDs []pgtype.UUID
		for _, r := range rows {
			if r.GeoSpaceID.Valid {
				spaceIDs = append(spaceIDs, r.GeoSpaceID)
			}
		}
		if len(spaceIDs) > 0 {
			if geoRows, gerr := s.store.listSpaceGeoByIDs(ctx, spaceIDs); gerr == nil {
				type geoParts struct {
					code, country string
					logo, cover   pgtype.UUID
				}
				geoByID := map[string]geoParts{}
				for _, g := range geoRows {
					code := ""
					if g.GeoCode != nil {
						code = *g.GeoCode
					}
					geoByID[g.ID.String()] = geoParts{
						code: code, country: previewCountryCode(g, code),
						logo: g.LogoMediaID, cover: g.HeaderImageID,
					}
				}
				for i, r := range rows {
					if r.GeoSpaceID.Valid {
						gp := geoByID[r.GeoSpaceID.String()]
						mergePreviewItem(previews[i], previewItemParts{geoCode: gp.code, countryCode: gp.country})
						if gp.logo.Valid {
							pendingMedia[i] = append(pendingMedia[i], struct {
								id   pgtype.UUID
								role previewMediaRole
							}{gp.logo, previewMediaAvatar})
						}
						if gp.cover.Valid {
							pendingMedia[i] = append(pendingMedia[i], struct {
								id   pgtype.UUID
								role previewMediaRole
							}{gp.cover, previewMediaHeader})
						}
					}
				}
			}
		}
	}

	// Explicit per-item images win the banner slot over space media.
	for i, r := range rows {
		if r.ImageID.Valid {
			pendingMedia[i] = append([]struct {
				id   pgtype.UUID
				role previewMediaRole
			}{{r.ImageID, previewMediaHeader}}, pendingMedia[i]...)
		}
	}

	// Media batch: every collected id → proxied URL in one query. Roles
	// decide the slot; merge never overwrites, so ordering sets precedence
	// (item image → space cover → space logo for the banner).
	if len(pendingMedia) > 0 {
		idSeen := map[string]bool{}
		var mediaIDs []pgtype.UUID
		for _, entries := range pendingMedia {
			for _, e := range entries {
				if k := e.id.String(); !idSeen[k] {
					idSeen[k] = true
					mediaIDs = append(mediaIDs, e.id)
				}
			}
		}
		if mediaRows, merr := s.store.getMediaByIDs(ctx, mediaIDs); merr == nil {
			urlByID := map[string]string{}
			for _, m := range mediaRows {
				urlByID[m.ID.String()] = media.MediaURL(m.StorageBucket, m.StoragePath, media.ImageProxyOpts{})
			}
			for i, entries := range pendingMedia {
				for _, e := range entries {
					u := urlByID[e.id.String()]
					if u == "" {
						continue
					}
					if e.role == previewMediaAvatar {
						mergePreviewItem(previews[i], previewItemParts{avatarURL: u})
					} else {
						mergePreviewItem(previews[i], previewItemParts{imageURL: u})
					}
				}
			}
		}
	}

	// item_ref resolution fills remaining gaps (university logos, wiki geo).
	// Deduped per ref, bounded concurrency — resolution is service-level, not
	// a SQL loop; failures leave fields empty. Resolved media ids (image_id =
	// space logo, cover_media_id = space header) resolve in a trailing batch.
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
			if previews[i].ImageURL != "" && previews[i].CountryCode != "" && previews[i].AvatarURL != "" {
				continue // explicit image + geo + avatar already cover the card
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
		resolvedByIdx := map[int]*resolver.ResolvedItem{}
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
				resolvedByIdx[j.idx] = cached
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()

		// Trailing media batch for media ids resolved from item refs —
		// image_id feeds the avatar slot (space logo); cover_media_id the
		// banner (space header image). Refs that point at a space without
		// surfacing its media (geo_space_id, university_id on course refs)
		// get the space's logo/header through a spaces lookup — the card
		// should carry the space avatar and cover regardless of which
		// resolver emitted the ref. All merge-into-empty only.
		type refMedia struct {
			id   pgtype.UUID
			role previewMediaRole
		}
		refPending := make(map[int][]refMedia)
		idSeen := map[string]bool{}
		var refMediaIDs []pgtype.UUID
		queueRefMedia := func(idx int, id pgtype.UUID, role previewMediaRole) {
			if !id.Valid {
				return
			}
			refPending[idx] = append(refPending[idx], refMedia{id, role})
			if k := id.String(); !idSeen[k] {
				idSeen[k] = true
				refMediaIDs = append(refMediaIDs, id)
			}
		}
		queueRefMediaRaw := func(idx int, raw string, role previewMediaRole) {
			var id pgtype.UUID
			if err := id.Scan(raw); err != nil {
				return
			}
			queueRefMedia(idx, id, role)
		}

		// Space ids carried on resolved metadata → the space's own media.
		refSpaceIDs := map[int]pgtype.UUID{}
		var spaceIDList []pgtype.UUID
		spaceSeen := map[string]bool{}
		for idx, cached := range resolvedByIdx {
			var sid pgtype.UUID
			for _, key := range []string{"geo_space_id", "university_id"} {
				if raw, _ := cached.Metadata[key].(string); raw != "" {
					if err := sid.Scan(raw); err == nil && sid.Valid {
						break
					}
				}
			}
			if !sid.Valid {
				continue
			}
			refSpaceIDs[idx] = sid
			if k := sid.String(); !spaceSeen[k] {
				spaceSeen[k] = true
				spaceIDList = append(spaceIDList, sid)
			}
		}
		if len(spaceIDList) > 0 {
			if geoRows, gerr := s.store.listSpaceGeoByIDs(ctx, spaceIDList); gerr == nil {
				type spaceMedia struct{ logo, cover pgtype.UUID }
				mediaBySpace := map[string]spaceMedia{}
				for _, g := range geoRows {
					mediaBySpace[g.ID.String()] = spaceMedia{logo: g.LogoMediaID, cover: g.HeaderImageID}
				}
				for idx, sid := range refSpaceIDs {
					sm := mediaBySpace[sid.String()]
					queueRefMedia(idx, sm.logo, previewMediaAvatar)
					queueRefMedia(idx, sm.cover, previewMediaHeader)
				}
			}
		}

		for idx, cached := range resolvedByIdx {
			if raw, _ := cached.Metadata["image_id"].(string); raw != "" {
				queueRefMediaRaw(idx, raw, previewMediaAvatar)
			}
			if raw, _ := cached.Metadata["cover_media_id"].(string); raw != "" {
				queueRefMediaRaw(idx, raw, previewMediaHeader)
			}
		}
		if len(refMediaIDs) > 0 {
			if mediaRows, merr := s.store.getMediaByIDs(ctx, refMediaIDs); merr == nil {
				urlByID := map[string]string{}
				for _, m := range mediaRows {
					urlByID[m.ID.String()] = media.MediaURL(m.StorageBucket, m.StoragePath, media.ImageProxyOpts{})
				}
				for idx, entries := range refPending {
					for _, e := range entries {
						u := urlByID[e.id.String()]
						if u == "" {
							continue
						}
						if e.role == previewMediaAvatar {
							mergePreviewItem(previews[idx], previewItemParts{avatarURL: u})
						} else {
							mergePreviewItem(previews[idx], previewItemParts{imageURL: u})
						}
					}
				}
			}
		}
	}

	for i, r := range rows {
		out[r.ContentID] = append(out[r.ContentID], *previews[i])
	}

	return out
}
