package jce

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// recommendationStore provides persistence for comparison recommendations.
//
// NOTE: the generated sqlc queries for comparison_recommendations include
// tenant_id. The brief signatures omit it, so the store methods below accept
// tenantID as the first parameter to stay consistent with the schema.
type recommendationStore struct {
	read  *gen.Queries
	write *gen.Queries
}

// newRecommendationStore creates a recommendationStore backed by the supplied database pools.
func newRecommendationStore(database *db.DB) *recommendationStore {
	return &recommendationStore{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// listByContent returns recommendations for a comparison and optional profile,
// ordered by priority desc then created_at asc, and truncated to the requested limit.
func (s *recommendationStore) listByContent(ctx context.Context, tenantID, contentID, profileID pgtype.UUID, limit int32) ([]ComparisonRecommendation, error) {
	rows, err := s.read.ListComparisonRecommendationsByContent(ctx, gen.ListComparisonRecommendationsByContentParams{

		ContentID: contentID,
		ProfileID: profileID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing comparison recommendations: %w", err)
	}

	if limit > 0 && int(limit) < len(rows) {
		rows = rows[:limit]
	}

	recommendations := make([]ComparisonRecommendation, 0, len(rows))

	for _, row := range rows {
		r := row
		recommendations = append(recommendations, comparisonRecommendationFromRow(r))
	}

	return recommendations, nil
}

// create inserts a comparison recommendation.
func (s *recommendationStore) create(ctx context.Context, tenantID, contentID, itemID, profileID, factorID pgtype.UUID, text, actionType, actionURL string, priority int32) (ComparisonRecommendation, error) {
	row, err := s.write.CreateComparisonRecommendation(ctx, gen.CreateComparisonRecommendationParams{

		ContentID:  contentID,
		ItemID:     itemID,
		ProfileID:  profileID,
		FactorID:   factorID,
		Text:       text,
		ActionType: ptrToNullString(actionType),
		ActionUrl:  ptrToNullString(actionURL),
		Priority:   priority,
	})
	if err != nil {
		return ComparisonRecommendation{}, fmt.Errorf("creating comparison recommendation: %w", err)
	}

	return comparisonRecommendationFromRow(row), nil
}

// deleteByContent removes recommendations for a comparison and optional profile.
func (s *recommendationStore) deleteByContent(ctx context.Context, tenantID, contentID, profileID pgtype.UUID) error {
	if err := s.write.DeleteComparisonRecommendationsByContent(ctx, gen.DeleteComparisonRecommendationsByContentParams{

		ContentID: contentID,
		ProfileID: profileID,
	}); err != nil {
		return fmt.Errorf("deleting comparison recommendations by content: %w", err)
	}

	return nil
}

// comparisonRecommendationFromRow converts a generated ComparisonRecommendation row into the domain model.
func comparisonRecommendationFromRow(row gen.ComparisonRecommendation) ComparisonRecommendation {
	return ComparisonRecommendation{
		ID:         row.ID.String(),
		ItemID:     uuidPtr(row.ItemID),
		FactorID:   uuidPtr(row.FactorID),
		Text:       row.Text,
		ActionType: row.ActionType,
		ActionURL:  row.ActionUrl,
		Priority:   int(row.Priority),
	}
}
