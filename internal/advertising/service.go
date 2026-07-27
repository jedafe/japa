package advertising

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
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
	TargetGeo           []string
	TargetSpaceIDs      []pgtype.UUID
}

// CreateCampaign creates a new ad campaign (admin-only in V1).
func (s *Service) CreateCampaign(ctx context.Context, tenantID, createdBy pgtype.UUID, input CreateCampaignInput) (AdCampaign, error) {
	if input.Name == "" {
		return AdCampaign{}, errors.New("name is required")
	}

	if input.SlotTypeName == "" {
		return AdCampaign{}, errors.New("slot_type_name is required")
	}

	if input.Budget != nil {
		minBudget := float64(10)
		if s.settings != nil {
			minBudget = s.settings.GetFloatOr(ctx, tenantID, "ads.min_campaign_budget", minBudget)
		}
		if float64(*input.Budget) < minBudget {
			return AdCampaign{}, fmt.Errorf("budget must be at least %.0f credits", minBudget)
		}
	}

	slotType, err := s.store.Queries().GetAdSlotTypeByName(ctx, gen.GetAdSlotTypeByNameParams{
		TenantID: tenantID,
		SlotName: input.SlotTypeName,
	})
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

	return s.store.Queries().CreateAdCampaign(ctx, gen.CreateAdCampaignParams{
		TenantID:            tenantID,
		Name:                input.Name,
		SlotTypeID:          slotType.ID,
		Content:             content,
		TargetUrl:           targetURL,
		Budget:              input.Budget,
		Status:              CampaignStatusDraft,
		Priority:            input.Priority,
		FrequencyCapPerUser: input.FrequencyCapPerUser,
		StartDate:           startDate,
		EndDate:             endDate,
		CreatedBy:           createdBy,
		TargetGeo:           input.TargetGeo,
		TargetSpaceIds:      input.TargetSpaceIDs,
	})
}

// GetCampaign retrieves a campaign by ID.
func (s *Service) GetCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.Queries().GetAdCampaignByID(ctx, gen.GetAdCampaignByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// ListCampaigns returns all campaigns for a tenant.
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AdCampaign, error) {
	return s.store.listAdCampaignsByTenant(ctx, tenantID, limit, offset)
}

// ActivateCampaign sets a campaign to active status.
func (s *Service) ActivateCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.Queries().GetAdCampaignByID(ctx, gen.GetAdCampaignByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return AdCampaign{}, err
	}

	if campaign.Status != CampaignStatusDraft && campaign.Status != CampaignStatusPaused {
		return AdCampaign{}, fmt.Errorf("campaign cannot be activated from status: %s", campaign.Status)
	}

	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status:   CampaignStatusActive,
		ID:       id,
		TenantID: tenantID,
	})
}

// PauseCampaign pauses an active campaign.
func (s *Service) PauseCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status:   CampaignStatusPaused,
		ID:       id,
		TenantID: tenantID,
	})
}

// EndCampaign permanently ends a campaign.
func (s *Service) EndCampaign(ctx context.Context, id, tenantID pgtype.UUID) (AdCampaign, error) {
	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status:   CampaignStatusEnded,
		ID:       id,
		TenantID: tenantID,
	})
}

