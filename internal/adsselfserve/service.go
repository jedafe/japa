// Package adsselfserve implements the Self-Serve Advertising surface
// (Section 61, ADR-079). Verified organisations can create, fund, and manage
// ad campaigns without admin involvement. Campaigns are paid upfront via
// credits, then enter the admin moderation queue via status = 'pending_review'
// for admin creative approval before going live.
// // Ref: japaconnect-engineering/sections/61-self-serve-ads.md
// //	japaconnect-engineering/adr/ADR-079-self-serve-ads.md
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
	TaskRefundCampaignCredits = "ads:refund_campaign_credits" // #nosec
	TaskNotifyCampaignStatus  = "ads:notify_campaign_status"
	TaskDeductCredits         = "wallet:deduct_credits" // #nosec
)

// Sentinel errors for typed failure modes. These are compared with errors.Is
// in the handler layer to produce the correct HTTP status and error code.
// // Ref: japaconnect-engineering/sections/61-self-serve-ads.md.
var (
	ErrFeatureDisabled         = errors.New("adsselfserve: feature disabled")
	ErrOrganizationNotVerified = errors.New("adsselfserve: organization_not_verified")
	ErrNotOrgMember            = errors.New("adsselfserve: not_org_member")
	ErrSlotTypeNotFound        = errors.New("adsselfserve: slot_type_not_found")
	ErrDurationTooShort        = errors.New("adsselfserve: duration_too_short")
	ErrInsufficientBalance     = errors.New("adsselfserve: insufficient_balance")
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Notifier dispatches user notifications (§35). Injected from server.go to
// avoid an import cycle with the notifications package.
type Notifier interface {
	DispatchNotification(
		ctx context.Context,
		recipientID pgtype.UUID,
		nType, subjectTitle string,
		actorID pgtype.UUID,
		subjectType string,
		subjectID pgtype.UUID,
		subjectURL, actorName, actorAvatar string,
	) error
}

// BalanceChecker verifies that an organization has sufficient credits to
// fund a campaign before the campaign row is created. The actual deduction
// remains fire-and-forget via wallet:deduct_credits (Rule 8), but this
// synchronous check prevents unpaid campaigns from entering the review queue.
// Injected from server.go to avoid an import cycle with the wallet package.
type BalanceChecker interface {
	HasSufficientCredits(ctx context.Context, orgID pgtype.UUID, amount int) (bool, error)
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
	store          *Store
	settings       *config.Settings
	enqueuer       TaskEnqueuer
	notifier       Notifier
	balanceChecker BalanceChecker
}

// NewService creates a self-serve ads service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetNotifier injects the notification dispatcher for campaign status alerts.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetBalanceChecker injects the wallet balance checker used to verify
// sufficient credits before campaign creation (§61 Payment & Escrow).
func (s *Service) SetBalanceChecker(bc BalanceChecker) { s.balanceChecker = bc }

// CalculateCost computes the total cost in credits for a campaign.
// cost = flat_rate × duration_days.
func CalculateCost(flatRate, durationDays int) int {
	return flatRate * durationDays
}

// ListSlots returns available ad slot types with flat_rate for self-serve.
func (s *Service) ListSlots(ctx context.Context) ([]SlotInfo, error) {
	rows, err := s.store.ListAdSlotTypes(ctx)
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
// create campaign, deduct credits, enqueue notification, enter review queue.
// // Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Review Gate)
// //	japaconnect-engineering/adr/ADR-079-self-serve-ads.md §3
func (s *Service) CreateCampaign(ctx context.Context, orgID, userID pgtype.UUID, input CreateCampaignInput) (gen.AdCampaign, error) {
	enabled, err := s.settings.GetSettingBool(ctx, "ads.self_serve_enabled")
	if err == nil && !enabled {
		return gen.AdCampaign{}, ErrFeatureDisabled
	}

	minDays := s.settings.GetIntOr(ctx, "ads.min_campaign_days", 3)
	if input.DurationDays < minDays {
		return gen.AdCampaign{}, fmt.Errorf("%w: must be at least %d days", ErrDurationTooShort, minDays)
	}

	verified, err := s.store.IsOrgVerified(ctx, orgID)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: check org verification: %w", err)
	}

	if !verified {
		return gen.AdCampaign{}, ErrOrganizationNotVerified
	}

	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: check org membership: %w", err)
	}

	if !isMember {
		return gen.AdCampaign{}, ErrNotOrgMember
	}

	slotTypeID := pgtype.UUID{}
	if err := slotTypeID.Scan(input.SlotTypeID); err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: invalid slot_type_id: %w", err)
	}

	// Get the slot type's flat_rate.
	slots, err := s.store.ListAdSlotTypes(ctx)
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
		return gen.AdCampaign{}, ErrSlotTypeNotFound
	}

	cost := CalculateCost(flatRate, input.DurationDays)

	// Verify the org has sufficient credits BEFORE creating the campaign.
	// The actual deduction remains fire-and-forget via wallet:deduct_credits
	// (Rule 8), but this synchronous check prevents unpaid campaigns from
	// entering the review queue. If the balance checker is not wired (nil),
	// the check is skipped — backward-compatible with existing deployments.
	// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Payment & Escrow)
	if s.balanceChecker != nil {
		sufficient, err := s.balanceChecker.HasSufficientCredits(ctx, orgID, cost)
		if err != nil {
			return gen.AdCampaign{}, fmt.Errorf("adsselfserve: check wallet balance: %w", err)
		}

		if !sufficient {
			return gen.AdCampaign{}, fmt.Errorf("%w: need %d credits for this campaign", ErrInsufficientBalance, cost)
		}
	}

	var targetURL *string
	if input.TargetURL != "" {
		targetURL = &input.TargetURL
	}

	// Default the campaign window to now + duration_days. If the caller
	// provides start/end dates we honour them, but we always ensure an end
	// date is set so the campaign terminates correctly.
	// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Payment & Escrow)
	startTime := time.Now()
	if input.StartDate != nil {
		startTime = *input.StartDate
	}

	endTime := startTime.AddDate(0, 0, input.DurationDays)
	if input.EndDate != nil {
		endTime = *input.EndDate
	}

	startDate := pgtype.Timestamptz{Time: startTime, Valid: true}
	endDate := pgtype.Timestamptz{Time: endTime, Valid: true}

	var freqCap *int32

	if input.FrequencyCapPerUser != nil {
		v := int32(*input.FrequencyCapPerUser) // #nosec
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
		OrganizationID:      orgID,
		Name:                input.Name,
		SlotTypeID:          slotTypeID,
		Content:             input.Content,
		TargetUrl:           targetURL,
		BudgetCredits:       int32(cost),     // #nosec
		FlatRate:            int32(flatRate), // #nosec
		FrequencyCapPerUser: freqCap,
		StartDate:           startDate,
		EndDate:             endDate,
		TargetGeo:           input.TargetGeo,
		TargetSpaceIds:      targetSpaceIDs,
		CreatedBy:           userID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: create campaign: %w", err)
	}

	// The campaign is created with status = 'pending_review' (set explicitly
	// in the CreateSelfServeAdCampaign SQL). This status IS the review-queue
	// entry — the admin moderation queue surfaces it via
	// GET /api/v1/admin/moderation-queue?source=ad_campaign. The
	// content_reviews table (§22) cannot hold ad campaigns because it has a
	// hard FK to contents(id); ad campaigns live in ad_campaigns and are
	// reviewed through the status workflow instead.
	// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Review Gate)

	// Deduct credits from org wallet (fire-and-forget per Rule 8).
	if s.enqueuer != nil {
		payload, _ := json.Marshal(map[string]interface{}{

			"organization_id": orgID.String(),
			"amount":          cost,
			"campaign_id":     campaign.ID.String(),
			"reason":          "ad_campaign_funding",
		})
		if err := s.enqueuer.Enqueue(TaskDeductCredits, payload); err != nil {
			slog.Warn("adsselfserve: enqueue deduct credits failed", "error", err)
		}

		// Enqueue notification.
		notifyPayload, _ := json.Marshal(map[string]string{
			"campaign_id":     campaign.ID.String(),
			"organization_id": orgID.String(),
			"status":          StatusPendingReview,
		})
		if err := s.enqueuer.Enqueue(TaskNotifyCampaignStatus, notifyPayload); err != nil {
			slog.Warn("adsselfserve: enqueue notify failed", "error", err)
		}
	}

	return campaign, nil
}

