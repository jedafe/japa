package content

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
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

	//nolint:exhaustive // only the relevant reflect.Kind cases are handled
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

	for _, f := range []struct {
		name   string
		isMeta bool
	}{
		{name: "Title", isMeta: false},
		{name: "Body", isMeta: false},
		{name: "BodyHtml", isMeta: false},
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

		resolved, _ := literals.Resolve(ctx, original, rc, tokens)
		setStringField(fv, resolved, f.isMeta)
	}

	return nil
}

func (s *Service) loadTemplateLiterals(ctx context.Context, tenantID pgtype.UUID) ([]literals.TemplateLiteral, error) {
	active := true

	rows, err := s.read.ListTemplateLiterals(ctx, gen.ListTemplateLiteralsParams{
		TenantID:   tenantID,
		Search:     "",
		IsActive:   &active,
		Sort:       "",
		Dir:        "",
		PageOffset: 0,
		PageLimit:  1000,
	})
	if err != nil {
		return nil, err
	}

	out := make([]literals.TemplateLiteral, len(rows))
	for i, r := range rows {
		out[i] = literals.TemplateLiteral{
			ID:          r.ID.String(),
			TenantID:    r.TenantID.String(),
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

func stringFieldValue(fv reflect.Value) (string, bool) {
	//nolint:exhaustive // only the relevant reflect.Kind cases are handled
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

// resolveLocale extracts a simple language tag from the Accept-Language header
// for use in the render context. Full i18n middleware integration is deferred.
func resolveLocale(r *http.Request) string {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return "en"
	}

	// "en-US,en;q=0.9" -> "en"
	primary := strings.Split(accept, ",")[0]
	primary = strings.TrimSpace(primary)
	primary = strings.Split(primary, "-")[0]
	primary = strings.ToLower(primary)

	if primary == "" {
		return "en"
	}

	return primary
}

// buildRenderContext constructs a literals.RenderContext for a content request.
// Geo and user-destination fields are left empty for this first implementation,
// matching the task requirement to wire GeoIP and user destination in a follow-up.
func buildRenderContext(r *http.Request, tenantID pgtype.UUID, tenantName string) literals.RenderContext {
	return literals.RenderContext{
		Now:                time.Now().UTC(),
		Locale:             resolveLocale(r),
		SourceCountry:      "",
		SourceCode:         "",
		DestinationCountry: "",
		DestinationCode:    "",
		TenantName:         tenantName,
		TenantID:           tenantID,
	}
}
