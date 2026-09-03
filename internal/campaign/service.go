// Package campaign — service.go implements the campaign business logic.
//
// Service handles campaign creation, AI planning, persona auto-provisioning,
// Studio batch dispatch, and status transitions. It enforces the hard persona
// uniqueness rule and validates all AI plan output before persisting it.
package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/personas"
	"github.com/jedafe/japa/internal/studio"
)

// ErrPersonaPoolTooSmall is returned when the available active persona pool is
// smaller than the requested campaign count and auto-provision is disabled or
// the caller lacks permission.
var ErrPersonaPoolTooSmall = errors.New("persona_pool_too_small")

// storer is the storage interface required by the service. The concrete
// *Store satisfies it; tests can provide a fake implementation.
type storer interface {
	CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in CreateCampaignInput) (gen.ContentCampaign, error)
	CreateCampaignItemsAndFinalize(ctx context.Context, campaign gen.ContentCampaign, items []CampaignItemPlan, plan CampaignPlanResult) ([]gen.ContentCampaignItem, error)
	UpdateCampaignStatus(ctx context.Context, id, tenantID pgtype.UUID, status string, planJSON []byte) (gen.ContentCampaign, error)
	GetCampaign(ctx context.Context, tenantID, id pgtype.UUID) (gen.ContentCampaign, error)
	ListCampaigns(ctx context.Context, tenantID pgtype.UUID, f CampaignListFilter) ([]gen.ContentCampaign, int64, error)
	ListCampaignItemsByCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) ([]gen.ContentCampaignItem, error)
	GetCampaignItem(ctx context.Context, tenantID, id pgtype.UUID) (gen.ContentCampaignItem, error)
	GetCampaignItemByStudioJob(ctx context.Context, tenantID, studioJobID pgtype.UUID) (gen.ContentCampaignItem, error)
	GetCampaignItemByReviewID(ctx context.Context, tenantID, reviewID pgtype.UUID) (gen.ContentCampaignItem, error)
	UpdateCampaignItem(ctx context.Context, params gen.UpdateContentCampaignItemParams) (gen.ContentCampaignItem, error)
	ListActivePersonas(ctx context.Context, tenantID pgtype.UUID) ([]gen.ListContentCampaignActivePersonasRow, error)
	ListActivePersonasByIDs(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID) ([]gen.ListContentCampaignActivePersonasByIDsRow, error)
	SpaceExists(ctx context.Context, tenantID, id pgtype.UUID) (bool, error)
}

// personaProvisioner is the subset of the personas service the campaign layer
// uses to auto-provision missing personas.
type personaProvisioner interface {
	ProvisionPersonas(ctx context.Context, tenantID, adminUserID pgtype.UUID, inputs []personas.PersonaInput) ([]personas.ProvisionResult, error)
}

// studioService is the subset of the studio service the campaign layer uses to
// create batches, retry jobs, and cancel jobs.
type studioService interface {
	CreateBatch(ctx context.Context, tenantID, createdBy pgtype.UUID, in studio.CreateBatchInput) (studio.Batch, []studio.Job, error)
	RetryJob(ctx context.Context, tenantID, id pgtype.UUID) error
	CancelJob(ctx context.Context, tenantID, id pgtype.UUID) error
	ListContentTypes(ctx context.Context, tenantID pgtype.UUID) studio.ContentTypeResponse
}

// permissionChecker is the RBAC resolver subset used to gate auto-provision.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// settingsReader is the system settings subset the service needs.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
}

// Service implements the content campaign business logic.
type Service struct {
	store    storer
	planner  Planner
	personas personaProvisioner
	studio   studioService
	perms    permissionChecker
	settings settingsReader
}

// NewService creates a campaign service backed by the given dependencies.
func NewService(store storer, planner Planner, personas personaProvisioner, studio studioService, perms permissionChecker, settings settingsReader) *Service {
	return &Service{
		store:    store,
		planner:  planner,
		personas: personas,
		studio:   studio,
		perms:    perms,
		settings: settings,
	}
}

