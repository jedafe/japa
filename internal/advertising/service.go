package advertising

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/crypto"
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
	// IsHouse marks the campaign as a house ad (fallback fill tier, §25
	// amendment). House ads are admin-only (organization_id stays NULL),
	// skip the minimum-budget check, and may be created directly 'active'.
	IsHouse bool
	// Status optionally overrides the default 'draft' status. Only honored
	// for house ads — paid/admin campaigns always start as 'draft'.
	Status string
}

// CreateCampaign creates a new ad campaign (admin-only in V1).
func (s *Service) CreateCampaign(ctx context.Context, createdBy pgtype.UUID, input CreateCampaignInput) (AdCampaign, error) {
	if input.Name == "" {
		return AdCampaign{}, errors.New("name is required")
	}

	if input.SlotTypeName == "" {
		return AdCampaign{}, errors.New("slot_type_name is required")
	}

	status := CampaignStatusDraft

	// House ads skip the review gate and budget floor — they are the
	// platform's own fallback inventory, not sellable placements.
	if input.IsHouse {
		if input.Status == CampaignStatusPendingReview {
			return AdCampaign{}, errors.New("house campaigns cannot enter pending_review")
		}
		if input.Status == CampaignStatusActive {
			status = CampaignStatusActive
		}
	} else if input.Budget != nil {
		minBudget := float64(10)
		if s.settings != nil {
			minBudget = s.settings.GetFloatOr(ctx, "ads.min_campaign_budget", minBudget)
		}

		if float64(*input.Budget) < minBudget {
			return AdCampaign{}, fmt.Errorf("budget must be at least %.0f credits", minBudget)
		}
	}

	slotType, err := s.store.Queries().GetAdSlotTypeByName(ctx,

		input.SlotTypeName,
	)
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

		Name:                input.Name,
		SlotTypeID:          slotType.ID,
		Content:             content,
		TargetUrl:           targetURL,
		Budget:              input.Budget,
		Status:              status,
		IsHouse:             input.IsHouse,
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
func (s *Service) GetCampaign(ctx context.Context, id pgtype.UUID) (AdCampaign, error) {
	return s.store.Queries().GetAdCampaignByID(ctx,
		id,
	)
}

// ListCampaigns returns all campaigns for a platform.
func (s *Service) ListCampaigns(ctx context.Context, limit, offset int32) ([]AdCampaign, error) {
	return s.store.listAdCampaignsByPlatform(ctx, limit, offset)
}

// ActivateCampaign sets a campaign to active status.
// For self-serve campaigns (status = pending_review), this is the admin
// "approve" action — it transitions the campaign to active so it starts
// serving impressions.
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Review Gate).
func (s *Service) ActivateCampaign(ctx context.Context, id pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.Queries().GetAdCampaignByID(ctx,
		id,
	)
	if err != nil {
		return AdCampaign{}, err
	}

	if campaign.Status != CampaignStatusDraft &&
		campaign.Status != CampaignStatusPaused &&
		campaign.Status != CampaignStatusPendingReview {
		return AdCampaign{}, fmt.Errorf("campaign cannot be activated from status: %s", campaign.Status)
	}

	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status: CampaignStatusActive,
		ID:     id,
	})
}

// RejectCampaign rejects a pending-review campaign (admin moderation action).
// Sets status to 'rejected'. The credit refund is handled by the
// ads:refund_campaign_credits Asynq task enqueued by the handler.
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Review Gate).
func (s *Service) RejectCampaign(ctx context.Context, id pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.Queries().GetAdCampaignByID(ctx,
		id,
	)
	if err != nil {
		return AdCampaign{}, err
	}

	if campaign.Status != CampaignStatusPendingReview {
		return AdCampaign{}, fmt.Errorf("campaign cannot be rejected from status: %s", campaign.Status)
	}

	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status: CampaignStatusRejected,
		ID:     id,
	})
}