// GetCampaign returns a single campaign scoped to the org.
func (s *Service) GetCampaign(ctx context.Context, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.GetCampaign(ctx, orgID, campaignID)
}

// ListCampaigns returns a paginated list of an org's campaigns.
func (s *Service) ListCampaigns(ctx context.Context, orgID pgtype.UUID, status string, page, limit int) ([]gen.AdCampaign, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.store.ListCampaigns(ctx, orgID, status, int32(limit), int32(offset)) // #nosec
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountCampaigns(ctx, orgID, status)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// UpdateCampaignCreative updates the creative content of a pending/paused campaign.
func (s *Service) UpdateCampaignCreative(ctx context.Context, orgID, campaignID pgtype.UUID, content json.RawMessage, targetURL string) (gen.AdCampaign, error) {
	var contentArg []byte = content

	var targetURLArg *string
	if targetURL != "" {
		targetURLArg = &targetURL
	}

	return s.store.UpdateCampaignCreative(ctx, gen.UpdateSelfServeAdCampaignCreativeParams{

		ID:             campaignID,
		OrganizationID: orgID,
		Content:        contentArg,
		TargetUrl:      targetURLArg,
	})
}

// PauseCampaign transitions an active campaign to paused.
func (s *Service) PauseCampaign(ctx context.Context, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.PauseCampaign(ctx, orgID, campaignID)
}

// ResumeCampaign transitions a paused campaign to active.
func (s *Service) ResumeCampaign(ctx context.Context, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	return s.store.ResumeCampaign(ctx, orgID, campaignID)
}

// GetCampaignStats returns impression/click/CTR/spend stats for a campaign.
func (s *Service) GetCampaignStats(ctx context.Context, orgID, campaignID pgtype.UUID) (CampaignStats, error) {
	row, err := s.store.GetCampaignStats(ctx, orgID, campaignID)
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
// Honours the ads.credit_refund_on_reject system setting (§61, default true).
func (s *Service) RefundCampaignCredits(ctx context.Context, campaignID pgtype.UUID) error {
	campaign, err := s.store.GetCampaignForRefund(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("adsselfserve: get campaign for refund: %w", err)
	}

	// Check the ads.credit_refund_on_reject setting before refunding.
	// When false, the campaign is still marked as refunded (budget zeroed,
	// status = rejected) but credits are NOT returned to the org wallet.
	refundEnabled := true
	if v, err := s.settings.GetSettingBool(ctx, "ads.credit_refund_on_reject"); err == nil {
		refundEnabled = v
	}

	if refundEnabled && s.enqueuer != nil && campaign.BudgetCredits > 0 {
		payload, _ := json.Marshal(map[string]interface{}{

			"organization_id": campaign.OrganizationID.String(),
			"amount":          int(campaign.BudgetCredits),
			"campaign_id":     campaign.ID.String(),
			"reason":          "ad_campaign_refund",
		})
		if err := s.enqueuer.Enqueue(TaskDeductCredits, payload); err != nil {
			slog.Warn("adsselfserve: enqueue refund failed", "error", err)
		}
	} else if !refundEnabled {
		slog.Info("adsselfserve: credit refund skipped (ads.credit_refund_on_reject=false)", "campaign_id", campaignID.String())
	}

	return s.store.MarkCampaignRefunded(ctx, campaignID)
}

// NotifyCampaignStatus sends a notification to the campaign creator when the
// review decision is made (approved or rejected). Called by the
// ads:notify_campaign_status worker task.
// // Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Asynq Tasks)
// //	japaconnect-engineering/adr/ADR-079-self-serve-ads.md §7
func (s *Service) NotifyCampaignStatus(ctx context.Context, campaignID pgtype.UUID) error {
	campaign, err := s.store.GetCampaignForRefund(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("adsselfserve: load campaign for notify: %w", err)
	}

	if s.notifier == nil {
		slog.Warn("adsselfserve: notifier not wired, skipping campaign status notification", "campaign_id", campaignID.String())
		return nil
	}

	if !campaign.CreatedBy.Valid {
		slog.Warn("adsselfserve: campaign has no created_by, cannot notify", "campaign_id", campaignID.String())
		return nil
	}

	var title, actionURL, nType string

	switch campaign.Status {
	case StatusActive:
		nType = "ad_campaign_approved"
		title = "Your ad campaign \"" + campaign.Name + "\" has been approved and is now active"
		actionURL = "/org/" + campaign.OrganizationID.String() + "/ads/" + campaign.ID.String()
	case StatusRejected:
		nType = "ad_campaign_rejected"
		title = "Your ad campaign \"" + campaign.Name + "\" was rejected. Credits have been refunded."
		actionURL = "/org/" + campaign.OrganizationID.String() + "/ads/" + campaign.ID.String()
	default:
		// Status hasn't changed to a review decision yet — nothing to notify.
		slog.Debug("adsselfserve: campaign status is not a review decision, skipping notify", "campaign_id", campaignID.String(), "status", campaign.Status)
		return nil
	}

	// System-sourced notification (no human actor).
	if err := s.notifier.DispatchNotification(
		ctx, campaign.CreatedBy,
		nType, title,
		pgtype.UUID{}, // no actor — system notification
		"ad_campaign", campaign.ID,
		actionURL, "", "",
	); err != nil {
		return fmt.Errorf("adsselfserve: dispatch notification: %w", err)
	}

	return nil
}