// CreateCampaign validates the input, creates a campaign row, plans it, and
// optionally runs it immediately.
func (s *Service) CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in CreateCampaignInput) (Campaign, []CampaignItem, error) {
	if !s.isEnabled(ctx, tenantID) {
		return Campaign{}, nil, fmt.Errorf("campaign: feature is disabled")
	}

	if err := s.validateCount(ctx, tenantID, in.Count); err != nil {
		return Campaign{}, nil, err
	}

	if in.TargetSpaceID != "" {
		if err := s.validateTargetSpace(ctx, tenantID, in.TargetSpaceID); err != nil {
			return Campaign{}, nil, err
		}
	}

	campaign, err := s.store.CreateCampaign(ctx, tenantID, userID, in)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: create campaign: %w", err)
	}

	planned, items, err := s.PlanCampaign(ctx, tenantID, userID, campaign.ID)
	if err != nil {
		return planned, nil, err
	}

	if in.RunImmediately {
		if s.perms == nil || !s.perms.HasPermission(ctx, userID, tenantID, "campaign.run") {
			return planned, items, fmt.Errorf("campaign: %w: run immediately requires campaign.run permission", errpkg.ErrForbidden)
		}
		if _, _, err := s.RunCampaign(ctx, tenantID, userID, campaign.ID); err != nil {
			// Run failure is logged but the campaign is still planned. The caller
			// can call /run again.
			slog.Warn("campaign: run immediately failed after plan", "campaign_id", campaign.ID, "error", err)
		}
		// Reload so the returned campaign and items reflect the latest state.
		reloaded, reloadedItems, reloadErr := s.GetCampaign(ctx, tenantID, campaign.ID)
		if reloadErr != nil {
			slog.Warn("campaign: reload after run failed", "campaign_id", campaign.ID, "error", reloadErr)
		} else {
			planned, items = reloaded, reloadedItems
		}
	}

	return planned, items, nil
}

// PlanCampaign validates the persona pool, provisions missing personas if
// allowed, generates a content plan, and persists the items with the campaign
// in the 'planned' status.
func (s *Service) PlanCampaign(ctx context.Context, tenantID, userID, campaignID pgtype.UUID) (Campaign, []CampaignItem, error) {
	campaign, err := s.store.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: get campaign: %w", err)
	}

	if !s.isEnabled(ctx, tenantID) {
		return Campaign{}, nil, fmt.Errorf("campaign: feature is disabled")
	}

	// Build the target persona pool.
	pool, err := s.buildPersonaPool(ctx, tenantID, campaign, userID)
	if err != nil {
		if errors.Is(err, ErrPersonaPoolTooSmall) || errors.Is(err, errpkg.ErrForbidden) {
			_ = s.failCampaign(ctx, campaign, err.Error())
		}
		return Campaign{}, nil, err
	}

	// Validate the requested count again against the final pool size.
	if err := s.validateCount(ctx, tenantID, int(campaign.RequestedCount)); err != nil {
		return Campaign{}, nil, err
	}

	allowedTypes := s.allowedContentTypes(ctx, tenantID, campaign.ContentType)

	provisionedCount := 0
	for _, p := range pool {
		if p.IsNew {
			provisionedCount++
		}
	}

	finalPool := make([]personaSummary, 0, len(pool))
	for _, p := range pool {
		finalPool = append(finalPool, personaSummary{ID: p.ID.String(), Name: p.Name, VoicePrompt: p.VoicePrompt})
	}

	targetSpace := ""
	if campaign.TargetSpaceID.Valid {
		targetSpace = campaign.TargetSpaceID.String()
	}
	targetTags := ""
	if len(campaign.TargetTags) > 0 {
		targetTags = encodeTags(campaign.TargetTags)
	}

	plan, err := s.planner.PlanContent(ctx, campaign.Prompt, allowedTypes, finalPool, int(campaign.RequestedCount), nil, targetSpace, targetTags)
	if err != nil {
		_ = s.failCampaign(ctx, campaign, err.Error())
		return Campaign{}, nil, fmt.Errorf("campaign: plan content failed: %w", err)
	}

	plan.NewPersonas = newPersonaSpecsFromPool(pool)

	if err := s.validatePlan(plan, campaign.RequestedCount, finalPool, allowedTypes); err != nil {
		_ = s.failCampaign(ctx, campaign, err.Error())
		return Campaign{}, nil, err
	}

	if err := s.validatePlanTargetSpaces(ctx, tenantID, campaign, plan.Items); err != nil {
		_ = s.failCampaign(ctx, campaign, err.Error())
		return Campaign{}, nil, err
	}

	items, err := s.store.CreateCampaignItemsAndFinalize(ctx, campaign, plan.Items, plan)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: finalize campaign: %w", err)
	}

	updated, err := s.store.GetCampaign(ctx, tenantID, campaign.ID)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: reload campaign: %w", err)
	}

	poolNames := make(map[string]string, len(pool))
	for _, p := range pool {
		poolNames[p.ID.String()] = p.Name
	}

	campaignDTO := campaignFromGen(updated)
	campaignDTO.ProvisionedPersonas = provisionedCount
	itemDTOs := make([]CampaignItem, 0, len(items))
	for _, it := range items {
		itemDTOs = append(itemDTOs, campaignItemFromGen(it, poolNames))
	}

	return campaignDTO, itemDTOs, nil
}

