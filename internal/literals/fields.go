package literals

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/literals/countrydata"
	"github.com/jedafe/japa/internal/literals/expr"
)

// resolvableFields maps struct field names that carry user-facing text to
// whether they are "meta" fields. Meta fields keep a nil *string when the
// resolved value would be empty (matching the original content resolver's
// semantics: an absent meta_title should stay absent, not become an
// allocated empty string).
var resolvableFields = map[string]bool{
	"Title":    false,
	"Body":     false,
	"BodyHtml": false,
	"BodyHTML": false,
	// Excerpt is derived from the body on list/feed projections and can
	// contain leading {{...}} tokens that would otherwise render raw.
	"Excerpt":         false,
	"MetaTitle":       true,
	"MetaDescription": true,
	// Generic content/body fields used by non-contents entities:
	// learning lessons (Content *string / ContentHTML), wiki revisions
	// (Content json.RawMessage / CurrentContent), space/track descriptions,
	// and comment/feed cards that join the parent title (ContentTitle,
	// ThreadTitle).
	"Content":        false,
	"ContentHtml":    false,
	"ContentHTML":    false,
	"Description":    false,
	"SeriesTitle":    false,
	"CurrentContent": false,
	"ContentTitle":   false,
	"ThreadTitle":    false,
	"ModuleTitle":    false,
	"TrackTitle":     false,
	// Name covers entity-name fields on search/autocomplete DTOs (space,
	// tag, organization results) which are authored like titles. Exact-only:
	// *Name suffixes (Username, FileName, HostName) are identifiers, not prose.
	"Name": false,
	// Label covers menu item labels on public nav responses.
	"Label": false,
	// Summary covers job/scholarship listing summaries.
	"Summary": false,
	// Note covers planning-group check-in notes and similar user text.
	"Note": false,
	// JobTitle covers job-application rows that join the listing title.
	"JobTitle": false,
}

// resolvableSuffixes extend coverage to composed field names without
// enumerating every variant — ThreadTitle, ItemTitle, PlainTextBody,
// EmployerNotes, RejectionReason, and friends are all user-facing text.
// Suffix matching is deliberately conservative: it only fires on a trailing
// boundary, so TitleID/BodyClass/TextColor/ContentType are unaffected.
var resolvableSuffixes = []string{
	"Title", "Subtitle", "Body", "Html", "Description", "Content", "Summary",
	"Excerpt", "Label", "Heading", "Subheading", "Question", "Answer",
	"Message", "Text", "Caption", "Quote", "Tagline", "Reason", "Details",
	"Bio", "Note", "Notes", "Subject", "Comment", "Instructions",
	// Plural collection fields ([]string bodies like Answers/Questions).
	"Answers", "Questions", "Comments",
}

// fieldResolvable reports whether a struct field name carries user-facing
// text, and whether it is a meta field. Exact map entries win (they carry the
// meta flag); otherwise a resolvableSuffixes tail match resolves as non-meta.
func fieldResolvable(name string) (bool, bool) {
	if isMeta, ok := resolvableFields[name]; ok {
		return true, isMeta
	}

	for _, s := range resolvableSuffixes {
		if strings.HasSuffix(name, s) {
			return true, false
		}
	}

	return false, false
}

// maxResolveDepth bounds recursion into nested structs/slices/maps so
// cyclic structures cannot hang the resolver.
const maxResolveDepth = 16

// ResolveFields resolves {{...}} placeholders in the named text fields of
// any content-like struct or slice of structs (Title, Body, BodyHtml,
// Excerpt, MetaTitle, MetaDescription, Content, Description, and friends —
// see resolvableFields). It works with sqlc-generated row types and feature
// DTOs via reflection, recursing into nested structs and slices (e.g.
// SearchResult.Item, TrackDetail.Modules[].Lessons[]).
//
// Content-scoped geo wins over request geo per struct — see applyContentGeo.
func ResolveFields(ctx context.Context, v any, rc RenderContext, tokens []TemplateLiteral) error {
	if v == nil || len(tokens) == 0 || rc.raw {
		return nil
	}

	resolveValue(ctx, reflect.ValueOf(v), rc, buildTokenMap(tokens), 0, false)

	return nil
}

// ResolveData resolves {{...}} placeholders throughout an arbitrary
// JSON-shaped payload (maps, slices, nested values) — used for rendered
// widget data and response envelopes where every string is user-facing
// text. Structs encountered inside the payload still get the named-field
// treatment via resolvableFields.
func ResolveData(ctx context.Context, v any, rc RenderContext, tokens []TemplateLiteral) {
	if v == nil || len(tokens) == 0 || rc.raw {
		return
	}

	resolveValue(ctx, reflect.ValueOf(v), rc, buildTokenMap(tokens), 0, true)
}

