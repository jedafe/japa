// Package adsselfserve implements the Self-Serve Advertising surface
// (Section 61, ADR-079). Verified organisations can create, fund, and manage
// ad campaigns without admin involvement. Campaigns are paid upfront via
// credits, then enter the admin moderation queue via status = 'pending_review'
// for admin creative approval before going live.
//
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md
//      japaconnect-engineering/adr/ADR-079-self-serve-ads.md
package adsselfserve

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for the self-serve ads surface.
// It is the data-access layer — no business logic here.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a self-serve ads store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Reader),
		pool:    database,
	}
}

// ListAdSlotTypes returns the active ad slot types (with flat_rate) available
// to self-serve advertisers for the given tenant.
func (s *Store) ListAdSlotTypes(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdSlotType, error) {
	rows, err := s.queries.ListAdSlotTypesForSelfServe(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("adsselfserve: list slot types: %w", err)
	}
	return rows, nil
}

// IsOrgVerified reports whether the organisation is verified (eligibility gate).
func (s *Store) IsOrgVerified(ctx context.Context, tenantID, orgID pgtype.UUID) (bool, error) {
	verified, err := s.queries.IsOrgVerified(ctx, gen.IsOrgVerifiedParams{
		TenantID: tenantID,
		ID:       orgID,
	})
	if err != nil {
		return false, fmt.Errorf("adsselfserve: is org verified: %w", err)
	}
	return verified, nil
}

// IsOrgMember reports whether the user is a member of the organisation.
func (s *Store) IsOrgMember(ctx context.Context, orgID, userID pgtype.UUID) (bool, error) {
	isMember, err := s.queries.IsOrgMember(ctx, gen.IsOrgMemberParams{
		OrgID:  orgID,
		UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("adsselfserve: is org member: %w", err)
	}
	return isMember, nil
}

// CreateCampaign inserts a new self-serve ad campaign with status
// 'pending_review'. The caller is responsible for eligibility checks and
// cost calculation before calling this.
func (s *Store) CreateCampaign(ctx context.Context, arg gen.CreateSelfServeAdCampaignParams) (gen.AdCampaign, error) {
	campaign, err := s.queries.CreateSelfServeAdCampaign(ctx, arg)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: create campaign: %w", err)
	}
	return campaign, nil
}

// GetCampaign returns a single campaign scoped to the organisation.
func (s *Store) GetCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	campaign, err := s.queries.GetSelfServeAdCampaign(ctx, gen.GetSelfServeAdCampaignParams{
		TenantID:       tenantID,
		ID:             campaignID,
		OrganizationID: orgID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: get campaign: %w", err)
	}
	return campaign, nil
}

// ListCampaigns returns a paginated list of an organisation's campaigns,
// optionally filtered by status. Pass an empty status string to return all.
func (s *Store) ListCampaigns(ctx context.Context, tenantID, orgID pgtype.UUID, status string, limit, offset int32) ([]gen.AdCampaign, error) {
	rows, err := s.queries.ListOrgAdCampaigns(ctx, gen.ListOrgAdCampaignsParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
		Status:         status,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("adsselfserve: list campaigns: %w", err)
	}
	return rows, nil
}

// CountCampaigns returns the total number of campaigns for the organisation,
// optionally filtered by status.
func (s *Store) CountCampaigns(ctx context.Context, tenantID, orgID pgtype.UUID, status string) (int64, error) {
	count, err := s.queries.CountOrgAdCampaigns(ctx, gen.CountOrgAdCampaignsParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
		Status:         status,
	})
	if err != nil {
		return 0, fmt.Errorf("adsselfserve: count campaigns: %w", err)
	}
	return count, nil
}

// UpdateCampaignCreative updates the content and target_url of a campaign
// that is in 'pending_review' or 'paused' status. Pass nil for content or
// target_url to leave that field unchanged.
func (s *Store) UpdateCampaignCreative(ctx context.Context, arg gen.UpdateSelfServeAdCampaignCreativeParams) (gen.AdCampaign, error) {
	campaign, err := s.queries.UpdateSelfServeAdCampaignCreative(ctx, arg)
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: update campaign creative: %w", err)
	}
	return campaign, nil
}

// PauseCampaign transitions an active campaign to 'paused'.
func (s *Store) PauseCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	campaign, err := s.queries.PauseAdCampaign(ctx, gen.PauseAdCampaignParams{
		TenantID:       tenantID,
		ID:             campaignID,
		OrganizationID: orgID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: pause campaign: %w", err)
	}
	return campaign, nil
}

// ResumeCampaign transitions a paused campaign to 'active'.
func (s *Store) ResumeCampaign(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	campaign, err := s.queries.ResumeAdCampaign(ctx, gen.ResumeAdCampaignParams{
		TenantID:       tenantID,
		ID:             campaignID,
		OrganizationID: orgID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: resume campaign: %w", err)
	}
	return campaign, nil
}

// GetCampaignStats returns impression/click/spend aggregates for a campaign
// scoped to the organisation.
func (s *Store) GetCampaignStats(ctx context.Context, tenantID, orgID, campaignID pgtype.UUID) (gen.GetAdCampaignStatsRow, error) {
	row, err := s.queries.GetAdCampaignStats(ctx, gen.GetAdCampaignStatsParams{
		TenantID:       tenantID,
		ID:             campaignID,
		OrganizationID: orgID,
	})
	if err != nil {
		return gen.GetAdCampaignStatsRow{}, fmt.Errorf("adsselfserve: get campaign stats: %w", err)
	}
	return row, nil
}

// GetCampaignForRefund loads a self-serve campaign (one with a non-null
// organization_id) for the refund task. The organisation_id is not required
// in the filter because the refund task is invoked by the admin/system, not
// the org.
func (s *Store) GetCampaignForRefund(ctx context.Context, tenantID, campaignID pgtype.UUID) (gen.AdCampaign, error) {
	campaign, err := s.queries.GetCampaignForRefund(ctx, gen.GetCampaignForRefundParams{
		TenantID: tenantID,
		ID:       campaignID,
	})
	if err != nil {
		return gen.AdCampaign{}, fmt.Errorf("adsselfserve: get campaign for refund: %w", err)
	}
	return campaign, nil
}

// MarkCampaignRefunded zeroes the budget_credits and sets status to 'rejected'.
// Called after the refund task has returned the unused credits to the org wallet.
func (s *Store) MarkCampaignRefunded(ctx context.Context, tenantID, campaignID pgtype.UUID) error {
	if err := s.queries.MarkCampaignRefunded(ctx, gen.MarkCampaignRefundedParams{
		TenantID: tenantID,
		ID:       campaignID,
	}); err != nil {
		return fmt.Errorf("adsselfserve: mark campaign refunded: %w", err)
	}
	return nil
}