// RunCampaign converts planned campaign items into a Studio batch and updates
// the campaign status to 'generating'.
func (s *Service) RunCampaign(ctx context.Context, tenantID, userID, campaignID pgtype.UUID) (studio.Batch, []studio.Job, error) {
	campaign, err := s.store.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return studio.Batch{}, nil, fmt.Errorf("campaign: get campaign: %w", err)
	}
	if campaign.Status != CampaignStatusPlanned {
		return studio.Batch{}, nil, fmt.Errorf("campaign: %w: campaign must be planned to run (current: %s)", errpkg.ErrInvalidStatusTransition, campaign.Status)
	}

	items, err := s.store.ListCampaignItemsByCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return studio.Batch{}, nil, fmt.Errorf("campaign: list items: %w", err)
	}

	jobs := make([]studio.CreateJobInput, 0, len(items))
	for _, it := range items {
		topic := it.TopicPrompt
		if it.Title != nil && *it.Title != "" {
			topic = *it.Title + ": " + topic
		}
		jobs = append(jobs, studio.CreateJobInput{
			PersonaID:     it.PersonaID,
			ContentType:   it.ContentType,
			TopicPrompt:   topic,
			TargetSpaceID: it.TargetSpaceID,
			TargetTags:    it.TargetTags,
		})
	}

	var desc *string
	batch, createdJobs, err := s.studio.CreateBatch(ctx, tenantID, userID, studio.CreateBatchInput{
		Name:        campaign.Name,
		Description: desc,
		Jobs:        jobs,
	})
	if err != nil {
		return studio.Batch{}, nil, fmt.Errorf("campaign: create studio batch: %w", err)
	}

	for i, it := range items {
		if i >= len(createdJobs) {
			break
		}
		_, uerr := s.store.UpdateCampaignItem(ctx, gen.UpdateContentCampaignItemParams{
			ID:          it.ID,
			TenantID:    it.TenantID,
			Status:      ItemStatusQueued,
			StudioJobID: createdJobs[i].ID,
		})
		if uerr != nil {
			slog.Warn("campaign: update item studio_job_id failed", "item_id", it.ID, "error", uerr)
		}
	}

	if _, err := s.store.UpdateCampaignStatus(ctx, campaign.ID, campaign.TenantID, CampaignStatusGenerating, nil); err != nil {
		return batch, createdJobs, fmt.Errorf("campaign: update campaign status: %w", err)
	}

	return batch, createdJobs, nil
}

// ListCampaigns returns a paginated list of campaigns for a tenant.
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, f CampaignListFilter) ([]Campaign, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	rows, total, err := s.store.ListCampaigns(ctx, tenantID, f)
	if err != nil {
		return nil, 0, fmt.Errorf("campaign: list: %w", err)
	}

	items := make([]Campaign, 0, len(rows))
	for _, r := range rows {
		items = append(items, campaignFromGen(r))
	}
	return items, total, nil
}

// GetCampaign returns a campaign and all its items.
func (s *Service) GetCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) (Campaign, []CampaignItem, error) {
	campaign, err := s.store.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: get campaign: %w", err)
	}

	items, err := s.store.ListCampaignItemsByCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return Campaign{}, nil, fmt.Errorf("campaign: list items: %w", err)
	}

	// Best-effort persona name map from current active personas.
	active, err := s.store.ListActivePersonas(ctx, tenantID)
	personaNames := make(map[string]string)
	if err == nil {
		for _, p := range active {
			personaNames[p.ID.String()] = p.ProfileName
		}
	}

	itemDTOs := make([]CampaignItem, 0, len(items))
	for _, it := range items {
		itemDTOs = append(itemDTOs, campaignItemFromGen(it, personaNames))
	}

	return campaignFromGen(campaign), itemDTOs, nil
}