//nolint:gocognit // reflection kind dispatch is inherently branchy
func resolveValue(ctx context.Context, rv reflect.Value, rc RenderContext, tokenMap map[string]TemplateLiteral, depth int, allStrings bool) {
	if !rv.IsValid() || depth > maxResolveDepth {
		return
	}

	//nolint:exhaustive // only the relevant reflect.Kind cases are handled
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return
		}

		e := rv.Elem()
		// Strings inside an interface hold a copy — resolve through the
		// settable interface itself.
		if e.Kind() == reflect.String {
			if allStrings && rv.CanSet() {
				if s := e.String(); strings.Contains(s, "{{") {
					rv.Set(reflect.ValueOf(resolveWithMap(ctx, s, rc.ToEnv(), tokenMap, nil)))
				}
			}

			return
		}

		resolveValue(ctx, e, rc, tokenMap, depth+1, allStrings)
	case reflect.Pointer:
		if rv.IsNil() {
			return
		}

		resolveValue(ctx, rv.Elem(), rc, tokenMap, depth+1, allStrings)
	case reflect.Struct:
		if isOpaqueType(rv.Type()) {
			return
		}

		resolveStructFields(ctx, rv, rc, tokenMap, depth, allStrings)
	case reflect.Slice, reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			// Byte-slice payloads (TipTap JSON, RawMessage) reached through
			// maps/slices resolve only in all-strings mode; named struct
			// fields are handled by resolveStructFields in both modes.
			if allStrings && rv.CanSet() && !rv.IsNil() && strings.Contains(string(rv.Bytes()), "{{") {
				rv.SetBytes([]byte(resolveWithMap(ctx, string(rv.Bytes()), rc.ToEnv(), tokenMap, nil)))
			}

			return
		}

		for i := range rv.Len() {
			resolveValue(ctx, rv.Index(i), rc, tokenMap, depth+1, allStrings)
		}
	case reflect.Map:
		if allStrings {
			resolveMapValues(ctx, rv, rc, tokenMap, depth)
		}
	case reflect.String:
		if allStrings && rv.CanSet() {
			if s := rv.String(); strings.Contains(s, "{{") {
				rv.SetString(resolveWithMap(ctx, s, rc.ToEnv(), tokenMap, nil))
			}
		}
	}
}

func resolveStructFields(ctx context.Context, rv reflect.Value, rc RenderContext, tokenMap map[string]TemplateLiteral, depth int, allStrings bool) {
	if !rv.CanAddr() {
		return
	}

	// Content-scoped geo wins over request geo: when the struct declares a
	// source/target country, {{Source_*}}/{{Destination_*}} describe the
	// entity's subject, not the reader's context.
	itemRC := rc
	applyContentGeo(rv, &itemRC)

	var env *expr.Env
	resolveStr := func(s string) string {
		if !strings.Contains(s, "{{") {
			return s
		}
		if env == nil {
			env = itemRC.ToEnv()
		}

		return resolveWithMap(ctx, s, env, tokenMap, nil)
	}

	// Walk every exported field: resolvable names (exact map or suffix rule)
	// get string resolution; composites recurse so DTOs like
	// SearchResult.Item or TrackDetail.Modules[].Lessons[] are covered —
	// including anonymous embedded structs (LessonDetail.Lesson.Title).
	t := rv.Type()

	for i := range rv.NumField() {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}

		fv := rv.Field(i)

		if res, isMeta := fieldResolvable(sf.Name); res {
			if original, ok := stringFieldValue(fv); ok {
				setStringField(fv, resolveStr(original), isMeta)

				continue
			}

			// Resolvable-named string slices (e.g. Answers []string) resolve
			// element-wise; named map[string]string values too.
			if fv.Kind() == reflect.Slice || fv.Kind() == reflect.Array {
				resolveNamedCollection(ctx, fv, itemRC, tokenMap, depth+1)
			}
		}

		//nolint:exhaustive // only composite kinds recurse
		switch fv.Kind() {
		case reflect.Struct, reflect.Pointer, reflect.Slice, reflect.Array, reflect.Interface:
			resolveValue(ctx, fv, itemRC, tokenMap, depth+1, allStrings)
		case reflect.Map:
			if allStrings {
				resolveValue(ctx, fv, itemRC, tokenMap, depth+1, allStrings)
			}
		}
	}
}

