package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Follower is the subset of the follow-system service that onboarding needs
// to persist tag/space/user selections from the wizard into real follow rows.
type Follower interface {
	EnsureFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error)
}

// Service provides onboarding business logic with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	settings *config.Settings
	follower Follower
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, settings *config.Settings) *Service {
	return &Service{
		store:    NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:       database,
		settings: settings,
	}
}

// SetFollower wires the follow-system service so onboarding can turn selected
// tags, spaces, and users into actual follow relationships.
func (s *Service) SetFollower(f Follower) {
	s.follower = f
}

// GetState returns the current onboarding state for the given user.
// If no state exists yet, returns a default starting state (step 1).
func (s *Service) GetState(ctx context.Context, userID pgtype.UUID) (*OnboardingStatusResponse, error) {
	row, err := s.store.getState(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &OnboardingStatusResponse{
				CurrentStep:    1,
				Completed:      false,
				StepData:       json.RawMessage(`{}`),
				TourDismissed:  false,
				TourDismissals: map[string]bool{},
				TourSteps:      s.tourSteps(ctx),
			}, nil
		}

		return nil, fmt.Errorf("getting onboarding state: %w", err)
	}

	dismissals := parseTourDismissals(row.TourDismissals)

	return &OnboardingStatusResponse{
		CurrentStep:    int(row.CurrentStep),
		Completed:      row.Completed,
		StepData:       row.StepData,
		TourDismissed:  row.TourDismissed || dismissals["main"],
		TourDismissals: dismissals,
		TourSteps:      s.tourSteps(ctx),
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// tourSteps loads the per-surface product tour step lists from the
// onboarding.tour_steps system setting (jsonb map surface → ordered list
// of data-tour target keys). Keys are validated to lowercase
// alphanumeric/hyphen slugs; invalid entries are dropped. Returns nil when
// the setting is unset or malformed — frontends fall back to their
// built-in default step lists.
func (s *Service) tourSteps(ctx context.Context) map[string][]string {
	raw, _ := s.settings.GetSetting(ctx, keyTourSteps)
	if raw == "" {
		return nil
	}

	var configured map[string][]string
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		slog.Warn("onboarding: malformed tour_steps setting", "error", err)
		return nil
	}

	steps := make(map[string][]string, len(configured))
	for surface, keys := range configured {
		surface = normalizeTourSurface(surface)
		cleaned := make([]string, 0, len(keys))
		for _, k := range keys {
			k = strings.ToLower(strings.TrimSpace(k))
			if isTourStepKey(k) {
				cleaned = append(cleaned, k)
			}
			if len(cleaned) >= 48 {
				break
			}
		}
		// First entry wins when several setting keys normalize to the same
		// surface (e.g. "Docs" + "docs") — avoids nondeterministic map-order
		// overwrites.
		if _, exists := steps[surface]; !exists {
			steps[surface] = cleaned
		}
	}

	return steps
}

