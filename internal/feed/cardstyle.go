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

	// The setting lives in JSONB, but the control PATCH path stores every
	// settings value as a JSON string (json.Marshal of a string), so an object
	// saved from the editor is double-encoded: the stored value is a JSON
	// string whose content is itself JSON. Unwrap one such layer, then decide.
	//   "reddit"                      -> scalar "reddit"      -> default reddit
	//   "{\"default\":...}"           -> inner object JSON    -> parse object
	//   {"default":...} (migration)   -> not a JSON string    -> parse object
	body := raw
	var unwrapped string
	if err := json.Unmarshal([]byte(raw), &unwrapped); err == nil {
		// raw was a JSON string. Its content is either a bare style scalar or
		// the JSON text of a {default, by_type} object.
		if validCardStyle(unwrapped) {
			cfg.Default = unwrapped
			return cfg
		}
		body = unwrapped
	}

	// Object form: {default, by_type} — either the direct JSONB object or the
	// inner text unwrapped above. Anything else falls back to the default.
	var obj CardStyleConfig
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
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
