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

// AdSlotType and the following aliases mirror the generated types for
// backward compatibility with the service layer.
type (
	AdSlotType        = gen.AdSlotType
	AdCampaign        = gen.AdCampaign
	AdEvent           = gen.AdEvent
	AdAnalyticsDaily  = gen.AdAnalyticsDaily
	OrganizationBoost = gen.OrganizationBoost
)

// ---------------------------------------------------------------------------
// Ad Slot Types
// ---------------------------------------------------------------------------

func (s *Store) listAdSlotTypes(ctx context.Context, limit, offset int32) ([]AdSlotType, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	return s.queries.ListAdSlotTypes(ctx, gen.ListAdSlotTypesParams{

		PageOffset: offset,
		PageLimit:  limit,
	})
}

func (s *Store) updateAdSlotType(ctx context.Context, id pgtype.UUID, name, description string) (AdSlotType, error) {
	return s.queries.UpdateAdSlotTypeDetails(ctx, gen.UpdateAdSlotTypeDetailsParams{
		ID: id,

		SlotName:    name,
		Description: &description,
	})
}

func (s *Store) deleteAdSlotType(ctx context.Context, id pgtype.UUID) error {
	return s.queries.DeleteAdSlotType(ctx,
		id,
	)
}

func (s *Store) deleteAdCampaign(ctx context.Context, id pgtype.UUID) error {
	return s.queries.DeleteAdCampaign(ctx,
		id,
	)
}

func (s *Store) getCampaignStats(ctx context.Context, campaignID pgtype.UUID) (map[string]any, error) {
	row, err := s.queries.GetCampaignStats(ctx,
		campaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("get campaign stats: %w", err)
	}

	// Ref: japaconnect-engineering/sections/25-ad-placement.md
	// Stats contract consumed by apps/admin ads page: campaign_id, impressions,
	// clicks, conversions, spend, ctr, cpc, daily_stats. Missing fields render
	// as `undefined` in the modal, so always include every key with a zero
	// default. Spend is read from ad_campaigns.spent via the GetCampaignStats
	// query (LEFT JOIN).
	ctr := 0.0
	if row.Impressions > 0 {
		ctr = float64(row.Clicks) / float64(row.Impressions) * 100
	}

	spend := float64(row.Spent)
	cpc := 0.0

	if row.Clicks > 0 {
		cpc = spend / float64(row.Clicks)
	}

	stats := map[string]any{
		"campaign_id":  campaignID.String(),
		"impressions":  row.Impressions,
		"clicks":       row.Clicks,
		"unique_users": row.UniqueUsers,
		"conversions":  row.Conversions,
		"spend":        spend,
		"ctr":          ctr,
		"cpc":          cpc,
		"daily_stats":  []map[string]any{},
	}

	return stats, nil
}

// ---------------------------------------------------------------------------
// Ad Campaigns
// ---------------------------------------------------------------------------

func (s *Store) listAdCampaignsByTenant(ctx context.Context, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	return s.queries.ListAdCampaignsByTenant(ctx, gen.ListAdCampaignsByTenantParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) listActiveCampaignsBySlot(ctx context.Context, slotTypeID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	return s.queries.ListActiveCampaignsBySlot(ctx, gen.ListActiveCampaignsBySlotParams{

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

// ---------------------------------------------------------------------------
// Ad Networks
// ---------------------------------------------------------------------------

// AdNetwork is an exported type.
type AdNetwork = gen.AdNetwork

func (s *Store) createAdNetwork(ctx context.Context, input gen.CreateAdNetworkParams) (AdNetwork, error) {
	return s.queries.CreateAdNetwork(ctx, input)
}

func (s *Store) getAdNetworkByID(ctx context.Context, id pgtype.UUID) (AdNetwork, error) {
	return s.queries.GetAdNetworkByID(ctx, id)
}

func (s *Store) getAdNetworkByProvider(ctx context.Context, provider string) (AdNetwork, error) {
	return s.queries.GetAdNetworkByProvider(ctx, provider)
}

func (s *Store) listAdNetworks(ctx context.Context, limit, offset int32) ([]AdNetwork, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	return s.queries.ListAdNetworks(ctx, gen.ListAdNetworksParams{

		PageOffset: offset,
		PageLimit:  limit,
	})
}

func (s *Store) listActiveAdNetworks(ctx context.Context) ([]AdNetwork, error) {
	return s.queries.ListActiveAdNetworks(ctx)
}

func (s *Store) updateAdNetwork(ctx context.Context, id pgtype.UUID, input gen.UpdateAdNetworkParams) (AdNetwork, error) {
	return s.queries.UpdateAdNetwork(ctx, input)
}

func (s *Store) deleteAdNetwork(ctx context.Context, id pgtype.UUID) error {
	return s.queries.DeleteAdNetwork(ctx, id)
}

func (s *Store) countAdNetworks(ctx context.Context) (int32, error) {
	return s.queries.CountAdNetworks(ctx)
}