// RenderSlot returns eligible campaigns for a slot.
// Returns empty slice (never an error with nil campaigns) when no match.
func (s *Service) RenderSlot(ctx context.Context, tenantID, userID pgtype.UUID, slotName string) ([]AdCampaign, error) {
	slotType, err := s.store.Queries().GetAdSlotTypeByName(ctx, gen.GetAdSlotTypeByNameParams{
		TenantID: tenantID,
		SlotName: slotName,
	})
	if err != nil {
		return []AdCampaign{}, fmt.Errorf("resolving slot type: %w", err)
	}

	campaigns, err := s.store.listActiveCampaignsBySlot(ctx, tenantID, slotType.ID,
		slotType.MaxDisplayPerPage, 0)
	if err != nil {
		return nil, fmt.Errorf("listing active campaigns: %w", err)
	}

	// If no campaigns exist or none are active, return empty — never a placeholder.
	if len(campaigns) == 0 {
		return []AdCampaign{}, nil
	}

	// Apply frequency capping per user/day.
	filtered := []AdCampaign{}

	globalFreqCap := 0
	if s.settings != nil {
		globalFreqCap = s.settings.GetIntOr(ctx, tenantID, "ads.frequency_cap_per_day", 3)
	}

	for _, c := range campaigns {
		cap := 0
		if c.FrequencyCapPerUser != nil {
			cap = int(*c.FrequencyCapPerUser)
		} else if globalFreqCap > 0 {
			cap = globalFreqCap
		}

		if cap > 0 && userID.Valid {
			count, err := s.store.Queries().CountImpressionsForUserToday(ctx, gen.CountImpressionsForUserTodayParams{
				UserID:     userID,
				CampaignID: c.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("checking frequency cap: %w", err)
			}

			if count >= int32(cap) {
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

		occurredAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		if !event.OccurredAt.IsZero() {
			occurredAt = pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}
		}

		_, err := s.store.Queries().InsertAdEvent(ctx, gen.InsertAdEventParams{
			TenantID:   tenantID,
			CampaignID: campaignID,
			UserID:     userID,
			EventType:  event.EventType,
			OccurredAt: occurredAt,
		})
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

// AdSlotTypeInput holds parameters for creating/updating a slot type.
type AdSlotTypeInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	MaxWidth    int    `json:"max_width"`
	MaxHeight   int    `json:"max_height"`
	Format      string `json:"format"`
}

// CreateSlotType creates a new ad slot type.
func (s *Service) CreateSlotType(ctx context.Context, tenantID pgtype.UUID, input AdSlotTypeInput) (AdSlotType, error) {
	return s.store.Queries().CreateAdSlotType(ctx, gen.CreateAdSlotTypeParams{
		TenantID:          tenantID,
		SlotName:          input.Name,
		Description:       &input.Description,
		MaxDisplayPerPage: 1,
	})
}

// UpdateSlotType updates an existing ad slot type.
func (s *Service) UpdateSlotType(ctx context.Context, id, tenantID pgtype.UUID, input AdSlotTypeInput) (AdSlotType, error) {
	return s.store.updateAdSlotType(ctx, id, tenantID, input.Name, input.Slug, input.Description, input.MaxWidth, input.MaxHeight, input.Format)
}

// DeleteSlotType deletes an ad slot type.
func (s *Service) DeleteSlotType(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteAdSlotType(ctx, id, tenantID)
}

// DeleteCampaign deletes a campaign (only if draft or ended).
func (s *Service) DeleteCampaign(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteAdCampaign(ctx, id, tenantID)
}

// GetCampaignStats returns aggregate stats for a campaign.
func (s *Service) GetCampaignStats(ctx context.Context, campaignID, tenantID pgtype.UUID) (map[string]any, error) {
	return s.store.getCampaignStats(ctx, campaignID, tenantID)
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
		return OrganizationBoost{}, errors.New("organization_id is required")
	}

	if !input.CampaignID.Valid {
		return OrganizationBoost{}, errors.New("campaign_id is required")
	}

	boostType := input.BoostType
	if boostType == "" {
		boostType = BoostTypeFeaturedPlacement
	}

	startDate := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	if !input.StartDate.IsZero() {
		startDate = pgtype.Timestamptz{Time: input.StartDate, Valid: true}
	}

	var endDate pgtype.Timestamptz
	if input.EndDate != nil {
		endDate = pgtype.Timestamptz{Time: *input.EndDate, Valid: true}
	}

	return s.store.Queries().CreateOrganizationBoost(ctx, gen.CreateOrganizationBoostParams{
		TenantID:       tenantID,
		OrganizationID: input.OrganizationID,
		CampaignID:     input.CampaignID,
		BoostType:      boostType,
		Status:         BoostStatusActive,
		StartDate:      startDate,
		EndDate:        endDate,
	})
}

// ListBoosts returns all boosts for an organization.
func (s *Service) ListBoosts(ctx context.Context, tenantID, organizationID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.store.Queries().ListOrganizationBoosts(ctx, gen.ListOrganizationBoostsParams{
		TenantID:       tenantID,
		OrganizationID: organizationID,
	})
}

// ListActiveBoosts returns all currently active boosts for a tenant.
func (s *Service) ListActiveBoosts(ctx context.Context, tenantID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.store.Queries().ListActiveBoosts(ctx, tenantID)
}

// EndBoost ends an active boost.
func (s *Service) EndBoost(ctx context.Context, id, tenantID pgtype.UUID) (OrganizationBoost, error) {
	return s.store.Queries().EndOrganizationBoost(ctx, gen.EndOrganizationBoostParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// UpdateBoost updates an existing boost's status and/or end date.
func (s *Service) UpdateBoost(ctx context.Context, id, tenantID pgtype.UUID, status *string, endDate *time.Time) (OrganizationBoost, error) {
	var ed pgtype.Timestamptz
	if endDate != nil {
		ed = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}
	return s.store.Queries().UpdateOrganizationBoost(ctx, gen.UpdateOrganizationBoostParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
		EndDate:  ed,
	})
}

// DeleteBoost removes a boost.
func (s *Service) DeleteBoost(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.Queries().DeleteOrganizationBoost(ctx, gen.DeleteOrganizationBoostParams{
		ID:       id,
		TenantID: tenantID,
	})
}
