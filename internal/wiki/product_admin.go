package wiki

// Product instance CRUD (§80 Product Domain) — admin write surface on
// products + product_providers, sibling to the read-side directory.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ProductInput is the create/update body for a product.
type ProductInput struct {
	TypeSlug     string   `json:"type_slug"` // product_types.slug — slug, never uuid
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Tagline      *string  `json:"tagline"`
	WebsiteURL   *string  `json:"website_url"`
	PricingModel *string  `json:"pricing_model"`
	PriceFrom    *float64 `json:"price_from"`
}

func (s *Service) productTypeID(ctx context.Context, typeSlug string) (pgtype.UUID, error) {
	pt, err := s.store.read.GetProductTypeBySlug(ctx, typeSlug)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pt.ID, nil
}

func numPtr(f *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if f != nil {
		_ = n.Scan(*f)
	}
	return n
}

func uuidOf(id string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(id)
	return u
}

// CreateProduct inserts a product under a type slug.
func (s *Service) CreateProduct(ctx context.Context, in ProductInput) (string, error) {
	if in.TypeSlug == "" || in.Slug == "" || in.Name == "" {
		return "", fmt.Errorf("type_slug, slug and name are required")
	}
	typeID, err := s.productTypeID(ctx, in.TypeSlug)
	if err != nil {
		return "", fmt.Errorf("product type %q not found", in.TypeSlug)
	}
	row, err := s.store.write.CreateProduct(ctx, gen.CreateProductParams{
		ProductTypeID: typeID, Slug: in.Slug, Name: in.Name,
		Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom),
	})
	if err != nil {
		return "", err
	}
	return row.Slug, nil
}

// UpdateProduct patches mutable fields on a product (by slug).
func (s *Service) UpdateProduct(ctx context.Context, slug string, in ProductInput) error {
	var name *string
	if in.Name != "" {
		name = &in.Name
	}
	// nil = keep (COALESCE), "" = clear (clear_fields) — PATCH can both
	// preserve and clear per key. price_from has no clear path (zero is a
	// valid price; clearing needs an explicit admin op).
	clear := []string{}
	for _, kv := range []struct {
		key string
		val *string
	}{{"tagline", in.Tagline}, {"website_url", in.WebsiteURL}, {"pricing_model", in.PricingModel}} {
		if kv.val != nil && *kv.val == "" {
			clear = append(clear, kv.key)
		}
	}
	return s.store.write.UpdateProduct(ctx, gen.UpdateProductParams{
		Name: name, Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom), Slug: slug,
		ClearFields: clear,
	})
}

// DeleteProduct soft-deletes a product by slug.
func (s *Service) DeleteProduct(ctx context.Context, slug string) error {
	return s.store.write.DeleteProduct(ctx, slug)
}

// SetProductProvider links/unlinks a provider with optional price+offer.
func (s *Service) SetProductProvider(ctx context.Context, productSlug string, providerID, priceStr, offerURL string, position int32, unlink bool) error {
	p, err := s.store.read.GetProductBySlug(ctx, productSlug)
	if err != nil {
		return err
	}
	pid := uuidOf(providerID)
	if unlink {
		return s.store.write.RemoveProductProvider(ctx, gen.RemoveProductProviderParams{ProductID: p.ID, ProviderID: pid})
	}
	var pf pgtype.Numeric
	if priceStr != "" {
		_ = pf.Scan(priceStr)
	}
	var ou *string
	if offerURL != "" {
		ou = &offerURL
	}
	return s.store.write.UpsertProductProvider(ctx, gen.UpsertProductProviderParams{
		ProductID: p.ID, ProviderID: pid, PriceFrom: pf, OfferUrl: ou, Position: position,
	})
}

// --- handlers ---

func (h *Handlers) HandleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	slug, err := h.service.CreateProduct(r.Context(), in)
	if err != nil {
		slog.Error("wiki: create product failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create product")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"slug": slug}})
}

