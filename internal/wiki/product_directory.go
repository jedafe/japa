package wiki

// Public product directory (§80 Product Domain — de-wikied): /products and
// /product/{slug} are dedicated surfaces. Products, providers and deals are
// comparison entities like mentors — their JCE resolvers keep them
// comparable on /compare, but nothing here surfaces under /wiki/e.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"log/slog"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
)

// ProductListItem is one card on /products — core identity plus the meta
// the card renders: type label, tagline, logo, price/rating, provider count.
type ProductListItem struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Tagline       string   `json:"tagline,omitempty"`
	Discriminator string   `json:"discriminator,omitempty"` // product type slug
	TypeLabel     string   `json:"type_label,omitempty"`
	LogoURL       string   `json:"logo_url,omitempty"`
	PriceFrom     *float64 `json:"price_from,omitempty"`
	RatingAvg     *float64 `json:"rating_avg,omitempty"`
	ReviewCount   int32    `json:"review_count"`
	ProviderCount int64    `json:"provider_count"`
	Href          string   `json:"href"`
}

// ProductTypeDTO is one enabled product type with a live entity count —
// the ?type= chips on /products.
type ProductTypeDTO struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	Icon        string `json:"icon,omitempty"`
	EntityCount int64  `json:"entity_count"`
}

// ProductProviderDTO is one provider offer row on the product page — the
// within-product provider matrix (§80).
type ProductProviderDTO struct {
	ProviderID   string   `json:"provider_id"`
	ProviderSlug string   `json:"provider_slug,omitempty"`
	Name         string   `json:"name"`
	PriceFrom    *float64 `json:"price_from,omitempty"`
	OfferURL     string   `json:"offer_url,omitempty"`
	HasDeal      bool     `json:"has_deal"`
}

// ProductDetailDTO is GET /products/{slug} — product core, provider
// offers, and the type's cfv field groups.
type ProductDetailDTO struct {
	ID           string                `json:"id"`
	Slug         string                `json:"slug"`
	Name         string                `json:"name"`
	Tagline      string                `json:"tagline,omitempty"`
	WebsiteURL   string                `json:"website_url,omitempty"`
	LogoURL      string                `json:"logo_url,omitempty"`
	PricingModel string                `json:"pricing_model,omitempty"`
	PriceFrom    *float64              `json:"price_from,omitempty"`
	RatingAvg    *float64              `json:"rating_avg,omitempty"`
	ReviewCount  int32                 `json:"review_count"`
	TypeSlug     string                `json:"type_slug"`
	TypeLabel    string                `json:"type_label"`
	Providers    []ProductProviderDTO  `json:"providers"`
	FieldGroups  []EntityFieldGroupDTO `json:"field_groups,omitempty"`
}

// mergeEntityGroupValues fills nil-valued fields on dst from src — the
// legacy 'product'-bucket cfv backfill for the type-slug bucket read.
func mergeEntityGroupValues(dst, src *EntityFieldGroupDTO) {
	byKey := make(map[string]*EntityFieldDTO, len(src.Fields))
	for i := range src.Fields {
		byKey[src.Fields[i].Key] = &src.Fields[i]
	}
	for i := range dst.Fields {
		if dst.Fields[i].Value == nil {
			if f, ok := byKey[dst.Fields[i].Key]; ok {
				dst.Fields[i].Value = f.Value
			}
		}
	}
}

func numToFloatPtr(n pgtype.Numeric) *float64 {
	if f, err := n.Float64Value(); err == nil && f.Valid {
		return &f.Float64
	}
	return nil
}

// HandleListProducts serves GET /products?type=&q=&limit=&offset= — the
// de-wikied product listing backing /products and the review form's
// product picker. ?type is a product_types slug; an unknown one 404s
// rather than silently listing every product.
func (h *Handlers) HandleListProducts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	typeSlug := r.URL.Query().Get("type")
	// Escape ILIKE wildcards in the search term — a bare "%"/"_" would
	// otherwise act as a pattern and dump the whole catalog (same guard
	// AdminListProducts already applies).
	q := ilikeEscaper.Replace(r.URL.Query().Get("q"))
	ctx := r.Context()

	label := "Products"
	if typeSlug != "" {
		pt, err := h.service.store.read.GetProductTypeBySlug(ctx, typeSlug)
		if err != nil || !pt.IsEnabled {
			errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Product type not found")
			return
		}
		label = pt.Label
	}

	rows, err := h.service.store.read.SearchProducts(ctx, gen.SearchProductsParams{
		Column1: typeSlug, Column2: q, LimitCount: int32(limit), OffsetCount: int32(offset),
	})
	if err != nil {
		slog.Error("products: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list products")
		return
	}
	total, err := h.service.store.read.CountProducts(ctx, gen.CountProductsParams{Column1: typeSlug, Column2: q})
	if err != nil {
		slog.Error("products: count failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list products")
		return
	}

	items := make([]ProductListItem, 0, len(rows))
	for _, p := range rows {
		it := ProductListItem{
			ID:            uuidStr(p.ID),
			Slug:          p.Slug,
			Name:          p.Name,
			Discriminator: p.TypeSlug,
			TypeLabel:     p.TypeLabel,
			PriceFrom:     numToFloatPtr(p.PriceFrom),
			RatingAvg:     numToFloatPtr(p.RatingAvg),
			ReviewCount:   p.ReviewCount,
			ProviderCount: p.ProviderCount,
			Href:          "/product/" + p.Slug,
		}
		if p.Tagline != nil {
			it.Tagline = *p.Tagline
		}
		if p.LogoStoragePath != nil && *p.LogoStoragePath != "" {
			bucket := "media"
			if p.LogoStorageBucket != nil && *p.LogoStorageBucket != "" {
				bucket = *p.LogoStorageBucket
			}
			it.LogoURL = media.MediaURL(bucket, *p.LogoStoragePath, media.ImageProxyOpts{})
		}
		items = append(items, it)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"items": items, "total": total, "kind": "product", "label": label},
	})
}

