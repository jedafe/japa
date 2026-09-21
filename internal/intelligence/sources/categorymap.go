package sources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

// defaultCategoryMapJSON holds the canonical category→method/pipeline/topics
// map from the intel-channels curation design (spec §2). It is the factory
// default for the `system_settings.intel.category_method_map` jsonb setting —
// admins can tune the live copy through the control settings UI without a
// redeploy; this embedded copy is what a fresh/empty/reset setting resolves
// against.
//
//go:embed category_map.json
var defaultCategoryMapJSON []byte

// CategoryMapping is one entry of the category→method map: what a single
// intel category implies about the entity types a source publishes, the crawl
// methods worth probing, the pipeline its records take, and the topic
// vocabulary that seeds channel relevance_topics (spec §2, §4).
type CategoryMapping struct {
	// EntityTypes are schemas-registry keys (news, job, article, event,
	// dataset, …) that records from matching channels validate against.
	// Empty for special-cased categories like video.
	EntityTypes []string `json:"entity_types"`
	// Methods are crawl methods (CrawlMethod values) the probe should
	// consider for the category. "channel_sync" on video is not an intel
	// crawl method — it marks the §106 video_channel flow; the resolver
	// returns it verbatim and callers route it.
	Methods []string `json:"methods"`
	// Pipeline is the processing path (extract|curate|hybrid) channels
	// under this category default to.
	Pipeline Pipeline `json:"pipeline"`
	// Topics seed channel.relevance_topics for the deterministic
	// relevance gate.
	Topics []string `json:"topics"`
}

// ResolvedMap is the merged result of resolving a set of selected categories
// against the category→method map: the inputs a caller needs to propose
// channels (entity_types for validation, methods to probe, pipeline for
// routing, topics for relevance seeding).
type ResolvedMap struct {
	// EntityTypes is the union of the selected categories' entity_types,
	// preserving first-seen order across the input category order.
	EntityTypes []string
	// Methods is the union of the selected categories' methods in
	// first-seen order — ranking probe candidates.
	Methods []string
	// Pipeline is the SOURCE-LEVEL merge: extract when ANY selected category
	// maps to extract; otherwise the first-seen pipeline of the resolved
	// categories. Empty when no category resolved. Per-channel assignment
	// uses ForMethod instead — a channel takes the pipeline of the
	// categories that nominated its method, so a /feed rss channel on a
	// [news, job] source curates (news) even though the merged pipeline is
	// extract (job forces it at the source level).
	Pipeline Pipeline
	// Topics is the union of the selected categories' topics in
	// first-seen order.
	Topics []string
	// Entries preserves each resolved category→mapping pair in input
	// order — the basis for per-method channel attribution (ForMethod).
	// Categories absent from the map are omitted: they may still
	// contribute a schema-registry entity type to the merged union but
	// nominate no methods.
	Entries []CategoryEntry
	// Warnings lists non-fatal resolution issues: unknown categories that
	// were skipped and a fallen-back-to-default note when settingsJSON
	// was unparseable. Callers decide whether to surface or log them;
	// resolution never errors on bad input.
	Warnings []string
}

// CategoryEntry is one resolved category→mapping pair, recorded in the
// input category order.
type CategoryEntry struct {
	Category string
	Mapping  CategoryMapping
}

// MethodResolution is the per-channel slice of the resolved map for one
// crawl method: the categories that listed the method nominate the
// channel's entity vocabulary and pipeline.
type MethodResolution struct {
	// EntityTypes is the union of the nominating categories' entity types,
	// first-seen order.
	EntityTypes []string
	// Pipeline is extract when ANY nominating category maps to extract;
	// otherwise the first-seen nominating pipeline.
	Pipeline Pipeline
}

// ForMethod resolves the channel-scoped view of the map for one crawl
// method. ok=false when no resolved category nominates the method —
// callers then fall back to the merged ResolvedMap fields (empty or
// map-absent selections) or the method default.
//
// The intel-channels exit scenario depends on this: on a [news, job]
// source the /feed rss channel is nominated by 'news' alone, so it
// materializes rss→curate→news even though the merged pipeline is
// extract ('job' forces extract at the source level).
func (r ResolvedMap) ForMethod(method string) (MethodResolution, bool) {
	var (
		out        MethodResolution
		first      Pipeline
		seenEntity = map[string]bool{}
		found      bool
	)

	for _, e := range r.Entries {
		if !slices.Contains(e.Mapping.Methods, method) {
			continue
		}
		found = true

		for _, et := range e.Mapping.EntityTypes {
			if et == "" || seenEntity[et] {
				continue
			}
			seenEntity[et] = true
			out.EntityTypes = append(out.EntityTypes, et)
		}
		if first == "" {
			first = e.Mapping.Pipeline
		}
		if e.Mapping.Pipeline == PipelineExtract {
			out.Pipeline = PipelineExtract
		}
	}

	if !found {
		return MethodResolution{}, false
	}
	if out.Pipeline != PipelineExtract {
		out.Pipeline = first
	}

	return out, true
}

