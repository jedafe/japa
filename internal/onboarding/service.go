package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides onboarding business logic with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	settings *config.Settings
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, settings *config.Settings) *Service {
	return &Service{
		store:    NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:       database,
		settings: settings,
	}
}

// GetState returns the current onboarding state for the given user.
// If no state exists yet, returns a default starting state (step 1).
func (s *Service) GetState(ctx context.Context, tenantID, userID pgtype.UUID) (*OnboardingStatusResponse, error) {
	row, err := s.store.getState(ctx, tenantID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &OnboardingStatusResponse{
				CurrentStep:   1,
				Completed:     false,
				StepData:      json.RawMessage(`{}`),
				TourDismissed: false,
			}, nil
		}

		return nil, fmt.Errorf("getting onboarding state: %w", err)
	}

	return &OnboardingStatusResponse{
		CurrentStep:   int(row.CurrentStep),
		Completed:     row.Completed,
		StepData:      row.StepData,
		TourDismissed: row.TourDismissed,
		CreatedAt:     row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// UpdateStep saves step data and advances the current step number.
func (s *Service) UpdateStep(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateStepRequest) error {
	// Read current state to merge step_data.
	current, err := s.store.getState(ctx, tenantID, userID)

	var currentStep int32 = 1

	var completed bool

	existingData := json.RawMessage(`{}`)

	var tourDismissed bool

	if err == nil {
		currentStep = current.CurrentStep
		completed = current.Completed
		existingData = current.StepData
		tourDismissed = current.TourDismissed
	}

	// Merge step_data: existing + new data (new keys overwrite old).
	merged := mergeJSON(existingData, req.StepData)

	// Advance to the requested step or next if step is 0/omitted.
	newStep := req.Step
	if newStep <= 0 {
		newStep = int(currentStep) + 1
	}

	if newStep > math.MaxInt32 {
		newStep = math.MaxInt32
	}

	if err := s.store.upsertState(ctx, tenantID, userID, int32(newStep), completed, merged, tourDismissed); err != nil { //nolint:gosec // G115: bounded by math.MaxInt32 check above
		return fmt.Errorf("updating onboarding step: %w", err)
	}

	return nil
}

// Complete marks onboarding as completed for the given user.
func (s *Service) Complete(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.store.completeOnboarding(ctx, tenantID, userID); err != nil {
		return fmt.Errorf("completing onboarding: %w", err)
	}

	return nil
}

// DismissTour marks the product tour as dismissed.
func (s *Service) DismissTour(ctx context.Context, tenantID, userID pgtype.UUID) error {
	if err := s.store.dismissTour(ctx, tenantID, userID); err != nil {
		return fmt.Errorf("dismissing tour: %w", err)
	}

	return nil
}

// GetRecommendations returns recommended spaces and users based on geo match,
// tag overlap, and reputation. Falls back to popular items when no personalized
// signals are available.
func (s *Service) GetRecommendations(ctx context.Context, tenantID, userID pgtype.UUID) ([]OnboardingRecommendation, error) {
	// Parse step_data to extract interests/tags and location for matching.
	state, err := s.store.getState(ctx, tenantID, userID)
	if err != nil {
		return s.getPopularItems(ctx, tenantID)
	}

	stepData := parseStepData(state.StepData)
	userTags := stepData.Interests
	userLocation := stepData.CurrentLocation

	recs := make([]OnboardingRecommendation, 0)

	// Query spaces with tag overlap.
	spaceRecs, err := s.recommendSpacesByTags(ctx, tenantID, userID, userTags)
	if err == nil {
		recs = append(recs, spaceRecs...)
	}

	// Query spaces/users with geo match.
	geoRecs, err := s.recommendByGeo(ctx, tenantID, userID, userLocation)
	if err == nil {
		recs = append(recs, geoRecs...)
	}

	// If no personalized recs, fall back to popular.
	if len(recs) == 0 {
		return s.getPopularItems(ctx, tenantID)
	}

	return recs, nil
}

// stepDataFields holds the extracted onboarding step data.
type stepDataFields struct {
	Interests       []string `json:"interests"`
	CurrentLocation string   `json:"current_location"`
	Destination     string   `json:"destination"`
	Name            string   `json:"name"`
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
func (s *Service) recommendSpacesByTags(ctx context.Context, tenantID, _ pgtype.UUID, tags []string) ([]OnboardingRecommendation, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	// Build an ILIKE pattern for each tag so the SQL can pre-filter spaces
	// whose name or description mentions at least one of the user's interests.
	// Build parameterized ILIKE clauses — each tag becomes its own $N placeholder.
	// fmt.Sprintf is used only for the parenthetical OR grouping, not for values.
	likePatterns := make([]string, 0, len(tags))
	args := []any{tenantID}

	for i, tag := range tags {
		args = append(args, "%"+strings.ToLower(tag)+"%")
		likePatterns = append(likePatterns, fmt.Sprintf("(LOWER(s.name) LIKE $%d OR LOWER(s.description) LIKE $%d)", i+2, i+2))
	}

	query := fmt.Sprintf(`
		SELECT s.id, s.name, s.slug, s.avatar_url, s.description
		FROM spaces s
		WHERE s.tenant_id = $1
		  AND s.status = 'active'
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

		var avatarURL, description pgtype.Text
		if err := rows.Scan(&id, &name, &slug, &avatarURL, &description); err != nil {
			continue
		}
		// Simple tag overlap scoring based on description matching.
		score := tagOverlapScore(description.String, tags)
		if score > 0 {
			recs = append(recs, OnboardingRecommendation{
				Type:      "space",
				ID:        id.String,
				Name:      name.String,
				Slug:      slug.String,
				AvatarURL: avatarURL.String,
				Reason:    "tag_overlap",
				Score:     score,
			})
		}
	}

	return recs, nil
}

// recommendByGeo finds spaces/users matching the user's location.
func (s *Service) recommendByGeo(ctx context.Context, tenantID, _ pgtype.UUID, location string) ([]OnboardingRecommendation, error) {
	if location == "" {
		return nil, nil
	}

	rows, err := s.db.Reader.Query(ctx, `
		SELECT s.id, s.name, s.slug, s.avatar_url
		FROM spaces s
		WHERE s.tenant_id = $1
		  AND s.status = 'active'
		  AND LOWER(s.name) LIKE '%' || LOWER($2) || '%'
		ORDER BY s.follower_count DESC
		LIMIT 5
	`, tenantID, location)
	if err != nil {
		return nil, fmt.Errorf("querying spaces for geo match: %w", err)
	}
	defer rows.Close()

	var recs []OnboardingRecommendation

	for rows.Next() {
		var id, name, slug, avatarURL pgtype.Text
		if err := rows.Scan(&id, &name, &slug, &avatarURL); err != nil {
			continue
		}

		recs = append(recs, OnboardingRecommendation{
			Type:      "space",
			ID:        id.String,
			Name:      name.String,
			Slug:      slug.String,
			AvatarURL: avatarURL.String,
			Reason:    "geo_match",
			Score:     0.8,
		})
	}

	return recs, nil
}

// getPopularItems returns popular spaces/users as a fallback.
func (s *Service) getPopularItems(ctx context.Context, tenantID pgtype.UUID) ([]OnboardingRecommendation, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT s.id, s.name, s.slug, s.avatar_url
		FROM spaces s
		WHERE s.tenant_id = $1
		  AND s.status = 'active'
		ORDER BY s.follower_count DESC
		LIMIT 6
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying popular spaces: %w", err)
	}
	defer rows.Close()

	var recs []OnboardingRecommendation

	for rows.Next() {
		var id, name, slug, avatarURL pgtype.Text
		if err := rows.Scan(&id, &name, &slug, &avatarURL); err != nil {
			continue
		}

		recs = append(recs, OnboardingRecommendation{
			Type:      "space",
			ID:        id.String,
			Name:      name.String,
			Slug:      slug.String,
			AvatarURL: avatarURL.String,
			Reason:    "reputation",
			Score:     0.5,
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
)

// GetAdminSettings reads onboarding configuration from system_settings.
func (s *Service) GetAdminSettings(ctx context.Context, tenantID pgtype.UUID) (*OnboardingAdminSettings, error) {
	minTags, _ := s.settings.GetSettingInt(ctx, tenantID, keyMinTagsFollow)
	minUsers, _ := s.settings.GetSettingInt(ctx, tenantID, keyMinUsersFollow)
	minSpaces, _ := s.settings.GetSettingInt(ctx, tenantID, keyMinSpacesJoin)

	promotedSpaces, _ := s.settings.GetSetting(ctx, tenantID, keyPromotedSpaces)
	promotedUsers, _ := s.settings.GetSetting(ctx, tenantID, keyPromotedUsers)
	promotedTags, _ := s.settings.GetSetting(ctx, tenantID, keyPromotedTags)

	var spacesArr, usersArr, tagsArr []string
	if promotedSpaces != "" {
		_ = json.Unmarshal([]byte(promotedSpaces), &spacesArr)
	}

	if promotedUsers != "" {
		_ = json.Unmarshal([]byte(promotedUsers), &usersArr)
	}

	if promotedTags != "" {
		_ = json.Unmarshal([]byte(promotedTags), &tagsArr)
	}

	// Defaults per Section 09
	if minTags == 0 {
		minTags = 3
	}

	if minUsers == 0 {
		minUsers = 2
	}

	if minSpaces == 0 {
		minSpaces = 2
	}

	return &OnboardingAdminSettings{
		MinTagsFollow:  minTags,
		MinUsersFollow: minUsers,
		MinSpacesJoin:  minSpaces,
		PromotedSpaces: spacesArr,
		PromotedUsers:  usersArr,
		PromotedTags:   tagsArr,
	}, nil
}

// UpdateAdminSettings writes onboarding configuration to system_settings.
func (s *Service) UpdateAdminSettings(ctx context.Context, tenantID pgtype.UUID, req *UpdateOnboardingSettingsRequest) error {
	if req.MinTagsFollow != nil {
		_ = s.settings.SetSetting(ctx, tenantID, keyMinTagsFollow, strconv.Itoa(*req.MinTagsFollow))
	}

	if req.MinUsersFollow != nil {
		_ = s.settings.SetSetting(ctx, tenantID, keyMinUsersFollow, strconv.Itoa(*req.MinUsersFollow))
	}

	if req.MinSpacesJoin != nil {
		_ = s.settings.SetSetting(ctx, tenantID, keyMinSpacesJoin, strconv.Itoa(*req.MinSpacesJoin))
	}

	if req.PromotedSpaces != nil {
		b, _ := json.Marshal(*req.PromotedSpaces)
		_ = s.settings.SetSetting(ctx, tenantID, keyPromotedSpaces, string(b))
	}

	if req.PromotedUsers != nil {
		b, _ := json.Marshal(*req.PromotedUsers)
		_ = s.settings.SetSetting(ctx, tenantID, keyPromotedUsers, string(b))
	}

	if req.PromotedTags != nil {
		b, _ := json.Marshal(*req.PromotedTags)
		_ = s.settings.SetSetting(ctx, tenantID, keyPromotedTags, string(b))
	}

	return nil
}

// ListTemplates returns all onboarding templates for the tenant.
func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID) ([]gen.OnboardingTemplate, error) {
	return s.store.listTemplates(ctx, tenantID)
}

// CreateTemplate creates a new onboarding template.
func (s *Service) CreateTemplate(ctx context.Context, tenantID pgtype.UUID, req *CreateTemplateRequest) (gen.OnboardingTemplate, error) {
	steps, err := json.Marshal(req.Steps)
	if err != nil {
		return gen.OnboardingTemplate{}, fmt.Errorf("marshaling steps: %w", err)
	}

	return s.store.createTemplate(ctx, tenantID, req.Name, req.Description, steps, req.IsActive, false)
}

// UpdateTemplate updates an existing onboarding template.
func (s *Service) UpdateTemplate(ctx context.Context, tenantID, id pgtype.UUID, req *UpdateTemplateRequest) error {
	steps, err := json.Marshal(req.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}

	return s.store.updateTemplate(ctx, tenantID, id, req.Name, req.Description, steps, req.IsActive)
}

// DeleteTemplate deletes an onboarding template.
func (s *Service) DeleteTemplate(ctx context.Context, tenantID, id pgtype.UUID) error {
	return s.store.deleteTemplate(ctx, tenantID, id)
}
