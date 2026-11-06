package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// productResolver resolves products for comparison. One resolver serves
// every product type via config.subtype (the visa-kind pattern — §30
// Sub-spec 7). kind is the product's entity_class for cfv overlay
// (e.g. 'ai-agent'); subtype = product_types.slug when set.
type productResolver struct {
	queries *gen.Queries
}

// NewProductResolver creates a resolver for products. Pass WithQueries to
// enable cfv merge + provider/rating hydration (§80 Product Domain).
func NewProductResolver(opts ...Option) Resolver {
	o := newResolverOptions(opts...)
	return &productResolver{queries: o.queries}
}

func (r *productResolver) Type() string { return "product" }

func (r *productResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)
	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("product item_ref missing id or slug")
	}

	var it ResolvedItem
	if idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		it = r.byID(ctx, id)
	} else {
		it = r.bySlug(ctx, slug)
	}
	if it.ID == "" {
		return ResolvedItem{}, false, nil
	}
	return it, true, nil
}

func (r *productResolver) bySlug(ctx context.Context, slug string) ResolvedItem {
	p, err := r.queries.GetProductBySlug(ctx, slug)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, p.ID, p.Slug, p.Name, p.Tagline, p.WebsiteUrl, p.PriceFrom, p.RatingAvg, p.ReviewCount, p.TypeSlug)
}

func (r *productResolver) byID(ctx context.Context, id pgtype.UUID) ResolvedItem {
	p, err := r.queries.GetProductByID(ctx, id)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, p.ID, p.Slug, p.Name, p.Tagline, p.WebsiteUrl, p.PriceFrom, p.RatingAvg, p.ReviewCount, p.TypeSlug)
}

func (r *productResolver) toItem(ctx context.Context, id pgtype.UUID, slug, name string, tagline, website *string, priceFrom, ratingAvg interface{}, reviewCount int32, typeSlug string) ResolvedItem {
	md := map[string]any{"type_slug": typeSlug}
	if tagline != nil && *tagline != "" {
		md["tagline"] = *tagline
	}
	if website != nil && *website != "" {
		md["website_url"] = *website
	}
	md["field_values"] = customfields.EntityFieldValues(ctx, r.queries, "product:"+typeSlug, id)
	providers, _ := r.queries.ListProductProviders(ctx, id)
	plist := make([]map[string]any, 0, len(providers))
	for _, pp := range providers {
		plist = append(plist, map[string]any{"provider_id": pp.ProviderID.String(), "name": pp.Name, "price_from": pp.PriceFrom, "offer_url": pp.OfferUrl})
	}
	md["providers"] = plist
	md["review_count"] = reviewCount
	md["rating_avg"] = ratingAvg
	md["has_deal"] = len(plist) > 0
	md["slug"] = slug
	return ResolvedItem{ID: id.String(), Title: name, Type: "product", Metadata: md}
}

func (r *productResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 || limit > MaxSearchLimit {
		limit = defaultSearchLimit
	}
	if offset < 0 {
		offset = 0
	}
	subtype, _ := filters["subtype"].(string)
	rows, err := r.queries.SearchProducts(ctx, gen.SearchProductsParams{
		Column1: subtype, Column2: query, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching products: %w", err)
	}
	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, ResolvedItem{ID: row.ID.String(), Title: row.Name, Type: "product",
			Metadata: map[string]any{"slug": row.Slug, "type_slug": row.TypeSlug, "tagline": row.Tagline}})
	}
	return out, nil
}
