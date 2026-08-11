// Package server — agent_adapters.go provides the wiring-layer adapters that
// bridge the AI Agent Executor's (Section 103, ADR-111) narrow dependency
// interfaces to the real service implementations.
//
// The adapters exist because:
//  1. The ai package's CacheClient uses plain (string, error) returns while
//     valkeycompat.Cmdable uses go-redis command types — a thin adapter
//     resolves the signature mismatch without polluting the ai package
//     with valkeycompat imports.
//  2. The ai package's RAGService returns []ai.RAGChunk (a mirror type)
//     to avoid an import cycle with internal/rag — the adapter converts
//     rag.RetrievedChunk to ai.RAGChunk.
//  3. The ContentCreator/ContentUpdater function bridges adapt
//     content.Service.CreateContent/UpdateContent into the ai package's
//     provider-agnostic input/output types, avoiding an import cycle
//     between internal/ai and internal/content.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6 wiring)
package server

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rag"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// agentCacheAdapter adapts valkeycompat.Cmdable to the ai.CacheClient
// interface. The ai package uses plain (string, error) returns so it
// doesn't depend on valkeycompat command types.
type agentCacheAdapter struct {
	rdb valkeycompat.Cmdable
}

// Get retrieves a string value from Valkey. Returns "" + the redis error
// on miss/failure (the caller treats any error as a cache miss).
func (a *agentCacheAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.rdb.Get(ctx, key).Result()
}

// Set writes a string value to Valkey with the given TTL in seconds.
func (a *agentCacheAdapter) Set(ctx context.Context, key, value string, ttlSeconds int) error {
	return a.rdb.Set(ctx, key, value, time.Duration(ttlSeconds)*time.Second).Err()
}

// agentRAGAdapter adapts rag.Service to the ai.RAGService interface. The
// ai package defines its own RAGChunk mirror type to avoid importing
// internal/rag (which imports internal/ai for the embedder adapter).
type agentRAGAdapter struct {
	svc *rag.Service
}

// Retrieve calls rag.Service.Retrieve and converts each RetrievedChunk to
// the ai-package mirror type.
func (a *agentRAGAdapter) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]ai.RAGChunk, error) {
	chunks, err := a.svc.Retrieve(ctx, tenantID, query, language, geoSpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, ai.RAGChunk{
			EntityID:   c.EntityID,
			EntityType: c.EntityType,
			ChunkText:  c.ChunkText,
			Similarity: c.Similarity,
			Tier:       c.Tier,
		})
	}
	return out, nil
}

// agentContentCreatorBridge adapts content.Service.CreateContent to the
// ai.ContentCreator function type. The ai package uses a provider-agnostic
// ContentCreateInput/ContentCreateRow so it doesn't import internal/content.
type agentContentCreatorBridge struct {
	svc *content.Service
}

func (b *agentContentCreatorBridge) create(ctx context.Context, tenantID pgtype.UUID, input ai.ContentCreateInput) (ai.ContentCreateRow, error) {
	row, err := b.svc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    input.AuthorID,
		SpaceID:     input.SpaceID,
		ContentType: input.ContentType,
		Title:       input.Title,
		Body:        input.Body,
		FieldValues: input.FieldValues,
		GeoSpaceID:  input.GeoSpaceID,
		Status:      input.Status,
	})
	if err != nil {
		return ai.ContentCreateRow{}, err
	}
	return ai.ContentCreateRow{
		ID:          row.ID,
		Title:       row.Title,
		Slug:        row.Slug,
		Status:      row.Status,
		ContentType: row.ContentType,
		AuthorID:    row.AuthorID,
		GeoSpaceID:  row.GeoSpaceID,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// agentContentUpdaterBridge adapts content.Service.UpdateContent to the
// ai.ContentUpdater function type.
type agentContentUpdaterBridge struct {
	svc *content.Service
}

func (b *agentContentUpdaterBridge) update(ctx context.Context, contentID, tenantID, editedBy pgtype.UUID, input ai.ContentUpdateInput) (ai.ContentUpdateRow, error) {
	// Build the content.UpdateContentInput from the ai-package input. Only
	// the pointer fields that are non-nil are set; the rest default to
	// zero values that content.Service treats as "unchanged" when the
	// corresponding sqlc.narg is invalid.
	var fieldValues []byte
	if input.FieldValues != nil {
		fieldValues = input.FieldValues
	}
	var geoSpaceID pgtype.UUID
	if input.GeoSpaceID != nil {
		geoSpaceID = *input.GeoSpaceID
	}

	row, err := b.svc.UpdateContent(ctx, contentID, tenantID, editedBy, content.UpdateContentInput{
		Title:       derefStr(input.Title),
		Slug:        derefStr(input.Slug),
		Body:        derefStr(input.Body),
		FieldValues: fieldValues,
		GeoSpaceID:  geoSpaceID,
		Status:      derefStr(input.Status),
	}, true) // bypassEditWindow: the agent operates on behalf of the user
	if err != nil {
		return ai.ContentUpdateRow{}, err
	}
	return ai.ContentUpdateRow{
		ID:        row.ID,
		Title:     row.Title,
		Slug:      row.Slug,
		Status:    row.Status,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// agentRateLimiterAdapter adapts valkeycompat.Cmdable to the ai.RateLimiter
// interface. The ai package uses int TTL seconds and plain error returns;
// valkeycompat uses time.Duration and command types.
type agentRateLimiterAdapter struct {
	rdb valkeycompat.Cmdable
}

// Incr increments the counter at key and returns the new value.
func (a *agentRateLimiterAdapter) Incr(ctx context.Context, key string) (int64, error) {
	return a.rdb.Incr(ctx, key).Result()
}

// Expire sets the TTL on key in seconds.
func (a *agentRateLimiterAdapter) Expire(ctx context.Context, key string, ttlSeconds int) error {
	return a.rdb.Expire(ctx, key, time.Duration(ttlSeconds)*time.Second).Err()
}

// derefStr returns *s if s is non-nil, else "".
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// agentComparisonAdapter adapts comparison.Service to the ai.ComparisonService
// interface (Phase 7). The ai package defines its own input/output mirror
// types (ComparisonTypeCreateInput, ComparisonCreateInput, etc.) to avoid
// importing internal/comparison — the adapter converts between the two.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7 wiring)
type agentComparisonAdapter struct {
	svc *comparison.Service
}

func (a *agentComparisonAdapter) ListComparisonTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error) {
	return a.svc.ListComparisonTypes(ctx, tenantID, limit, offset)
}

func (a *agentComparisonAdapter) GetComparisonTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetComparisonTypeBySlugRow, error) {
	return a.svc.GetComparisonTypeBySlug(ctx, slug, tenantID)
}

func (a *agentComparisonAdapter) CreateComparisonType(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonTypeCreateInput) (gen.CreateComparisonTypeRow, error) {
	return a.svc.CreateComparisonType(ctx, tenantID, comparison.CreateComparisonTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Icon:         input.Icon,
		FieldGroupID: input.FieldGroupID,
		ItemLabel:    input.ItemLabel,
		IsSystem:     input.IsSystem,
	})
}