// HandleListProductTypes serves GET /products/types — enabled product
// types with live entity counts (the /products ?type= chips + the review
// form's type picker).
func (h *Handlers) HandleListProductTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.service.store.read.ListProductTypesWithCounts(r.Context())
	if err != nil {
		slog.Error("products: list types failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list product types")
		return
	}
	items := make([]ProductTypeDTO, 0, len(rows))
	for _, t := range rows {
		d := ProductTypeDTO{Slug: t.Slug, Label: t.Label, EntityCount: t.EntityCount}
		if t.Icon != nil {
			d.Icon = *t.Icon
		}
		items = append(items, d)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"items": items}})
}

// HandleGetProduct serves GET /products/{slug} — the /product/{slug}
// detail surface: product core, provider offers, and the type's cfv field
// groups hydrated through the comparison type's dimension pool.
func (h *Handlers) HandleGetProduct(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := h.service.store.read.GetPublicProductBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "product_not_found", "Product not found")
			return
		}
		slog.Error("products: get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to load product")
		return
	}

	var userID pgtype.UUID
	if u := auth.FromContext(ctx); u != nil {
		userID = u.UserID
	}

	dto := ProductDetailDTO{
		ID:          uuidStr(p.ID),
		Slug:        p.Slug,
		Name:        p.Name,
		ReviewCount: p.ReviewCount,
		TypeSlug:    p.TypeSlug,
		TypeLabel:   p.TypeLabel,
		PriceFrom:   numToFloatPtr(p.PriceFrom),
		RatingAvg:   numToFloatPtr(p.RatingAvg),
		Providers:   []ProductProviderDTO{},
	}
	if p.Tagline != nil {
		dto.Tagline = *p.Tagline
	}
	if p.WebsiteUrl != nil {
		dto.WebsiteURL = *p.WebsiteUrl
	}
	if p.PricingModel != nil {
		dto.PricingModel = *p.PricingModel
	}
	if p.LogoStoragePath != nil && *p.LogoStoragePath != "" {
		bucket := "media"
		if p.LogoStorageBucket != nil && *p.LogoStorageBucket != "" {
			bucket = *p.LogoStorageBucket
		}
		dto.LogoURL = media.MediaURL(bucket, *p.LogoStoragePath, media.ImageProxyOpts{})
	}

	provs, err := h.service.store.read.ListProductProviders(ctx, p.ID)
	if err == nil {
		for _, pp := range provs {
			d := ProductProviderDTO{
				ProviderID:   pp.ProviderID.String(),
				ProviderSlug: pp.ProviderSlug,
				Name:         pp.Name,
				PriceFrom:    numToFloatPtr(pp.PriceFrom),
				HasDeal:      pp.HasDeal,
			}
			if pp.OfferUrl != nil {
				d.OfferURL = *pp.OfferUrl
			}
			dto.Providers = append(dto.Providers, d)
		}
	}

	// Field groups — the comparison type's pool is the type's group set
	// post-de-wiki; cfv reads run under the type-slug entity_class bucket,
	// the same convention the product resolver hydrates on /compare.
	if ct, err := h.service.store.read.GetComparisonTypeBySlug(ctx, p.TypeSlug); err == nil {
		if groups, gerr := h.service.store.read.ListComparisonTypeFieldGroups(ctx, ct.ID); gerr == nil {
			for _, g := range groups {
				grp, ferr := h.service.getFieldsForEntity(ctx, userID, p.TypeSlug, p.ID, g.GroupID, p.GeoSpaceID)
				if ferr == nil || grp == nil {
					continue
				}
				// Legacy backfill — the retired wiki page path keyed cfv on
				// the 'product' kind bucket, not the type slug. Values
				// already stored there fill the gaps rather than vanish.
				needsBackfill := false
				for _, f := range grp.Fields {
					if f.Value == nil {
						needsBackfill = true
						break
					}
				}
				if needsBackfill {
					if alt, aerr := h.service.getFieldsForEntity(ctx, userID, "product", p.ID, g.GroupID, p.GeoSpaceID); aerr == nil && alt != nil {
						mergeEntityGroupValues(grp, alt)
					}
				}
				dto.FieldGroups = append(dto.FieldGroups, *grp)
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dto})
}
