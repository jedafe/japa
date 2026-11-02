package content

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/media"
)

// Ref: japaconnect-engineering/sections/14-content-type-system.md — list-page
// cards (ContentGridCard on /guides, /stories, /lists, /blog, entity-collection
// blocks) read featured_image_url/header_image_url, but the list queries return
// bare content rows with only the *_image_id columns. The detail mapper emits
// the URLs from a media join; list payloads get them batch-resolved here.

// AttachListImageURLs injects "featured_image_url" and "header_image_url" keys
// into every element of a []gen.*Row slice returned by the /content list
// endpoints. Rows are re-encoded to maps (gen structs have no *_url fields to
// mutate); media is resolved with a single GetMediaByIDs for the whole page.
// Reflection keeps this one code path across the ~5 row shapes ListContent can
// return — the same trick literals.ResolveFields uses on the same payload.
func (s *Service) AttachListImageURLs(ctx context.Context, v any) (any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return v, nil
	}

	imageID := func(el reflect.Value, field string) (pgtype.UUID, bool) {
		f := el.FieldByName(field)
		if !f.IsValid() {
			return pgtype.UUID{}, false
		}
		id, ok := f.Interface().(pgtype.UUID)
		return id, ok && id.Valid
	}

	// Pass 1: collect the distinct media IDs referenced by this page.
	seen := map[pgtype.UUID]struct{}{}
	var ids []pgtype.UUID
	for i := 0; i < rv.Len(); i++ {
		el := rv.Index(i)
		for _, field := range [2]string{"FeaturedImageID", "HeaderImageID"} {
			if id, ok := imageID(el, field); ok {
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					ids = append(ids, id)
				}
			}
		}
	}
	if len(ids) == 0 {
		return v, nil
	}

	mediaRows, err := s.read.GetMediaByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	urls := make(map[pgtype.UUID]string, len(mediaRows))
	for _, m := range mediaRows {
		urls[m.ID] = media.MediaURL(m.StorageBucket, m.StoragePath, media.ImageProxyOpts{})
	}

	// Pass 2: marshal each row back to a map and add the URL keys — the
	// existing fields keep their gen json tags, so the response contract is
	// unchanged apart from the two new keys.
	out := make([]map[string]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		el := rv.Index(i)
		raw, err := json.Marshal(el.Interface())
		if err != nil {
			return nil, err
		}
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if id, ok := imageID(el, "FeaturedImageID"); ok && urls[id] != "" {
			item["featured_image_url"] = urls[id]
		}
		if id, ok := imageID(el, "HeaderImageID"); ok && urls[id] != "" {
			item["header_image_url"] = urls[id]
		}
		out[i] = item
	}
	return out, nil
}