// RetryItem retries a failed campaign item by re-enqueueing its Studio job.
func (s *Service) RetryItem(ctx context.Context, tenantID, userID, itemID pgtype.UUID) error {
	item, err := s.store.GetCampaignItem(ctx, tenantID, itemID)
	if err != nil {
		return fmt.Errorf("campaign: get item: %w", err)
	}

	if item.Status != ItemStatusFailed {
		return fmt.Errorf("campaign: %w: only failed items can be retried (current: %s)", errpkg.ErrInvalidStatusTransition, item.Status)
	}

	if item.StudioJobID.Valid {
		if err := s.studio.RetryJob(ctx, tenantID, item.StudioJobID); err != nil {
			return fmt.Errorf("campaign: retry studio job: %w", err)
		}
	}

	_, err = s.store.UpdateCampaignItem(ctx, gen.UpdateContentCampaignItemParams{
		ID:           item.ID,
		TenantID:     item.TenantID,
		Status:       ItemStatusQueued,
		ErrorMessage: nil,
	})
	return err
}

// CancelCampaign cancels a campaign and fails all non-final items.
func (s *Service) CancelCampaign(ctx context.Context, tenantID, userID, campaignID pgtype.UUID) error {
	campaign, err := s.store.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("campaign: get campaign: %w", err)
	}

	if campaign.Status == CampaignStatusCancelled || campaign.Status == CampaignStatusCompleted || campaign.Status == CampaignStatusFailed {
		return fmt.Errorf("campaign: %w: cannot cancel campaign in status %s", errpkg.ErrInvalidStatusTransition, campaign.Status)
	}

	items, err := s.store.ListCampaignItemsByCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return fmt.Errorf("campaign: list items: %w", err)
	}

	finalStatuses := map[string]bool{ItemStatusPublished: true, ItemStatusApproved: true, ItemStatusRejected: true, ItemStatusFailed: true}
	for _, it := range items {
		if finalStatuses[it.Status] {
			continue
		}
		msg := "cancelled"
		if it.StudioJobID.Valid && it.Status == ItemStatusQueued {
			if cerr := s.studio.CancelJob(ctx, tenantID, it.StudioJobID); cerr != nil && !errors.Is(cerr, studio.ErrJobNotCancellable) {
				slog.Warn("campaign: cancel studio job failed", "job_id", it.StudioJobID, "error", cerr)
			}
		}
		_, uerr := s.store.UpdateCampaignItem(ctx, gen.UpdateContentCampaignItemParams{
			ID:           it.ID,
			TenantID:     it.TenantID,
			Status:       ItemStatusFailed,
			ErrorMessage: &msg,
		})
		if uerr != nil {
			slog.Warn("campaign: cancel item failed", "item_id", it.ID, "error", uerr)
		}
	}

	_, err = s.store.UpdateCampaignStatus(ctx, campaign.ID, campaign.TenantID, CampaignStatusCancelled, nil)
	return err
}

// SyncCampaignItemByStudioJob updates the campaign item linked to the given
// Studio job and, when every item has reached a terminal state, transitions the
// campaign to 'completed'. It is safe to call for jobs that do not belong to a
// campaign; in that case it returns nil without error.
func (s *Service) SyncCampaignItemByStudioJob(ctx context.Context, tenantID, studioJobID pgtype.UUID, status string, contentID, reviewID pgtype.UUID, errMsg *string) error {
	item, err := s.store.GetCampaignItemByStudioJob(ctx, tenantID, studioJobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("campaign: find item by studio job: %w", err)
	}

	_, err = s.store.UpdateCampaignItem(ctx, gen.UpdateContentCampaignItemParams{
		ID:           item.ID,
		TenantID:     item.TenantID,
		Status:       status,
		ContentID:    contentID,
		ReviewID:     reviewID,
		ErrorMessage: errMsg,
	})
	if err != nil {
		return fmt.Errorf("campaign: sync item by studio job: %w", err)
	}

	if err := s.maybeCompleteCampaign(ctx, tenantID, item.CampaignID); err != nil {
		slog.Warn("campaign: check completion failed", "campaign_id", item.CampaignID, "error", err)
	}
	return nil
}

