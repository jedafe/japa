package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// scoreFactorStore provides persistence for comparison score factors.
type scoreFactorStore struct {
	read  *gen.Queries
	write *gen.Queries
}

// newScoreFactorStore creates a scoreFactorStore backed by the supplied database pools.
func newScoreFactorStore(database *db.DB) *scoreFactorStore {
	return &scoreFactorStore{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// create inserts a new score factor and returns the populated domain model.
func (s *scoreFactorStore) create(ctx context.Context, in ScoreFactorInput) (ScoreFactor, error) {
	cfg := in.Config
	if cfg.IdealTexts == nil {
		cfg.IdealTexts = []string{}
	}

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("marshaling score factor config: %w", err)
	}

	row, err := s.write.CreateScoreFactor(ctx, gen.CreateScoreFactorParams{

		ComparisonTypeID: stringToUUID(ptrString(in.ComparisonTypeID)),
		FactorKey:        in.FactorKey,
		Label:            in.Label,
		Description:      ptrToNullString(in.Description),
		DimensionSlug:    in.DimensionSlug,
		DataSource:       in.DataSource,
		DefaultWeight:    numericFromFloat(in.DefaultWeight),
		Config:           cfgBytes,
		IsSystem:         in.IsSystem,
	})
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("creating score factor: %w", err)
	}

	return scoreFactorFromRow(row)
}

// listByType returns score factors for a comparison type, optionally including global factors.
func (s *scoreFactorStore) listByType(ctx context.Context, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error) {
	include := &includeGlobal

	rows, err := s.read.ListScoreFactorsByType(ctx, gen.ListScoreFactorsByTypeParams{

		ComparisonTypeID: comparisonTypeID,
		IncludeGlobal:    include,
		DataSource:       nil,
	})
	if err != nil {
		return nil, fmt.Errorf("listing score factors by type: %w", err)
	}

	factors := make([]ScoreFactor, 0, len(rows))

	for _, row := range rows {
		f, err := scoreFactorFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("mapping score factor row: %w", err)
		}

		factors = append(factors, f)
	}

	return factors, nil
}

// get returns a score factor by id and tenant.
func (s *scoreFactorStore) get(ctx context.Context, id pgtype.UUID) (ScoreFactor, error) {
	row, err := s.read.GetScoreFactor(ctx,
		id,
	)
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("getting score factor: %w", err)
	}

	return scoreFactorFromRow(row)
}

// update modifies a score factor and returns the updated domain model.
func (s *scoreFactorStore) update(ctx context.Context, id pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error) {
	cfg := in.Config
	if cfg.IdealTexts == nil {
		cfg.IdealTexts = []string{}
	}

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("marshaling score factor config: %w", err)
	}

	row, err := s.write.UpdateScoreFactor(ctx, gen.UpdateScoreFactorParams{
		ComparisonTypeID: stringToUUID(ptrString(in.ComparisonTypeID)),
		FactorKey:        in.FactorKey,
		Label:            in.Label,
		Description:      ptrToNullString(in.Description),
		DimensionSlug:    in.DimensionSlug,
		DataSource:       in.DataSource,
		DefaultWeight:    numericFromFloat(in.DefaultWeight),
		Config:           cfgBytes,
		IsSystem:         in.IsSystem,
		ID:               id,
	})
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("updating score factor: %w", err)
	}

	return scoreFactorFromRow(row)
}

// delete removes a score factor.
func (s *scoreFactorStore) delete(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteScoreFactor(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting score factor: %w", err)
	}

	return nil
}

// getByKey fetches a factor by tenant, comparison type, and factor key. A nil
// comparison type selects the global factor for that key.
//
// The generated GetScoreFactorByKey query uses `comparison_type_id = $2`, which
// cannot match NULL when $2 is NULL, so global lookups are routed through
// ListScoreFactorsByType with includeGlobal=true.
func (s *scoreFactorStore) getByKey(ctx context.Context, comparisonTypeID pgtype.UUID, factorKey string) (ScoreFactor, error) {
	if !comparisonTypeID.Valid {
		factors, err := s.listByType(ctx, comparisonTypeID, true)
		if err != nil {
			return ScoreFactor{}, fmt.Errorf("getting score factor by key: %w", err)
		}

		for _, f := range factors {
			if f.FactorKey == factorKey {
				return f, nil
			}
		}

		return ScoreFactor{}, pgx.ErrNoRows
	}

	row, err := s.read.GetScoreFactorByKey(ctx, gen.GetScoreFactorByKeyParams{

		ComparisonTypeID: comparisonTypeID,
		FactorKey:        factorKey,
	})
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("getting score factor by key: %w", err)
	}

	return scoreFactorFromRow(row)
}

// ScoreFactorFromRow converts a generated ComparisonScoreFactor row into the domain model.
// Exported so the scoring subpackage can map persisted factors without duplicating logic.
func ScoreFactorFromRow(row gen.ComparisonScoreFactor) (ScoreFactor, error) {
	return scoreFactorFromRow(row)
}

// scoreFactorFromRow converts a generated ComparisonScoreFactor row into the domain model.
func scoreFactorFromRow(row gen.ComparisonScoreFactor) (ScoreFactor, error) {
	var cfg ScoreFactorConfig
	if len(row.Config) > 0 && string(row.Config) != "null" {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return ScoreFactor{}, fmt.Errorf("unmarshaling score factor config: %w", err)
		}
	}

	return ScoreFactor{
		ID: row.ID.String(),

		ComparisonTypeID: uuidPtr(row.ComparisonTypeID),
		FactorKey:        row.FactorKey,
		Label:            row.Label,
		Description:      ptrString(row.Description),
		DimensionSlug:    uuidPtrFromText(row.DimensionSlug),
		DataSource:       row.DataSource,
		DefaultWeight:    numericToFloat64(row.DefaultWeight),
		Config:           cfg,
		Version:          int(row.Version),
		IsSystem:         row.IsSystem,
		CreatedAt:        row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:        row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// numericFromFloat converts a float64 to a pgtype.Numeric.
func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))

	return n
}

// numericToFloat64 converts a pgtype.Numeric to float64, returning 0 for NULL.
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

// uuidPtrFromText returns a pointer to a non-empty string, or nil.
func uuidPtrFromText(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}

	ss := *s

	return &ss
}
