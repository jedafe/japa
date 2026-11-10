package content

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ref: japaconnect-engineering/sections/14-content-type-system.md
// content_media_fields tracks every media row a contents row references so
// deletes can report "in use" instead of silently orphaning rendered
// images. The set is replaced wholesale on each save (delete + insert in
// the same write) — a save that drops a reference drops the tracking row.

// mediaRef is one tracked reference: which media, under which field_key
// ('header_image' | 'featured_image' | 'item_media' | 'body_block' | a
// custom field key), at which position within that key.
type mediaRef struct {
	MediaID  pgtype.UUID
	FieldKey string
	Position int32
}

// MediaRefSource carries the values that were actually written to the
// contents row — for updates this is the post-merge params (preserve-
// on-omit already resolved), not the raw input.
type MediaRefSource struct {
	ContentType     string
	GuideCategory   *string
	FieldValues     []byte
	Body            string
	BodyHTML        string
	OgImageURL      string
	HeaderImageID   pgtype.UUID
	FeaturedImageID pgtype.UUID
}

// mediaServeURL matches /media/serve/{bucket}/{path} substrings wherever
// they appear — TipTap image src attrs, markdown image syntax, or raw
// <img> HTML. The path captures through query/fragment-free characters.
var mediaServeURL = regexp.MustCompile(`media/serve/([a-z0-9_-]+)/([0-9A-Za-z_\-\./%]+)`)

// mediaValueIDs extracts media IDs from one media-typed custom field value
// using the documented storage shapes (the same set wiki's
// parseMediaFieldRefs accepts, plus media_id/media_ids keys):
//
//	"<uuid>"                          — bare id
//	{"value": "<uuid>"}               — wrapped single (select convention)
//	{"id": "<uuid>"} / {"media_id": "<uuid>"} — object with id key
//	["<uuid>", …]                     — id array
//	{"value": ["<uuid>", …]}          — wrapped array
//	{"media_ids": ["<uuid>", …]}      — §80 documented envelope
//	[{"id": "<uuid>"}, …]             — object array
//
// Structured extraction beats a bare UUID regex: a UUID-shaped string in
// an unrelated key (a label, a description, a foreign reference) is not a
// media reference.
func mediaValueIDs(raw json.RawMessage) []pgtype.UUID {
	var ids []pgtype.UUID

	addID := func(s string) {
		var id pgtype.UUID
		if err := id.Scan(s); err == nil && id.Valid {
			ids = append(ids, id)
		}
	}
	addObj := func(m map[string]any) {
		for _, k := range []string{"id", "media_id"} {
			if s, _ := m[k].(string); s != "" {
				addID(s)
			}
		}
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}

	// Unwrap the {"value": …} select-style envelope and the
	// {"media_ids": […]} §80 envelope.
	if m, ok := probe.(map[string]any); ok {
		if inner, exists := m["value"]; exists {
			probe = inner
		} else if inner, exists := m["media_ids"]; exists {
			probe = inner
		}
	}

	switch t := probe.(type) {
	case string:
		addID(t)
	case map[string]any:
		addObj(t)
	case []any:
		for _, el := range t {
			switch e := el.(type) {
			case string:
				addID(e)
			case map[string]any:
				addObj(e)
			}
		}
	}

	return ids
}

// SyncMediaRefs replaces the content's tracked media references with the
// set derived from src. write is the querier for the delete+insert so the
// sync joins the caller's transaction when one is open; read resolves the
// referenced rows. A collection error aborts before the delete — a
// transient read failure must never wipe tracking rows the content still
// references.
//
// Exported for writers that bypass the content service (roadmap and
// mentorship shadow contents rows insert via their own queries).
func SyncMediaRefs(ctx context.Context, read, write *gen.Queries, contentID pgtype.UUID, src MediaRefSource) error {
	refs, err := collectMediaRefs(ctx, read, write, src)
	if err != nil {
		return err
	}

	if err := write.DeleteContentMediaFields(ctx, contentID); err != nil {
		return err
	}

	if len(refs) == 0 {
		return nil
	}

	params := gen.InsertContentMediaFieldsParams{
		ContentID: contentID,
		MediaIds:  make([]pgtype.UUID, 0, len(refs)),
		FieldKeys: make([]string, 0, len(refs)),
		Positions: make([]int32, 0, len(refs)),
	}

	for _, r := range refs {
		params.MediaIds = append(params.MediaIds, r.MediaID)
		params.FieldKeys = append(params.FieldKeys, r.FieldKey)
		params.Positions = append(params.Positions, r.Position)
	}

	return write.InsertContentMediaFields(ctx, params)
}