// isTourStepKey reports whether s is a safe data-tour attribute value:
// lowercase alphanumeric + hyphen, 1–64 chars.
func isTourStepKey(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// parseTourDismissals decodes the tour_dismissals jsonb column into a
// surface→dismissed map. Unknown/absent values degrade to an empty map.
func parseTourDismissals(raw []byte) map[string]bool {
	dismissals := make(map[string]bool)
	if len(raw) == 0 {
		return dismissals
	}

	if err := json.Unmarshal(raw, &dismissals); err != nil {
		slog.Warn("onboarding: corrupted tour_dismissals data", "error", err)
		return map[string]bool{}
	}

	return dismissals
}

// totalSteps is the number of steps in the onboarding wizard per Section 09.
const totalSteps = 5

// UpdateStep saves step data and advances the current step number.
// When req.Skip is true, the step minimum is auto-fulfilled using the ranked
// recommendation algorithm per Section 09 skip semantics (the user always
// ends up meeting the platform's baseline density target).
func (s *Service) UpdateStep(ctx context.Context, userID pgtype.UUID, req *UpdateStepRequest) error {
	// Read current state to merge step_data.
	current, err := s.store.getState(ctx, userID)

	var currentStep int32 = 1

	var completed bool

	existingData := json.RawMessage(`{}`)

	var tourDismissed bool

	tourDismissals := json.RawMessage(`{}`)

	if err == nil {
		currentStep = current.CurrentStep
		completed = current.Completed
		existingData = current.StepData
		tourDismissed = current.TourDismissed
		tourDismissals = current.TourDismissals
	}

	// The step being submitted (defaults to the current step).
	submitStep := req.Step
	if submitStep <= 0 {
		submitStep = int(currentStep)
	}

	// BUG-004 fix: validate the submitted step is in the valid range [1, totalSteps].
	// Without this, a malicious client could send step: 999 to bypass the 5-step flow.
	if submitStep < 1 || submitStep > totalSteps {
		return fmt.Errorf("%w: step %d out of range [1, %d]", errpkg.ErrInvalidStatusTransition, submitStep, totalSteps)
	}

	// Section 09 skip semantics: if the user skips a step with a minimum
	// greater than zero, auto-fulfill that minimum using the ranked
	// algorithm rather than bypassing the requirement.
	if req.Skip {
		fulfilled := s.autoFulfillSkip(ctx, userID, submitStep)
		if fulfilled != nil {
			// Merge the auto-fulfilled data with any explicit data the
			// user also provided (explicit data wins).
			merged := mergeJSON(fulfilled, req.StepData)
			return s.persistStep(ctx, userID, submitStep, currentStep, completed, existingData, merged, tourDismissed, tourDismissals)
		}
	}

	// Merge step_data: existing + new data (new keys overwrite old).
	merged := mergeJSON(existingData, req.StepData)

	return s.persistStep(ctx, userID, submitStep, currentStep, completed, existingData, merged, tourDismissed, tourDismissals)
}

// persistStep merges, auto-joins geo spaces, and upserts the onboarding state.
func (s *Service) persistStep(ctx context.Context, userID pgtype.UUID, submitStep int, currentStep int32, completed bool, existingData, merged json.RawMessage, tourDismissed bool, tourDismissals json.RawMessage) error {
	// Auto-join primary geo spaces when location fields are present (Section 05, ADR-005).
	fields := parseStepData(merged)
	if fields.CountryCode != "" {
		if err := s.autoJoinGeoSpaces(ctx, userID, fields); err != nil {
			slog.Warn("auto-join geo spaces failed", "error", err)
			// Non-fatal — don't block onboarding.
		}
	}

	// ADR-064a: When the user selects a destination country in Step 2, save
	// the ISO geo_code as the `destination_country` custom field so the
	// geo-aware personalization feature (VisitorContext resolver, feed
	// geo-boost, search geo-weighting) picks it up without the user having
	// to set it again in Settings. Failures are non-fatal — onboarding
	// must not block on the custom-field write.
	if fields.DestinationCountryCode != "" {
		if err := s.setDestinationCountry(ctx, userID, fields.DestinationCountryCode); err != nil {
			slog.Warn("onboarding: failed to save destination_country custom field", "error", err)
			// Non-fatal — don't block onboarding.
		}
	}

	// Persist tag/space/user selections as real follow rows. Failures are
	// non-fatal — don't block onboarding on a single bad recommendation.
	if err := s.persistFollows(ctx, userID, fields); err != nil {
		slog.Warn("onboarding: failed to persist follow selections", "error", err)
	}

	// BUG-001/002 fix: current_step represents the step the user is
	// currently ON (about to work on), not the last completed step. When
	// the user submits step N data, they advance to step N+1. Storing
	// submitStep (the step just completed) caused two bugs:
	//   1. Complete() checks CurrentStep >= 5, but the last PUT stores 4,
	//      so completion always failed.
	//   2. On refresh, GetState returned the step just completed, sending
	//      the user backwards.
	newStep := submitStep + 1

	// Clamp to totalSteps — step 5 is the terminal step; advancing past it
	// is handled by POST /onboarding/complete, not by advancing current_step.
	if newStep > totalSteps {
		newStep = totalSteps
	}

	if err := s.store.upsertState(ctx, userID, int32(newStep), completed, merged, tourDismissed, tourDismissals); err != nil { //nolint:gosec // G115: newStep is bounded by totalSteps (5)
		return fmt.Errorf("updating onboarding step: %w", err)
	}

	return nil
}

// persistFollows turns Step 3 tag slugs and Step 4 recommendation IDs into
// actual follow rows using the follow-system service. It is best-effort and
// never blocks onboarding. Recommendations are resolved to their entity type
// from the same ranked list used by the wizard.
func (s *Service) persistFollows(ctx context.Context, userID pgtype.UUID, fields stepDataFields) error {
	if s.follower == nil {
		return nil
	}

	// Step 3: follow selected tags by slug.
	tagIDs := make([]pgtype.UUID, 0, len(fields.Tags))
	for _, slug := range fields.Tags {
		tag, err := s.store.write.GetTagBySlug(ctx,

			slug,
		)
		if err != nil {
			slog.Warn("onboarding: failed to resolve tag slug", "slug", slug, "error", err)
			continue
		}
		tagIDs = append(tagIDs, tag.ID)
	}

	for _, id := range tagIDs {
		if _, err := s.follower.EnsureFollow(ctx, userID, "tag", id); err != nil {
			slog.Warn("onboarding: failed to follow tag", "tag_id", id, "error", err)
		}
	}

	// Step 4: follow selected recommendations by ID. We resolve type from
	// the current recommendation list so we don't toggle-unfollow an entity
	// the user already follows (e.g. an auto-joined geo space).
	if len(fields.Follows) == 0 {
		return nil
	}

	recs, err := s.GetRecommendations(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting recommendations for follow persistence: %w", err)
	}

	recTypeByID := make(map[string]string, len(recs))
	for _, r := range recs {
		recTypeByID[r.ID] = r.Type
	}

	for _, rawID := range fields.Follows {
		var id pgtype.UUID
		if err := id.Scan(rawID); err != nil {
			slog.Warn("onboarding: invalid recommendation id in follows", "id", rawID, "error", err)
			continue
		}

		entityType, ok := recTypeByID[rawID]
		if !ok {
			// Fallback to space when the recommendation is no longer in
			// the list; the follow service will reject an invalid entity.
			entityType = "space"
		}

		if _, err := s.follower.EnsureFollow(ctx, userID, entityType, id); err != nil {
			slog.Warn("onboarding: failed to follow recommendation", "entity_id", rawID, "entity_type", entityType, "error", err)
		}
	}

	return nil
}

// autoFulfillSkip returns auto-populated step_data for a skipped step that has
// a minimum requirement (step 3 = tags, step 4 = follows). Returns nil if the
// step has no minimum (no auto-fulfill needed). Per Section 09, the user always
// ends up meeting the baseline density target — they just choose between manual
// curation or accepting defaults.
func (s *Service) autoFulfillSkip(ctx context.Context, userID pgtype.UUID, step int) json.RawMessage {
	switch step {
	case 3:
		// Step 3: Interests/Tags — auto-pick min_tags_follow promoted tags.
		minTags := s.getMinTagsFollow(ctx)
		if minTags <= 0 {
			return nil
		}

		promotedTagsJSON, _ := s.settings.GetSetting(ctx, keyPromotedTags)

		var promotedTags []string

		if promotedTagsJSON != "" {
			_ = json.Unmarshal([]byte(promotedTagsJSON), &promotedTags)
		}

		if len(promotedTags) >= minTags {
			promotedTags = promotedTags[:minTags]
		}

		if len(promotedTags) == 0 {
			return nil // no promoted tags to auto-fulfill with
		}

		data, _ := json.Marshal(map[string]any{
			"tags":      promotedTags,
			"interests": promotedTags,
			"skipped":   true,
		})

		return data

	case 4:
		// Step 4: Recommended Spaces & Connections — auto-follow min spaces
		// and min users from the recommendation engine.
		minSpaces := s.getMinSpacesJoin(ctx)
		minUsers := s.getMinUsersFollow(ctx)

		if minSpaces <= 0 && minUsers <= 0 {
			return nil
		}

		recs, err := s.GetRecommendations(ctx, userID)
		if err != nil || len(recs) == 0 {
			return nil
		}

		var follows []string

		spaceCount := 0
		userCount := 0

		for _, r := range recs {
			if r.Type == "space" && spaceCount < minSpaces {
				follows = append(follows, r.ID)
				spaceCount++
			}

			if r.Type == "user" && userCount < minUsers {
				follows = append(follows, r.ID)
				userCount++
			}

			if spaceCount >= minSpaces && userCount >= minUsers {
				break
			}
		}

		if len(follows) == 0 {
			return nil
		}

		data, _ := json.Marshal(map[string]any{
			"follows": follows,
			"skipped": true,
		})

		return data

	default:
		// Steps 1, 2, 5 have no minimum — skip is a no-op pass-through.
		return nil
	}
}

// getMinTagsFollow reads the onboarding.min_tags_follow setting with the
// Section 09 default of 3.
func (s *Service) getMinTagsFollow(ctx context.Context) int {
	v, _ := s.settings.GetSettingInt(ctx, keyMinTagsFollow)
	if v == 0 {
		return 3
	}

	return v
}

// getMinUsersFollow reads the onboarding.min_users_follow setting with the
// Section 09 default of 2.
func (s *Service) getMinUsersFollow(ctx context.Context) int {
	v, _ := s.settings.GetSettingInt(ctx, keyMinUsersFollow)
	if v == 0 {
		return 2
	}

	return v
}

// getMinSpacesJoin reads the onboarding.min_spaces_join setting with the
// Section 09 default of 2.
func (s *Service) getMinSpacesJoin(ctx context.Context) int {
	v, _ := s.settings.GetSettingInt(ctx, keyMinSpacesJoin)
	if v == 0 {
		return 2
	}

	return v
}

// Complete marks onboarding as completed for the given user.
// Per Section 09, completion is the terminal action of the 5-step flow —
// a user must have reached step 5 before they can complete. This prevents
// a malicious client from calling POST /onboarding/complete immediately
// after signup and bypassing the baseline density target.
func (s *Service) Complete(ctx context.Context, userID pgtype.UUID) error {
	state, err := s.store.getState(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: onboarding not started", errpkg.ErrInvalidStatusTransition)
		}

		return fmt.Errorf("getting onboarding state for completion: %w", err)
	}

	if state.Completed {
		return nil // idempotent — already completed
	}

	// Ref: japaconnect-engineering/sections/09-onboarding-flow.md
	// The 5-step flow requires the user to reach step 5 before completing.
	if state.CurrentStep < 5 {
		return fmt.Errorf("%w: onboarding not complete (current step %d, need 5)", errpkg.ErrInvalidStatusTransition, state.CurrentStep)
	}

	if err := s.store.completeOnboarding(ctx, userID); err != nil {
		return fmt.Errorf("completing onboarding: %w", err)
	}

	return nil
}

