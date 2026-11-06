package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// providerResolver resolves affiliate_providers for comparison — "which
// reseller to use" (§30 Sub-spec 7, providers comparison type).
type providerResolver struct {
	queries *gen.Queries
}

func NewProviderResolver(opts ...Option) Resolver {
	o := newResolverOptions(opts...)
	return &providerResolver{queries: o.queries}
}

func (r *providerResolver) Type() string { return "provider" }

func (r *providerResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	idStr, _ := ref["id"].(string)
	slug, _ := ref["slug"].(string)
	if idStr == "" && slug == "" {
		return ResolvedItem{}, false, errors.New("provider item_ref missing id or slug")
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

func (r *providerResolver) bySlug(ctx context.Context, slug string) ResolvedItem {
	p, err := r.queries.GetProviderBySlug(ctx, slug)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, p.ID, p.Slug, p.Name, p.ProviderType, p.Category, p.BaseUrl, p.CommissionRate, p.IsFeatured)
}

func (r *providerResolver) byID(ctx context.Context, id pgtype.UUID) ResolvedItem {
	p, err := r.queries.GetProviderByID(ctx, id)
	if err != nil {
		return ResolvedItem{}
	}
	return r.toItem(ctx, p.ID, p.Slug, p.Name, p.ProviderType, p.Category, p.BaseUrl, p.CommissionRate, p.IsFeatured)
}

func (r *providerResolver) toItem(ctx context.Context, id pgtype.UUID, slug, name, providerType string, category, baseURL *string, commission any, featured bool) ResolvedItem {
	md := map[string]any{"slug": slug, "provider_type": providerType}
	if category != nil && *category != "" {
		md["category"] = *category
	}
	if baseURL != nil && *baseURL != "" {
		md["base_url"] = *baseURL
	}
	md["commission_rate"] = commission
	md["is_featured"] = featured
	md["field_values"] = customfields.EntityFieldValues(ctx, r.queries, "provider", id)
	return ResolvedItem{ID: id.String(), Title: name, Type: "provider", Metadata: md}
}

func (r *providerResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 || limit > MaxSearchLimit {
		limit = defaultSearchLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.queries.SearchProviders(ctx, gen.SearchProvidersParams{
		Column1: query, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching providers: %w", err)
	}
	out := make([]ResolvedItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, ResolvedItem{ID: row.ID.String(), Title: row.Name, Type: "provider",
			Metadata: map[string]any{"slug": row.Slug, "provider_type": row.ProviderType, "category": row.Category}})
	}
	return out, nil
}