// collectMediaRefs gathers every media reference the saved values carry.
// References pointing at media rows that don't exist are dropped rather
// than failing the write — the tracking table must never reject a save
// the contents row itself accepted. Resolution errors propagate so the
// caller aborts before touching stored rows.
func collectMediaRefs(ctx context.Context, read, write *gen.Queries, src MediaRefSource) ([]mediaRef, error) {
	var refs []mediaRef

	if src.HeaderImageID.Valid {
		refs = append(refs, mediaRef{src.HeaderImageID, "header_image", 0})
	}
	if src.FeaturedImageID.Valid {
		refs = append(refs, mediaRef{src.FeaturedImageID, "featured_image", 0})
	}

	if src.ContentType == TypeList {
		refs = append(refs, listItemMediaRefs(src.FieldValues)...)
	}

	bodyRefs, err := bodyMediaRefs(ctx, write, src)
	if err != nil {
		return nil, err
	}
	refs = append(refs, bodyRefs...)

	cfRefs, err := customFieldMediaRefs(ctx, write, src)
	if err != nil {
		return nil, err
	}
	refs = append(refs, cfRefs...)

	// Validate the referenced rows exist — a ref pointing at a missing
	// media row would violate the FK on insert. Soft-deleted rows count:
	// the FK still holds, the object keeps serving, and dropping the ref
	// would let the cleanup sweep purge media the content still renders.
	if len(refs) == 0 {
		return nil, nil
	}

	ids := make([]pgtype.UUID, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.MediaID)
	}

	existing, err := read.ListExistingMediaIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("resolving media references: %w", err)
	}

	live := make(map[pgtype.UUID]bool, len(existing))
	for _, id := range existing {
		live[id] = true
	}

	out := refs[:0]
	for _, r := range refs {
		if live[r.MediaID] {
			out = append(out, r)
		}
	}

	return out, nil
}

// listItemMediaRefs extracts item media references with their item index —
// position is the item slot so duplicate media across items stays tracked.
func listItemMediaRefs(fieldValues []byte) []mediaRef {
	var parsed struct {
		Items []listItem `json:"items"`
	}

	if err := json.Unmarshal(fieldValues, &parsed); err != nil {
		return nil
	}

	var refs []mediaRef
	for i, it := range parsed.Items {
		if it.MediaID == "" {
			continue
		}

		var id pgtype.UUID
		if err := id.Scan(it.MediaID); err != nil {
			continue
		}

		refs = append(refs, mediaRef{id, "item_media", int32(i)})
	}

	return refs
}

// bodyMediaRefs resolves /media/serve/{bucket}/{path} URLs in body,
// body_html, and og_image_url back to media rows. Body positions follow
// match order across the combined text; og_image_url tracks under
// 'og_image' position 0 — the same media embedded in both surfaces is
// tracked once per field.
func bodyMediaRefs(ctx context.Context, q *gen.Queries, src MediaRefSource) ([]mediaRef, error) {
	combined := src.Body + "\n" + src.BodyHTML
	matches := mediaServeURL.FindAllStringSubmatch(combined, -1)
	ogMatch := mediaServeURL.FindStringSubmatch(src.OgImageURL)
	if len(matches) == 0 && ogMatch == nil {
		return nil, nil
	}

	// Dedupe coordinate pairs before resolving — the same URL can appear
	// in body (TipTap src), body_html (rendered <img>), and og_image_url.
	type coord struct{ bucket, path string }
	var buckets, paths []string
	seen := map[coord]bool{}
	addCoord := func(m []string) {
		c := coord{m[1], m[2]}
		if seen[c] {
			return
		}
		seen[c] = true
		buckets = append(buckets, c.bucket)
		paths = append(paths, c.path)
	}

	for _, m := range matches {
		addCoord(m)
	}
	if ogMatch != nil {
		addCoord(ogMatch)
	}

	rows, err := q.ListMediaIDsByStoragePaths(ctx, gen.ListMediaIDsByStoragePathsParams{
		Buckets: buckets,
		Paths:   paths,
	})
	if err != nil {
		return nil, fmt.Errorf("resolving body media paths: %w", err)
	}

	byCoord := make(map[coord]pgtype.UUID, len(rows))
	for _, r := range rows {
		byCoord[coord{r.StorageBucket, r.StoragePath}] = r.ID
	}

	// Re-walk matches in order; first occurrence of each media row claims a
	// position — the same image embedded twice tracks once per field.
	seenMedia := map[pgtype.UUID]bool{}
	var refs []mediaRef
	pos := int32(0)

	for _, m := range matches {
		id, ok := byCoord[coord{m[1], m[2]}]
		if !ok || seenMedia[id] {
			continue
		}
		seenMedia[id] = true
		refs = append(refs, mediaRef{id, "body_block", pos})
		pos++
	}

	if ogMatch != nil {
		if id, ok := byCoord[coord{ogMatch[1], ogMatch[2]}]; ok {
			refs = append(refs, mediaRef{id, "og_image", 0})
		}
	}

	return refs, nil
}

// customFieldMediaRefs extracts media IDs from media-typed custom field
// values in field_values. Keys come from the groups assigned to the
// content's entity classes — the same classes validateRequiredFields uses.
func customFieldMediaRefs(ctx context.Context, q *gen.Queries, src MediaRefSource) ([]mediaRef, error) {
	if len(src.FieldValues) == 0 {
		return nil, nil
	}

	classes := []string{"content_type", "content_type:" + src.ContentType}
	if src.ContentType == TypeGuide && src.GuideCategory != nil && *src.GuideCategory != "" {
		classes = append(classes, "content_type:guide:"+*src.GuideCategory)
	}

	keys, err := q.ListMediaFieldKeysByEntityClasses(ctx, classes)
	if err != nil {
		return nil, fmt.Errorf("listing media field keys: %w", err)
	}

	if len(keys) == 0 {
		return nil, nil
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal(src.FieldValues, &values); err != nil {
		return nil, nil
	}

	var refs []mediaRef
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}

		for pos, id := range mediaValueIDs(raw) {
			refs = append(refs, mediaRef{id, key, int32(pos)})
		}
	}

	return refs, nil
}


