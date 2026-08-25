// Package campaign — store.go wraps the sqlc-generated queries for content
// campaigns and campaign items.
//
// The Store follows the read/write pool split pattern and exposes high-level
// methods that wrap transactions internally so the service stays focused on
// business logic.
package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated queries for campaign persistence.
// All reads go through the read pool; all writes go through the write pool.
// Transactional methods use db.WithTx/WithTxResult internally.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a campaign store bound to the provided database pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}

// DB exposes the underlying pool for callers that need direct tx control.
func (s *Store) DB() *db.DB { return s.db }

// CreateCampaign inserts a new content_campaigns row in the 'planning' status.
func (s *Store) CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in CreateCampaignInput) (gen.ContentCampaign, error) {
	var contentType *string
	if in.ContentType != "" {
		contentType = &in.ContentType
	}
	var targetSpaceID pgtype.UUID
	if in.TargetSpaceID != "" {
		if err := targetSpaceID.Scan(in.TargetSpaceID); err != nil {
			return gen.ContentCampaign{}, fmt.Errorf("campaign: invalid target_space_id: %w", err)
		}
	}

	var personaPool []pgtype.UUID
	if len(in.PersonaPool) > 0 {
		personaPool = make([]pgtype.UUID, 0, len(in.PersonaPool))
		for _, id := range in.PersonaPool {
			var pid pgtype.UUID
			if err := pid.Scan(id); err != nil {
				return gen.ContentCampaign{}, fmt.Errorf("campaign: invalid persona_pool id %q: %w", id, err)
			}
			personaPool = append(personaPool, pid)
		}
	}

	name := in.Name
	if name == "" {
		name = campaignNameFromPrompt(in.Prompt)
	}

	return s.write.CreateContentCampaign(ctx, gen.CreateContentCampaignParams{
		TenantID:              tenantID,
		Name:                  name,
		Prompt:                in.Prompt,
		Status:                CampaignStatusPlanning,
		ContentType:           contentType,
		RequestedCount:        int32(in.Count),
		PersonaPool:           personaPool,
		AutoProvisionPersonas: in.AutoProvisionPersonas,
		AutoAssign:            in.AutoAssign,
		TargetSpaceID:         targetSpaceID,
		TargetTags:            in.TargetTags,
		CreatedBy:             userID,
	})
}

// CreateCampaignItemsAndFinalize inserts the planned campaign items and
// transitions the campaign to the 'planned' status in a single transaction.
func (s *Store) CreateCampaignItemsAndFinalize(ctx context.Context, campaign gen.ContentCampaign, items []CampaignItemPlan, plan CampaignPlanResult) ([]gen.ContentCampaignItem, error) {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("campaign: marshal plan_json: %w", err)
	}

	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) ([]gen.ContentCampaignItem, error) {
		created := make([]gen.ContentCampaignItem, 0, len(items))
		for i, it := range items {
			params, perr := campaignItemParams(campaign, it, i)
			if perr != nil {
				return nil, perr
			}
			row, perr := q.CreateContentCampaignItem(ctx, params)
			if perr != nil {
				return nil, fmt.Errorf("campaign: create campaign item: %w", perr)
			}
			created = append(created, row)
		}

		_, perr := q.UpdateContentCampaignStatus(ctx, gen.UpdateContentCampaignStatusParams{
			ID:       campaign.ID,
			TenantID: campaign.TenantID,
			Status:   CampaignStatusPlanned,
			PlanJson: planJSON,
		})
		if perr != nil {
			return nil, fmt.Errorf("campaign: finalize campaign status: %w", perr)
		}
		return created, nil
	})
}

// UpdateCampaignStatus updates a campaign's status and optional plan_json.
func (s *Store) UpdateCampaignStatus(ctx context.Context, id, tenantID pgtype.UUID, status string, planJSON []byte) (gen.ContentCampaign, error) {
	return s.write.UpdateContentCampaignStatus(ctx, gen.UpdateContentCampaignStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
		PlanJson: planJSON,
	})
}

// GetCampaign returns a single campaign by ID.
func (s *Store) GetCampaign(ctx context.Context, tenantID, id pgtype.UUID) (gen.ContentCampaign, error) {
	return s.read.GetContentCampaign(ctx, gen.GetContentCampaignParams{ID: id, TenantID: tenantID})
}