// OnStudioReviewApproved is called by the HITL review pipeline when a
// source='studio' review is approved. It marks the linked campaign item as
// 'published' and may complete the campaign.
func (s *Service) OnStudioReviewApproved(ctx context.Context, tenantID, reviewID, reviewerID pgtype.UUID) error {
	return s.updateItemByReview(ctx, tenantID, reviewID, ItemStatusPublished)
}

// OnStudioReviewRejected is called by the HITL review pipeline when a
// source='studio' review is rejected. It marks the linked campaign item as
// 'rejected' and may complete the campaign.
func (s *Service) OnStudioReviewRejected(ctx context.Context, tenantID, reviewID, reviewerID pgtype.UUID) error {
	return s.updateItemByReview(ctx, tenantID, reviewID, ItemStatusRejected)
}

func (s *Service) updateItemByReview(ctx context.Context, tenantID, reviewID pgtype.UUID, status string) error {
	item, err := s.store.GetCampaignItemByReviewID(ctx, tenantID, reviewID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("campaign: find item by review: %w", err)
	}

	_, err = s.store.UpdateCampaignItem(ctx, gen.UpdateContentCampaignItemParams{
		ID:       item.ID,
		TenantID: item.TenantID,
		Status:   status,
	})
	if err != nil {
		return fmt.Errorf("campaign: sync item by review: %w", err)
	}

	if err := s.maybeCompleteCampaign(ctx, tenantID, item.CampaignID); err != nil {
		slog.Warn("campaign: check completion failed", "campaign_id", item.CampaignID, "error", err)
	}
	return nil
}

func (s *Service) maybeCompleteCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) error {
	campaign, err := s.store.GetCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}
	if campaign.Status != CampaignStatusGenerating {
		return nil
	}

	items, err := s.store.ListCampaignItemsByCampaign(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}

	terminal := map[string]bool{
		ItemStatusInReview: true,
		ItemStatusApproved: true,
		ItemStatusPublished: true,
		ItemStatusRejected: true,
		ItemStatusFailed:    true,
	}
	for _, it := range items {
		if !terminal[it.Status] {
			return nil
		}
	}

	_, err = s.store.UpdateCampaignStatus(ctx, campaignID, tenantID, CampaignStatusCompleted, nil)
	return err
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// personaPoolEntry is the internal representation of a persona in the final
// pool during planning.
type personaPoolEntry struct {
	ID          pgtype.UUID
	Name        string
	VoicePrompt string
	IsNew       bool
	Spec        *PersonaSpec
}

