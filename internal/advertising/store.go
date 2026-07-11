package advertising

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for advertising operations.
// Uses sqlc-generated queries for all operations.
type Store struct {
	queries *gen.Queries
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
	}
}

// Queries returns the sqlc-generated queries for direct access.
func (s *Store) Queries() *gen.Queries {
	return s.queries
}

// Type aliases for backward compatibility with service layer.
type AdSlotType = gen.AdSlotType
type AdCampaign = gen.AdCampaign
type AdEvent = gen.AdEvent
type AdAnalyticsDaily = gen.AdAnalyticsDaily
type OrganizationBoost = gen.OrganizationBoost

// ---------------------------------------------------------------------------
// Ad Slot Types
// ---------------------------------------------------------------------------

func (s *Store) listAdSlotTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdSlotType, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	return s.queries.ListAdSlotTypes(ctx, gen.ListAdSlotTypesParams{
		TenantID:   tenantID,
		PageOffset: offset,
		PageLimit:  limit,
	})
}

func (s *Store) updateAdSlotType(ctx context.Context, id, tenantID pgtype.UUID, name, slug, description string, maxWidth, maxHeight int, format string) (AdSlotType, error) {
	return s.queries.UpdateAdSlotTypeDetails(ctx, gen.UpdateAdSlotTypeDetailsParams{
		ID:          id,
		TenantID:    tenantID,
		SlotName:    name,
		Description: &description,
	})
}

func (s *Store) deleteAdSlotType(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.queries.DeleteAdSlotType(ctx, gen.DeleteAdSlotTypeParams{
		ID:       id,
		TenantID: tenantID,
	})
}

func (s *Store) deleteAdCampaign(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.queries.DeleteAdCampaign(ctx, gen.DeleteAdCampaignParams{
		ID:       id,
		TenantID: tenantID,
	})
}

func (s *Store) getCampaignStats(ctx context.Context, campaignID, tenantID pgtype.UUID) (map[string]any, error) {
	row, err := s.queries.GetCampaignStats(ctx, gen.GetCampaignStatsParams{
		CampaignID: campaignID,
		TenantID:   tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("get campaign stats: %w", err)
	}

	stats := map[string]any{
		"impressions":  row.Impressions,
		"clicks":       row.Clicks,
		"unique_users": row.UniqueUsers,
		"conversions":  row.Conversions,
	}

	if row.Impressions > 0 {
		stats["ctr"] = float64(row.Clicks) / float64(row.Impressions) * 100
	} else {
		stats["ctr"] = 0.0
	}

	return stats, nil
}

// ---------------------------------------------------------------------------
// Ad Campaigns
// ---------------------------------------------------------------------------

func (s *Store) listAdCampaignsByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	return s.queries.ListAdCampaignsByTenant(ctx, gen.ListAdCampaignsByTenantParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) listActiveCampaignsBySlot(ctx context.Context, tenantID, slotTypeID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	return s.queries.ListActiveCampaignsBySlot(ctx, gen.ListActiveCampaignsBySlotParams{
		TenantID:   tenantID,
		SlotTypeID: slotTypeID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// ---------------------------------------------------------------------------
// Ad Events (batched ingestion)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Organization Boosts
// ---------------------------------------------------------------------------