func (h *Handlers) HandleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	var in ProductInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if err := h.service.UpdateProduct(r.Context(), chi.URLParam(r, "slug"), in); err != nil {
		slog.Error("wiki: update product failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

func (h *Handlers) HandleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteProduct(r.Context(), chi.URLParam(r, "slug")); err != nil {
		slog.Error("wiki: delete product failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete product")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleCreateEntityInstance serves POST /wiki/e/{type_slug} — creates an
// entity on its canonical listing (§80): product types → products,
// 'program' → university_course_catalog, 'course' → offerings (with
// university + program resolution). Pages lazy-materialize on first view.
func (h *Handlers) HandleCreateEntityInstance(w http.ResponseWriter, r *http.Request) {
	typeSlug := chi.URLParam(r, "type_slug")
	ctx := r.Context()

	var body struct {
		Slug           string `json:"slug"`
		Name           string `json:"name"`
		Title          string `json:"title"`
		Language       string `json:"language"`
		UniversitySlug  string   `json:"university_slug"`
		UniversitySlugs []string `json:"university_slugs"`
		ProgramSlug     string   `json:"program_slug"`
		HostSlug        string   `json:"host_slug"`
		DestinationSlug string   `json:"destination_slug"`
		VisaCategory    string   `json:"visa_category"`
		CountryCode     string   `json:"country_code"`
		Country         string   `json:"country"`
		SourceURL       string   `json:"source_url"`
		GovSourceURL    string   `json:"gov_source_url"`
		Tagline        string `json:"tagline"`
		WebsiteURL     string `json:"website_url"`
		PricingModel   string `json:"pricing_model"`
		PriceFrom      *float64 `json:"price_from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}
	name := body.Name
	if name == "" {
		name = body.Title
	}

	href := "/wiki/e/" + typeSlug + "/" + body.Slug + "/overview"
	out := map[string]any{"slug": body.Slug, "href": href}

	// Product-type slugs → products (slug-keyed creation).
	if pt, err := h.service.store.read.GetProductTypeBySlug(ctx, typeSlug); err == nil && pt.IsEnabled {
		slug, err := h.service.CreateProduct(ctx, ProductInput{
			TypeSlug: typeSlug, Slug: body.Slug, Name: name,
			Tagline: strPtr(body.Tagline), WebsiteURL: strPtr(body.WebsiteURL),
			PricingModel: strPtr(body.PricingModel), PriceFrom: body.PriceFrom,
		})
		if err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}
		out["slug"] = slug
		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": out})
		return
	}

	// Table-backed kinds.
	kind, err := h.service.store.read.GetWikiEntityKind(ctx, typeSlug)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Entity type not found")
		return
	}

	switch kind.EntityKind {
	case "program":
		lang := body.Language
		if lang == "" {
			lang = "en"
		}
		if _, err := h.service.store.write.CreateProgramForWiki(ctx, gen.CreateProgramForWikiParams{
			Title: name, Slug: body.Slug, Language: lang,
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}
	case "course":
		// Universities are multiselect — one offering per selected uni
		// (uni-prefixed slugs like unilag-bsc-cs).
		uniSlugs := body.UniversitySlugs
		if len(uniSlugs) == 0 && body.UniversitySlug != "" {
			uniSlugs = []string{body.UniversitySlug}
		}
		if len(uniSlugs) == 0 || body.ProgramSlug == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_parents", "university_slugs and program_slug are required for courses")
			return
		}
		prog, err := h.service.store.read.GetProgramForWiki(ctx, body.ProgramSlug)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "program_not_found", "Program not found")
			return
		}
		created := []string{}
		skipped := []string{}
		for _, uslug := range uniSlugs {
			uni, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: uslug, TypeSlug: "education"})
			if err != nil {
				skipped = append(skipped, uslug)
				continue // skip unknown universities rather than fail the batch
			}
			oslug := body.Slug
			if len(uniSlugs) > 1 {
				oslug = uslug + "-" + body.Slug
			}
			if _, err := h.service.store.write.CreateOfferingForWiki(ctx, gen.CreateOfferingForWikiParams{
				CourseID: prog.ID, UniversityID: uni, Slug: oslug,
			}); err == nil {
				created = append(created, oslug)
			} else {
				skipped = append(skipped, uslug) // existing slug or other conflict
			}
		}
		if len(skipped) > 0 {
			out["skipped"] = skipped
		}
		if len(created) == 0 {
			errpkg.WriteError(w, http.StatusBadRequest, "university_not_found", "No matching universities")
			return
		}
		out["created"] = created
		out["slug"] = created[0]
		out["href"] = "/wiki/e/" + typeSlug + "/" + created[0] + "/overview"

	case "embassy":
		host, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.HostSlug, TypeSlug: "country"})
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "host_not_found", "Host country not found")
			return
		}
		if _, err := h.service.store.write.CreateEmbassyForWiki(ctx, gen.CreateEmbassyForWikiParams{
			Name: name, Slug: body.Slug, HostSpaceID: host,
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "bank":
		if body.CountryCode != "" && (len(body.CountryCode) != 2 || !isAlpha(body.CountryCode)) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_country_code", "Country code must be 2 letters (ISO 3166-1)")
			return
		}
		var geo pgtype.UUID
		if body.HostSlug != "" {
			if g, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.HostSlug, TypeSlug: "country"}); err == nil {
				geo = g
			}
		}
		if _, err := h.service.store.write.CreateBankForWiki(ctx, gen.CreateBankForWikiParams{
			Name: name, Slug: body.Slug, GeoSpaceID: geo, CountryCode: strPtr(body.CountryCode),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "visa_route":
		dest, err := h.service.store.read.GetSpaceBySlugKind(ctx, gen.GetSpaceBySlugKindParams{Slug: body.DestinationSlug, TypeSlug: "country"})
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "destination_not_found", "Destination country not found")
			return
		}
		if _, err := h.service.store.write.CreateVisaRouteForWiki(ctx, gen.CreateVisaRouteForWikiParams{
			Name: name, Slug: body.Slug, DestinationSpaceID: dest,
			VisaCategory: body.VisaCategory, GovSourceUrl: strPtr(body.GovSourceURL),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}

	case "scholarship":
		if _, err := h.service.store.write.CreateScholarshipForWiki(ctx, gen.CreateScholarshipForWikiParams{
			Title: name, Slug: body.Slug,
			SourceUrl: strPtr(body.SourceURL), Country: strPtr(body.Country),
		}); err != nil {
			h.failEntityCreate(w, typeSlug, err)
			return
		}
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "kind_not_creatable", "This entity kind cannot be created here")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": out})
}

func (h *Handlers) failEntityCreate(w http.ResponseWriter, typeSlug string, err error) {
	var pge *pgconn.PgError
	if errors.As(err, &pge) && pge.Code == "23505" {
		errpkg.WriteError(w, http.StatusConflict, "slug_conflict", "An entity with this slug already exists")
		return
	}
	slog.Error("wiki: create entity failed", "type", typeSlug, "error", err)
	errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create entity")
}

func isAlpha(s string) bool {
	for _, c := range s {
		if c < 'a' || c > 'z' {
			if c < 'A' || c > 'Z' {
				return false
			}
		}
	}
	return true
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// HandleListProductProviders serves GET /wiki/e/{type_slug}/{entity}/providers —
// the providers repeater source for the review form + product CTAs.
func (h *Handlers) HandleListProductProviders(w http.ResponseWriter, r *http.Request) {
	p, err := h.service.store.read.GetProductBySlug(r.Context(), chi.URLParam(r, "entity"))
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	rows, err := h.service.store.read.ListProductProviders(r.Context(), p.ID)
	if err != nil {
		slog.Error("wiki: list providers failed", "slug", p.Slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list providers")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, pp := range rows {
		items = append(items, map[string]any{
			"provider_id": pp.ProviderID.String(), "name": pp.Name,
			"price_from": pp.PriceFrom, "offer_url": pp.OfferUrl,
		})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"items": items}})
}

// HandleSetProductProvider serves POST+DELETE
// /admin/wiki/products/{slug}/providers/{provider_id} — the N:M link.
func (h *Handlers) HandleSetProductProvider(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider_id")
	slug := chi.URLParam(r, "slug")

	unlink := r.Method == http.MethodDelete
	var price, offer string
	var pos int32
	if !unlink {
		var body struct {
			PriceFrom string `json:"price_from"`
			OfferURL  string `json:"offer_url"`
			Position  int32  `json:"position"`
		}
		// EOF = empty body (bare link) — valid; other decode errors 400.
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		price, offer, pos = body.PriceFrom, body.OfferURL, body.Position
	}
	if err := h.service.SetProductProvider(r.Context(), slug, providerID, price, offer, pos, unlink); err != nil {
		slog.Error("wiki: set product provider failed", "slug", slug, "provider", providerID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "set_failed", "Failed to set product provider")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleListPickerSpaces serves GET /wiki/pickers/spaces?type={type_slug} —
// university/country option lists for the entity create forms.
func (h *Handlers) HandleListPickerSpaces(w http.ResponseWriter, r *http.Request) {
	typeSlug := r.URL.Query().Get("type")
	if typeSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "type_required", "type query param required")
		return
	}
	rows, err := h.service.store.read.ListSpacesByTypeKind(r.Context(), gen.ListSpacesByTypeKindParams{
		TypeSlug: typeSlug, Q: r.URL.Query().Get("q"),
	})
	if err != nil {
		slog.Error("wiki: picker spaces failed", "type", typeSlug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list spaces")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID.String(), "slug": row.Slug, "name": row.Name})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"items": items}})
}
