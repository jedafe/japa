package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// profileStore provides persistence for comparison profiles.
type profileStore struct {
	database *db.DB
	read     *gen.Queries
	write    *gen.Queries
}

// newProfileStore creates a profileStore backed by the supplied database pools.
func newProfileStore(database *db.DB) *profileStore {
	return &profileStore{
		database: database,
		read:     gen.New(database.Reader),
		write:    gen.New(database.Writer),
	}
}

// create inserts a new comparison profile and returns the populated domain model.
func (s *profileStore) create(ctx context.Context, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	return s.createWithTx(ctx, s.write, userID, in)
}

// createWithTx creates a profile inside an existing transaction.
func (s *profileStore) createWithTx(ctx context.Context, q *gen.Queries, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	row, err := q.CreateComparisonProfile(ctx, gen.CreateComparisonProfileParams{

		UserID:         userID,
		Title:          in.Title,
		SourceSpaceID:  stringToUUID(ptrString(in.SourceSpaceID)),
		TargetSpaceIds: stringsToUUIDs(in.TargetSpaceIDs),
		Goals:          marshalProfileGoals(in.Goals),
		Preferences:    marshalProfilePreferences(in.Preferences),
		Demographics:   marshalProfileDemographics(in.Demographics),
		IsDefault:      in.IsDefault,
		IsTemplate:     in.IsTemplate,
	})
	if err != nil {
		return ComparisonProfile{}, fmt.Errorf("creating comparison profile: %w", err)
	}

	return profileFromRow(row)
}

// listByUser returns a user's profiles (non-template), ordered by default first, then updated_at desc.
func (s *profileStore) listByUser(ctx context.Context, userID pgtype.UUID) ([]ComparisonProfile, error) {
	rows, err := s.read.ListComparisonProfilesByUser(ctx,

		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing comparison profiles by user: %w", err)
	}

	profiles := make([]ComparisonProfile, 0, len(rows))

	for _, row := range rows {
		if row.IsTemplate {
			continue
		}

		p, err := profileFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("mapping comparison profile row: %w", err)
		}

		profiles = append(profiles, p)
	}

	return profiles, nil
}

// listByPlatform returns all non-deleted profiles for the platform, ordered by default first, then updated_at desc.
func (s *profileStore) listByPlatform(ctx context.Context) ([]ComparisonProfile, error) {
	rows, err := s.read.ListComparisonProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing comparison profiles by platform: %w", err)
	}

	profiles := make([]ComparisonProfile, 0, len(rows))

	for _, row := range rows {
		p, err := profileFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("mapping comparison profile row: %w", err)
		}

		profiles = append(profiles, p)
	}

	return profiles, nil
}

// listTemplates returns all platform-level profile templates, ordered by title.
func (s *profileStore) listTemplates(ctx context.Context) ([]ComparisonProfile, error) {
	rows, err := s.read.ListComparisonProfileTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing comparison profile templates: %w", err)
	}

	profiles := make([]ComparisonProfile, 0, len(rows))

	for _, row := range rows {
		p, err := profileFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("mapping comparison profile template row: %w", err)
		}

		profiles = append(profiles, p)
	}

	return profiles, nil
}

// get returns a profile by id and platform.
func (s *profileStore) get(ctx context.Context, id pgtype.UUID) (ComparisonProfile, error) {
	row, err := s.read.GetComparisonProfile(ctx,
		id,
	)
	if err != nil {
		return ComparisonProfile{}, fmt.Errorf("getting comparison profile: %w", err)
	}

	return profileFromRow(row)
}

// update modifies a profile and returns the updated domain model.
func (s *profileStore) update(ctx context.Context, id, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	return s.updateWithTx(ctx, s.write, id, userID, in)
}

// updateWithTx updates a profile inside an existing transaction.
func (s *profileStore) updateWithTx(ctx context.Context, q *gen.Queries, id, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	_ = userID // userID is enforced at the service/handler layer; not used by the update query.

	row, err := q.UpdateComparisonProfile(ctx, gen.UpdateComparisonProfileParams{
		Title:          in.Title,
		SourceSpaceID:  stringToUUID(ptrString(in.SourceSpaceID)),
		TargetSpaceIds: stringsToUUIDs(in.TargetSpaceIDs),
		Goals:          marshalProfileGoals(in.Goals),
		Preferences:    marshalProfilePreferences(in.Preferences),
		Demographics:   marshalProfileDemographics(in.Demographics),
		IsDefault:      in.IsDefault,
		IsTemplate:     in.IsTemplate,
		ID:             id,
	})
	if err != nil {
		return ComparisonProfile{}, fmt.Errorf("updating comparison profile: %w", err)
	}

	return profileFromRow(row)
}

// delete soft-deletes a profile.
func (s *profileStore) delete(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteComparisonProfile(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting comparison profile: %w", err)
	}

	return nil
}

// setDefault clears any existing default for the user and marks the target profile as default.
func (s *profileStore) setDefault(ctx context.Context, id, userID pgtype.UUID) error {
	return s.setDefaultWithTx(ctx, s.write, id, userID)
}

