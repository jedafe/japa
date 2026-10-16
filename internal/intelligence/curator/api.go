package curator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// API curates api crawl-method channels: the raw doc body is a JSON
// response (already path-selected by the fetcher when provider_config.path
// is set). Each object in the result becomes one record, mapped through
// the channel's provider_config field_map — so an API source produces
// fully structured intel_records without touching an LLM.
//
// provider_config contract (channel-level):
//
//	{
//	  "field_map":   {"title": "oppTitle", "deadline": "closeDate", "country": "countryCode"},
//	  "title_field": "oppTitle",            // fallback title key when field_map lacks "title"
//	  "key_fields":  ["oppId"],             // external_key hash inputs; default: whole record
//	  "entity_type": "scholarship",         // record entity-type override
//	  "constants":   {"country": "USA"}     // field values every record gets — the endpoint's
//	}                                       // implied context (a US grants API is always USA)
//
// field_map values are dot paths inside one record object ("address.city",
// "0" indexes arrays). Unmapped scalar keys are preserved verbatim under
// the `data` jsonb field so nothing is lost for review. When no field_map
// is configured, common key names are guessed (title/name, url/link,
// deadline/closeDate, description/summary, country/location…); a record
// with no resolvable title falls back to the extract pipeline.
type API struct{}

// Handles returns the crawl method this curator serves.
func (API) Handles() string { return string(sources.CrawlMethodAPI) }

// apiConfig is the channel provider_config contract for API curation.
type apiConfig struct {
	// FieldMap maps a schema field name to a dot path within one record.
	FieldMap map[string]string `json:"field_map"`
	// TitleField names the record key used for the required `title` field
	// when field_map has no title entry.
	TitleField string `json:"title_field"`
	// KeyFields are record keys whose values hash into the external key.
	KeyFields []string `json:"key_fields"`
	// EntityType overrides the record entity type.
	EntityType string `json:"entity_type"`
	// RecordsPath, when set, selects the array of record objects within the
	// body (dot path). The fetcher may already have narrowed the body via
	// provider_config.path — both are honoured.
	RecordsPath string `json:"records_path"`
	// Constants stamps a fixed value on every record — for facts implied by
	// the endpoint itself rather than present in the payload (a national
	// grants API implies its country; a scholarship database implies its
	// domain). Applied after field_map/guesses, so it only fills gaps.
	Constants map[string]string `json:"constants"`
	// URLTemplate renders application_url per record — "{field}"
	// placeholders resolve from the record's top-level scalar values
	// (e.g. "https://www.grants.gov/search-results-detail/{id}").
	URLTemplate string `json:"url_template"`
}

// commonTitleKeys/… are the guess lists used when provider_config carries
// no field_map — generic JSON APIs reuse a small vocabulary.
var (
	apiTitleKeys       = []string{"title", "name", "oppTitle", "program", "programName", "subject", "headline", "position", "jobTitle"}
	apiURLKeys         = []string{"url", "link", "applicationUrl", "application_url", "applyUrl", "oppUrl", "webUrl", "permalink"}
	apiDeadlineKeys    = []string{"deadline", "closeDate", "closingDate", "endDate", "dueDate", "expiresAt", "validThrough", "applicationDeadline"}
	apiDescriptionKeys = []string{"description", "summary", "desc", "abstract", "synopsis", "about"}
	apiCountryKeys     = []string{"country", "countryCode", "country_code", "location", "hostCountry", "destination"}
	apiAmountKeys      = []string{"amount", "funding", "value", "awardCeiling", "awardFloor", "salary", "award"}
	apiIDKeys          = []string{"id", "uid", "oppId", "number", "oppNumber", "slug", "key"}
)

// Curate maps each JSON object in the body to a record.
func (API) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	if len(body) == 0 {
		return nil, errors.New("curator: api: empty body")
	}

	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("curator: api: unmarshal body: %w", err)
	}

	cfg := parseAPIConfig(ch.ProviderConfig)

	objects, err := apiRecordObjects(root, cfg.RecordsPath)
	if err != nil {
		return nil, err
	}

	if len(objects) == 0 {
		return nil, errors.New("curator: api: no record objects in body")
	}

	entityType := resolveEntityType(cfg.EntityType, ch)
	schema, _ := schemas.ForEntityType(entityType)

	out := make([]Record, 0, len(objects))
	thin := 0

	for _, obj := range objects {
		ext, key := apiObjectToExtraction(obj, cfg, schema, ch)
		if ext == nil {
			thin++
			continue
		}

		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: key,
			EntityType:  entityType,
		})
	}

	// Every object lacking a title means the field_map doesn't match this
	// payload — defer to the extract pipeline rather than persist junk.
	if len(out) == 0 {
		if thin > 0 {
			return nil, fmt.Errorf("%w: %d record objects had no resolvable title", ErrNoCurator, thin)
		}

		return nil, errors.New("curator: api: no usable records")
	}

	return out, nil
}

