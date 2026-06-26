package advertising

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateCampaignInput holds parameters for creating a new ad campaign.
type CreateCampaignInput struct {
	Name                string
	SlotTypeName        string
	Content             map[string]interface{}
	TargetURL           string
	Budget              *int64
	Priority            int32
	FrequencyCapPerUser *int32
	StartDate           time.Time
	EndDate             *time.Time
}

// CreateCampaign creates a new ad campaign (admin-only in V1).
func (s *Service) CreateCampaign(ctx context.Context, tenantID, createdBy pgtype.UUID, input CreateCampaignInput) (AdCampaign, error) {
	if input.Name == "" {
		return AdCampaign{}, fmt.Errorf("name is required")
	}
	if input.SlotTypeName == "" {
		return AdCampaign{}, fmt.Errorf("slot_type_name is required")
	}

	slotType, err := s.store.getAdSlotTypeByName(ctx, tenantID, input.SlotTypeName)
	if err != nil {
		return AdCampaign{}, fmt.Errorf("resolving slot type: %w", err)
	}

	content, err := json.Marshal(input.Content)
	if err != nil {
		return AdCampaign{}, fmt.Errorf("marshaling content: %w", err)
	}

	var targetURL *string
	if input.TargetURL != "" {
		targetURL = &input.TargetURL
	}

	startDate := pgtype.Timestamptz{}
	if !input.StartDate.IsZero() {
		startDate = pgtype.Timestamptz{Time: input.StartDate, Valid: true}
	}

	var endDate pgtype.Timestamptz
	if input.EndDate != nil {
		endDate = pgtype.Timestamptz{Time: *input.EndDate, Valid: true}
	}

	return s.store.createAdCampaign(ctx, tenantID, input.Name, slotType.ID,
		content, targetURL, input.Budget, CampaignStatusDraft, input.Priority,
		input.FrequencyCapPerUser, startDate, endDate, createdBy)
}

// GetCampaign retrieves a campaign by ID.
func (s *Service) GetCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.getAdCampaignByID(ctx, id, tenantID)
}

// ListCampaigns returns all campaigns for a tenant.
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	return s.store.listAdCampaignsByTenant(ctx, tenantID, limit, offset)
}

// ActivateCampaign sets a campaign to active status.
func (s *Service) ActivateCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.getAdCampaignByID(ctx, id, tenantID)
	if err != nil {
		return AdCampaign{}, err
	}
	if campaign.Status != CampaignStatusDraft && campaign.Status != CampaignStatusPaused {
		return AdCampaign{}, fmt.Errorf("campaign cannot be activated from status: %s", campaign.Status)
	}
	return s.store.updateAdCampaignStatus(ctx, id, tenantID, CampaignStatusActive)
}

// PauseCampaign pauses an active campaign.
func (s *Service) PauseCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.updateAdCampaignStatus(ctx, id, tenantID, CampaignStatusPaused)
}

// EndCampaign permanently ends a campaign.
func (s *Service) EndCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.updateAdCampaignStatus(ctx, id, tenantID, CampaignStatusEnded)
}

// RenderSlot returns eligible campaigns for a slot.
// Returns empty slice (never an error with nil campaigns) when no match.
func (s *Service) RenderSlot(ctx context.Context, tenantID, userID pgtype.UUID, slotName string) ([]AdCampaign, error) {
	slotType, err := s.store.getAdSlotTypeByName(ctx, tenantID, slotName)
	if err != nil {
		return []AdCampaign{}, fmt.Errorf("resolving slot type: %w", err)
	}

	campaigns, err := s.store.listActiveCampaignsBySlot(ctx, tenantID, slotType.ID,
		int32(slotType.MaxDisplayPerPage), 0)
	if err != nil {
		return nil, fmt.Errorf("listing active campaigns: %w", err)
	}

	// If no campaigns exist or none are active, return empty — never a placeholder.
	if len(campaigns) == 0 {
		return []AdCampaign{}, nil
	}

	// Apply frequency capping per user/day.
	filtered := []AdCampaign{}
	for _, c := range campaigns {
		if c.FrequencyCapPerUser != nil && userID.Valid {
			count, err := s.store.countImpressionsForUserToday(ctx, userID, c.ID)
			if err != nil {
				return nil, fmt.Errorf("checking frequency cap: %w", err)
			}
			if count >= *c.FrequencyCapPerUser {
				continue
			}
		}
		filtered = append(filtered, c)
	}

	return filtered, nil
}