// resolveNamedCollection resolves string/[]byte elements of a resolvable-named
// slice or array (e.g. Answers []string). Non-string elements are left for the
// normal recursion path.
func resolveNamedCollection(ctx context.Context, rv reflect.Value, rc RenderContext, tokenMap map[string]TemplateLiteral, depth int) {
	if depth >= maxResolveDepth {
		return
	}

	resolveStr := func(s string) string {
		return resolveWithMap(ctx, s, rc.ToEnv(), tokenMap, nil)
	}

	for i := range rv.Len() {
		el := rv.Index(i)
		if !el.CanSet() {
			continue
		}

		switch el.Kind() {
		case reflect.String:
			if s := resolveStr(el.String()); s != el.String() {
				el.SetString(s)
			}
		case reflect.Slice:
			if el.Type().Elem().Kind() == reflect.Uint8 && el.Len() > 0 &&
				strings.Contains(string(el.Bytes()), "{{") {
				el.SetBytes([]byte(resolveStr(string(el.Bytes()))))
			}
		}
	}
}

// resolveMapValues resolves strings nested inside a map — values may be
// strings, byte slices, or further maps/slices/structs (JSON payloads).
func resolveMapValues(ctx context.Context, rv reflect.Value, rc RenderContext, tokenMap map[string]TemplateLiteral, depth int) {
	if rv.IsNil() || rv.Type().Key().Kind() != reflect.String {
		return
	}

	var env *expr.Env
	resolveStr := func(s string) string {
		if !strings.Contains(s, "{{") {
			return s
		}
		if env == nil {
			env = rc.ToEnv()
		}

		return resolveWithMap(ctx, s, env, tokenMap, nil)
	}

	for _, k := range rv.MapKeys() {
		mv := rv.MapIndex(k)

		switch mv.Kind() {
		case reflect.String:
			rv.SetMapIndex(k, reflect.ValueOf(resolveStr(mv.String())))
		case reflect.Interface:
			if mv.IsNil() {
				continue
			}

			e := mv.Elem()

			switch e.Kind() {
			case reflect.String:
				rv.SetMapIndex(k, reflect.ValueOf(resolveStr(e.String())))
			case reflect.Slice:
				if e.Type().Elem().Kind() == reflect.Uint8 {
					if s := string(e.Bytes()); strings.Contains(s, "{{") {
						rv.SetMapIndex(k, reflect.ValueOf([]byte(resolveStr(s))))
					}

					continue
				}

				resolveValue(ctx, e, rc, tokenMap, depth+1, true)
			case reflect.Map:
				resolveValue(ctx, e, rc, tokenMap, depth+1, true)
			case reflect.Struct, reflect.Pointer:
				// Map values are unaddressable copies — make an addressable
				// copy, resolve it, and store it back.
				cp := reflect.New(e.Type()).Elem()
				cp.Set(e)
				resolveValue(ctx, cp, rc, tokenMap, depth+1, true)
				rv.SetMapIndex(k, cp)
			}
		case reflect.Map, reflect.Slice:
			resolveValue(ctx, mv, rc, tokenMap, depth+1, true)
		}
	}
}

// isOpaqueType reports whether a struct type belongs to an external/stdlib
// package and therefore can never contain resolvable fields. Only this
// module's own types are walked.
func isOpaqueType(t reflect.Type) bool {
	pkg := t.PkgPath()

	return pkg != "" && !strings.HasPrefix(pkg, "github.com/jedafe/japa")
}

// applyContentGeo makes {{Source_Country}}/{{Destination_Country}} fully
// content-scoped: the request's geo context is cleared first so the reader's
// own journey can never leak into a post's prose, then the content's own geo
// metadata is applied. Priority: source_country/target_country columns, then
// field_values (origin_country / destination_country / country keys — ISO
// code or name). A post with no geo renders the literal's fallback.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md
// (Global Template Literals)
func applyContentGeo(rv reflect.Value, rc *RenderContext) {
	rc.SourceCountry, rc.SourceCode = "", ""
	rc.DestinationCountry, rc.DestinationCode = "", ""

	src := namedStringField(rv, "SourceCountry")
	dst := namedStringField(rv, "TargetCountry")

	if src == "" || dst == "" {
		fv := fieldValuesMap(rv)
		if src == "" {
			src = geoFieldValue(fv, "source_country", "origin_country")
		}
		if dst == "" {
			dst = geoFieldValue(fv, "target_country", "destination_country", "country")
		}
	}

	if code := toCountryCode(src); code != "" {
		rc.SourceCode = code
		rc.SourceCountry = countrydata.NameForCode(code)
	}

	if code := toCountryCode(dst); code != "" {
		rc.DestinationCode = code
		rc.DestinationCountry = countrydata.NameForCode(code)
	}
}

