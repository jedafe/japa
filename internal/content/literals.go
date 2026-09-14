package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/literals/countrydata"
)

// ResolveContent resolves template literals on a full *gen.Content row.
// It is a thin wrapper around ResolveContentFields for callers that already
// have a concrete Content model.
func (s *Service) ResolveContent(ctx context.Context, c *gen.Content, rc literals.RenderContext) error {
	return s.ResolveContentFields(ctx, c, rc)
}

// ResolveContentFields resolves {{...}} placeholders in the title, body,
// body_html, meta_title, and meta_description fields of any content-like
// struct or slice of structs. It works with the various sqlc-generated
// row types (GetContentBySlugRow, ListContentsByTenantRow, etc.) by using
// reflection to find and mutate those fields by name.
func (s *Service) ResolveContentFields(ctx context.Context, v any, rc literals.RenderContext) error {
	if v == nil {
		return nil
	}

	tokens, err := s.loadTemplateLiterals(ctx, rc.TenantID)
	if err != nil {
		return fmt.Errorf("loading template literals: %w", err)
	}

	rv := reflect.ValueOf(v)

	return s.resolveValue(ctx, rv, tokens, rc)
}

func (s *Service) resolveValue(ctx context.Context, rv reflect.Value, tokens []literals.TemplateLiteral, rc literals.RenderContext) error {
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}

		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}

		rv = rv.Elem()
	}

	// nolint:exhaustive // only the relevant reflect.Kind cases are handled
	switch rv.Kind() {
	case reflect.Struct:
		return s.resolveStruct(ctx, rv, tokens, rc)
	case reflect.Slice, reflect.Array:
		for i := range rv.Len() {
			if err := s.resolveValue(ctx, rv.Index(i), tokens, rc); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported content type: %s", rv.Kind())
	}
}

func (s *Service) resolveStruct(ctx context.Context, rv reflect.Value, tokens []literals.TemplateLiteral, rc literals.RenderContext) error {
	if !rv.CanAddr() {
		return errors.New("content struct is not addressable")
	}

	// Content-scoped geo wins over request geo: when the content declares a
	// source/target country, {{Source_*}}/{{Destination_*}} describe the
	// post's subject (e.g. a Ghana→Mexico guide), not the reader's context.
	itemRC := rc
	applyContentGeo(rv, &itemRC)

	for _, f := range []struct {
		name   string
		isMeta bool
	}{
		{name: "Title", isMeta: false},
		{name: "Body", isMeta: false},
		{name: "BodyHtml", isMeta: false},
		// Excerpt is derived from the body on list/feed projections and can
		// contain leading {{...}} tokens that would otherwise render raw.
		{name: "Excerpt", isMeta: false},
		{name: "MetaTitle", isMeta: true},
		{name: "MetaDescription", isMeta: true},
	} {
		fv := rv.FieldByName(f.name)
		if !fv.IsValid() {
			continue
		}

		original, ok := stringFieldValue(fv)
		if !ok {
			continue
		}

		resolved, _ := literals.Resolve(ctx, original, itemRC, tokens)
		setStringField(fv, resolved, f.isMeta)
	}

	return nil
}

func (s *Service) loadTemplateLiterals(ctx context.Context, tenantID pgtype.UUID) ([]literals.TemplateLiteral, error) {
	if s.catalog != nil {
		return s.catalog.GetActive(ctx, tenantID)
	}

	active := true

	rows, err := s.read.ListTemplateLiterals(ctx, gen.ListTemplateLiteralsParams{

		Search:     "",
		IsActive:   &active,
		Kind:       "",
		Sort:       "",
		Dir:        "",
		PageOffset: 0,
		PageLimit:  literals.CatalogMaxLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]literals.TemplateLiteral, len(rows))
	for i, r := range rows {
		out[i] = literals.TemplateLiteral{
			ID: r.ID.String(),

			Key:         r.Key,
			Kind:        r.Kind,
			Value:       r.Value,
			Fallback:    r.Fallback,
			Label:       r.Label,
			Description: r.Description,
			Category:    r.Category,
			IsSystem:    r.IsSystem,
			IsActive:    r.IsActive,
		}
	}

	return out, nil
}

// applyContentGeo makes {{Source_Country}}/{{Destination_Country}} fully
// content-scoped: the request's geo context is cleared first so the reader's
// own journey can never leak into a post's prose, then the content's own geo
// metadata is applied. Priority: source_country/target_country columns, then
// field_values (origin_country / destination_country / country keys — ISO
// code or name). A post with no geo renders the literal's fallback.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md
// (Global Template Literals)
func applyContentGeo(rv reflect.Value, rc *literals.RenderContext) {
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

func stringFieldValue(fv reflect.Value) (string, bool) {
	// nolint:exhaustive // only the relevant reflect.Kind cases are handled
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true
	case reflect.Pointer:
		if fv.IsNil() {
			return "", true
		}

		if fv.Elem().Kind() == reflect.String {
			return fv.Elem().String(), true
		}
	}

	return "", false
}

func setStringField(fv reflect.Value, value string, isMeta bool) {
	// nolint:exhaustive // only the relevant reflect.Kind cases are handled
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

			if fv.CanSet() {
				v := value
				fv.Set(reflect.ValueOf(&v))
			}

			return
		}

		e := fv.Elem()
		if e.Kind() == reflect.String && e.CanSet() {
			e.SetString(value)
		}
	}
}

// buildRenderContext constructs a literals.RenderContext for a content request.
// Geo fields come from the ADR-064 VisitorContext (profile origin_country /
// destination_country custom fields, jc_journey_token for guests, GeoIP
// fallback) and the GeoIP middleware context (detected country name/code).
func buildRenderContext(r *http.Request, tenantName string) literals.RenderContext {
	return literals.NewRequestContext(r, tenantName)
}