func (s *Service) buildPersonaPool(ctx context.Context, tenantID pgtype.UUID, campaign gen.ContentCampaign, userID pgtype.UUID) ([]personaPoolEntry, error) {
	pool := make([]personaPoolEntry, 0)

	if len(campaign.PersonaPool) > 0 {
		rows, err := s.store.ListActivePersonasByIDs(ctx, tenantID, campaign.PersonaPool)
		if err != nil {
			return nil, fmt.Errorf("campaign: load persona pool: %w", err)
		}
		if len(rows) != len(campaign.PersonaPool) {
			return nil, fmt.Errorf("campaign: %w: one or more persona_pool ids are not active or do not belong to this tenant", errpkg.ErrInvalid)
		}
		for _, r := range rows {
			pool = append(pool, personaPoolEntry{
				ID:          r.ID,
				Name:        r.ProfileName,
				VoicePrompt: r.VoicePrompt,
			})
		}
	} else {
		rows, err := s.store.ListActivePersonas(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("campaign: load active personas: %w", err)
		}
		for _, r := range rows {
			pool = append(pool, personaPoolEntry{
				ID:          r.ID,
				Name:        r.ProfileName,
				VoicePrompt: r.VoicePrompt,
			})
		}
	}

	deficit := int(campaign.RequestedCount) - len(pool)
	if deficit > 0 {
		if !campaign.AutoProvisionPersonas {
			return nil, fmt.Errorf("%w: need %d personas, have %d", ErrPersonaPoolTooSmall, campaign.RequestedCount, len(pool))
		}
		if s.perms == nil || !s.perms.HasPermission(ctx, userID, tenantID, "persona.manage") {
			return nil, fmt.Errorf("campaign: %w: auto-provision requires persona.manage permission", errpkg.ErrForbidden)
		}

		specs, err := s.planner.PlanPersonas(ctx, campaign.Prompt, deficit, voiceDiversityHints(campaign.Prompt))
		if err != nil {
			return nil, fmt.Errorf("campaign: plan personas: %w", err)
		}

		inputs := make([]personas.PersonaInput, 0, len(specs))
		for i, sp := range specs {
			in, perr := toPersonaInput(sp, i)
			if perr != nil {
				return nil, perr
			}
			inputs = append(inputs, in)
		}

		results, err := s.personas.ProvisionPersonas(ctx, tenantID, userID, inputs)
		if err != nil {
			return nil, fmt.Errorf("campaign: provision personas: %w", err)
		}

		for i, res := range results {
			if res.Error != "" {
				return nil, fmt.Errorf("campaign: persona provisioning failed for %q: %s", res.Input.Name, res.Error)
			}
			var pid pgtype.UUID
			if err := pid.Scan(res.ID); err != nil {
				return nil, fmt.Errorf("campaign: invalid provisioned persona id %q: %w", res.ID, err)
			}
			pool = append(pool, personaPoolEntry{
				ID:          pid,
				Name:        res.Input.Name,
				VoicePrompt: res.Input.VoicePrompt,
				IsNew:       true,
				Spec:        &specs[i],
			})
		}
	}

	if len(pool) < int(campaign.RequestedCount) {
		return nil, fmt.Errorf("%w: need %d personas, have %d", ErrPersonaPoolTooSmall, campaign.RequestedCount, len(pool))
	}

	return pool, nil
}

func (s *Service) validateCount(ctx context.Context, tenantID pgtype.UUID, count int) error {
	if count <= 0 {
		return fmt.Errorf("campaign: %w: count must be greater than 0", errpkg.ErrInvalid)
	}
	maxCount := 100
	if s.settings != nil {
		maxCount = s.settings.GetIntOr(ctx, tenantID, "campaign.max_count", 50)
	}
	if maxCount > 100 {
		maxCount = 100
	}
	if count > maxCount {
		return fmt.Errorf("campaign: %w: count %d exceeds campaign.max_count %d", errpkg.ErrInvalid, count, maxCount)
	}
	return nil
}

func (s *Service) allowedContentTypes(ctx context.Context, tenantID pgtype.UUID, campaignContentType *string) []string {
	if campaignContentType != nil && *campaignContentType != "" {
		return []string{*campaignContentType}
	}

	resp := s.studio.ListContentTypes(ctx, tenantID)
	types := make([]string, 0, len(resp.Types))
	for _, t := range resp.Types {
		types = append(types, t.Type)
	}
	sort.Strings(types)
	return types
}

func (s *Service) validatePlan(plan CampaignPlanResult, requestedCount int32, pool []personaSummary, allowedTypes []string) error {
	if len(plan.Items) != int(requestedCount) {
		return fmt.Errorf("campaign: %w: plan returned %d items, requested %d", errpkg.ErrInvalid, len(plan.Items), requestedCount)
	}

	allowed := make(map[string]bool, len(allowedTypes))
	for _, t := range allowedTypes {
		allowed[t] = true
	}

	poolIDs := make(map[string]bool, len(pool))
	for _, p := range pool {
		poolIDs[p.ID] = true
	}

	seen := make(map[string]bool, len(plan.Items))
	for i, it := range plan.Items {
		if it.PersonaID == "" {
			return fmt.Errorf("campaign: %w: item %d missing persona_id", errpkg.ErrInvalid, i)
		}
		if !poolIDs[it.PersonaID] {
			return fmt.Errorf("campaign: %w: item %d persona_id %s is not in the final pool", errpkg.ErrInvalid, i, it.PersonaID)
		}
		if seen[it.PersonaID] {
			return fmt.Errorf("campaign: %w: persona %s reused in plan (duplicate assignment)", errpkg.ErrDuplicate, it.PersonaID)
		}
		seen[it.PersonaID] = true

		if it.ContentType == "" {
			return fmt.Errorf("campaign: %w: item %d missing content_type", errpkg.ErrInvalid, i)
		}
		if !allowed[it.ContentType] {
			return fmt.Errorf("campaign: %w: item %d content_type %q is not allowed", errpkg.ErrInvalid, i, it.ContentType)
		}
		if it.TopicPrompt == "" {
			return fmt.Errorf("campaign: %w: item %d missing topic_prompt", errpkg.ErrInvalid, i)
		}
	}

	return nil
}

