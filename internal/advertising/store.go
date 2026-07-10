package advertising

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for advertising operations.
// Uses sqlc-generated queries where available, with hand-rolled queries
// for operations not yet covered by sqlc.
type Store struct {
	write   *db.DB
	queries *gen.Queries
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{
		write:   database,
		queries: gen.New(database.Reader),
	}
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

func (s *Store) getAdSlotTypeByName(ctx context.Context, tenantID pgtype.UUID, slotName string) (AdSlotType, error) {
	return s.queries.GetAdSlotTypeByName(ctx, gen.GetAdSlotTypeByNameParams{
		TenantID: tenantID,
		SlotName: slotName,
	})
}

func (s *Store) createAdSlotType(ctx context.Context, tenantID pgtype.UUID, name, slug, description string, maxWidth, maxHeight int, format string) (AdSlotType, error) {
	return s.queries.CreateAdSlotType(ctx, gen.CreateAdSlotTypeParams{
		TenantID:          tenantID,
		SlotName:          name,
		Description:       &description,
		MaxDisplayPerPage: 1,
	})
}

func (s *Store) updateAdSlotType(ctx context.Context, id, tenantID pgtype.UUID, name, slug, description string, maxWidth, maxHeight int, format string) (AdSlotType, error) {
	// sqlc-generated UpdateAdSlotType only updates max_display_per_page and is_active.
	// Use hand-rolled query to update slot_name and description.
	var r AdSlotType
	err := s.write.Writer.QueryRow(ctx, `
		UPDATE ad_slot_types
		SET slot_name = $3, description = $4
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, slot_name, description, max_display_per_page, is_active, created_at
	`, id, tenantID, name, description).Scan(
		&r.ID, &r.TenantID, &r.SlotName, &r.Description,
		&r.MaxDisplayPerPage, &r.IsActive, &r.CreatedAt,
	)
	if err != nil {
		return AdSlotType{}, fmt.Errorf("update ad slot type: %w", err)
	}
	return r, nil
}

func (s *Store) deleteAdSlotType(ctx context.Context, id, tenantID pgtype.UUID) error {
	_, err := s.write.Writer.Exec(ctx, `
		DELETE FROM ad_slot_types WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete ad slot type: %w", err)
	}
	return nil
}

func (s *Store) deleteAdCampaign(ctx context.Context, id, tenantID pgtype.UUID) error {
	result, err := s.write.Writer.Exec(ctx, `
		DELETE FROM ad_campaigns
		WHERE id = $1 AND tenant_id = $2 AND status IN ('draft', 'ended')
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete ad campaign: %w", err)
	}
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("campaign not found or cannot be deleted")
	}
	return nil
}