// DismissTour marks the product tour as dismissed for the given surface.
// The surface is a short slug ("main", "docs", "learn"); an empty surface
// defaults to "main" for backward compatibility with the original
// tour_dismissed boolean. Dismissals are stored per-surface in the
// tour_dismissals jsonb map so each app manages its own tour state.
func (s *Service) DismissTour(ctx context.Context, userID pgtype.UUID, surface string) error {
	surface = normalizeTourSurface(surface)
	if surface == "" {
		return fmt.Errorf("%w: surface contains no valid characters", errpkg.ErrInvalid)
	}

	dismissals, err := json.Marshal(map[string]bool{surface: true})
	if err != nil {
		return fmt.Errorf("marshaling tour dismissal: %w", err)
	}

	// The legacy tour_dismissed boolean is kept in sync for the main surface.
	if err := s.store.dismissTour(ctx, userID, surface == "main", dismissals); err != nil {
		return fmt.Errorf("dismissing tour: %w", err)
	}

	return nil
}

// normalizeTourSurface clamps the surface slug to a safe value: lowercase
// alphanumeric/hyphen, max 32 chars. An absent/blank surface defaults to
// "main" for backward compatibility; input containing no valid characters
// returns "" so the caller can reject it.
func normalizeTourSurface(surface string) string {
	surface = strings.ToLower(strings.TrimSpace(surface))
	if surface == "" {
		return "main"
	}

	var b strings.Builder
	for _, r := range surface {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}

	return b.String()
}

