package feed

import "encoding/json"

// Allowed feed card-style values.
const (
	CardStyleReddit   = "reddit"
	CardStyleCompact  = "compact"
	CardStyleGrid     = "grid"
	CardStyleMagazine = "magazine"
)

// DefaultCardStyle applies when no setting is present or a value is invalid.
const DefaultCardStyle = CardStyleReddit

// CardStyleConfig is a tenant's feed card-style configuration: a default
// style plus optional per-content-type overrides.
type CardStyleConfig struct {
	Default string            `json:"default"`
	ByType  map[string]string `json:"by_type"`
}

// validCardStyle reports whether s is one of the four allowed styles.
func validCardStyle(s string) bool {
	switch s {
	case CardStyleReddit, CardStyleCompact, CardStyleGrid, CardStyleMagazine:
		return true
	default:
		return false
	}
}

// resolveCardStyleConfig normalizes the raw JSONB value of the feed.card_style
// setting. It accepts an empty string (no setting), a legacy JSON string
// ("reddit"), or a JSON object {default, by_type}. Any invalid style is
// coerced: an invalid default becomes reddit; an invalid per-type entry is
// dropped. ByType is always non-nil.
func resolveCardStyleConfig(raw string) CardStyleConfig {
	cfg := CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}}

	if raw == "" {
		return cfg
	}

	// Legacy scalar: the value is a JSON string like "reddit".
	var scalar string
	if err := json.Unmarshal([]byte(raw), &scalar); err == nil {
		if validCardStyle(scalar) {
			cfg.Default = scalar
		}
		return cfg
	}

	// Object form: {default, by_type}.
	var obj CardStyleConfig
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return cfg
	}

	if validCardStyle(obj.Default) {
		cfg.Default = obj.Default
	}
	for ct, style := range obj.ByType {
		if validCardStyle(style) {
			cfg.ByType[ct] = style
		}
	}
	return cfg
}
