package advertising

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Store provides low-level database access for advertising operations.
// Uses raw pgxpool queries until sqlc generates the corresponding types.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *db.DB
}

// NewStore creates a Store backed by the given database pool.
func NewStore(database *db.DB) *Store {
	return &Store{write: database}
}

// ---------------------------------------------------------------------------
// Inline types — replace with generated gen.* types after running sqlc generate.
// ---------------------------------------------------------------------------

// AdSlotType represents a row in the ad_slot_types table.
type AdSlotType struct {
	ID                pgtype.UUID        `json:"id"`
	TenantID          pgtype.UUID        `json:"tenant_id"`
	SlotName          string             `json:"slot_name"`
	Description       *string            `json:"description,omitempty"`
	MaxDisplayPerPage int32              `json:"max_display_per_page"`
	IsActive          bool               `json:"is_active"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
}

// AdCampaign represents a row in the ad_campaigns table.
type AdCampaign struct {
	ID                  pgtype.UUID        `json:"id"`
	TenantID            pgtype.UUID        `json:"tenant_id"`
	Name                string             `json:"name"`
	SlotTypeID          pgtype.UUID        `json:"slot_type_id"`
	Content             []byte             `json:"content"`
	TargetURL           *string            `json:"target_url,omitempty"`
	Budget              *int64             `json:"budget,omitempty"`
	Spent               int64              `json:"spent"`
	Status              string             `json:"status"`
	Priority            int32              `json:"priority"`
	FrequencyCapPerUser *int32             `json:"frequency_cap_per_user,omitempty"`
	StartDate           pgtype.Timestamptz `json:"start_date"`
	EndDate             pgtype.Timestamptz `json:"end_date"`
	CreatedBy           pgtype.UUID        `json:"created_by"`
	CreatedAt           pgtype.Timestamptz `json:"created_at"`
	UpdatedAt           pgtype.Timestamptz `json:"updated_at"`
}

// AdEvent represents a row in the ad_events table.
type AdEvent struct {
	ID         pgtype.UUID        `json:"id"`
	TenantID   pgtype.UUID        `json:"tenant_id"`
	CampaignID pgtype.UUID        `json:"campaign_id"`
	UserID     pgtype.UUID        `json:"user_id"`
	EventType  string             `json:"event_type"`
	OccurredAt pgtype.Timestamptz `json:"occurred_at"`
	CreatedAt  pgtype.Timestamptz `json:"created_at"`
}

// AdAnalyticsDaily represents a row in the ad_analytics_daily table.
type AdAnalyticsDaily struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
	CampaignID  pgtype.UUID        `json:"campaign_id"`
	SlotTypeID  pgtype.UUID        `json:"slot_type_id"`
	Date        pgtype.Date        `json:"date"`
	Impressions int32              `json:"impressions"`
	Clicks      int32              `json:"clicks"`
	Spent       int64              `json:"spent"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

// OrganizationBoost represents a row in the organization_boosts table.
type OrganizationBoost struct {
	ID             pgtype.UUID        `json:"id"`
	TenantID       pgtype.UUID        `json:"tenant_id"`
	OrganizationID pgtype.UUID        `json:"organization_id"`
	CampaignID     pgtype.UUID        `json:"campaign_id"`
	BoostType      string             `json:"boost_type"`
	Status         string             `json:"status"`
	StartDate      pgtype.Timestamptz `json:"start_date"`
	EndDate        pgtype.Timestamptz `json:"end_date"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Ad Slot Types
// ---------------------------------------------------------------------------

func (s *Store) listAdSlotTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdSlotType, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.write.Reader.Query(ctx,
		`SELECT id, tenant_id, slot_name, description, max_display_per_page, is_active, created_at
		 FROM ad_slot_types
		 WHERE tenant_id = $1
		 ORDER BY slot_name
		 LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing ad slot types: %w", err)
	}

	defer rows.Close()

	var result []AdSlotType

	for rows.Next() {
		var r AdSlotType
		if err := rows.Scan(&r.ID, &r.TenantID, &r.SlotName, &r.Description,
			&r.MaxDisplayPerPage, &r.IsActive, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning ad slot type: %w", err)
		}

		result = append(result, r)
	}

	if result == nil {
		result = []AdSlotType{}
	}

	return result, nil
}

func (s *Store) getAdSlotTypeByName(ctx context.Context, tenantID pgtype.UUID, slotName string) (AdSlotType, error) {
	var r AdSlotType

	err := s.write.Reader.QueryRow(
		ctx,
		`SELECT id, tenant_id, slot_name, description, max_display_per_page, is_active, created_at
		 FROM ad_slot_types
		 WHERE tenant_id = $1 AND slot_name = $2`, tenantID, slotName,
	).Scan(&r.ID, &r.TenantID, &r.SlotName, &r.Description,
		&r.MaxDisplayPerPage, &r.IsActive, &r.CreatedAt)
	if err != nil {
		return AdSlotType{}, fmt.Errorf("getting ad slot type by name: %w", err)
	}

	return r, nil
}