// apiRecordObjects resolves the list of record objects: records_path
// selects a nested array; a top-level array is used directly; a bare
// object is treated as one record.
func apiRecordObjects(root any, recordsPath string) ([]map[string]any, error) {
	node := root

	if recordsPath != "" {
		v, err := apiWalkPath(root, recordsPath)
		if err != nil {
			return nil, fmt.Errorf("curator: api: records_path %q: %w", recordsPath, err)
		}

		node = v
	}

	switch t := node.(type) {
	case []any:
		out := make([]map[string]any, 0, len(t))

		for _, el := range t {
			if m, ok := el.(map[string]any); ok {
				out = append(out, m)
			}
		}

		return out, nil
	case map[string]any:
		// A bare object that itself holds a single dominant array of
		// objects ({"results": [...], "total": 5}) — use the largest object
		// array so the common envelope shape works without config.
		var best []any

		for _, v := range t {
			arr, ok := v.([]any)
			if !ok || len(arr) == 0 {
				continue
			}

			if _, isObj := arr[0].(map[string]any); !isObj {
				continue
			}

			if len(arr) > len(best) {
				best = arr
			}
		}

		if len(best) > 0 {
			out := make([]map[string]any, 0, len(best))

			for _, el := range best {
				if m, ok := el.(map[string]any); ok {
					out = append(out, m)
				}
			}

			return out, nil
		}

		return []map[string]any{t}, nil
	default:
		return nil, errors.New("curator: api: body is not a JSON object or array")
	}
}

// apiObjectToExtraction maps one JSON object to an extraction. Returns nil
// when no title-like field resolves — a record without a title fails
// schema validation and pollutes the queue.
func apiObjectToExtraction(obj map[string]any, cfg apiConfig, schema schemas.EntitySchema, ch sources.Channel) (extractor.Extraction, string) {
	ext := make(extractor.Extraction)
	mappedKeys := map[string]bool{} // top-level obj keys consumed by field_map

	// Configured field_map first — the declared contract.
	for fieldName, path := range cfg.FieldMap {
		v, err := apiWalkPath(obj, path)
		if err != nil || v == nil {
			continue
		}

		mappedKeys[strings.SplitN(path, ".", 2)[0]] = true

		if s := apiScalar(v); s != "" {
			field(ext, fieldName, apiCoerce(schema, fieldName, v, s), apiEvidence(path, v))
		} else if nonScalar := schemaFieldType(schema, fieldName); nonScalar == "text_array" || nonScalar == "country_array" || nonScalar == "jsonb_object" {
			// Arrays and objects pass through for composite-typed fields.
			field(ext, fieldName, v, apiEvidence(path, v))
		}
	}

	// Guessed keys fill whatever the map didn't cover.
	guess := func(fieldName string, keys []string) {
		if hasField(ext, fieldName) {
			return
		}

		for _, k := range keys {
			if v, ok := lookupKeyFold(obj, k); ok {
				if s := apiScalar(v); s != "" {
					fieldConf(ext, fieldName, apiCoerce(schema, fieldName, v, s), apiEvidence(k, v), heuristicFieldConfidence)
					return
				}
			}
		}
	}

	guess("title", append([]string{cfg.TitleField}, apiTitleKeys...))
	guess("application_url", apiURLKeys)
	guess("deadline", apiDeadlineKeys)
	guess("description_summary", apiDescriptionKeys)
	guess("country", apiCountryKeys)
	guess("funding", apiAmountKeys)

	// Constants fill fields the payload never carries.
	for fieldName, value := range cfg.Constants {
		if !hasField(ext, fieldName) && strings.TrimSpace(value) != "" {
			field(ext, fieldName, apiCoerce(schema, fieldName, value, value), "provider_config.constants."+fieldName)
		}
	}

	// URL template renders the apply link when the payload lacks one.
	if !hasField(ext, "application_url") && cfg.URLTemplate != "" {
		if u := renderURLTemplate(cfg.URLTemplate, obj); u != "" {
			field(ext, "application_url", u, cfg.URLTemplate)
		}
	}

	if !hasField(ext, "title") {
		return nil, ""
	}

	// Preserve unmapped scalar fields under `data` — reviewers see the full
	// source object even where no schema field exists for it.
	data := map[string]any{}

	for k, v := range obj {
		if mappedKeys[k] {
			continue
		}

		if s := apiScalar(v); s != "" {
			data[k] = v
		}
	}

	if len(data) > 0 {
		if raw, err := json.Marshal(obj); err == nil {
			fieldConf(ext, "data", data, string(raw), heuristicFieldConfidence)
		}
	}

	if ch.EntryUrl != "" {
		field(ext, "source_url", ch.EntryUrl, ch.EntryUrl)
	}

	return ext, apiRecordKey(obj, cfg)
}