func (s *Service) validateTargetSpace(ctx context.Context, tenantID pgtype.UUID, id string) error {
	var sid pgtype.UUID
	if err := sid.Scan(id); err != nil {
		return fmt.Errorf("campaign: %w: invalid target_space_id %q", errpkg.ErrInvalid, id)
	}
	exists, err := s.store.SpaceExists(ctx, tenantID, sid)
	if err != nil {
		return fmt.Errorf("campaign: validate target space: %w", err)
	}
	if !exists {
		return fmt.Errorf("campaign: %w: target space %s does not exist or does not belong to this tenant", errpkg.ErrInvalid, id)
	}
	return nil
}

func (s *Service) validatePlanTargetSpaces(ctx context.Context, tenantID pgtype.UUID, campaign gen.ContentCampaign, items []CampaignItemPlan) error {
	seen := make(map[string]struct{})
	ids := make([]pgtype.UUID, 0, len(items)+1)

	if campaign.TargetSpaceID.Valid {
		ids = append(ids, campaign.TargetSpaceID)
		seen[campaign.TargetSpaceID.String()] = struct{}{}
	}

	for i, it := range items {
		if it.TargetSpaceID == "" {
			continue
		}
		if _, ok := seen[it.TargetSpaceID]; ok {
			continue
		}
		var sid pgtype.UUID
		if err := sid.Scan(it.TargetSpaceID); err != nil {
			return fmt.Errorf("campaign: %w: item %d has invalid target_space_id %q", errpkg.ErrInvalid, i, it.TargetSpaceID)
		}
		seen[it.TargetSpaceID] = struct{}{}
		ids = append(ids, sid)
	}

	for _, sid := range ids {
		exists, err := s.store.SpaceExists(ctx, tenantID, sid)
		if err != nil {
			return fmt.Errorf("campaign: validate target space: %w", err)
		}
		if !exists {
			return fmt.Errorf("campaign: %w: target space %s does not exist or does not belong to this tenant", errpkg.ErrInvalid, sid)
		}
	}

	return nil
}

func (s *Service) failCampaign(ctx context.Context, campaign gen.ContentCampaign, msg string) error {
	planJSON, _ := json.Marshal(map[string]string{"error": msg})
	_, err := s.store.UpdateCampaignStatus(ctx, campaign.ID, campaign.TenantID, CampaignStatusFailed, planJSON)
	return err
}

func (s *Service) isEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "campaign.enabled", true)
}

func toPersonaInput(sp PersonaSpec, index int) (personas.PersonaInput, error) {
	var ap personas.ActivityPolicy
	if len(sp.ActivityPolicy) > 0 {
		b, err := json.Marshal(sp.ActivityPolicy)
		if err != nil {
			return personas.PersonaInput{}, fmt.Errorf("campaign: marshal activity_policy for persona %d: %w", index, err)
		}
		if err := json.Unmarshal(b, &ap); err != nil {
			return personas.PersonaInput{}, fmt.Errorf("campaign: parse activity_policy for persona %d: %w", index, err)
		}
	}

	return personas.PersonaInput{
		Name:        sp.Name,
		Slug:        sp.Slug,
		Tagline:     sp.Tagline,
		VoicePrompt: sp.VoicePrompt,
		ActivityPolicy: ap,
	}, nil
}

func newPersonaSpecsFromPool(pool []personaPoolEntry) []PersonaSpec {
	var out []PersonaSpec
	for _, p := range pool {
		if p.IsNew && p.Spec != nil {
			out = append(out, *p.Spec)
		}
	}
	return out
}

func voiceDiversityHints(prompt string) []string {
	// Simple, deterministic diversity hints derived from the brief.
	hints := []string{"expertise", "tone", "background", "region"}
	if len(prompt) > 0 {
		return append(hints, "brief-specific angles")
	}
	return hints
}

func encodeTags(tags []string) string {
	return strings.Join(tags, ", ")
}