// createAdSlotType inserts a new ad slot type.
func (s *Store) createAdSlotType(ctx context.Context, tenantID pgtype.UUID, name, slug, description string, maxWidth, maxHeight int, format string) (AdSlotType, error) {
	var r AdSlotType

	err := s.write.Writer.QueryRow(ctx, `
		INSERT INTO ad_slot_types (tenant_id, slot_name, description, max_display_per_page, is_active)
		VALUES ($1, $2, $3, 1, true)
		RETURNING id, tenant_id, slot_name, description, max_display_per_page, is_active, created_at
	`, tenantID, name, description).Scan(
		&r.ID, &r.TenantID, &r.SlotName, &r.Description,
		&r.MaxDisplayPerPage, &r.IsActive, &r.CreatedAt,
	)
	if err != nil {
		return AdSlotType{}, fmt.Errorf("create ad slot type: %w", err)
	}

	return r, nil
}

// updateAdSlotType updates an existing ad slot type.
func (s *Store) updateAdSlotType(ctx context.Context, id, tenantID pgtype.UUID, name, slug, description string, maxWidth, maxHeight int, format string) (AdSlotType, error) {
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

// deleteAdSlotType deletes an ad slot type.
func (s *Store) deleteAdSlotType(ctx context.Context, id, tenantID pgtype.UUID) error {
	_, err := s.write.Writer.Exec(ctx, `
		DELETE FROM ad_slot_types WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete ad slot type: %w", err)
	}

	return nil
}

// deleteAdCampaign deletes a campaign (only if draft or ended).
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

// getCampaignStats returns aggregate stats for a campaign.
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
	var r AdCampaign

	err := s.write.Writer.QueryRow(
		ctx,
		`INSERT INTO ad_campaigns (
			tenant_id, name, slot_type_id, content, target_url,
			budget, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, tenant_id, name, slot_type_id, content, target_url,
			budget, spent, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by, created_at, updated_at`,
		tenantID, name, slotTypeID, content, targetURL, budget,
		status, priority, frequencyCapPerUser,
		startDate, endDate, createdBy,
	).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SlotTypeID, &r.Content, &r.TargetURL,
		&r.Budget, &r.Spent, &r.Status, &r.Priority, &r.FrequencyCapPerUser,
		&r.StartDate, &r.EndDate, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return AdCampaign{}, fmt.Errorf("creating ad campaign: %w", err)
	}

	return r, nil
}

func (s *Store) getAdCampaignByID(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	var r AdCampaign

	err := s.write.Reader.QueryRow(
		ctx,
		`SELECT id, tenant_id, name, slot_type_id, content, target_url,
			budget, spent, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by, created_at, updated_at
		 FROM ad_campaigns
		 WHERE id = $1 AND tenant_id = $2`, id, tenantID,
	).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SlotTypeID, &r.Content, &r.TargetURL,
		&r.Budget, &r.Spent, &r.Status, &r.Priority, &r.FrequencyCapPerUser,
		&r.StartDate, &r.EndDate, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return AdCampaign{}, fmt.Errorf("getting ad campaign by ID: %w", err)
	}

	return r, nil
}

func (s *Store) listAdCampaignsByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.write.Reader.Query(ctx,
		`SELECT id, tenant_id, name, slot_type_id, content, target_url,
			budget, spent, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by, created_at, updated_at
		 FROM ad_campaigns
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing ad campaigns by tenant: %w", err)
	}

	defer rows.Close()

	var result []AdCampaign

	for rows.Next() {
		var r AdCampaign
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.SlotTypeID, &r.Content, &r.TargetURL,
			&r.Budget, &r.Spent, &r.Status, &r.Priority, &r.FrequencyCapPerUser,
			&r.StartDate, &r.EndDate, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ad campaign: %w", err)
		}

		result = append(result, r)
	}

	if result == nil {
		result = []AdCampaign{}
	}

	return result, nil
}

func (s *Store) listActiveCampaignsBySlot(ctx context.Context, tenantID, slotTypeID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.write.Reader.Query(ctx,
		`SELECT id, tenant_id, name, slot_type_id, content, target_url,
			budget, spent, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by, created_at, updated_at
		 FROM ad_campaigns
		 WHERE tenant_id = $1
		   AND slot_type_id = $2
		   AND status = 'active'
		   AND (start_date IS NULL OR start_date <= now())
		   AND (end_date IS NULL OR end_date >= now())
		 ORDER BY priority DESC, created_at ASC
		 LIMIT $3 OFFSET $4`, tenantID, slotTypeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing active campaigns by slot: %w", err)
	}

	defer rows.Close()

	var result []AdCampaign

	for rows.Next() {
		var r AdCampaign
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.SlotTypeID, &r.Content, &r.TargetURL,
			&r.Budget, &r.Spent, &r.Status, &r.Priority, &r.FrequencyCapPerUser,
			&r.StartDate, &r.EndDate, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning active campaign: %w", err)
		}

		result = append(result, r)
	}

	if result == nil {
		result = []AdCampaign{}
	}

	return result, nil
}