// DefaultCategoryMap returns the embedded canonical category→method map JSON
// (spec §2), suitable for seeding `system_settings.intel.category_method_map`
// or for callers that want the factory default without going through
// ResolveCategoryMap.
func DefaultCategoryMap() []byte {
	return defaultCategoryMapJSON
}

// ParseCategoryMap decodes a category→method map document (the shape stored
// in system_settings.intel.category_method_map) into per-category mappings.
// An empty/nil document returns an empty map, not an error — callers decide
// whether to fall back to DefaultCategoryMap. Returns an error only when the
// document is not a JSON object of CategoryMapping values.
func ParseCategoryMap(settingsJSON []byte) (map[string]CategoryMapping, error) {
	if len(settingsJSON) == 0 {
		return map[string]CategoryMapping{}, nil
	}

	var catMap map[string]CategoryMapping
	if err := json.Unmarshal(settingsJSON, &catMap); err != nil {
		return nil, fmt.Errorf("sources: category map: %w", err)
	}

	return catMap, nil
}

// ResolveCategoryMap merges the map entries for the selected categories into
// one ResolvedMap. Resolution is deterministic and driven by the input
// category order (map iteration order never leaks in):
//
//   - entity_types, methods, topics: union preserving first-seen order —
//     category order first, then each entry's array order.
//   - pipeline: extract when ANY selected category is extract, else the
//     first-seen pipeline of the resolved categories.
//   - unknown, blank, or duplicate categories are skipped; each unknown
//     category adds a Warnings entry (duplicates warn once).
//
// An empty/nil settingsJSON resolves against the embedded DefaultCategoryMap;
// a malformed one falls back to the default with a warning so a corrupt
// setting degrades to spec defaults rather than resolving nothing.
//
// The video category is returned faithfully (empty entity_types, method
// "channel_sync", curate pipeline) — it is not an intel channel; callers
// route it to the §106 video_channel flow.
func ResolveCategoryMap(settingsJSON []byte, categories []string) ResolvedMap {
	var resolved ResolvedMap

	catMap, err := ParseCategoryMap(settingsJSON)
	if err != nil {
		resolved.Warnings = append(resolved.Warnings,
			fmt.Sprintf("category map unparseable, using embedded default: %v", err))
		catMap, err = ParseCategoryMap(defaultCategoryMapJSON)
		if err != nil {
			// The embedded map is shipped data — if it cannot parse the
			// resolver still must not error; every category becomes a
			// warning below.
			resolved.Warnings = append(resolved.Warnings,
				fmt.Sprintf("embedded category map unparseable: %v", err))
			catMap = map[string]CategoryMapping{}
		}
	} else if len(catMap) == 0 {
		// Absent/empty setting means "factory default".
		catMap, err = ParseCategoryMap(defaultCategoryMapJSON)
		if err != nil {
			resolved.Warnings = append(resolved.Warnings,
				fmt.Sprintf("embedded category map unparseable: %v", err))
			catMap = map[string]CategoryMapping{}
		}
	}

	seenCat := map[string]bool{}
	seenEntity := map[string]bool{}
	seenMethod := map[string]bool{}
	seenTopic := map[string]bool{}
	anyExtract := false
	var firstPipeline Pipeline

	for _, raw := range categories {
		cat := strings.TrimSpace(raw)
		if cat == "" || seenCat[cat] {
			continue
		}
		seenCat[cat] = true

		mapping, ok := catMap[cat]
		if ok {
			resolved.Entries = append(resolved.Entries, CategoryEntry{Category: cat, Mapping: mapping})
		}
		if !ok {
			// Categories absent from the map still resolve their entity
			// type through the schemas registry (remittance, announcement,
			// housing, …) — otherwise their channels materialize with empty
			// entity_types and are skipped, leaving the approved source
			// with no channel at all. Methods/topics stay empty for these:
			// an unmapped category contributes no method restriction and no
			// topic vocabulary.
			if et := schemas.CategoryToEntityType(cat); et != "" {
				resolved.Warnings = append(resolved.Warnings,
					fmt.Sprintf("category %q not in category map; entity type resolved via schema registry", cat))
				if !seenEntity[et] {
					seenEntity[et] = true
					resolved.EntityTypes = append(resolved.EntityTypes, et)
				}
			} else {
				resolved.Warnings = append(resolved.Warnings,
					fmt.Sprintf("unknown category %q skipped", cat))
			}
			continue
		}

		for _, et := range mapping.EntityTypes {
			if !seenEntity[et] {
				seenEntity[et] = true
				resolved.EntityTypes = append(resolved.EntityTypes, et)
			}
		}
		for _, m := range mapping.Methods {
			if !seenMethod[m] {
				seenMethod[m] = true
				resolved.Methods = append(resolved.Methods, m)
			}
		}
		for _, t := range mapping.Topics {
			if !seenTopic[t] {
				seenTopic[t] = true
				resolved.Topics = append(resolved.Topics, t)
			}
		}

		if firstPipeline == "" {
			firstPipeline = mapping.Pipeline
		}
		if mapping.Pipeline == PipelineExtract {
			anyExtract = true
		}
	}

	if anyExtract {
		resolved.Pipeline = PipelineExtract
	} else {
		resolved.Pipeline = firstPipeline
	}

	return resolved
}