func (s *Store) getCampaignStats(ctx context.Context, campaignID, tenantID pgtype.UUID) (map[string]any, error) {
	stats := make(map[string]any)
	var impressions, clicks, uniqueUsers, conversions int64

	err := s.write.Reader.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(impressions), 0),
			COALESCE(SUM(clicks), 0),
			COUNT(DISTINCT user_id),
			COALESCE(SUM(conversions), 0)
		FROM ad_events
		WHERE campaign_id = $1 AND tenant_id = $2
	`, campaignID, tenantID).Scan(&impressions, &clicks, &uniqueUsers, &conversions)
	if err != nil {
		return nil, fmt.Errorf("get campaign stats: %w", err)
	}

	stats["impressions"] = impressions
	stats["clicks"] = clicks
	stats["unique_users"] = uniqueUsers
	stats["conversions"] = conversions

	if impressions > 0 {
		stats["ctr"] = float64(clicks) / float64(impressions) * 100
	} else {
		stats["ctr"] = 0.0
	}

	return stats, nil
}

// ---------------------------------------------------------------------------
// Ad Campaigns
// ---------------------------------------------------------------------------

func (s *Store) createAdCampaign(ctx context.Context, tenantID pgtype.UUID, name string,
	slotTypeID pgtype.UUID, content []byte, targetURL *string, budget *int64,
	status string, priority int32, frequencyCapPerUser *int32,
	startDate, endDate pgtype.Timestamptz, createdBy pgtype.UUID,
) (AdCampaign, error) {
	return s.queries.CreateAdCampaign(ctx, gen.CreateAdCampaignParams{
		TenantID:            tenantID,
		Name:                name,
		SlotTypeID:          slotTypeID,
		Content:             content,
		TargetUrl:           targetURL,
		Budget:              budget,
		Status:              status,
		Priority:            priority,
		FrequencyCapPerUser: frequencyCapPerUser,
		StartDate:           startDate,
		EndDate:             endDate,
		CreatedBy:           createdBy,
	})
}

func (s *Store) getAdCampaignByID(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.queries.GetAdCampaignByID(ctx, gen.GetAdCampaignByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
}

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

func (s *Store) updateAdCampaignStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (AdCampaign, error) {
	return s.queries.UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status:   status,
		ID:       id,
		TenantID: tenantID,
	})
}

// ---------------------------------------------------------------------------
// Ad Events (batched ingestion)
// ---------------------------------------------------------------------------

func (s *Store) insertAdEvent(ctx context.Context, tenantID, campaignID, userID pgtype.UUID,
	eventType string, occurredAt pgtype.Timestamptz,
) (AdEvent, error) {
	return s.queries.InsertAdEvent(ctx, gen.InsertAdEventParams{
		TenantID:   tenantID,
		CampaignID: campaignID,
		UserID:     userID,
		EventType:  eventType,
		OccurredAt: occurredAt,
	})
}

func (s *Store) countImpressionsForUserToday(ctx context.Context, userID, campaignID pgtype.UUID) (int32, error) {
	var count int32
	err := s.write.Reader.QueryRow(
		ctx,
		`SELECT COUNT(*)::int FROM ad_events
		 WHERE user_id = $1
		   AND campaign_id = $2
		   AND event_type = 'impression'
		   AND occurred_at >= current_date`, userID, campaignID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting impressions for user today: %w", err)
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Organization Boosts
// ---------------------------------------------------------------------------

func (s *Store) createOrganizationBoost(ctx context.Context, tenantID, organizationID,
	campaignID pgtype.UUID, boostType, status string, startDate, endDate pgtype.Timestamptz,
) (OrganizationBoost, error) {
	return s.queries.CreateOrganizationBoost(ctx, gen.CreateOrganizationBoostParams{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		CampaignID:     campaignID,
		BoostType:      boostType,
		Status:         status,
		StartDate:      startDate,
		EndDate:        endDate,
	})
}

func (s *Store) listOrganizationBoosts(ctx context.Context, tenantID, organizationID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.queries.ListOrganizationBoosts(ctx, gen.ListOrganizationBoostsParams{
		TenantID:       tenantID,
		OrganizationID: organizationID,
	})
}

func (s *Store) listActiveBoosts(ctx context.Context, tenantID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.queries.ListActiveBoosts(ctx, tenantID)
}

func (s *Store) endOrganizationBoost(ctx context.Context, id, tenantID pgtype.UUID) (OrganizationBoost, error) {
	// sqlc doesn't generate an end boost method, use hand-rolled query
	var r OrganizationBoost
	err := s.write.Writer.QueryRow(
		ctx,
		`UPDATE organization_boosts
		 SET status = 'ended', end_date = now()
		 WHERE id = $1 AND tenant_id = $2
		 RETURNING id, tenant_id, organization_id, campaign_id,
			boost_type, status, start_date, end_date, created_at`,
		id, tenantID,
	).Scan(
		&r.ID, &r.TenantID, &r.OrganizationID, &r.CampaignID,
		&r.BoostType, &r.Status, &r.StartDate, &r.EndDate, &r.CreatedAt,
	)
	if err != nil {
		return OrganizationBoost{}, fmt.Errorf("ending organization boost: %w", err)
	}
	return r, nil
}
