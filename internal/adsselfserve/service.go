// Package adsselfserve implements the Self-Serve Advertising surface
// (Section 61, ADR-079). Verified organisations can create, fund, and manage
// ad campaigns without admin involvement. Campaigns are paid upfront via
// credits, then enter the existing content_reviews queue for admin creative
// approval before going live.
//
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md
//      japaconnect-engineering/adr/ADR-079-self-serve-ads.md
package adsselfserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Status constants for ad campaign lifecycle.
const (
	StatusPendingReview = "pending_review"
	StatusActive        = "active"
	StatusPaused        = "paused"
	StatusEnded         = "ended"
	StatusRejected      = "rejected"
	StatusDraft         = "draft"
)

// Task name constants.
const (
	TaskRefundCampaignCredits = "ads:refund_campaign_credits"
	TaskNotifyCampaignStatus  = "ads:notify_campaign_status"
	TaskDeductCredits         = "wallet:deduct_credits"
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte) error
}

// CreateCampaignInput is the request body for creating a self-serve campaign.
type CreateCampaignInput struct {
	Name                string          `json:"name"`
	SlotTypeID          string          `json:"slot_type_id"`
	Content             json.RawMessage `json:"content"`
	TargetURL           string          `json:"target_url"`
	StartDate           *time.Time      `json:"start_date"`
	EndDate             *time.Time      `json:"end_date"`
	TargetGeo           []string        `json:"target_geo"`
	TargetSpaceIDs      []string        `json:"target_space_ids"`
	FrequencyCapPerUser *int            `json:"frequency_cap_per_user"`
	DurationDays        int             `json:"duration_days"`
}

// CampaignStats holds aggregated stats for a campaign.
type CampaignStats struct {
	Impressions   int64   `json:"impressions"`
	Clicks        int64   `json:"clicks"`
	CTR           float64 `json:"ctr"`
	Spent         int64   `json:"spent"`
	BudgetCredits int     `json:"budget_credits"`
}

// SlotInfo is the public representation of an ad slot type.
type SlotInfo struct {
	ID                string `json:"id"`
	SlotName          string `json:"slot_name"`
	Description       string `json:"description"`
	FlatRate          int    `json:"flat_rate"`
	MaxDisplayPerPage int    `json:"max_display_per_page"`
}

// Service implements the self-serve ads business logic.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a self-serve ads service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// CalculateCost computes the total cost in credits for a campaign.
// cost = flat_rate × duration_days.
func CalculateCost(flatRate int, durationDays int) int {
	return flatRate * durationDays
}

// ListSlots returns available ad slot types with flat_rate for self-serve.
func (s *Service) ListSlots(ctx context.Context, tenantID pgtype.UUID) ([]SlotInfo, error) {
	rows, err := s.store.ListAdSlotTypes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]SlotInfo, 0, len(rows))
	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		result = append(result, SlotInfo{
			ID:                r.ID.String(),
			SlotName:          r.SlotName,
			Description:       desc,
			FlatRate:          int(r.FlatRate),
			MaxDisplayPerPage: int(r.MaxDisplayPerPage),
		})
	}
	return result, nil
}

// CreateCampaign creates a self-serve ad campaign with upfront payment.
// Steps: verify org, verify member, check min days, calculate cost,
// create campaign, deduct credits, enqueue notification.
func (s *Service) CreateCampaign(ctx context.Context, tenantID, orgID, userID pgtype.UUID, input CreateCampaignInput) (gen.AdCampaign, error) {
	enabled, err := s.settings.GetSettingBool(ctx, tenantID, "ads.self_serve_enabled")
	if err == nil && !enabled {
		return gen.AdCampaign{}, errors.New("adsselfserve: self-serve ads are disabled")
	}

	minDays := s.settings.GetIntOr(ctx, tenantID, "ads.min_campaign_days", 3)
	if input.DurationDays < minDays {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: campaign duration must be at least %d days", minDays)
	}

	verified, err := s.store.IsOrgVerified(ctx, tenantID, orgID)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: check org verification: %w", err)
	}
	if !verified {
		return gen.AdCampaign{}, errors.New("adsselfserve: organization_not_verified")
	}

	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: check org membership: %w", err)
	}
	if !isMember {
		return gen.AdCampaign{}, errors.New("adsselfserve: user is not a member of this organization")
	}

	slotTypeID := pgtype.UUID{}
	if err := slotTypeID.Scan(input.SlotTypeID); err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: invalid slot_type_id: %w", err)
	}

	// Get the slot type's flat_rate.
	slots, err := s.store.ListAdSlotTypes(ctx, tenantID)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: list slot types: %w", err)
	}
	var flatRate int
	for _, sl := range slots {
		if sl.ID.String() == input.SlotTypeID {
			flatRate = int(sl.FlatRate)
			break
		}
	}
	if flatRate == 0 {
		return gen.AdCampaign{}, errors.New("adsselfserve: slot type not found or has zero rate")
	}

	cost := CalculateCost(flatRate, input.DurationDays)

	var targetURL *string
	if input.TargetURL != "" {
		targetURL = &input.TargetURL
	}

	var startDate, endDate pgtype.Timestamptz
	if input.StartDate != nil {
		startDate = pgtype.Timestamptz{Time: *input.StartDate, Valid: true}
	}
	if input.EndDate != nil {
		endDate = pgtype.Timestamptz{Time: *input.EndDate, Valid: true}
	}

	var freqCap *int32
	if input.FrequencyCapPerUser != nil {
		v := int32(*input.FrequencyCapPerUser)
		freqCap = &v
	}

	var targetSpaceIDs []pgtype.UUID
	for _, idStr := range input.TargetSpaceIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err == nil {
			targetSpaceIDs = append(targetSpaceIDs, id)
		}
	}

	campaign, err := s.store.CreateCampaign(ctx, gen.CreateSelfServeAdCampaignParams{
		TenantID:           tenantID,
		OrganizationID:     orgID,
		Name:               input.Name,
		SlotTypeID:         slotTypeID,
		Content:            input.Content,
		TargetUrl:          targetURL,
		BudgetCredits:      int32(cost),
		FlatRate:           int32(flatRate),
		FrequencyCapPerUser: freqCap,
		StartDate:          startDate,
		EndDate:            endDate,
		TargetGeo:          input.TargetGeo,
		TargetSpaceIds:     targetSpaceIDs,
		CreatedBy:          userID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: create campaign: %w", err)
	}

	// Deduct credits from org wallet (fire-and-forget per Rule 8).
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id":      tenantID.String(),
			"organization_id": orgID.String(),
			"amount":         cost,
			"campaign_id":    campaign.ID.String(),
			"reason":         "ad_campaign_funding",
		})
		if err := s.enqueuer.Enqueue(TaskDeductCredits, payload); err != nil {
			slog.Warn("adsselfserve: enqueue deduct credits failed", "error", err)
		}

		// Enqueue notification.
		notifyPayload, _ := json.Marshal(map[string]string{
			"campaign_id":  campaign.ID.String(),
			"organization_id": orgID.String(),
			"status":       StatusPendingReview,
		})
		if err := s.enqueuer.Enqueue(TaskNotifyCampaignStatus, notifyPayload); err != nil {
			slog.Warn("adsselfserve: enqueue notify failed", "error", err)
		}
	}

	return campaign, nil
}