// GetRecommendations returns recommended spaces and users using the tiered
// deterministic algorithm from Section 09:
//  1. Admin-pinned promotions first (onboarding.promoted_spaces/users)
//  2. Exact geo match (user's country/state/city spaces)
//  3. Interest-tag overlap
//  4. Reputation-based fallback fill (popular spaces)
//
// Results are deduplicated by (type, id) and ordered by tier. Falls back to
// popular items only when no personalized signals are available.
func (s *Service) GetRecommendations(ctx context.Context, userID pgtype.UUID) ([]OnboardingRecommendation, error) {
	// Parse step_data to extract interests/tags and location for matching.
	state, err := s.store.getState(ctx, userID)
	if err != nil {
		// BUG-008: only fall back to popular on "no state yet" (ErrNoRows);
		// return real DB/infra errors instead of masking them.
		if errors.Is(err, pgx.ErrNoRows) {
			return s.getPopularItems(ctx)
		}

		return nil, fmt.Errorf("getting onboarding state for recommendations: %w", err)
	}

	stepData := parseStepData(state.StepData)
	// Tags is the field the frontend wizard sends; Interests is a legacy
	// alias. Prefer Tags, fall back to Interests for backward compatibility.
	userTags := stepData.Tags
	if len(userTags) == 0 {
		userTags = stepData.Interests
	}

	// Tier 1: Admin-pinned promotions (Section 09).
	promotedRecs, err := s.getPromotedRecommendations(ctx)
	if err != nil {
		slog.Warn("onboarding: failed to load promoted entities", "error", err)
		// Non-fatal — continue without promoted tier.
		promotedRecs = nil
	}

	// Tier 2: Exact geo match (user's country/state/city geo spaces).
	geoRecs, err := s.recommendByGeo(ctx, userID, stepData)
	if err != nil {
		slog.Warn("onboarding: geo recommendation failed", "error", err)

		geoRecs = nil
	}

	// Tier 3: Interest-tag overlap.
	tagRecs, err := s.recommendSpacesByTags(ctx, userID, userTags)
	if err != nil {
		slog.Warn("onboarding: tag recommendation failed", "error", err)

		tagRecs = nil
	}

	// Merge tiers in order, deduplicating by (type, id).
	merged := mergeRecommendations(promotedRecs, geoRecs, tagRecs)

	// Tier 4: If no personalized recs at all, fall back to popular.
	if len(merged) == 0 {
		return s.getPopularItems(ctx)
	}

	return merged, nil
}

// mergeRecommendations concatenates recommendation tiers in order, skipping
// entries whose (type, id) pair was already added by an earlier tier.
func mergeRecommendations(tiers ...[]OnboardingRecommendation) []OnboardingRecommendation {
	seen := make(map[string]struct{}, 32)
	recs := make([]OnboardingRecommendation, 0, 32)

	for _, tier := range tiers {
		for _, r := range tier {
			key := r.Type + ":" + r.ID
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}

			recs = append(recs, r)
		}
	}

	return recs
}