// PauseCampaign pauses an active campaign.
// Only campaigns in 'active' status can be paused.
func (s *Service) PauseCampaign(ctx context.Context, id pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.Queries().GetAdCampaignByID(ctx,
		id,
	)
	if err != nil {
		return AdCampaign{}, err
	}

	if campaign.Status != CampaignStatusActive {
		return AdCampaign{}, fmt.Errorf("campaign cannot be paused from status: %s", campaign.Status)
	}

	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status: CampaignStatusPaused,
		ID:     id,
	})
}

// EndCampaign permanently ends a campaign.
// Only campaigns in 'active' or 'paused' status can be ended.
func (s *Service) EndCampaign(ctx context.Context, id pgtype.UUID) (AdCampaign, error) {
	campaign, err := s.store.Queries().GetAdCampaignByID(ctx,
		id,
	)
	if err != nil {
		return AdCampaign{}, err
	}

	if campaign.Status != CampaignStatusActive && campaign.Status != CampaignStatusPaused {
		return AdCampaign{}, fmt.Errorf("campaign cannot be ended from status: %s", campaign.Status)
	}

	return s.store.Queries().UpdateAdCampaignStatus(ctx, gen.UpdateAdCampaignStatusParams{
		Status: CampaignStatusEnded,
		ID:     id,
	})
}

// UpdateCampaignDetails partially updates a campaign's editable fields
// (name, budget, is_house). ad_campaigns has no description column; the
// previous raw-SQL handler referenced one and would 500. Uses the generated
// UpdateAdCampaignDetails query (COALESCE-based partial update).
func (s *Service) UpdateCampaignDetails(ctx context.Context, id pgtype.UUID, name string, budget *int64, isHouse *bool) (AdCampaign, error) {
	return s.store.Queries().UpdateAdCampaignDetails(ctx, gen.UpdateAdCampaignDetailsParams{
		ID: id,

		Name:    name,
		Budget:  budget,
		IsHouse: isHouse,
	})
}

// GetSlotType retrieves a slot type by ID. Uses the generated
// GetAdSlotTypeByID query; the previous raw-SQL handler referenced
// non-existent width/height columns and would 500.
func (s *Service) GetSlotType(ctx context.Context, id pgtype.UUID) (AdSlotType, error) {
	return s.store.Queries().GetAdSlotTypeByID(ctx,
		id,
	)
}

// RenderSlot returns eligible campaigns for a slot, filtered by geo/space
// targeting. Paid/self-serve campaigns (is_house=false) are tried first; when
// none are eligible the house tier (is_house=true) fills the slot as
// fallback inventory. Returns an empty slice (never an error) when nothing
// matches — including for unknown slot names.
// Ref: sections/25-ad-placement.md (Amendment — serving targeting + house tier).
func (s *Service) RenderSlot(ctx context.Context, userID pgtype.UUID, slotName, userCountry, spaceID string) ([]AdCampaign, error) {
	slotType, err := s.store.Queries().GetAdSlotTypeByName(ctx,

		slotName,
	)
	if err != nil {
		return []AdCampaign{}, nil // unknown slot → empty, never an error to the caller
	}

	// Paid/self-serve tier first.
	campaigns, err := s.store.listEligibleCampaigns(ctx, slotType.ID, userCountry, spaceID, false, slotType.MaxDisplayPerPage)
	if err != nil {
		return nil, fmt.Errorf("listing eligible campaigns: %w", err)
	}

	filtered, err := s.filterByFrequencyCap(ctx, campaigns, userID)
	if err != nil {
		return nil, err
	}

	if len(filtered) == 0 {
		// House tier — fill unsold inventory when no paid/self-serve
		// campaign survives eligibility + frequency capping.
		campaigns, err = s.store.listEligibleCampaigns(ctx, slotType.ID, userCountry, spaceID, true, slotType.MaxDisplayPerPage)
		if err != nil {
			return nil, fmt.Errorf("listing house campaigns: %w", err)
		}
		filtered, err = s.filterByFrequencyCap(ctx, campaigns, userID)
		if err != nil {
			return nil, err
		}
	}

	return filtered, nil
}

