// Package resolver resolves item_ref references into normalized items that can
// be compared in the Japa Comparison Engine. It keeps the comparison package
// decoupled from the concrete services that own each entity type.
package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultSearchLimit int32 = 20
	// MaxSearchLimit is the largest page size the resolver will fetch for a single search.
	// This is intentionally large so that a comparison-type directory can load every item
	// without requiring server-side pagination for the initial release.
	MaxSearchLimit int32 = 100000
)

// ResolvedItem is the normalized representation of an entity that can be
// selected as a comparison item.
type ResolvedItem struct {
	Type        string         `json:"type"`
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	URL         string         `json:"url,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Resolver knows how to resolve and search one entity type.
type Resolver interface {
	Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error)
	Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error)
}

// Counter is implemented by resolvers that can report a total count for the
// same query and filters used by Search. A return value of -1 means the
// resolver does not support counting.
type Counter interface {
	Count(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID) (int64, error)
}

// Registry maps entity types to resolvers.
type Registry struct {
	resolvers map[string]Resolver
}

// NewRegistry creates an empty resolver registry.
func NewRegistry() *Registry {
	return &Registry{resolvers: make(map[string]Resolver)}
}

// Register adds a resolver for an entity type.
func (r *Registry) Register(entityType string, resolver Resolver) {
	if r == nil {
		return
	}
	if r.resolvers == nil {
		r.resolvers = make(map[string]Resolver)
	}
	if resolver == nil {
		return
	}
	r.resolvers[entityType] = resolver
}

// Resolve resolves an item_ref for the type declared in ref["type"].
func (r *Registry) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, error) {
	if r == nil || r.resolvers == nil {
		return ResolvedItem{}, errors.New("resolver registry not initialized")
	}
	typ, ok := ref["type"].(string)
	if !ok || typ == "" {
		return ResolvedItem{}, errors.New("item_ref missing type")
	}
	res, ok := r.resolvers[typ]
	if !ok {
		return ResolvedItem{}, fmt.Errorf("unsupported item_ref type: %s", typ)
	}
	item, found, err := res.Resolve(ctx, ref, tenantID)
	if err != nil {
		return ResolvedItem{}, fmt.Errorf("resolving %s: %w", typ, err)
	}
	if !found {
		return ResolvedItem{}, fmt.Errorf("%s item not found", typ)
	}
	item.Type = typ
	return item, nil
}

// Search searches for items of a given entity type.
func (r *Registry) Search(ctx context.Context, entityType, query string, filters map[string]any, tenantID pgtype.UUID, limit int32, offset int32) ([]ResolvedItem, error) {
	if r == nil || r.resolvers == nil {
		return nil, errors.New("resolver registry not initialized")
	}
	res, ok := r.resolvers[entityType]
	if !ok {
		return nil, fmt.Errorf("unsupported entity type: %s", entityType)
	}
	items, err := res.Search(ctx, query, filters, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", entityType, err)
	}
	for i := range items {
		items[i].Type = entityType
	}
	return items, nil
}

// Count returns the total number of matching items for the given entity type,
// or -1 if the resolver does not implement counting.
func (r *Registry) Count(ctx context.Context, entityType, query string, filters map[string]any, tenantID pgtype.UUID) (int64, error) {
	if r == nil || r.resolvers == nil {
		return -1, errors.New("resolver registry not initialized")
	}
	res, ok := r.resolvers[entityType]
	if !ok {
		return -1, fmt.Errorf("unsupported entity type: %s", entityType)
	}
	c, ok := res.(Counter)
	if !ok {
		return -1, nil
	}
	total, err := c.Count(ctx, query, filters, tenantID)
	if err != nil {
		return -1, fmt.Errorf("counting %s: %w", entityType, err)
	}
	return total, nil
}

// ParseUUID parses a UUID string into a pgtype.UUID.
func ParseUUID(s string) (pgtype.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid: %w", err)
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

func parseUUIDFilter(filters map[string]any, key string) pgtype.UUID {
	if v, ok := filters[key].(string); ok && v != "" {
		if id, err := ParseUUID(v); err == nil {
			return id
		}
	}
	return pgtype.UUID{}
}

func parseStringFilter(filters map[string]any, key string) string {
	if v, ok := filters[key].(string); ok {
		return v
	}
	return ""
}

// deepestGeoFilter returns the most specific geo scope ID present in filters.
// Priority (most specific first): explicit geo_space_id, university_id, city_id,
// state_id, country_id. This lets a content or DB-backed resolver narrow its
// search to the space selected in the /compare scope chain.
func deepestGeoFilter(filters map[string]any) pgtype.UUID {
	for _, key := range []string{"geo_space_id", "university_id", "city_id", "state_id", "country_id"} {
		if id := parseUUIDFilter(filters, key); id.Valid {
			return id
		}
	}
	return pgtype.UUID{}
}

// IsNotFound reports whether an error chain indicates a missing entity.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// CoerceMap coerces a JSON-encoded item_ref into a map[string]any.
func CoerceMap(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing item_ref: %w", err)
	}
	return m, nil
}

// Excerpt returns a short plain-text excerpt from a possibly HTML or JSON body.
func Excerpt(body string, maxLen int) string {
	plain := stripTagsAndJSON(body)
	plain = strings.Join(strings.Fields(plain), " ")
	if maxLen <= 0 {
		maxLen = 200
	}
	if len(plain) <= maxLen {
		return plain
	}
	return plain[:maxLen] + "…"
}

func stripTagsAndJSON(s string) string {
	var out strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if inTag {
			continue
		}
		out.WriteByte(c)
	}
	text := out.String()
	if len(text) > 0 && text[0] == '{' {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			var sb strings.Builder
			flattenJSONMap(m, &sb)
			return sb.String()
		}
	}
	return text
}

// coerceNumber converts a value to a numeric any (float64 or int) when
// possible. Strings that represent numbers and json.Number values are parsed.
// Pointers are dereferenced. It returns the value and true when a number was
// produced.
func coerceNumber(v any) (any, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int64(n), true
	case uint64:
		// Convert to float64 to avoid int64 overflow for values > math.MaxInt64.
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil, false
		}
		return f, true
	case string:
		if n == "" {
			return nil, false
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
		return nil, false
	case *float64:
		if n == nil {
			return nil, false
		}
		return *n, true
	case *float32:
		if n == nil {
			return nil, false
		}
		return float64(*n), true
	case *int:
		if n == nil {
			return nil, false
		}
		return *n, true
	case *int8:
		if n == nil {
			return nil, false
		}
		return int(*n), true
	case *int16:
		if n == nil {
			return nil, false
		}
		return int(*n), true
	case *int32:
		if n == nil {
			return nil, false
		}
		return int64(*n), true
	case *int64:
		if n == nil {
			return nil, false
		}
		return *n, true
	}
	return nil, false
}

// fieldValueNumber extracts a numeric value from a parsed field-values map,
// trying a list of possible keys in order.
func fieldValueNumber(fields map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := fields[k]; ok && v != nil {
			if n, ok := coerceNumber(v); ok {
				return n
			}
		}
	}
	return nil
}

// fieldValueBool extracts a boolean from a parsed field-values map, trying
// several possible keys in order.
func fieldValueBool(fields map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := fields[k]; ok && v != nil {
			if b, ok := coerceBool(v); ok {
				return b
			}
		}
	}
	return nil
}

// fieldValueStrings extracts a string slice from a parsed field-values map.
func fieldValueStrings(fields map[string]any, keys ...string) []string {
	for _, k := range keys {
		if v, ok := fields[k]; ok && v != nil {
			if s, ok := stringSlice(v); ok {
				return s
			}
		}
	}
	return nil
}

// stringSlice coerces a value to []string when it is a string slice or an
// []any where every element is a string.
func stringSlice(v any) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else {
				out = append(out, fmt.Sprint(item))
			}
		}
		return out, true
	case string:
		if x != "" {
			return []string{x}, true
		}
	}
	return nil, false
}

// coerceBool converts a value to a boolean when possible.
func coerceBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return reflectAnyToFloat(x) != 0, true
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if s == "" {
			return false, false
		}
		if b, err := strconv.ParseBool(s); err == nil {
			return b, true
		}
		switch s {
		case "yes", "y", "on", "enabled", "available", "allowed":
			return true, true
		case "no", "n", "off", "disabled", "unavailable", "not allowed":
			return false, true
		}
	}
	return false, false
}

// reflectAnyToFloat returns a float64 for numeric any values. It is a small
// helper used by coerceBool for int/float cases.
func reflectAnyToFloat(v any) float64 {
	if n, ok := coerceNumber(v); ok {
		f, _ := n.(float64)
		if i, ok := n.(int); ok {
			return float64(i)
		}
		if i, ok := n.(int64); ok {
			return float64(i)
		}
		return f
	}
	return 0
}

// languagesForGeoCode returns a best-effort language list for a 2- or 3-letter
// country code. It returns an empty slice when no mapping is known.
func languagesForGeoCode(code string) []string {
	if code == "" {
		return []string{}
	}
	c := strings.ToUpper(strings.TrimSpace(code))
	// Keep only the first 2- or 3-letter segment in case the code is compound.
	if len(c) > 3 {
		if idx := strings.Index(c, "-"); idx > 0 && idx <= 3 {
			c = c[:idx]
		} else if idx := strings.Index(c, "_"); idx > 0 && idx <= 3 {
			c = c[:idx]
		} else {
			c = c[:2]
		}
	}
	m := map[string][]string{
		"NG": {"English"},
		"US": {"English"},
		"GB": {"English"},
		"CA": {"English", "French"},
		"AU": {"English"},
		"NZ": {"English"},
		"IE": {"English"},
		"DE": {"German"},
		"AT": {"German"},
		"CH": {"German", "French", "Italian"},
		"FR": {"French"},
		"BE": {"Dutch", "French", "German"},
		"MC": {"French"},
		"ES": {"Spanish"},
		"MX": {"Spanish"},
		"AR": {"Spanish"},
		"CO": {"Spanish"},
		"PT": {"Portuguese"},
		"BR": {"Portuguese"},
		"AO": {"Portuguese"},
		"IT": {"Italian"},
		"NL": {"Dutch"},
		"SE": {"Swedish"},
		"NO": {"Norwegian"},
		"DK": {"Danish"},
		"FI": {"Finnish", "Swedish"},
		"RU": {"Russian"},
		"CN": {"Chinese"},
		"TW": {"Chinese"},
		"HK": {"Chinese", "English"},
		"JP": {"Japanese"},
		"KR": {"Korean"},
		"IN": {"Hindi", "English"},
		"PK": {"Urdu", "English"},
		"GH": {"English"},
		"KE": {"English", "Swahili"},
		"ZA": {"English", "Zulu"},
		"EG": {"Arabic"},
		"SA": {"Arabic"},
		"AE": {"Arabic", "English"},
		"IL": {"Hebrew", "Arabic"},
		"TR": {"Turkish"},
		"GR": {"Greek"},
		"PL": {"Polish"},
		"UA": {"Ukrainian"},
		"CZ": {"Czech"},
		"HU": {"Hungarian"},
		"RO": {"Romanian"},
		"TH": {"Thai"},
		"VN": {"Vietnamese"},
		"ID": {"Indonesian"},
		"MY": {"Malay"},
		"PH": {"Filipino", "English"},
		"SG": {"English", "Malay", "Chinese"},
		"IS": {"Icelandic"},
		"MT": {"Maltese", "English"},
		"LU": {"Luxembourgish", "French", "German"},
	}
	if langs, ok := m[c]; ok {
		return langs
	}
	return []string{}
}

func flattenJSONMap(m map[string]any, sb *strings.Builder) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch val := m[k].(type) {
		case string:
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(val)
		case []any:
			for _, item := range val {
				if m2, ok := item.(map[string]any); ok {
					flattenJSONMap(m2, sb)
				}
			}
		}
	}
}
