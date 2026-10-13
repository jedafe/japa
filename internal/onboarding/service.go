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
	"github.com/jedafe/japa/internal/japastage"
	"github.com/jedafe/japa/internal/visitor"
)

// Follower is the subset of the follow-system service that onboarding needs
// to persist tag/space/user selections from the wizard into real follow rows.
type Follower interface {
	EnsureFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error)
}

// JourneyClassWriter merges journey-class values (primary_goal → japa_intent,
// japa_status → japa_stage) into the canonical user_profiles columns.
// *japastage.Service satisfies it (ADR-128).
type JourneyClassWriter interface {
	ApplyJourneyClass(ctx context.Context, userID pgtype.UUID, rawIntent, stage, source string) error
}

// Service provides onboarding business logic with reader/writer pool separation.
type Service struct {
	store         *Store
	db            *db.DB
	settings      *config.Settings
	follower      Follower
	journeyWriter JourneyClassWriter
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

// SetJourneyClassWriter wires the journey-class writer (japastage.Service) so
// wizard answers promote into japa_intent / japa_stage (ADR-128). Nil skips
// the promotion writes.
func (s *Service) SetJourneyClassWriter(w JourneyClassWriter) {
	s.journeyWriter = w
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
// Steps 4–6 are the per-type follow steps (users, spaces, sources).
const totalSteps = 7

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
	// Without this, a malicious client could send step: 999 to bypass the wizard flow.
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

	// ADR-128: promote the journey answers into the canonical fields so
	// intent-aware personalization (menus, recommendations, space discovery)
	// is active from onboarding rather than only after the member discovers
	// /japa-stage/me. Step 1 japa_status feeds the japa_status custom field
	// and the japa_stage column; Step 2 primary_goal feeds japa_intent and
	// the purpose custom field. All writes are best-effort — failures log
	// and never block onboarding.
	if submitStep == 1 && fields.JapaStatus != "" {
		if err := s.setProfileCustomField(ctx, userID, "japa_status", fields.JapaStatus); err != nil {
			slog.Warn("onboarding: failed to save japa_status custom field", "error", err)
		}
	}

	if submitStep == 2 {
		if purpose := japastage.PurposeForIntent(fields.PrimaryGoal); purpose != "" {
			if err := s.setProfileCustomField(ctx, userID, "purpose", purpose); err != nil {
				slog.Warn("onboarding: failed to save purpose custom field", "error", err)
			}
		}
	}

	if s.journeyWriter != nil && (submitStep == 1 || submitStep == 2) {
		stage := visitor.NormalizeJourneyStage(fields.JapaStatus)
		if err := s.journeyWriter.ApplyJourneyClass(ctx, userID, fields.PrimaryGoal, stage, "onboarding"); err != nil {
			slog.Warn("onboarding: failed to apply journey class", "error", err)
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
	//   1. Complete() checks CurrentStep >= totalSteps, but the last PUT
	//      stores totalSteps-1, so completion always failed.
	//   2. On refresh, GetState returned the step just completed, sending
	//      the user backwards.
	newStep := submitStep + 1

	// Clamp to totalSteps — the last step is the terminal step; advancing
	// past it is handled by POST /onboarding/complete, not by advancing
	// current_step.
	if newStep > totalSteps {
		newStep = totalSteps
	}

	if err := s.store.upsertState(ctx, userID, int32(newStep), completed, merged, tourDismissed, tourDismissals); err != nil { //nolint:gosec // G115: newStep is bounded by totalSteps (7)
		return fmt.Errorf("updating onboarding step: %w", err)
	}

	return nil
}

// persistFollows turns Step 3 tag slugs, Step 4/5 recommendation IDs, and
// Step 6 source IDs into actual follow rows using the follow-system service.
// It is best-effort and never blocks onboarding. User/space recommendations
// are resolved to their entity type from the same ranked list used by the
// wizard; sources are followed directly by ID as the "source" entity type.
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

	// Step 6: follow selected sources by ID. Sources come from the public
	// directory (GET /api/v1/sources), not the recommendation engine, so the
	// entity type is known directly — no rec-list resolution needed.
	for _, rawID := range fields.SourceFollows {
		var id pgtype.UUID
		if err := id.Scan(rawID); err != nil {
			slog.Warn("onboarding: invalid source id in source_follows", "id", rawID, "error", err)
			continue
		}
		if _, err := s.follower.EnsureFollow(ctx, userID, "source", id); err != nil {
			slog.Warn("onboarding: failed to follow source", "entity_id", rawID, "error", err)
		}
	}

	// Steps 4/5: follow selected recommendations by ID. We resolve type from
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
// a minimum requirement (step 3 = tags, step 4 = users, step 5 = spaces).
// Returns nil if the step has no minimum (no auto-fulfill needed). Per
// Section 09, the user always ends up meeting the baseline density target —
// they just choose between manual curation or accepting defaults.
func (s *Service) autoFulfillSkip(ctx context.Context, userID pgtype.UUID, step int) json.RawMessage {
	switch step {
	case 3:
		// Step 3: Interests/Tags — auto-pick min_tags_follow promoted tags.
		minTags := s.getMinTagsFollow(ctx)
		if minTags <= 0 {
			return nil
		}

		promotedTagsJSON, _ := s.settings.GetSetting(ctx, keyPromotedTags)

		var promoted []string

		if promotedTagsJSON != "" {
			_ = json.Unmarshal([]byte(promotedTagsJSON), &promoted)
		}

		if len(promoted) >= minTags {
			promoted = promoted[:minTags]
		}

		if len(promoted) == 0 {
			return nil // no promoted tags to auto-fulfill with
		}

		// onboarding.promoted_tags stores tag UUIDs (per the settings
		// catalog), but the tags/interests step_data fields carry slugs —
		// persistFollows resolves them via GetTagBySlug. Resolve each UUID
		// to its slug here; entries that fail to resolve (or legacy
		// slug-valued entries that already parse as slugs) are kept as-is
		// so the slug path still covers them.
		promotedSlugs := make([]string, 0, len(promoted))
		for _, entry := range promoted {
			var id pgtype.UUID
			if err := id.Scan(entry); err == nil && id.Valid {
				if tag, terr := s.store.read.GetTagByID(ctx, id); terr == nil {
					promotedSlugs = append(promotedSlugs, tag.Slug)
					continue
				}
			}
			promotedSlugs = append(promotedSlugs, entry)
		}

		data, _ := json.Marshal(map[string]any{
			"tags":      promotedSlugs,
			"interests": promotedSlugs,
			"skipped":   true,
		})

		return data

	case 4:
		// Step 4: Recommended Users — auto-follow min_users_follow users
		// from the recommendation engine.
		return s.autoFulfillFollows(ctx, userID, "user", s.getMinUsersFollow(ctx))

	case 5:
		// Step 5: Recommended Spaces — auto-follow min_spaces_join spaces
		// from the recommendation engine.
		return s.autoFulfillFollows(ctx, userID, "space", s.getMinSpacesJoin(ctx))

	default:
		// Steps 1, 2, 6, 7 have no minimum — skip is a no-op pass-through.
		return nil
	}
}

// autoFulfillFollows builds the auto-fulfilled step_data for a follow-type
// step (4 = users, 5 = spaces): the first `min` recommendations of the given
// entity type from the ranked list. Returns nil when the step has no minimum
// or no recommendations are available.
func (s *Service) autoFulfillFollows(ctx context.Context, userID pgtype.UUID, recType string, min int) json.RawMessage {
	if min <= 0 {
		return nil
	}

	recs, err := s.GetRecommendations(ctx, userID)
	if err != nil || len(recs) == 0 {
		return nil
	}

	follows := make([]string, 0, min)

	for _, r := range recs {
		if r.Type == recType {
			follows = append(follows, r.ID)
			if len(follows) >= min {
				break
			}
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
// Per Section 09, completion is the terminal action of the wizard —
// a user must have reached the final step before they can complete. This
// prevents a malicious client from calling POST /onboarding/complete
// immediately after signup and bypassing the baseline density target.
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
	// The wizard requires the user to reach the terminal step before
	// completing.
	if state.CurrentStep < totalSteps {
		return fmt.Errorf("%w: onboarding not complete (current step %d, need %d)", errpkg.ErrInvalidStatusTransition, state.CurrentStep, totalSteps)
	}

	if err := s.store.completeOnboarding(ctx, userID); err != nil {
		return fmt.Errorf("completing onboarding: %w", err)
	}

	return nil
}

// DismissTour marks the product tour as dismissed for the given surface.
// The surface is a short slug ("main", "docs", "academy"); an empty surface
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
	// Legacy alias: the Learn surface was renamed to Academy.
	if surface == "learn" {
		return "academy"
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
//  4. Popular fill (spaces + users) — always appended beneath the
//     personalized tiers so promoted/pinned entries get precedence without
//     hiding the rest of the directory.
//
// Results are deduplicated by (type, id) and ordered by tier.
func (s *Service) GetRecommendations(ctx context.Context, userID pgtype.UUID) ([]OnboardingRecommendation, error) {
	// Parse step_data to extract interests/tags and location for matching.
	state, err := s.store.getState(ctx, userID)
	if err != nil {
		// BUG-008: only fall back to popular on "no state yet" (ErrNoRows);
		// return real DB/infra errors instead of masking them.
		if errors.Is(err, pgx.ErrNoRows) {
			return s.getPopularItems(ctx, userID)
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

	// Merge personalized tiers in order, deduplicating by (type, id).
	merged := mergeRecommendations(promotedRecs, geoRecs, tagRecs)

	// Tier 4: always append the popular fill (spaces + users). Earlier
	// tiers keep precedence — dedup by (type, id) means a promoted or
	// geo/tag-matched entity occupies its higher slot and is not repeated.
	// Filling unconditionally matters because the geo/tag tiers only fire
	// when step_data already holds location/interests, and a promoted-only
	// list would otherwise leave the per-type follow steps nearly empty.
	popular, err := s.getPopularItems(ctx, userID)
	if err != nil {
		slog.Warn("onboarding: popular fill failed", "error", err)

		popular = nil
	}

	merged = mergeRecommendations(merged, popular)

	// ADR-128: demote space types irrelevant to the member's declared journey
	// class — Step 2 primary_goal resolves via geo_personalization.intent_matrix
	// demote_space_types. Demoted spaces move to the tail of the space
	// results; they stay discoverable.
	if rule := visitor.IntentRuleFor(ctx, s.settings, japastage.NormalizeIntent(stepData.PrimaryGoal)); len(rule.DemoteSpaceTypes) > 0 {
		if out, err := s.demoteSpaceTypes(ctx, merged, rule.DemoteSpaceTypes); err == nil {
			merged = out
		} else {
			slog.Warn("onboarding: space-type demotion failed", "error", err)
		}
	}

	return merged, nil
}

// demoteSpaceTypes stable-sorts recs so space entries whose space_types.slug
// is in demote move to the tail while everything else keeps its tier order
// (ADR-128). One batch lookup resolves the type slugs.
func (s *Service) demoteSpaceTypes(ctx context.Context, recs []OnboardingRecommendation, demote []string) ([]OnboardingRecommendation, error) {
	demoteSet := make(map[string]bool, len(demote))
	for _, d := range demote {
		demoteSet[d] = true
	}

	var spaceIDs []pgtype.UUID
	for _, r := range recs {
		if r.Type != "space" {
			continue
		}
		var id pgtype.UUID
		if err := id.Scan(r.ID); err == nil {
			spaceIDs = append(spaceIDs, id)
		}
	}

	if len(spaceIDs) == 0 {
		return recs, nil
	}

	rows, err := s.store.read.ListSpaceTypeSlugsByIDs(ctx, spaceIDs)
	if err != nil {
		return nil, fmt.Errorf("listing space type slugs: %w", err)
	}

	demoted := make(map[string]bool, len(rows))
	for _, row := range rows {
		if demoteSet[row.SpaceTypeSlug] {
			demoted[row.ID.String()] = true
		}
	}

	var head, tail []OnboardingRecommendation
	for _, r := range recs {
		if r.Type == "space" && demoted[r.ID] {
			tail = append(tail, r)
		} else {
			head = append(head, r)
		}
	}

	return append(head, tail...), nil
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
//	Step 4: follows (array of user IDs)
//	Step 5: follows (array of space IDs)
//	Step 6: source_follows (array of source IDs from the public directory)
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
	// SourceFollows holds source IDs selected on the sources step. Sources
	// are not part of the recommendation engine — the step lists the public
	// directory — so they travel under their own key and persist as
	// entity_type "source" follows.
	SourceFollows []string `json:"source_follows"`
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
func (s *Service) getPopularItems(ctx context.Context, userID pgtype.UUID) ([]OnboardingRecommendation, error) {
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

	users, err := s.recommendPopularUsers(ctx, userID)
	if err != nil {
		slog.Warn("onboarding: popular-users fallback failed", "error", err)
	} else {
		recs = append(recs, users...)
	}

	return recs, nil
}

// recommendPopularUsers returns prominent public profiles for the Recommended
// Users step — the reputation tier for the "user" entity type. The filter set
// mirrors ListPublicUsers (active, non-synthetic, search-indexable) so the
// wizard only suggests accounts that also appear in the public directory; the
// requesting user is excluded so they can never be offered to follow
// themselves.
func (s *Service) recommendPopularUsers(ctx context.Context, userID pgtype.UUID) ([]OnboardingRecommendation, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT u.id, u.username, up.display_name, up.avatar_url
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		LEFT JOIN user_privacy_settings ups ON ups.user_id = u.id
		WHERE u.status = 'active'
		  AND u.is_synthetic = false
		  AND u.id <> $1
		  AND COALESCE(ups.search_indexable, true) = true
		ORDER BY u.follower_count DESC NULLS LAST, u.created_at DESC
		LIMIT 8
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying popular users: %w", err)
	}
	defer rows.Close()

	var recs []OnboardingRecommendation

	for rows.Next() {
		var id pgtype.UUID
		var username, displayName, avatarURL pgtype.Text
		if err := rows.Scan(&id, &username, &displayName, &avatarURL); err != nil {
			continue
		}

		name := displayName.String
		if name == "" {
			name = username.String
		}

		recs = append(recs, OnboardingRecommendation{
			Type:      "user",
			ID:        id.String(),
			Name:      name,
			Slug:      username.String,
			AvatarURL: avatarURL.String,
			Reason:    "reputation",
			Score:     0.45,
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
	return s.setProfileCustomField(ctx, userID, "destination_country", strings.ToUpper(strings.TrimSpace(geoCode)))
}

// setProfileCustomField upserts a value_text custom field on the user's
// profile (ADR-064a: entity_type='user_profile', entity_id=profile id).
// Empty values are skipped; the field key must already be seeded in
// custom_fields.
func (s *Service) setProfileCustomField(ctx context.Context, userID pgtype.UUID, key, value string) error {
	if value == "" {
		return nil
	}

	field, err := s.store.read.GetCustomFieldByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("looking up %s custom field: %w", key, err)
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
		ValueText:  &value,
	}); err != nil {
		return fmt.Errorf("upserting %s custom field value: %w", key, err)
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
// the 7-step flow is hardcoded in GetState/the frontend. Template-driven
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