// filterByFrequencyCap applies the per-user/day impression cap
// (per-campaign override or the ads.frequency_cap_per_day setting) to a
// candidate campaign list.
func (s *Service) filterByFrequencyCap(ctx context.Context, campaigns []AdCampaign, userID pgtype.UUID) ([]AdCampaign, error) {
	filtered := []AdCampaign{}

	globalFreqCap := 0
	if s.settings != nil {
		globalFreqCap = s.settings.GetIntOr(ctx, "ads.frequency_cap_per_day", 3)
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

			// nolint:gosec // G115: bounded
			// nolint:gosec // G115: cap is bounded by caller
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

// IngestAdEvents records a batch of ad events. This should be called from a
// background worker, not synchronously on every page view. Guest events
// (empty UserID) are supported — user_id is nullable in ad_events.
func (s *Service) IngestAdEvents(ctx context.Context, events []IngestAdEventInput) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	ingested := 0

	for _, event := range events {
		if event.EventType != EventTypeImpression && event.EventType != EventTypeClick && event.EventType != EventTypeConversion {
			continue
		}

		var campaignID pgtype.UUID
		if err := campaignID.Scan(event.CampaignID); err != nil {
			continue
		}

		// user_id is nullable — skip scan for empty strings (guest events).
		var userID pgtype.UUID
		if event.UserID != "" {
			if err := userID.Scan(event.UserID); err != nil {
				continue
			}
		}

		occurredAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		if !event.OccurredAt.IsZero() {
			occurredAt = pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}
		}

		_, err := s.store.Queries().InsertAdEvent(ctx, gen.InsertAdEventParams{

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

// ListSlotTypes returns all slot types for a platform.
func (s *Service) ListSlotTypes(ctx context.Context, limit, offset int32) ([]AdSlotType, error) {
	return s.store.listAdSlotTypes(ctx, limit, offset)
}

// AdSlotTypeInput holds parameters for creating/updating a slot type.
// Only slot_name and description are writable — ad_slot_types has no
// slug/max_width/max_height/format columns (migration 000024).
type AdSlotTypeInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateSlotType creates a new ad slot type.
func (s *Service) CreateSlotType(ctx context.Context, input AdSlotTypeInput) (AdSlotType, error) {
	return s.store.Queries().CreateAdSlotType(ctx, gen.CreateAdSlotTypeParams{

		SlotName:          input.Name,
		Description:       &input.Description,
		MaxDisplayPerPage: 1,
	})
}

// UpdateSlotType updates an existing ad slot type's name and description.
func (s *Service) UpdateSlotType(ctx context.Context, id pgtype.UUID, input AdSlotTypeInput) (AdSlotType, error) {
	return s.store.updateAdSlotType(ctx, id, input.Name, input.Description)
}

// DeleteSlotType deletes an ad slot type.
func (s *Service) DeleteSlotType(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteAdSlotType(ctx, id)
}

// DeleteCampaign deletes a campaign (only if draft or ended).
func (s *Service) DeleteCampaign(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteAdCampaign(ctx, id)
}

// GetCampaignStats returns aggregate stats for a campaign.
func (s *Service) GetCampaignStats(ctx context.Context, campaignID pgtype.UUID) (map[string]any, error) {
	return s.store.getCampaignStats(ctx, campaignID)
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
func (s *Service) CreateBoost(ctx context.Context, input CreateBoostInput) (OrganizationBoost, error) {
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

		OrganizationID: input.OrganizationID,
		CampaignID:     input.CampaignID,
		BoostType:      boostType,
		Status:         BoostStatusActive,
		StartDate:      startDate,
		EndDate:        endDate,
	})
}

// ListBoosts returns all boosts for an organization.
func (s *Service) ListBoosts(ctx context.Context, organizationID pgtype.UUID) ([]OrganizationBoost, error) {
	return s.store.Queries().ListOrganizationBoosts(ctx,

		organizationID,
	)
}

// ListActiveBoosts returns all currently active boosts for a platform.
func (s *Service) ListActiveBoosts(ctx context.Context) ([]gen.ListActiveBoostsRow, error) {
	return s.store.Queries().ListActiveBoosts(ctx)
}

// EndBoost ends an active boost.
func (s *Service) EndBoost(ctx context.Context, id pgtype.UUID) (OrganizationBoost, error) {
	return s.store.Queries().EndOrganizationBoost(ctx,
		id,
	)
}

// UpdateBoost updates an existing boost's status and/or end date.
func (s *Service) UpdateBoost(ctx context.Context, id pgtype.UUID, status *string, endDate *time.Time) (OrganizationBoost, error) {
	var ed pgtype.Timestamptz
	if endDate != nil {
		ed = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}

	return s.store.Queries().UpdateOrganizationBoost(ctx, gen.UpdateOrganizationBoostParams{
		ID: id,

		Status:  status,
		EndDate: ed,
	})
}

// DeleteBoost removes a boost.
func (s *Service) DeleteBoost(ctx context.Context, id pgtype.UUID) error {
	return s.store.Queries().DeleteOrganizationBoost(ctx,
		id,
	)
}

// ---------------------------------------------------------------------------
// Ad Networks
// ---------------------------------------------------------------------------

// CreateAdNetworkInput holds parameters for creating an ad network.
type CreateAdNetworkInput struct {
	Provider    string          `json:"provider"`
	Name        string          `json:"name"`
	ClientID    *string         `json:"client_id,omitempty"`
	APIKey      *string         `json:"api_key,omitempty"`
	SlotMapping json.RawMessage `json:"slot_mapping"`
	Config      json.RawMessage `json:"config"`
	IsActive    bool            `json:"is_active"`
	Priority    int32           `json:"priority"`
}

// UpdateAdNetworkInput holds parameters for updating an ad network.
type UpdateAdNetworkInput struct {
	Name        *string         `json:"name,omitempty"`
	ClientID    *string         `json:"client_id,omitempty"`
	APIKey      *string         `json:"api_key,omitempty"`
	SlotMapping json.RawMessage `json:"slot_mapping,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
	IsActive    *bool           `json:"is_active,omitempty"`
	Priority    *int32          `json:"priority,omitempty"`
}

// validProviders is the set of allowed ad network provider values
// (§25 amendment — must match the ad_networks_provider_check constraint in
// migration 000814). Exposed to the admin UI via /admin/ads/networks/providers.
var validProviders = map[string]bool{
	"adpushup": true, "adsense": true, "amazon_aps": true, "buysellads": true,
	"carbon": true, "criteo": true, "custom": true, "ezoic": true,
	"freestar": true, "google_admanager": true, "index_exchange": true,
	"magnite": true, "media_net": true, "mediavine": true, "monumetric": true,
	"openx": true, "prebid": true, "publift": true, "pubmatic": true,
	"raptive": true, "setupad": true, "she_media": true, "snigel": true,
	"sovrn": true, "valueimpression": true,
}

// ListAdNetworkProviders returns the sorted list of allowed ad network
// provider values. Exposed via GET /admin/ads/networks/providers so the
// admin frontend can populate the provider <select> from the backend
// instead of hardcoding the list (Rule 16).
// Ref: japaconnect-engineering/sections/25-ad-placement.md (Ad Networks).
func (s *Service) ListAdNetworkProviders() []string {
	providers := make([]string, 0, len(validProviders))
	for p := range validProviders {
		providers = append(providers, p)
	}

	sort.Strings(providers)

	return providers
}

// maskAPIKey returns a masked representation of an API key for display:
// shows the first 4 and last 4 characters with **** in between. Empty keys
// return an empty string. Short keys (≤8 chars) return all-masked.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	if len(key) <= 8 {
		return "****"
	}

	return key[:4] + "****" + key[len(key)-4:]
}

// AdNetworkDTO is the safe JSON representation of an ad network for API
// responses. The encrypted API key is never serialized; instead a masked
// version is derived for admin display.
type AdNetworkDTO struct {
	ID pgtype.UUID `json:"id"`

	Provider     string             `json:"provider"`
	Name         string             `json:"name"`
	ClientID     *string            `json:"client_id"`
	APIKeyMasked string             `json:"api_key_masked"`
	HasAPIKey    bool               `json:"has_api_key"`
	SlotMapping  json.RawMessage    `json:"slot_mapping"`
	Config       json.RawMessage    `json:"config"`
	IsActive     bool               `json:"is_active"`
	Priority     int32              `json:"priority"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
	UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
}

// toAdNetworkDTO converts a raw AdNetwork (with encrypted api_key) to a safe
// DTO with a masked key for display. Decryption is best-effort: if it fails
// (e.g. legacy plaintext that isn't in v1:nonce:ciphertext format), the raw
// value is masked directly.
func toAdNetworkDTO(n AdNetwork) AdNetworkDTO {
	dto := AdNetworkDTO{
		ID: n.ID,

		Provider:    n.Provider,
		Name:        n.Name,
		ClientID:    n.ClientID,
		SlotMapping: json.RawMessage(n.SlotMapping),
		Config:      json.RawMessage(n.Config),
		IsActive:    n.IsActive,
		Priority:    n.Priority,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
	}
	if n.ApiKeyEncrypted != nil && *n.ApiKeyEncrypted != "" {
		dto.HasAPIKey = true
		if plain, err := crypto.Decrypt(*n.ApiKeyEncrypted); err == nil {
			dto.APIKeyMasked = maskAPIKey(plain)
		} else if strings.HasPrefix(*n.ApiKeyEncrypted, "v1:") {
			// Encrypted format but decryption failed (corrupted/tampered) —
			// do not mask the ciphertext blob, show a safe placeholder.
			slog.Warn("advertising: failed to decrypt api key", "error", err)

			dto.APIKeyMasked = "****"
		} else {
			// Legacy plaintext (pre-encryption) — mask directly.
			dto.APIKeyMasked = maskAPIKey(*n.ApiKeyEncrypted)
		}
	}

	return dto
}

// encryptAPIKey encrypts a plaintext API key using AES-256-GCM. Returns nil
// when the input is empty (no key provided).
func encryptAPIKey(plain *string) *string {
	if plain == nil || *plain == "" {
		return nil
	}

	enc, err := crypto.Encrypt(*plain)
	if err != nil {
		slog.Error("advertising: failed to encrypt api key", "error", err)
		return nil
	}

	return &enc
}

// CreateAdNetwork is an exported method.
func (s *Service) CreateAdNetwork(ctx context.Context, input CreateAdNetworkInput) (AdNetwork, error) {
	if input.Provider == "" {
		return AdNetwork{}, errors.New("provider is required")
	}

	if !validProviders[input.Provider] {
		return AdNetwork{}, fmt.Errorf("invalid provider: %s (see GET /admin/ads/networks/providers)", input.Provider)
	}

	if input.Name == "" {
		return AdNetwork{}, errors.New("name is required")
	}

	slotMapping := input.SlotMapping
	if slotMapping == nil {
		slotMapping = json.RawMessage(`{}`)
	}

	config := input.Config
	if config == nil {
		config = json.RawMessage(`{}`)
	}

	return s.store.createAdNetwork(ctx, gen.CreateAdNetworkParams{

		Provider:        input.Provider,
		Name:            input.Name,
		ClientID:        input.ClientID,
		ApiKeyEncrypted: encryptAPIKey(input.APIKey),
		SlotMapping:     slotMapping,
		Config:          config,
		IsActive:        input.IsActive,
		Priority:        input.Priority,
	})
}

// GetAdNetwork is an exported method.
func (s *Service) GetAdNetwork(ctx context.Context, id pgtype.UUID) (AdNetwork, error) {
	return s.store.getAdNetworkByID(ctx, id)
}

// ListAdNetworks is an exported method.
func (s *Service) ListAdNetworks(ctx context.Context, pageSize, offset int32) ([]AdNetwork, error) {
	if pageSize <= 0 {
		pageSize = 20
	}

	if offset < 0 {
		offset = 0
	}

	return s.store.listAdNetworks(ctx, pageSize, offset)
}

// ListActiveAdNetworks is an exported method.
func (s *Service) ListActiveAdNetworks(ctx context.Context) ([]AdNetwork, error) {
	return s.store.listActiveAdNetworks(ctx)
}

// UpdateAdNetwork is an exported method.
func (s *Service) UpdateAdNetwork(ctx context.Context, id pgtype.UUID, input UpdateAdNetworkInput) (AdNetwork, error) {
	existing, err := s.store.getAdNetworkByID(ctx, id)
	if err != nil {
		return AdNetwork{}, fmt.Errorf("network not found: %w", err)
	}

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}

	clientID := existing.ClientID
	if input.ClientID != nil {
		clientID = input.ClientID
	}

	apiKey := existing.ApiKeyEncrypted
	if input.APIKey != nil {
		apiKey = encryptAPIKey(input.APIKey)
	}

	slotMapping := existing.SlotMapping
	if input.SlotMapping != nil {
		slotMapping = input.SlotMapping
	}

	config := existing.Config
	if input.Config != nil {
		config = input.Config
	}

	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	priority := existing.Priority
	if input.Priority != nil {
		priority = *input.Priority
	}

	return s.store.updateAdNetwork(ctx, id, gen.UpdateAdNetworkParams{
		ID: id,

		Name:            name,
		ClientID:        clientID,
		ApiKeyEncrypted: apiKey,
		SlotMapping:     slotMapping,
		Config:          config,
		IsActive:        isActive,
		Priority:        priority,
	})
}

// DeleteAdNetwork is an exported method.
func (s *Service) DeleteAdNetwork(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteAdNetwork(ctx, id)
}

// CountAdNetworks is an exported method.
func (s *Service) CountAdNetworks(ctx context.Context) (int32, error) {
	return s.store.countAdNetworks(ctx)
}

// GetAdNetworkDTO returns a single ad network as a safe DTO (masked key).
func (s *Service) GetAdNetworkDTO(ctx context.Context, id pgtype.UUID) (AdNetworkDTO, error) {
	n, err := s.store.getAdNetworkByID(ctx, id)
	if err != nil {
		return AdNetworkDTO{}, err
	}

	return toAdNetworkDTO(n), nil
}

// ListAdNetworkDTOs returns ad networks as safe DTOs (masked keys).
// pageSize and offset are the pagination parameters (offset-based).
func (s *Service) ListAdNetworkDTOs(ctx context.Context, pageSize, offset int32) ([]AdNetworkDTO, int32, error) {
	if pageSize <= 0 {
		pageSize = 20
	}

	if offset < 0 {
		offset = 0
	}

	networks, err := s.store.listAdNetworks(ctx, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.countAdNetworks(ctx)
	if err != nil {
		return nil, 0, err
	}

	dtos := make([]AdNetworkDTO, 0, len(networks))
	for _, n := range networks {
		dtos = append(dtos, toAdNetworkDTO(n))
	}

	return dtos, total, nil
}

// CreateAdNetworkDTO creates an ad network and returns the safe DTO.
func (s *Service) CreateAdNetworkDTO(ctx context.Context, input CreateAdNetworkInput) (AdNetworkDTO, error) {
	n, err := s.CreateAdNetwork(ctx, input)
	if err != nil {
		return AdNetworkDTO{}, err
	}

	return toAdNetworkDTO(n), nil
}

// UpdateAdNetworkDTO updates an ad network and returns the safe DTO.
func (s *Service) UpdateAdNetworkDTO(ctx context.Context, id pgtype.UUID, input UpdateAdNetworkInput) (AdNetworkDTO, error) {
	n, err := s.UpdateAdNetwork(ctx, id, input)
	if err != nil {
		return AdNetworkDTO{}, err
	}

	return toAdNetworkDTO(n), nil
}
