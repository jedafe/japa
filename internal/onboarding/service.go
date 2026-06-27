package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides onboarding business logic with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:    database,
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
		CreatedAt:     row.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     row.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// UpdateStep saves step data and advances the current step number.
func (s *Service) UpdateStep(ctx context.Context, tenantID, userID pgtype.UUID, req *UpdateStepRequest) error {
	// Read current state to merge step_data.
	current, err := s.store.getState(ctx, tenantID, userID)
	var currentStep int32 = 1
	var completed bool
	var existingData json.RawMessage = json.RawMessage(`{}`)
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

	if err := s.store.upsertState(ctx, tenantID, userID, int32(newStep), completed, merged, tourDismissed); err != nil {
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
	_ = json.Unmarshal(data, &fields)
	return fields
}

// mergeJSON merges two JSON objects, with the second overwriting the first.
func mergeJSON(base, overlay json.RawMessage) json.RawMessage {
	var baseMap, overlayMap map[string]any
	if len(base) > 0 {
		_ = json.Unmarshal(base, &baseMap)
	} else {
		baseMap = make(map[string]any)
	}
	if len(overlay) > 0 {
		_ = json.Unmarshal(overlay, &overlayMap)
	} else {
		overlayMap = make(map[string]any)
	}
	for k, v := range overlayMap {
		baseMap[k] = v
	}
	merged, _ := json.Marshal(baseMap)
	return merged
}

// recommendSpacesByTags finds spaces that overlap with the user's interests.
func (s *Service) recommendSpacesByTags(ctx context.Context, tenantID, userID pgtype.UUID, tags []string) ([]OnboardingRecommendation, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	rows, err := s.db.Reader.Query(ctx, `
		SELECT s.id, s.name, s.slug, s.avatar_url, s.description
		FROM spaces s
		WHERE s.tenant_id = $1
		  AND s.status = 'active'
		ORDER BY s.follower_count DESC
		LIMIT 10
	`, tenantID)
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
func (s *Service) recommendByGeo(ctx context.Context, tenantID, userID pgtype.UUID, location string) ([]OnboardingRecommendation, error) {
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