// getPromotedRecommendations resolves admin-pinned promoted spaces and users
// from system_settings (onboarding.promoted_spaces / promoted_users) into
// recommendation records with reason="promoted" and the highest score (1.0),
// so they always appear first in the tiered ordering per Section 09.
func (s *Service) getPromotedRecommendations(ctx context.Context) ([]OnboardingRecommendation, error) {
	promotedSpacesJSON, _ := s.settings.GetSetting(ctx, keyPromotedSpaces)
	promotedUsersJSON, _ := s.settings.GetSetting(ctx, keyPromotedUsers)

	var spaceIDs, userIDs []string
	if promotedSpacesJSON != "" {
		_ = json.Unmarshal([]byte(promotedSpacesJSON), &spaceIDs)
	}

	if promotedUsersJSON != "" {
		_ = json.Unmarshal([]byte(promotedUsersJSON), &userIDs)
	}

	recs := make([]OnboardingRecommendation, 0, len(spaceIDs)+len(userIDs))

	// Resolve promoted spaces by ID.
	for _, idStr := range spaceIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			continue
		}

		space, err := s.store.read.GetSpaceByID(ctx,
			id,
		)
		if err != nil {
			continue
		}

		recs = append(recs, OnboardingRecommendation{
			Type:   "space",
			ID:     space.ID.String(),
			Name:   space.Name,
			Slug:   space.Slug,
			Reason: "promoted",
			Score:  1.0,
		})
	}

	// Resolve promoted users by ID.
	for _, idStr := range userIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			continue
		}

		user, err := s.store.read.GetUserByID(ctx,

			id,
		)
		if err != nil {
			continue
		}

		recs = append(recs, OnboardingRecommendation{
			Type:   "user",
			ID:     user.ID.String(),
			Name:   user.Username,
			Slug:   user.Username,
			Reason: "promoted",
			Score:  1.0,
		})
	}

	return recs, nil
}

// stepDataFields holds the extracted onboarding step data.
// Field names match what the frontend wizard sends in step_data:
//
//	Step 1: name, japa_status, current_location
//	Step 2: destination, destination_country_code, primary_goal
//	Step 3: tags (array of tag slugs — used for interest-based recommendations)
//	Step 4: follows (array of space/user IDs)
type stepDataFields struct {
	Name            string `json:"name"`
	JapaStatus      string `json:"japa_status"`
	CurrentLocation string `json:"current_location"`
	CountryCode     string `json:"country_code"`
	StateID         string `json:"state_id"`
	CityID          string `json:"city_id"`
	Destination     string `json:"destination"`
	// DestinationCountryCode is the ISO 3166-1 alpha-2 code for the
	// destination country selected in Step 2. When present, it is saved as
	// the `destination_country` custom field (ADR-064a) so the geo-aware
	// personalization feature picks it up without the user re-entering it
	// in Settings. The legacy `Destination` name is kept for backward
	// compatibility with existing step_data consumers.
	DestinationCountryCode string   `json:"destination_country_code"`
	PrimaryGoal            string   `json:"primary_goal"`
	Tags                   []string `json:"tags"`
	Interests              []string `json:"interests"` // legacy alias; Tags takes precedence
	Follows                []string `json:"follows"`
}

func parseStepData(data json.RawMessage) stepDataFields {
	var fields stepDataFields
	if len(data) == 0 {
		return fields
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		slog.Warn("onboarding: corrupted step data", "error", err)
	}

	return fields
}

// mergeJSON merges two JSON objects, with the second overwriting the first.
func mergeJSON(base, overlay json.RawMessage) json.RawMessage {
	baseMap := make(map[string]any)
	if len(base) > 0 {
		if err := json.Unmarshal(base, &baseMap); err != nil {
			slog.Warn("failed to unmarshal base onboarding data", "error", err)
		}
	}

	overlayMap := make(map[string]any)
	if len(overlay) > 0 {
		if err := json.Unmarshal(overlay, &overlayMap); err != nil {
			slog.Warn("failed to unmarshal overlay onboarding data", "error", err)
		}
	}

	for k, v := range overlayMap {
		baseMap[k] = v
	}

	merged, _ := json.Marshal(baseMap)

	return merged
}