func (s *Store) updateAdCampaignStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (AdCampaign, error) {
	var r AdCampaign

	err := s.write.Writer.QueryRow(
		ctx,
		`UPDATE ad_campaigns
		 SET status = $1, updated_at = now()
		 WHERE id = $2 AND tenant_id = $3
		 RETURNING id, tenant_id, name, slot_type_id, content, target_url,
			budget, spent, status, priority, frequency_cap_per_user,
			start_date, end_date, created_by, created_at, updated_at`,
		status, id, tenantID,
	).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SlotTypeID, &r.Content, &r.TargetURL,
		&r.Budget, &r.Spent, &r.Status, &r.Priority, &r.FrequencyCapPerUser,
		&r.StartDate, &r.EndDate, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return AdCampaign{}, fmt.Errorf("updating ad campaign status: %w", err)
	}

	return r, nil
}

// ---------------------------------------------------------------------------
// Ad Events (batched ingestion)
// ---------------------------------------------------------------------------

func (s *Store) insertAdEvent(ctx context.Context, tenantID, campaignID, userID pgtype.UUID,
	eventType string, occurredAt pgtype.Timestamptz,
) (AdEvent, error) {
	var r AdEvent

	err := s.write.Writer.QueryRow(
		ctx,
		`INSERT INTO ad_events (tenant_id, campaign_id, user_id, event_type, occurred_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, campaign_id, user_id, event_type, occurred_at, created_at`,
		tenantID, campaignID, userID, eventType, occurredAt,
	).Scan(&r.ID, &r.TenantID, &r.CampaignID, &r.UserID, &r.EventType, &r.OccurredAt, &r.CreatedAt)
	if err != nil {
		return AdEvent{}, fmt.Errorf("inserting ad event: %w", err)
	}

	return r, nil
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
	var r OrganizationBoost

	err := s.write.Writer.QueryRow(
		ctx,
		`INSERT INTO organization_boosts (
			tenant_id, organization_id, campaign_id, boost_type,
			status, start_date, end_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tenant_id, organization_id, campaign_id,
			boost_type, status, start_date, end_date, created_at`,
		tenantID, organizationID, campaignID, boostType,
		status, startDate, endDate,
	).Scan(
		&r.ID, &r.TenantID, &r.OrganizationID, &r.CampaignID,
		&r.BoostType, &r.Status, &r.StartDate, &r.EndDate, &r.CreatedAt,
	)
	if err != nil {
		return OrganizationBoost{}, fmt.Errorf("creating organization boost: %w", err)
	}

	return r, nil
}

func (s *Store) listOrganizationBoosts(ctx context.Context, tenantID, organizationID pgtype.UUID) ([]OrganizationBoost, error) {
	rows, err := s.write.Reader.Query(ctx,
		`SELECT id, tenant_id, organization_id, campaign_id,
			boost_type, status, start_date, end_date, created_at
		 FROM organization_boosts
		 WHERE tenant_id = $1 AND organization_id = $2
		 ORDER BY created_at DESC`, tenantID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("listing organization boosts: %w", err)
	}
	defer rows.Close()

	var result []OrganizationBoost

	for rows.Next() {
		var r OrganizationBoost
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.OrganizationID, &r.CampaignID,
			&r.BoostType, &r.Status, &r.StartDate, &r.EndDate, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning organization boost: %w", err)
		}

		result = append(result, r)
	}

	if result == nil {
		result = []OrganizationBoost{}
	}

	return result, nil
}

func (s *Store) listActiveBoosts(ctx context.Context, tenantID pgtype.UUID) ([]OrganizationBoost, error) {
	rows, err := s.write.Reader.Query(ctx,
		`SELECT id, tenant_id, organization_id, campaign_id,
			boost_type, status, start_date, end_date, created_at
		 FROM organization_boosts
		 WHERE tenant_id = $1 AND status = 'active'
		   AND (end_date IS NULL OR end_date >= now())
		 ORDER BY start_date DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing active boosts: %w", err)
	}
	defer rows.Close()

	var result []OrganizationBoost

	for rows.Next() {
		var r OrganizationBoost
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.OrganizationID, &r.CampaignID,
			&r.BoostType, &r.Status, &r.StartDate, &r.EndDate, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning active boost: %w", err)
		}

		result = append(result, r)
	}

	if result == nil {
		result = []OrganizationBoost{}
	}

	return result, nil
}

func (s *Store) endOrganizationBoost(ctx context.Context, id, tenantID pgtype.UUID) (OrganizationBoost, error) {
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
