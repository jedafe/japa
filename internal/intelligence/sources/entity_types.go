package sources

import (
	"encoding/json"
	"fmt"
)

// entityTypesKey is the provider_config key that declares the extraction
// domains a source publishes. When present and non-empty, the intel:extract
// task runs one extraction pass per listed entity_type instead of the single
// type derived from intel_sources.category — this is what lets a mixed-domain
// source (e.g. a site publishing both scholarships and jobs) populate every
// pill on its public /sources/:slug feed. Ref: sources directory v2 design
// (2026-09-11-sources-directory-design.md, "A single source can publish
// multiple entity types").
const entityTypesKey = "entity_types"

// EntityTypesFromProviderConfig reads the entity_types list out of a source's
// provider_config document. Values are returned verbatim — callers that need
// schema validation filter through schemas.ForEntityType. Returns nil when
// the key is absent, malformed, or an empty array.
func EntityTypesFromProviderConfig(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}

	ets, ok := doc[entityTypesKey]
	if !ok || len(ets) == 0 {
		return nil
	}

	var out []string
	if err := json.Unmarshal(ets, &out); err != nil {
		return nil
	}

	return out
}

// SetEntityTypesInProviderConfig merges an entity_types override into a
// source's provider_config document, preserving every other key (API-source
// endpoint config, crawl hints, …). An empty or nil types slice removes the
// key so the source falls back to its category-derived entity type. Returns
// the updated document, or an error when the existing document is not a JSON
// object.
func SetEntityTypesInProviderConfig(raw []byte, types []string) ([]byte, error) {
	doc := map[string]json.RawMessage{}

	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("sources: provider_config is not a JSON object: %w", err)
		}
	}

	if len(types) == 0 {
		delete(doc, entityTypesKey)
	} else {
		encoded, err := json.Marshal(types)
		if err != nil {
			return nil, fmt.Errorf("sources: encode entity_types: %w", err)
		}
		doc[entityTypesKey] = encoded
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("sources: encode provider_config: %w", err)
	}

	return out, nil
}