// GetCampaign returns a single campaign scoped to the org.
func (s *Service) GetCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.GetCampaign(ctx, tenantID, orgID, campaignID)
}

// ListCampaigns returns a paginated list of an org's campaigns.
func (s *Service) ListCampaigns(ctx context.Context, tenantID, orgID pgtype.UUID, status string, page, limit int) ([]gen.AdCampaign, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	rows, err := s.store.ListCampaigns(ctx, tenantID, orgID, status, int32(limit), int32(offset))
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountCampaigns(ctx, tenantID, orgID, status)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpdateCampaignCreative updates the creative content of a pending/paused campaign.
func (s *Service) UpdateCampaignCreative(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID, content json.RawMessage, targetURL string) (gen.AdCampaign, error) {
	var contentArg []byte = content
	var targetURLArg *string
	if targetURL != "" {
		targetURLArg = &targetURL
	}
	return s.store.UpdateCampaignCreative(ctx, gen.UpdateSelfServeAdCampaignCreativeParams{
		TenantID:       tenantID,
		ID:             campaignID,
		OrganizationID: orgID,
		Content:        contentArg,
		TargetUrl:      targetURLArg,
	})
}

// PauseCampaign transitions an active campaign to paused.
func (s *Service) PauseCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.PauseCampaign(ctx, tenantID, orgID, campaignID)
}

// ResumeCampaign transitions a paused campaign to active.
func (s *Service) ResumeCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.ResumeCampaign(ctx, tenantID, orgID, campaignID)
}

// GetCampaignStats returns impression/click/CTR/spend stats for a campaign.
func (s *Service) GetCampaignStats(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (CampaignStats, error) {
	row, err := s.store.GetCampaignStats(ctx, tenantID, orgID, campaignID)
	if err != nil {
		return CampaignStats{}, err
	}
	stats := CampaignStats{
		Impressions:   row.Impressions,
		Clicks:        row.Clicks,
		Spent:         row.Spent,
		BudgetCredits: int(row.BudgetCredits),
	}
	if row.Impressions > 0 {
		stats.CTR = float64(row.Clicks) / float64(row.Impressions) * 100
	}
	return stats, nil
}

// RefundCampaignCredits refunds unused credits on rejection or early end.
// Called by the ads:refund_campaign_credits worker task.
func (s *Service) RefundCampaignCredits(ctx context.Context, tenantID, campaignID pgtype.UUID) error {
	campaign, err := s.store.GetCampaignForRefund(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("adsselfserve: get campaign for refund: %w", err)
	}

	// Refund remaining budget_credits to org wallet (fire-and-forget).
	if s.enqueuer != nil && campaign.BudgetCredits > 0 {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id":       tenantID.String(),
			"organization_id": campaign.OrganizationID.String(),
			"amount":          int(campaign.BudgetCredits),
			"campaign_id":     campaign.ID.String(),
			"reason":          "ad_campaign_refund",
		})
		if err := s.enqueuer.Enqueue(TaskDeductCredits, payload); err != nil {
			slog.Warn("adsselfserve: enqueue refund failed", "error", err)
		}
	}

	return s.store.MarkCampaignRefunded(ctx, tenantID, campaignID)
}