// namedStringField reads a string (or *string / pgtype.Text) struct field by
// name, returning "" when the field is absent or null.
func namedStringField(rv reflect.Value, name string) string {
	fv := rv.FieldByName(name)
	if !fv.IsValid() {
		return ""
	}

	if fv.Type() == reflect.TypeOf(pgtype.Text{}) {
		t := fv.Interface().(pgtype.Text) //nolint:forcetypeassert // guarded by type check
		if !t.Valid {
			return ""
		}

		return t.String
	}

	v, ok := stringFieldValue(fv)
	if !ok {
		return ""
	}

	return v
}

// fieldValuesMap parses a content row's field_values JSONB blob into a map.
// Returns nil when the field is absent or unparsable.
func fieldValuesMap(rv reflect.Value) map[string]any {
	fv := rv.FieldByName("FieldValues")
	if !fv.IsValid() || fv.Kind() != reflect.Slice || len(fv.Bytes()) == 0 {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(fv.Bytes(), &m); err != nil {
		return nil
	}

	return m
}

// geoFieldValue returns the first non-empty string value among the given
// field_values keys. Values may be plain strings or {"value": "..."} objects.
func geoFieldValue(fv map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := fv[k]
		if !ok {
			continue
		}

		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case map[string]any:
			if s, ok := t["value"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}

	return ""
}

// ToCountryCode normalizes a stored geo value to an ISO 3166-1 alpha-2
// code — accepts either the code ("GB") or the country name
// ("United Kingdom"). Exported for callers that resolve geo fields outside
// literal rendering (e.g. content store geo sync).
func ToCountryCode(v string) string { return toCountryCode(v) }

// GeoFieldValue returns the first non-empty string value among the given
// field_values keys. Exported for the same reason as ToCountryCode.
func GeoFieldValue(fv map[string]any, keys ...string) string { return geoFieldValue(fv, keys...) }

// toCountryCode normalizes a stored geo value to an ISO 3166-1 alpha-2 code —
// accepts either the code ("GB") or the country name ("United Kingdom").
func toCountryCode(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	if up := strings.ToUpper(v); len(up) == 2 && countrydata.NameForCode(up) != "" {
		return up
	}

	return countrydata.CodeForName(v)
}

// stringFieldValue reads a field's text content — string, *string, or a
// byte-slice kind (Body []byte, Content json.RawMessage, and pointer
// variants like *json.RawMessage). Nil pointers to non-string types report
// false so setStringField never attempts an incompatible reflect.Set.
func stringFieldValue(fv reflect.Value) (string, bool) {
	//nolint:exhaustive // only the relevant reflect.Kind cases are handled
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true
	case reflect.Pointer:
		elem := fv.Type().Elem()

		switch {
		case elem.Kind() == reflect.String:
			if fv.IsNil() {
				return "", true
			}

			return fv.Elem().String(), true
		case elem.Kind() == reflect.Slice && elem.Elem().Kind() == reflect.Uint8:
			if fv.IsNil() {
				return "", false
			}

			return string(fv.Elem().Bytes()), true
		}
	case reflect.Slice:
		if fv.Type().Elem().Kind() == reflect.Uint8 {
			if fv.IsNil() {
				return "", true
			}

			return string(fv.Bytes()), true
		}
	}

	return "", false
}

// setStringField writes resolved text back to a string, *string, or
// byte-slice field. Nil *string meta fields stay nil when the resolved
// value is empty.
func setStringField(fv reflect.Value, value string, isMeta bool) {
	//nolint:exhaustive // only the relevant reflect.Kind cases are handled
	switch fv.Kind() {
	case reflect.String:
		if fv.CanSet() {
			fv.SetString(value)
		}
	case reflect.Pointer:
		if fv.IsNil() {
			if isMeta && value == "" {
				return
			}

			if fv.CanSet() && fv.Type().Elem().Kind() == reflect.String {
				v := value
				fv.Set(reflect.ValueOf(&v))
			}

			return
		}

		e := fv.Elem()

		switch e.Kind() {
		case reflect.String:
			if e.CanSet() {
				e.SetString(value)
			}
		case reflect.Slice:
			if e.Type().Elem().Kind() == reflect.Uint8 && e.CanSet() {
				e.SetBytes([]byte(value))
			}
		}
	case reflect.Slice:
		if fv.Type().Elem().Kind() == reflect.Uint8 && fv.CanSet() && !fv.IsNil() {
			fv.SetBytes([]byte(value))
		}
	}
}