// recommendSpacesByTags finds spaces that overlap with the user's interests.
func (s *Service) recommendSpacesByTags(ctx context.Context, _ pgtype.UUID, tags []string) ([]OnboardingRecommendation, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	// Build an ILIKE pattern for each tag so the SQL can pre-filter spaces
	// whose name or description mentions at least one of the user's interests.
	// Build parameterized ILIKE clauses — each tag becomes its own $N placeholder.
	// fmt.Sprintf is used only for the parenthetical OR grouping, not for values.
	likePatterns := make([]string, 0, len(tags))
	args := []any{}

	for i, tag := range tags {
		args = append(args, "%"+strings.ToLower(tag)+"%")
		likePatterns = append(likePatterns, fmt.Sprintf("(LOWER(s.name) LIKE $%d OR LOWER(s.description) LIKE $%d)", i+1, i+1))
	}

	query := fmt.Sprintf(`
		SELECT s.id, s.name, s.slug, s.description
		FROM spaces s
		WHERE s.status = 'active'
		  AND (%s)
		ORDER BY s.follower_count DESC
		LIMIT 10
	`, strings.Join(likePatterns, " OR "))

	rows, err := s.db.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying spaces for recommendations: %w", err)
	}
	defer rows.Close()

	var recs []OnboardingRecommendation

	for rows.Next() {
		var id, name, slug pgtype.Text

		var description pgtype.Text
		if err := rows.Scan(&id, &name, &slug, &description); err != nil {
			continue
		}
		// Simple tag overlap scoring based on description matching.
		score := tagOverlapScore(description.String, tags)
		if score > 0 {
			recs = append(recs, OnboardingRecommendation{
				Type:   "space",
				ID:     id.String,
				Name:   name.String,
				Slug:   slug.String,
				Reason: "tag_overlap",
				Score:  score,
			})
		}
	}

	return recs, nil
}

// recommendByGeo finds the user's geo spaces (country, state, city) using the
// geo hierarchy (Section 09 "exact geo match"), not a fuzzy name LIKE. The
// country is resolved by geo_code, state/city by ID — the same approach as
// autoJoinGeoSpaces. Sibling spaces at each level are not fuzzy-matched.
func (s *Service) recommendByGeo(ctx context.Context, _ pgtype.UUID, fields stepDataFields) ([]OnboardingRecommendation, error) {
	if fields.CountryCode == "" && fields.StateID == "" && fields.CityID == "" {
		return nil, nil
	}

	queries := gen.New(s.db.Reader)
	recs := make([]OnboardingRecommendation, 0, 3)

	// Country space (resolved by geo_code).
	if fields.CountryCode != "" {
		countrySpace, err := queries.GetSpaceByGeoCode(ctx,
			&fields.CountryCode,
		)
		if err == nil {
			recs = append(recs, OnboardingRecommendation{
				Type:   "space",
				ID:     countrySpace.ID.String(),
				Name:   countrySpace.Name,
				Slug:   countrySpace.Slug,
				Reason: "geo_match",
				Score:  0.8,
			})
		}
	}

	// State space (resolved by ID).
	if fields.StateID != "" {
		var stateID pgtype.UUID
		if err := stateID.Scan(fields.StateID); err == nil {
			stateSpace, err := queries.GetSpaceByID(ctx,
				stateID,
			)
			if err == nil {
				recs = append(recs, OnboardingRecommendation{
					Type:   "space",
					ID:     stateSpace.ID.String(),
					Name:   stateSpace.Name,
					Slug:   stateSpace.Slug,
					Reason: "geo_match",
					Score:  0.85,
				})
			}
		}
	}

	// City space (resolved by ID — deepest level, highest score).
	if fields.CityID != "" {
		var cityID pgtype.UUID
		if err := cityID.Scan(fields.CityID); err == nil {
			citySpace, err := queries.GetSpaceByID(ctx,
				cityID,
			)
			if err == nil {
				recs = append(recs, OnboardingRecommendation{
					Type:   "space",
					ID:     citySpace.ID.String(),
					Name:   citySpace.Name,
					Slug:   citySpace.Slug,
					Reason: "geo_match",
					Score:  0.9,
				})
			}
		}
	}

	return recs, nil
}

