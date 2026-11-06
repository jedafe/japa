package wiki

// Product instance CRUD (§80 Product Domain) — admin write surface on
// products + product_providers, sibling to the read-side directory.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
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
	return s.store.write.UpdateProduct(ctx, gen.UpdateProductParams{
		Name: name, Tagline: in.Tagline, WebsiteUrl: in.WebsiteURL,
		PricingModel: in.PricingModel, PriceFrom: numPtr(in.PriceFrom), Slug: slug,
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
// entity on its canonical listing (§80). Product-type slugs create a
// product; the wiki page lazy-materializes on first view.
func (h *Handlers) HandleCreateEntityInstance(w http.ResponseWriter, r *http.Request) {
	typeSlug := chi.URLParam(r, "type_slug")

	// Only product-type slugs are creatable for now — the chain resolves
	// the type's kind when other kinds gain creation.
	pt, err := h.service.store.read.GetProductTypeBySlug(r.Context(), typeSlug)
	if err != nil || !pt.IsEnabled {
		errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Entity type not found")
		return
	}

	var body ProductInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	body.TypeSlug = typeSlug
	slug, err := h.service.CreateProduct(r.Context(), body)
	if err != nil {
		slog.Error("wiki: create entity failed", "type", typeSlug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create entity")
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"slug": slug, "href": "/wiki/e/" + typeSlug + "/" + slug + "/overview",
	}})
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
		_ = json.NewDecoder(r.Body).Decode(&body)
		price, offer, pos = body.PriceFrom, body.OfferURL, body.Position
	}
	if err := h.service.SetProductProvider(r.Context(), slug, providerID, price, offer, pos, unlink); err != nil {
		slog.Error("wiki: set product provider failed", "slug", slug, "provider", providerID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "set_failed", "Failed to set product provider")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}