// ListCampaigns returns a paginated list of campaigns for a tenant.
func (s *Store) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, f CampaignListFilter) ([]gen.ContentCampaign, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	items, err := s.read.ListContentCampaigns(ctx, gen.ListContentCampaignsParams{
		TenantID:   tenantID,
		Status:     f.Status,
		PageLimit:  f.Limit,
		PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.read.CountContentCampaigns(ctx, gen.CountContentCampaignsParams{
		TenantID: tenantID,
		Status:   f.Status,
	})
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// ListCampaignItemsByCampaign returns all items for a campaign.
func (s *Store) ListCampaignItemsByCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) ([]gen.ContentCampaignItem, error) {
	return s.read.ListContentCampaignItemsByCampaign(ctx, gen.ListContentCampaignItemsByCampaignParams{
		CampaignID: campaignID,
		TenantID:   tenantID,
	})
}

// GetCampaignItem returns a single campaign item by ID.
func (s *Store) GetCampaignItem(ctx context.Context, tenantID, id pgtype.UUID) (gen.ContentCampaignItem, error) {
	return s.read.GetContentCampaignItem(ctx, gen.GetContentCampaignItemParams{ID: id, TenantID: tenantID})
}

// GetCampaignItemByStudioJob returns the campaign item linked to a Studio job.
func (s *Store) GetCampaignItemByStudioJob(ctx context.Context, tenantID, studioJobID pgtype.UUID) (gen.ContentCampaignItem, error) {
	return s.read.GetContentCampaignItemByStudioJob(ctx, gen.GetContentCampaignItemByStudioJobParams{StudioJobID: studioJobID, TenantID: tenantID})
}

// GetCampaignItemByReviewID returns the campaign item linked to a HITL review.
func (s *Store) GetCampaignItemByReviewID(ctx context.Context, tenantID, reviewID pgtype.UUID) (gen.ContentCampaignItem, error) {
	return s.read.GetContentCampaignItemByReviewID(ctx, gen.GetContentCampaignItemByReviewIDParams{ReviewID: reviewID, TenantID: tenantID})
}

// UpdateCampaignItem updates an item's status and optional related IDs.
func (s *Store) UpdateCampaignItem(ctx context.Context, params gen.UpdateContentCampaignItemParams) (gen.ContentCampaignItem, error) {
	return s.write.UpdateContentCampaignItem(ctx, params)
}

// ListActivePersonas returns all active personas for a tenant with the
// information needed by the content planner.
func (s *Store) ListActivePersonas(ctx context.Context, tenantID pgtype.UUID) ([]gen.ListContentCampaignActivePersonasRow, error) {
	return s.read.ListContentCampaignActivePersonas(ctx, tenantID)
}

// ListActivePersonasByIDs returns the active personas from a provided pool.
func (s *Store) ListActivePersonasByIDs(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID) ([]gen.ListContentCampaignActivePersonasByIDsRow, error) {
	return s.read.ListContentCampaignActivePersonasByIDs(ctx, gen.ListContentCampaignActivePersonasByIDsParams{
		TenantID: tenantID,
		Ids:      ids,
	})
}

// SpaceExists checks whether a space exists and belongs to the given tenant.
func (s *Store) SpaceExists(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	_, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func campaignItemParams(campaign gen.ContentCampaign, it CampaignItemPlan, sortOrder int) (gen.CreateContentCampaignItemParams, error) {
	var personaID pgtype.UUID
	if err := personaID.Scan(it.PersonaID); err != nil {
		return gen.CreateContentCampaignItemParams{}, fmt.Errorf("campaign: invalid item persona_id %q: %w", it.PersonaID, err)
	}

	var targetSpaceID pgtype.UUID
	targetSpaceID = campaign.TargetSpaceID
	if it.TargetSpaceID != "" {
		if err := targetSpaceID.Scan(it.TargetSpaceID); err != nil {
			return gen.CreateContentCampaignItemParams{}, fmt.Errorf("campaign: invalid item target_space_id %q: %w", it.TargetSpaceID, err)
		}
	}

	targetTags := it.TargetTags
	if len(targetTags) == 0 && len(campaign.TargetTags) > 0 {
		targetTags = campaign.TargetTags
	}

	var title *string
	if it.Title != "" {
		title = &it.Title
	}

	return gen.CreateContentCampaignItemParams{
		TenantID:      campaign.TenantID,
		CampaignID:    campaign.ID,
		SortOrder:     int32(sortOrder),
		PersonaID:     personaID,
		ContentType:   it.ContentType,
		TopicPrompt:   it.TopicPrompt,
		Title:         title,
		TargetSpaceID: targetSpaceID,
		TargetTags:    targetTags,
	}, nil
}

func campaignNameFromPrompt(prompt string) string {
	runes := []rune(prompt)
	if len(runes) <= 80 {
		return prompt
	}
	return string(runes[:80])
}