// ---------------------------------------------------------------------------
// Ad Events — batched ingestion
// ---------------------------------------------------------------------------

// IngestAdEventInput holds parameters for a single ad event.
type IngestAdEventInput struct {
	CampaignID string
	UserID     string
	EventType  string
	OccurredAt time.Time
}

// IngestAdEvents records a batch of ad events asynchronously.
// This should be called from a background worker, not synchronously on every
// page view.
func (s *Service) IngestAdEvents(ctx context.Context, tenantID pgtype.UUID, events []IngestAdEventInput) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	ingested := 0
	for _, event := range events {
		if event.EventType != EventTypeImpression && event.EventType != EventTypeClick {
			continue
		}

		var campaignID pgtype.UUID
		if err := campaignID.Scan(event.CampaignID); err != nil {
			continue
		}
		var userID pgtype.UUID
		if err := userID.Scan(event.UserID); err != nil {
			continue
		}

		occurredAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
		if !event.OccurredAt.IsZero() {
			occurredAt = pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}
		}

		_, err := s.store.insertAdEvent(ctx, tenantID, campaignID, userID, event.EventType, occurredAt)
		if err != nil {
			return ingested, fmt.Errorf("inserting ad event at index %d: %w", ingested, err)
		}
		ingested++
	}

	return ingested, nil
}

// ---------------------------------------------------------------------------
// List Slot Types
// ---------------------------------------------------------------------------

// ListSlotTypes returns all slot types for a tenant.
func (s *Service) ListSlotTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdSlotType, error) {
	return s.store.listAdSlotTypes(ctx, tenantID, limit, offset)
}

// ---------------------------------------------------------------------------
// Organization Boosts
// ---------------------------------------------------------------------------

// CreateBoostInput holds parameters for creating an organization boost.
type CreateBoostInput struct {
	OrganizationID pgtype.UUID
	CampaignID     pgtype.UUID
	BoostType      string
	StartDate      time.Time
	EndDate        *time.Time
}

// CreateBoost creates an organization boost linked to a campaign.
func (s *Service) CreateBoost(ctx context.Context, tenantID pgtype.UUID, input CreateBoostInput) (OrganizationBoost, error) {
	if !input.OrganizationID.Valid {
		return OrganizationBoost{}, fmt.Errorf("organization_id is required")
	}
	if !input.CampaignID.Valid {
		return OrganizationBoost{}, fmt.Errorf("campaign_id is required")
	}

	boostType := input.BoostType
	if boostType == "" {
		boostType = BoostTypeFeaturedPlacement
	}

	startDate := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if !input.StartDate.IsZero() {
		startDate = pgtype.Timestamptz{Time: input.StartDate, Valid: true}
	}

	var endDate pgtype.Timestamptz
	if input.EndDate != nil {
		endDate = pgtype.Timestamptz{Time: *input.EndDate, Valid: true}
	}

	return s.store.createOrganizationBoost(ctx, tenantID, input.OrganizationID,
		input.CampaignID, boostType, BoostStatusActive, startDate, endDate)
}

// ListBoosts returns all boosts for an organization.
func (s *Service) ListBoosts(ctx context.Context, tenantID, organizationID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.store.listOrganizationBoosts(ctx, tenantID, organizationID)
}

// ListActiveBoosts returns all currently active boosts for a tenant.
func (s *Service) ListActiveBoosts(ctx context.Context, tenantID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.store.listActiveBoosts(ctx, tenantID)
}

// EndBoost ends an active boost.
func (s *Service) EndBoost(ctx context.Context, id, tenantID pgtype.UUID) (OrganizationBoost, error) {
	return s.store.endOrganizationBoost(ctx, id, tenantID)
}