// apiWalkPath resolves a dot path inside a decoded JSON value — "a.b.c"
// walks objects, numeric segments index arrays.
func apiWalkPath(v any, path string) (any, error) {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[seg]
			if !ok {
				return nil, fmt.Errorf("missing segment %q", seg)
			}

			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(node) {
				return nil, fmt.Errorf("bad array index %q", seg)
			}

			cur = node[i]
		default:
			return nil, fmt.Errorf("cannot traverse %q", seg)
		}
	}

	return cur, nil
}

// lookupKeyFold does a case-insensitive single-level key lookup — APIs are
// inconsistent about camelCase vs snake_case for the same concept.
func lookupKeyFold(obj map[string]any, want string) (any, bool) {
	for k, v := range obj {
		if strings.EqualFold(k, want) {
			return v, true
		}
	}

	return nil, false
}

// apiScalar renders a JSON value as a string for the value/evidence pair.
// Objects and arrays are skipped — they are not leaf values.
func apiScalar(v any) string {
	switch t := v.(type) {
	case string:
		// APIs embed HTML entities in display strings (&ndash;, &amp;).
		return strings.TrimSpace(html.UnescapeString(t))
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

// apiCoerce shapes a value for its schema field type: array-typed fields
// get []any; date fields get ISO-normalized strings; everything else keeps
// the JSON-native value (the numeric coercer accepts both).
func apiCoerce(schema schemas.EntitySchema, fieldName string, v any, scalar string) any {
	switch {
	case isAPIArrayTyped(schema, fieldName):
		if arr, ok := v.([]any); ok {
			return arr
		}

		return []any{scalar}
	case schemaFieldType(schema, fieldName) == "date":
		if iso := parseHumanDate(scalar); iso != "" {
			return iso
		}

		if iso := ldjsonDate(scalar); iso != "" {
			return iso
		}

		return scalar
	case schemaFieldType(schema, fieldName) == "numeric":
		// Numeric fields keep the JSON-native value so float64 lands cleanly.
		return v
	default:
		// Strings take the unescaped scalar — the raw JSON value can carry
		// HTML entities (&ndash;) that apiScalar already cleaned.
		return scalar
	}
}

func schemaFieldType(schema schemas.EntitySchema, fieldName string) string {
	for _, f := range schema.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	return ""
}

func isAPIArrayTyped(schema schemas.EntitySchema, fieldName string) bool {
	t := schemaFieldType(schema, fieldName)

	return t == "text_array" || t == "country_array"
}

// apiEvidence renders a compact "path=value" evidence string.
func apiEvidence(path string, v any) string {
	if s := apiScalar(v); s != "" {
		if len(s) > 200 {
			s = s[:197] + "..."
		}

		return path + "=" + s
	}

	if raw, err := json.Marshal(v); err == nil {
		s := string(raw)
		if len(s) > 200 {
			s = s[:197] + "..."
		}

		return path + "=" + s
	}

	return path
}

// apiRecordKey derives a stable external key: configured key_fields values
// hashed, else a hash over the whole object.
func apiRecordKey(obj map[string]any, cfg apiConfig) string {
	var parts []string

	for _, kf := range cfg.KeyFields {
		if v, err := apiWalkPath(obj, kf); err == nil {
			if s := apiScalar(v); s != "" {
				parts = append(parts, s)
			}
		}
	}

	if len(parts) == 0 {
		for _, kf := range apiIDKeys {
			if v, ok := lookupKeyFold(obj, kf); ok {
				if s := apiScalar(v); s != "" {
					parts = append(parts, s)
					break
				}
			}
		}
	}

	if len(parts) == 0 {
		if raw, err := json.Marshal(obj); err == nil {
			parts = append(parts, string(raw))
		}
	}

	return "api:" + hashKey(parts...)
}

// renderURLTemplate substitutes {field} placeholders with the record's
// top-level scalar values. An unresolved placeholder yields "" — a link
// with a hole in it is worse than none.
func renderURLTemplate(tmpl string, obj map[string]any) string {
	out := tmpl

	for {
		start := strings.Index(out, "{")
		if start < 0 {
			return out
		}

		end := strings.Index(out[start:], "}")
		if end < 0 {
			return ""
		}

		key := out[start+1 : start+end]

		v, ok := obj[key]
		if !ok {
			return ""
		}

		s := apiScalar(v)
		if s == "" {
			return ""
		}

		out = out[:start] + s + out[start+end+1:]
	}
}

func parseAPIConfig(raw []byte) apiConfig {
	var cfg apiConfig
	if len(raw) == 0 {
		return cfg
	}

	_ = json.Unmarshal(raw, &cfg)

	return cfg
}
