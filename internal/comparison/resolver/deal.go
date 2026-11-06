package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// dealResolver resolves coupons for comparison — "Kilo's Kimi deal vs
// Devin's Kimi deal" (§30 Sub-spec 7, deals comparison type).
type dealResolver struct {
	queries *gen.Queries
}

func NewDealResolver(opts ...Option) Resolver {
	o := newResolverOptions(opts...)
	return &dealResolver{queries: o.queries}
}

func (r *dealResolver) Type() string { return "deal" }

func (r *dealResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)
	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("deal item_ref missing id or slug")
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

func (r *dealResolver) bySlug(ctx context.Context, slug string) ResolvedItem {
	d, err := r.queries.GetDealBySlug(ctx, slug)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, d.ID, d.Slug, d.Title, d.CouponType, d.DiscountType, d.DiscountValue, d.Currency, d.ProviderName)
}

func (r *dealResolver) byID(ctx context.Context, id pgtype.UUID) ResolvedItem {
	d, err := r.queries.GetDealByID(ctx, id)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, d.ID, d.Slug, d.Title, d.CouponType, d.DiscountType, d.DiscountValue, d.Currency, d.ProviderName)
}

func (r *dealResolver) toItem(ctx context.Context, id pgtype.UUID, slug, title, couponType string, discountType *string, discountValue any, currency, providerName *string) ResolvedItem {
	md := map[string]any{"slug": slug, "coupon_type": couponType, "discount_value": discountValue}
	if discountType != nil && *discountType != "" {
		md["discount_type"] = *discountType
	}
	if currency != nil && *currency != "" {
		md["currency"] = *currency
	}
	if providerName != nil && *providerName != "" {
		md["provider_name"] = *providerName
	}
	md["field_values"] = customfields.EntityFieldValues(ctx, r.queries, "deal", id)
	return ResolvedItem{ID: id.String(), Title: title, Type: "deal", Metadata: md}
}

func (r *dealResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 || limit > MaxSearchLimit {
		limit = defaultSearchLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.queries.SearchDeals(ctx, gen.SearchDealsParams{
		Column1: query, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching deals: %w", err)
	}
	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, ResolvedItem{ID: row.ID.String(), Title: row.Title, Type: "deal",
			Metadata: map[string]any{"slug": row.Slug, "provider_name": row.ProviderName}})
	}
	return out, nil
}