// setDefaultWithTx clears existing defaults and marks the target profile as the
// default inside an existing transaction.
func (s *profileStore) setDefaultWithTx(ctx context.Context, q *gen.Queries, id, userID pgtype.UUID) error {
	if err := q.ClearDefaultProfileForUser(ctx, gen.ClearDefaultProfileForUserParams{

		UserID:    userID,
		ExcludeID: id,
	}); err != nil {
		return fmt.Errorf("clearing default profile: %w", err)
	}

	existing, err := q.GetComparisonProfile(ctx,
		id,
	)
	if err != nil {
		return fmt.Errorf("getting profile to mark default: %w", err)
	}

	if _, err := q.UpdateComparisonProfile(ctx, gen.UpdateComparisonProfileParams{
		Title:          existing.Title,
		SourceSpaceID:  existing.SourceSpaceID,
		TargetSpaceIds: existing.TargetSpaceIds,
		Goals:          existing.Goals,
		Preferences:    existing.Preferences,
		Demographics:   existing.Demographics,
		IsDefault:      true,
		IsTemplate:     existing.IsTemplate,
		ID:             id,
	}); err != nil {
		return fmt.Errorf("setting profile default flag: %w", err)
	}

	return nil
}

// countBySourceSpace returns the number of non-template profiles per source space.
func (s *profileStore) countBySourceSpace(ctx context.Context, userID pgtype.UUID) ([]ProfileStat, error) {
	rows, err := s.read.CountComparisonProfilesBySourceSpace(ctx,

		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("counting comparison profiles by source space: %w", err)
	}

	stats := make([]ProfileStat, 0, len(rows))

	for _, row := range rows {
		sid := ""
		if row.SourceSpaceID.Valid {
			sid = row.SourceSpaceID.String()
		}

		stats = append(stats, ProfileStat{
			SourceSpaceID:   sid,
			ProfileCount:    row.ProfileCount,
			SourceSpaceName: row.SourceSpaceName,
			SourceSpaceSlug: row.SourceSpaceSlug,
		})
	}

	return stats, nil
}

// ProfileFromRow converts a generated ComparisonProfile row into the domain model.
// Exported so the scoring subpackage can map persisted profiles without duplicating logic.
func ProfileFromRow(row gen.ComparisonProfile) (ComparisonProfile, error) {
	return profileFromRow(row)
}

// profileFromRow converts a generated ComparisonProfile row into the domain model.
func profileFromRow(row gen.ComparisonProfile) (ComparisonProfile, error) {
	var goals []ProfileGoal
	if len(row.Goals) > 0 && string(row.Goals) != "null" {
		if err := json.Unmarshal(row.Goals, &goals); err != nil {
			return ComparisonProfile{}, fmt.Errorf("unmarshaling profile goals: %w", err)
		}
	}

	preferences := make(map[string]float64)
	if len(row.Preferences) > 0 && string(row.Preferences) != "null" {
		if err := json.Unmarshal(row.Preferences, &preferences); err != nil {
			return ComparisonProfile{}, fmt.Errorf("unmarshaling profile preferences: %w", err)
		}
	}

	var demographics ProfileDemographics
	if len(row.Demographics) > 0 && string(row.Demographics) != "null" {
		if err := json.Unmarshal(row.Demographics, &demographics); err != nil {
			return ComparisonProfile{}, fmt.Errorf("unmarshaling profile demographics: %w", err)
		}
	}

	targetSpaceIDs := make([]string, 0, len(row.TargetSpaceIds))

	for _, u := range row.TargetSpaceIds {
		if u.Valid {
			targetSpaceIDs = append(targetSpaceIDs, u.String())
		}
	}

	return ComparisonProfile{
		ID: row.ID.String(),

		UserID:         uuidPtr(row.UserID),
		Title:          row.Title,
		SourceSpaceID:  uuidPtr(row.SourceSpaceID),
		TargetSpaceIDs: targetSpaceIDs,
		Goals:          goals,
		Preferences:    preferences,
		Demographics:   demographics,
		IsDefault:      row.IsDefault,
		IsTemplate:     row.IsTemplate,
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// marshalProfileGoals serializes profile goals for the goals JSONB column.
func marshalProfileGoals(goals []ProfileGoal) []byte {
	if goals == nil {
		return []byte("[]")
	}

	b, err := json.Marshal(goals)
	if err != nil {
		return []byte("[]")
	}

	return b
}

// marshalProfilePreferences serializes preferences for the preferences JSONB column.
func marshalProfilePreferences(p map[string]float64) []byte {
	if p == nil {
		return []byte("{}")
	}

	b, err := json.Marshal(p)
	if err != nil {
		return []byte("{}")
	}

	return b
}

// marshalProfileDemographics serializes demographics for the demographics JSONB column.
func marshalProfileDemographics(d ProfileDemographics) []byte {
	b, err := json.Marshal(d)
	if err != nil {
		return []byte("{}")
	}

	return b
}

// stringToUUID parses a UUID string into a pgtype.UUID; an empty or invalid string returns an invalid UUID.
func stringToUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if s == "" {
		return u
	}

	if err := u.Scan(s); err != nil {
		return u
	}

	return u
}

// stringsToUUIDs converts a slice of UUID strings into pgtype.UUID values, skipping invalid ones.
// An empty or nil input returns an empty, non-nil slice so the uuid[] column stays non-null.
func stringsToUUIDs(ss []string) []pgtype.UUID {
	uuids := make([]pgtype.UUID, 0, len(ss))

	for _, s := range ss {
		u := stringToUUID(s)
		if u.Valid {
			uuids = append(uuids, u)
		}
	}

	return uuids
}