// getPopularItems returns popular spaces/users as a fallback.
func (s *Service) getPopularItems(ctx context.Context) ([]OnboardingRecommendation, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT s.id, s.name, s.slug
		FROM spaces s
		WHERE s.status = 'active'
		ORDER BY s.follower_count DESC
		LIMIT 6
	`)
	if err != nil {
		return nil, fmt.Errorf("querying popular spaces: %w", err)
	}
	defer rows.Close()

	var recs []OnboardingRecommendation

	for rows.Next() {
		var id, name, slug pgtype.Text
		if err := rows.Scan(&id, &name, &slug); err != nil {
			continue
		}

		recs = append(recs, OnboardingRecommendation{
			Type:   "space",
			ID:     id.String,
			Name:   name.String,
			Slug:   slug.String,
			Reason: "reputation",
			Score:  0.5,
		})
	}

	return recs, nil
}

// tagOverlapScore computes a simple 0-1 score based on keyword overlap.
func tagOverlapScore(description string, tags []string) float64 {
	if description == "" || len(tags) == 0 {
		return 0
	}

	descLower := strings.ToLower(description)
	matchCount := 0

	for _, tag := range tags {
		if strings.Contains(descLower, strings.ToLower(tag)) {
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0
	}

	return math.Min(float64(matchCount)/float64(len(tags)), 1.0)
}

// autoJoinGeoSpaces joins the user to the country, state, and city geo spaces
// based on the onboarding step data. Failures are non-fatal — the caller logs
// and continues without blocking onboarding. All joins + the geo-space update
// run in a single transaction so a mid-sequence failure rolls back the entire
// geo-join (BUG-002 fix, §6.2 transaction boundary, Section 09 space auto-join).
func (s *Service) autoJoinGeoSpaces(ctx context.Context, userID pgtype.UUID, fields stepDataFields) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning geo-join transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }() // no-op after commit

	queries := gen.New(tx)

	// Find country space by geo_code.
	countrySpace, err := queries.GetSpaceByGeoCode(ctx,
		&fields.CountryCode,
	)
	if err != nil {
		return fmt.Errorf("finding country space for %s: %w", fields.CountryCode, err)
	}

	// Join country space (idempotent via ON CONFLICT DO NOTHING).
	if err := queries.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
		SpaceID: countrySpace.ID,
		UserID:  userID,
	}); err != nil {
		slog.Warn("failed to join country space", "space_id", countrySpace.ID, "error", err)
	}

	geoSpaceID := countrySpace.ID

	// Join state space if provided.
	if fields.StateID != "" {
		var stateID pgtype.UUID
		if err := stateID.Scan(fields.StateID); err == nil {
			stateSpace, err := queries.GetSpaceByID(ctx,
				stateID,
			)
			if err == nil {
				if err := queries.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
					SpaceID: stateSpace.ID,
					UserID:  userID,
				}); err != nil {
					slog.Warn("failed to join state space", "space_id", stateSpace.ID, "error", err)
				}

				geoSpaceID = stateSpace.ID
			}
		}
	}

	// Join city space if provided.
	if fields.CityID != "" {
		var cityID pgtype.UUID
		if err := cityID.Scan(fields.CityID); err == nil {
			citySpace, err := queries.GetSpaceByID(ctx,
				cityID,
			)
			if err == nil {
				if err := queries.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
					SpaceID: citySpace.ID,
					UserID:  userID,
				}); err != nil {
					slog.Warn("failed to join city space", "space_id", citySpace.ID, "error", err)
				}

				geoSpaceID = citySpace.ID
			}
		}
	}

	// Set current_geo_space_id to the deepest resolved level.
	if err := queries.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
		CurrentGeoSpaceID: geoSpaceID,
		ID:                userID,
	}); err != nil {
		return fmt.Errorf("setting user geo space: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing geo-join transaction: %w", err)
	}

	return nil
}

// setDestinationCountry saves the given ISO 3166-1 alpha-2 code as the
// `destination_country` custom field on the user's profile (ADR-064a). This
// mirrors the write path in users.Store.setGeoCustomField so the geo-aware
// VisitorContext resolver picks up the destination selected during onboarding
// without the user having to re-enter it in Settings.
func (s *Service) setDestinationCountry(ctx context.Context, userID pgtype.UUID, geoCode string) error {
	code := strings.ToUpper(strings.TrimSpace(geoCode))
	if code == "" {
		return nil
	}

	field, err := s.store.read.GetCustomFieldByKey(ctx,
		"destination_country",
	)
	if err != nil {
		return fmt.Errorf("looking up destination_country custom field: %w", err)
	}

	profileID, err := s.store.read.GetUserProfileID(ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf("getting user profile id: %w", err)
	}

	if _, err := s.store.write.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    field.ID,
		EntityType: "user_profile",
		EntityID:   profileID,
		ValueText:  &code,
	}); err != nil {
		return fmt.Errorf("upserting destination_country custom field value: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Admin onboarding settings and template management
// ---------------------------------------------------------------------------

const (
	keyMinTagsFollow  = "onboarding.min_tags_follow"
	keyMinUsersFollow = "onboarding.min_users_follow"
	keyMinSpacesJoin  = "onboarding.min_spaces_join"
	keyPromotedSpaces = "onboarding.promoted_spaces"
	keyPromotedUsers  = "onboarding.promoted_users"
	keyPromotedTags   = "onboarding.promoted_tags"
	keyTourSteps      = "onboarding.tour_steps"
)

// ListTemplates returns all onboarding templates for the platform.
//
// NOTE: Templates CRUD is wired (route catalog + admin API) but the wizard
// flow does not yet consume the active template to drive step definitions —
// the 5-step flow is hardcoded in GetState/the frontend. Template-driven
// onboarding is a future enhancement (§21); the CRUD surface is preserved
// so admins can pre-configure templates ahead of the consumer-side wiring.
func (s *Service) ListTemplates(ctx context.Context) ([]gen.OnboardingTemplate, error) {
	return s.store.listTemplates(ctx)
}

// GetTemplate retrieves a single onboarding template by ID.
func (s *Service) GetTemplate(ctx context.Context, id pgtype.UUID) (gen.OnboardingTemplate, error) {
	return s.store.getTemplate(ctx, id)
}

// CreateTemplate creates a new onboarding template.
func (s *Service) CreateTemplate(ctx context.Context, req *CreateTemplateRequest) (gen.OnboardingTemplate, error) {
	steps, err := json.Marshal(req.Steps)
	if err != nil {
		return gen.OnboardingTemplate{}, fmt.Errorf("marshaling steps: %w", err)
	}

	return s.store.createTemplate(ctx, req.Name, req.Description, steps, req.IsActive, false)
}

// UpdateTemplate updates an existing onboarding template.
// System templates (is_system = true) cannot be modified — the API enforces
// this guard so a direct API call cannot bypass the frontend's hidden edit
// button (BUG-003 fix, §09 system template protection).
func (s *Service) UpdateTemplate(ctx context.Context, id pgtype.UUID, req *UpdateTemplateRequest) error {
	existing, err := s.store.getTemplate(ctx, id)
	if err != nil {
		return fmt.Errorf("checking template before update: %w", err)
	}

	if existing.IsSystem {
		return fmt.Errorf("%w: system templates cannot be modified", errpkg.ErrForbidden)
	}

	steps, err := json.Marshal(req.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}

	return s.store.updateTemplate(ctx, id, req.Name, req.Description, steps, req.IsActive)
}

// DeleteTemplate deletes an onboarding template.
// System templates (is_system = true) cannot be deleted — the API enforces
// this guard so a direct API call cannot bypass the frontend's hidden delete
// button (BUG-003 fix, §09 system template protection).
func (s *Service) DeleteTemplate(ctx context.Context, id pgtype.UUID) error {
	existing, err := s.store.getTemplate(ctx, id)
	if err != nil {
		return fmt.Errorf("checking template before delete: %w", err)
	}

	if existing.IsSystem {
		return fmt.Errorf("%w: system templates cannot be deleted", errpkg.ErrForbidden)
	}

	return s.store.deleteTemplate(ctx, id)
}

// GetAdminSettings returns the platform's onboarding configuration as exposed
// on the admin Onboarding Settings surface (Section 09 — minimum-count fields
// and promoted-entities picker). Reads the six onboarding.* system_settings
// keys and assembles them into OnboardingAdminSettings. Missing keys fall
// back to the Section 09 defaults (3 tags, 2 users, 2 spaces, empty lists).
func (s *Service) GetAdminSettings(ctx context.Context) (*OnboardingAdminSettings, error) {
	settings := &OnboardingAdminSettings{
		MinTagsFollow:  s.getMinTagsFollow(ctx),
		MinUsersFollow: s.getMinUsersFollow(ctx),
		MinSpacesJoin:  s.getMinSpacesJoin(ctx),
	}

	if raw, _ := s.settings.GetSetting(ctx, keyPromotedSpaces); raw != "" {
		_ = json.Unmarshal([]byte(raw), &settings.PromotedSpaces)
	}

	if raw, _ := s.settings.GetSetting(ctx, keyPromotedUsers); raw != "" {
		_ = json.Unmarshal([]byte(raw), &settings.PromotedUsers)
	}

	if raw, _ := s.settings.GetSetting(ctx, keyPromotedTags); raw != "" {
		_ = json.Unmarshal([]byte(raw), &settings.PromotedTags)
	}

	if settings.PromotedSpaces == nil {
		settings.PromotedSpaces = []string{}
	}

	if settings.PromotedUsers == nil {
		settings.PromotedUsers = []string{}
	}

	if settings.PromotedTags == nil {
		settings.PromotedTags = []string{}
	}

	return settings, nil
}

// UpdateAdminSettings applies a partial update to the platform's onboarding
// configuration. Only fields present in the request body are written; nil
// fields preserve the existing value. All writes are applied atomically via
// UpdateSettingsBatch so the admin never observes a half-applied config.
// Ref: Section 09 — admin Onboarding Settings surface.
func (s *Service) UpdateAdminSettings(ctx context.Context, req *UpdateOnboardingSettingsRequest) error {
	pending := make(map[string][]byte, 6)

	if req.MinTagsFollow != nil {
		v, _ := json.Marshal(*req.MinTagsFollow)
		pending[keyMinTagsFollow] = v
	}

	if req.MinUsersFollow != nil {
		v, _ := json.Marshal(*req.MinUsersFollow)
		pending[keyMinUsersFollow] = v
	}

	if req.MinSpacesJoin != nil {
		v, _ := json.Marshal(*req.MinSpacesJoin)
		pending[keyMinSpacesJoin] = v
	}

	if req.PromotedSpaces != nil {
		v, _ := json.Marshal(*req.PromotedSpaces)
		pending[keyPromotedSpaces] = v
	}

	if req.PromotedUsers != nil {
		v, _ := json.Marshal(*req.PromotedUsers)
		pending[keyPromotedUsers] = v
	}

	if req.PromotedTags != nil {
		v, _ := json.Marshal(*req.PromotedTags)
		pending[keyPromotedTags] = v
	}

	if len(pending) == 0 {
		return nil
	}

	if err := s.settings.UpdateSettingsBatch(ctx, pending); err != nil {
		return fmt.Errorf("updating onboarding settings: %w", err)
	}

	return nil
}