func (a *agentComparisonAdapter) CreateComparison(ctx context.Context, tenantID, userID pgtype.UUID, input ai.ComparisonCreateInput) (ai.ComparisonCreateResult, error) {
	items := make([]comparison.ComparisonItemInput, 0, len(input.Items))
	for _, it := range input.Items {
		items = append(items, comparison.ComparisonItemInput{Name: it.Name, Slug: it.Slug})
	}
	res, err := a.svc.CreateComparison(ctx, tenantID, userID, comparison.CreateComparisonInput{
		Title:      input.Title,
		Body:       input.Body,
		Visibility: input.Visibility,
		Status:     input.Status,
		SpaceID:    input.SpaceID,
		TypeID:     input.TypeID,
		Items:      items,
		Dimensions: input.Dimensions,
	})
	if err != nil {
		return ai.ComparisonCreateResult{}, err
	}
	return ai.ComparisonCreateResult{
		ID:         res.ID,
		TenantID:   res.TenantID,
		AuthorID:   res.AuthorID,
		Title:      res.Title,
		Slug:       res.Slug,
		Visibility: res.Visibility,
		SpaceID:    res.SpaceID,
	}, nil
}

func (a *agentComparisonAdapter) GetComparisonDetail(ctx context.Context, id, tenantID pgtype.UUID) (ai.ComparisonDetail, error) {
	d, err := a.svc.GetComparisonDetail(ctx, id, tenantID)
	if err != nil {
		return ai.ComparisonDetail{}, err
	}
	return ai.ComparisonDetail{
		ID:         d.ID,
		TenantID:   d.TenantID,
		AuthorID:   d.AuthorID,
		Title:      d.Title,
		Slug:       d.Slug,
		Body:       d.Body,
		Visibility: d.Visibility,
		Status:     d.Status,
		SpaceID:    d.SpaceID,
		Items:      d.Items,
		Dimensions: d.Dimensions,
	}, nil
}

func (a *agentComparisonAdapter) GetComparisonMatrix(ctx context.Context, contentID, tenantID pgtype.UUID) ([]ai.ComparisonMatrixItem, error) {
	matrix, err := a.svc.GetComparisonMatrix(ctx, contentID, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.ComparisonMatrixItem, 0, len(matrix))
	for _, m := range matrix {
		dims := make([]ai.ComparisonMatrixDimension, 0, len(m.Dimensions))
		for _, d := range m.Dimensions {
			dims = append(dims, ai.ComparisonMatrixDimension{
				DimensionID:  d.DimensionID,
				Label:        d.Label,
				Type:         d.Type,
				Value:        d.Value,
				ValueRating:  d.ValueRating,
				ValueBoolean: d.ValueBoolean,
			})
		}
		out = append(out, ai.ComparisonMatrixItem{
			Item:       m.Item,
			Dimensions: dims,
		})
	}
	return out, nil
}

func (a *agentComparisonAdapter) CreateComparisonDimension(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonDimensionCreateInput) (gen.CreateComparisonDimensionRow, error) {
	return a.svc.CreateComparisonDimension(ctx, tenantID, comparison.CreateComparisonDimensionInput{
		ContentID:     input.ContentID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
}

func (a *agentComparisonAdapter) UpsertComparisonValueWithMetadata(ctx context.Context, tenantID pgtype.UUID, input ai.ComparisonValueUpsertInput, metadata []byte) (gen.ComparisonValue, error) {
	return a.svc.UpsertComparisonValueWithMetadata(ctx, tenantID, comparison.UpsertComparisonValueInput{
		DimensionID:  input.DimensionID,
		ItemID:       input.ItemID,
		Value:        input.Value,
		ValueRating:  input.ValueRating,
		ValueBoolean: input.ValueBoolean,
	}, metadata)
}
