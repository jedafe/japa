package comparison

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// itemRefDedupKey returns a stable string key for an item_ref byte slice.
// Empty or "{}" refs are normalized to empty so null JSON and empty JSON
// objects do not produce different keys for otherwise identical rows.
func itemRefDedupKey(ref []byte) string {
	s := string(ref)
	if s == "" || s == "{}" {
		return ""
	}

	return s
}

// dedupeComparisonItems removes duplicate comparison items, keeping the first
// occurrence per (position, item_ref, title). It returns the deduplicated slice
// and a map from any removed item ID to the surviving item ID.
func dedupeComparisonItems(items []gen.ListComparisonItemsByContentRow) ([]gen.ListComparisonItemsByContentRow, map[pgtype.UUID]pgtype.UUID) {
	seen := make(map[string]pgtype.UUID, len(items))
	deduped := make([]gen.ListComparisonItemsByContentRow, 0, len(items))
	idMap := make(map[pgtype.UUID]pgtype.UUID, len(items))

	for _, it := range items {
		key := fmt.Sprintf("%d:%s:%s", it.Position, itemRefDedupKey(it.ItemRef), it.Title)
		if existingID, ok := seen[key]; ok {
			idMap[it.ID] = existingID
			continue
		}

		seen[key] = it.ID
		idMap[it.ID] = it.ID
		deduped = append(deduped, it)
	}

	return deduped, idMap
}

// dedupeComparisonDimensions removes duplicate comparison dimensions, keeping
// the first occurrence per (position, label). It returns the deduplicated slice
// and a map from any removed dimension ID to the surviving dimension ID.
func dedupeComparisonDimensions(dims []gen.ListComparisonDimensionsByContentRow) ([]gen.ListComparisonDimensionsByContentRow, map[pgtype.UUID]pgtype.UUID) {
	seen := make(map[string]pgtype.UUID, len(dims))
	deduped := make([]gen.ListComparisonDimensionsByContentRow, 0, len(dims))
	idMap := make(map[pgtype.UUID]pgtype.UUID, len(dims))

	for _, d := range dims {
		key := fmt.Sprintf("%d:%s", d.Position, d.Label)
		if existingID, ok := seen[key]; ok {
			idMap[d.ID] = existingID
			continue
		}

		seen[key] = d.ID
		idMap[d.ID] = d.ID
		deduped = append(deduped, d)
	}

	return deduped, idMap
}

// mergeComparisonValues returns a value whose fields are filled from the first
// non-nil/valid occurrence across existing and incoming. This is used when the
// database contains duplicate item or dimension rows and the surviving row must
// inherit any data stored against the removed duplicates.
func mergeComparisonValues(existing, incoming gen.ComparisonValue) gen.ComparisonValue {
	if existing.Value == nil {
		existing.Value = incoming.Value
	}

	if !existing.ValueRating.Valid && incoming.ValueRating.Valid {
		existing.ValueRating = incoming.ValueRating
	}

	if existing.ValueBoolean == nil {
		existing.ValueBoolean = incoming.ValueBoolean
	}

	if existing.Metadata == nil || len(existing.Metadata) == 0 {
		existing.Metadata = incoming.Metadata
	}

	return existing
}

// dedupeJCEItemInputs removes duplicate JCE item inputs, keeping the first
// occurrence per (position, item_ref, title). Positions are preserved.
func dedupeJCEItemInputs(items []JCEItemInput) []JCEItemInput {
	seen := make(map[string]bool, len(items))
	deduped := make([]JCEItemInput, 0, len(items))

	for _, it := range items {
		key := fmt.Sprintf("%d:%s:%s", it.Position, itemRefDedupKey(it.ItemRef), it.Title)
		if seen[key] {
			continue
		}

		seen[key] = true

		deduped = append(deduped, it)
	}

	return deduped
}

// dedupeTokenItemRefs collapses duplicate guest-selection refs by their
// canonical item_ref bytes — the same normalization the matrix assembler
// uses — so mint-time max_item_count enforcement counts logical items.
func dedupeTokenItemRefs(refs []map[string]any) []map[string]any {
	seen := make(map[string]bool, len(refs))
	out := make([]map[string]any, 0, len(refs))

	for _, ref := range refs {
		b, err := json.Marshal(ref)
		if err != nil {
			continue
		}

		key := itemRefDedupKey(b)
		if key == "" {
			key = string(b)
		}

		if seen[key] {
			continue
		}

		seen[key] = true

		out = append(out, ref)
	}

	return out
}

// dedupeComparisonItemInputs removes duplicate legacy comparison item inputs,
// keeping the first occurrence per (index, name). The index becomes the
// position on insert, so duplicates with the same position/title are collapsed.
func dedupeComparisonItemInputs(items []ComparisonItemInput) []ComparisonItemInput {
	seen := make(map[string]bool, len(items))
	deduped := make([]ComparisonItemInput, 0, len(items))

	for i, it := range items {
		key := fmt.Sprintf("%d:%s", i, it.Name)
		if seen[key] {
			continue
		}

		seen[key] = true

		deduped = append(deduped, it)
	}

	return deduped
}

// dedupeStringDimensions removes duplicate dimension labels at the same
// position, keeping the first occurrence. The position is the slice index.
func dedupeStringDimensions(dims []string) []string {
	seen := make(map[string]bool, len(dims))
	deduped := make([]string, 0, len(dims))

	for i, d := range dims {
		if d == "" {
			continue
		}

		key := fmt.Sprintf("%d:%s", i, d)
		if seen[key] {
			continue
		}

		seen[key] = true

		deduped = append(deduped, d)
	}

	return deduped
}
