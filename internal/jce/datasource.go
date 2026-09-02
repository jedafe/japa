package jce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// validDataSourceKinds is the set of values accepted by comparison_data_sources.source_kind.
var validDataSourceKinds = map[string]bool{
	"official":    true,
	"third_party": true,
	"ai_estimate": true,
}

// Data-source feature flag and permission keys.
const (
	dataSourcesFeatureFlag = "feature.jce.data_sources"
	dataSourceManagePerm   = "jce.data_source.manage"
	dataSourceViewPerm     = "jce.data_source.view"
)

// dataSourceStore provides persistence for comparison_data_sources.
type dataSourceStore struct {
	read  *gen.Queries
	write *gen.Queries
}

// newDataSourceStore creates a dataSourceStore backed by the supplied database pools.
func newDataSourceStore(database *db.DB) *dataSourceStore {
	return &dataSourceStore{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// dataSourceMapped holds the validated, DB-ready fields of a DataSourceInput.
type dataSourceMapped struct {
	entityType    *string
	entityID      pgtype.UUID
	dimensionSlug *string
	sourceName    string
	sourceURL     *string
	sourceKind    string
	trustLevel    int32
	config        []byte
}

// create inserts a new data source and returns the populated domain model.
func (s *dataSourceStore) create(ctx context.Context, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error) {
	params, err := dataSourceCreateParams(tenantID, in)
	if err != nil {
		return DataSource{}, err
	}

	row, err := s.write.CreateDataSource(ctx, params)
	if err != nil {
		return DataSource{}, fmt.Errorf("creating data source: %w", err)
	}

	return toDataSource(row)
}

// list returns a paginated slice of data sources for the tenant and the total
// count, optionally filtered by entity_type and entity_id.
func (s *dataSourceStore) list(ctx context.Context, tenantID pgtype.UUID, entityType *string, entityID *pgtype.UUID, limit, offset int32) ([]DataSource, int64, error) {
	eid := pgtype.UUID{}
	if entityID != nil {
		eid = *entityID
	}

	rows, err := s.read.ListDataSources(ctx, gen.ListDataSourcesParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   eid,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing data sources: %w", err)
	}

	out := make([]DataSource, 0, len(rows))
	for _, row := range rows {
		ds, err := toDataSource(row)
		if err != nil {
			return nil, 0, fmt.Errorf("mapping data source row: %w", err)
		}
		out = append(out, ds)
	}

	total, err := s.read.CountDataSources(ctx, gen.CountDataSourcesParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   eid,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("counting data sources: %w", err)
	}

	return out, int64(total), nil
}

// get returns a data source by id and tenant.
func (s *dataSourceStore) get(ctx context.Context, id, tenantID pgtype.UUID) (DataSource, error) {
	row, err := s.read.GetDataSource(ctx, gen.GetDataSourceParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return DataSource{}, fmt.Errorf("getting data source: %w", err)
	}

	return toDataSource(row)
}

// update modifies a data source and returns the updated domain model.
func (s *dataSourceStore) update(ctx context.Context, id, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error) {
	params, err := dataSourceUpdateParams(id, tenantID, in)
	if err != nil {
		return DataSource{}, err
	}

	row, err := s.write.UpdateDataSource(ctx, params)
	if err != nil {
		return DataSource{}, fmt.Errorf("updating data source: %w", err)
	}

	return toDataSource(row)
}

// delete removes a data source by id and tenant.
func (s *dataSourceStore) delete(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteDataSource(ctx, gen.DeleteDataSourceParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting data source: %w", err)
	}
	return nil
}

// touch updates the freshness bookkeeping columns for a data source.
func (s *dataSourceStore) touch(ctx context.Context, id, tenantID pgtype.UUID, lastVerifiedAt pgtype.Timestamptz, freshnessScore *int32) error {
	if err := s.write.TouchDataSource(ctx, gen.TouchDataSourceParams{
		ID:             id,
		TenantID:       tenantID,
		LastVerifiedAt: lastVerifiedAt,
		FreshnessScore: freshnessScore,
	}); err != nil {
		return fmt.Errorf("touching data source: %w", err)
	}
	return nil
}

// match finds the most specific data source for the given scope, falling back to more
// general scopes. It returns nil when no registered source matches.
func (s *dataSourceStore) match(ctx context.Context, tenantID pgtype.UUID, entityType *string, entityID *pgtype.UUID, dimensionSlug, sourceName string, sourceKind *string) (*DataSource, error) {
	passes := buildMatchPasses(tenantID, sourceName, entityType, entityID, dimensionSlug, sourceKind)

	for i, p := range passes {
		row, err := s.read.MatchDataSource(ctx, p)
		if err == nil {
			ds, err := toDataSource(row)
			if err != nil {
				return nil, fmt.Errorf("mapping matched data source: %w", err)
			}
			return &ds, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("matching data source pass %d: %w", i+1, err)
		}
	}

	return nil, nil
}

// buildMatchPasses builds the ordered specificity passes for MatchDataSource.
//
// The order is:
//  1. (entityType, entityID, dimensionSlug)
//  2. (entityType, entityID, NULL)
//  3. (entityType, NULL, dimensionSlug)
//  4. (entityType, NULL, NULL)
//     5-8. repeat 1-4 with entityType = NULL
//
// sourceName is used as supplied; an empty sourceName matches any source_name in
// the query. Callers that want to fall back from an unknown source_name to the
// best scoped source can call MatchDataSource first with the name and then with "".
//
// This is the "five-pass" specificity fallback described in the Sub-spec 4 plan:
// four entity-specific passes, then a global-entity-type repeat when no match is found.
func buildMatchPasses(tenantID pgtype.UUID, sourceName string, entityType *string, entityID *pgtype.UUID, dimensionSlug string, sourceKind *string) []gen.MatchDataSourceParams {
	eid := pgtype.UUID{}
	if entityID != nil {
		eid = *entityID
	}

	var dim *string
	if dimensionSlug != "" {
		dim = &dimensionSlug
	}

	passes := make([]gen.MatchDataSourceParams, 0, 8)
	add := func(et *string, e pgtype.UUID, d *string) {
		passes = append(passes, gen.MatchDataSourceParams{
			TenantID:      tenantID,
			SourceName:    sourceName,
			EntityType:    et,
			EntityID:      e,
			DimensionSlug: d,
			SourceKind:    sourceKind,
		})
	}

	// Passes 1-4: scoped to the provided entity_type (if any).
	if entityType != nil {
		if eid.Valid {
			if dim != nil {
				add(entityType, eid, dim)
			}
			add(entityType, eid, nil)
		}
		if dim != nil {
			add(entityType, pgtype.UUID{}, dim)
		}
		add(entityType, pgtype.UUID{}, nil)
	}

	// Passes 5-8: global entity_type fallback.
	if eid.Valid {
		if dim != nil {
			add(nil, eid, dim)
		}
		add(nil, eid, nil)
	}
	if dim != nil {
		add(nil, pgtype.UUID{}, dim)
	}
	add(nil, pgtype.UUID{}, nil)

	// Remove any duplicate passes that can occur when entityType is already nil.
	deduped := make([]gen.MatchDataSourceParams, 0, len(passes))
	seen := make(map[string]struct{}, len(passes))
	for _, p := range passes {
		key := matchPassKey(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, p)
	}

	return deduped
}

// matchPassKey returns a string key for deduplicating match passes.
func matchPassKey(p gen.MatchDataSourceParams) string {
	et := "<nil>"
	if p.EntityType != nil {
		et = *p.EntityType
	}
	eid := "<nil>"
	if p.EntityID.Valid {
		eid = p.EntityID.String()
	}
	dim := "<nil>"
	if p.DimensionSlug != nil {
		dim = *p.DimensionSlug
	}
	sk := "<nil>"
	if p.SourceKind != nil {
		sk = *p.SourceKind
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s", et, eid, dim, p.SourceName, sk)
}

// toDataSource converts a generated ComparisonDataSource row into the domain model.
func toDataSource(row gen.ComparisonDataSource) (DataSource, error) {
	var cfg map[string]any
	if len(row.Config) > 0 && string(row.Config) != "null" {
		cfg = make(map[string]any)
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return DataSource{}, fmt.Errorf("unmarshaling data source config: %w", err)
		}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	var entityType *string
	if row.EntityType != nil && *row.EntityType != "" {
		entityType = row.EntityType
	}

	var entityID *string
	if row.EntityID.Valid {
		s := row.EntityID.String()
		entityID = &s
	}

	var dimensionSlug *string
	if row.DimensionSlug != nil && *row.DimensionSlug != "" {
		dimensionSlug = row.DimensionSlug
	}

	var sourceURL *string
	if row.SourceUrl != nil && *row.SourceUrl != "" {
		sourceURL = row.SourceUrl
	}

	var lastVerifiedAt *string
	if row.LastVerifiedAt.Valid {
		s := row.LastVerifiedAt.Time.Format(time.RFC3339)
		lastVerifiedAt = &s
	}

	return DataSource{
		ID:             row.ID.String(),
		TenantID:       row.TenantID.String(),
		EntityType:     entityType,
		EntityID:       entityID,
		DimensionSlug:  dimensionSlug,
		SourceName:     row.SourceName,
		SourceURL:      sourceURL,
		SourceKind:     row.SourceKind,
		TrustLevel:     int(row.TrustLevel),
		FreshnessScore: int(row.FreshnessScore),
		LastVerifiedAt: lastVerifiedAt,
		Config:         cfg,
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      row.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// fromDataSourceInput validates and maps a DataSourceInput to a DB-ready struct.
func fromDataSourceInput(in DataSourceInput) (dataSourceMapped, error) {
	if in.SourceName == "" {
		return dataSourceMapped{}, fmt.Errorf("%w: source_name is required", errpkg.ErrInvalid)
	}
	if !validDataSourceKinds[in.SourceKind] {
		return dataSourceMapped{}, fmt.Errorf("%w: invalid source_kind %q", errpkg.ErrInvalid, in.SourceKind)
	}
	if in.TrustLevel < 1 || in.TrustLevel > 5 {
		return dataSourceMapped{}, fmt.Errorf("%w: trust_level must be between 1 and 5", errpkg.ErrInvalid)
	}

	var entityType *string
	if in.EntityType != nil && *in.EntityType != "" {
		entityType = in.EntityType
	}

	var entityID pgtype.UUID
	if in.EntityID != nil {
		entityID = stringToUUID(*in.EntityID)
		if !entityID.Valid {
			return dataSourceMapped{}, fmt.Errorf("%w: invalid entity_id", errpkg.ErrInvalid)
		}
	}

	var dimensionSlug *string
	if in.DimensionSlug != nil && *in.DimensionSlug != "" {
		dimensionSlug = in.DimensionSlug
	}

	var sourceURL *string
	if in.SourceURL != nil && *in.SourceURL != "" {
		sourceURL = in.SourceURL
	}

	cfg := map[string]any{}
	if in.Config != nil {
		cfg = in.Config
	}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return dataSourceMapped{}, fmt.Errorf("%w: unable to marshal config: %w", errpkg.ErrInvalid, err)
	}

	return dataSourceMapped{
		entityType:    entityType,
		entityID:      entityID,
		dimensionSlug: dimensionSlug,
		sourceName:    in.SourceName,
		sourceURL:     sourceURL,
		sourceKind:    in.SourceKind,
		trustLevel:    int32(in.TrustLevel),
		config:        cfgBytes,
	}, nil
}

// dataSourceCreateParams builds the sqlc params for creating a data source.
func dataSourceCreateParams(tenantID pgtype.UUID, in DataSourceInput) (gen.CreateDataSourceParams, error) {
	m, err := fromDataSourceInput(in)
	if err != nil {
		return gen.CreateDataSourceParams{}, err
	}

	return gen.CreateDataSourceParams{
		TenantID:      tenantID,
		EntityType:    m.entityType,
		EntityID:      m.entityID,
		DimensionSlug: m.dimensionSlug,
		SourceName:    m.sourceName,
		SourceUrl:     m.sourceURL,
		SourceKind:    m.sourceKind,
		TrustLevel:    m.trustLevel,
		Config:        m.config,
	}, nil
}

// dataSourceUpdateParams builds the sqlc params for updating a data source.
// FreshnessScore and LastVerifiedAt are intentionally left nil/invalid so the
// existing values are preserved.
func dataSourceUpdateParams(id, tenantID pgtype.UUID, in DataSourceInput) (gen.UpdateDataSourceParams, error) {
	m, err := fromDataSourceInput(in)
	if err != nil {
		return gen.UpdateDataSourceParams{}, err
	}

	return gen.UpdateDataSourceParams{
		EntityType:     m.entityType,
		EntityID:       m.entityID,
		DimensionSlug:  m.dimensionSlug,
		SourceName:     m.sourceName,
		SourceUrl:      m.sourceURL,
		SourceKind:     m.sourceKind,
		TrustLevel:     m.trustLevel,
		FreshnessScore: nil,
		LastVerifiedAt: pgtype.Timestamptz{},
		Config:         m.config,
		ID:             id,
		TenantID:       tenantID,
	}, nil
}

// CreateDataSource creates a new data source for the tenant.
func (s *Service) CreateDataSource(ctx context.Context, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error) {
	if !s.featureEnabled(ctx, tenantID, dataSourcesFeatureFlag) {
		return DataSource{}, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, dataSourcesFeatureFlag)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID, tenantID)(dataSourceManagePerm) {
		return DataSource{}, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, dataSourceManagePerm)
	}

	ds, err := s.dataSources.create(ctx, tenantID, in)
	if err != nil {
		return DataSource{}, fmt.Errorf("creating data source: %w", err)
	}
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:freshness:%s:*", tenantID.String()))
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:advisor:%s:*", tenantID.String()))
	return ds, nil
}

// ListDataSources returns the tenant's data sources, optionally filtered by entity.
func (s *Service) ListDataSources(ctx context.Context, tenantID pgtype.UUID, entityType *string, entityID *pgtype.UUID, page, pageSize int) ([]DataSource, int64, error) {
	if !s.featureEnabled(ctx, tenantID, dataSourcesFeatureFlag) {
		return nil, 0, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, dataSourcesFeatureFlag)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID, tenantID)(dataSourceViewPerm) {
		return nil, 0, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, dataSourceViewPerm)
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	offset := safeInt32((page - 1) * pageSize)
	limit := safeInt32(pageSize)

	return s.dataSources.list(ctx, tenantID, entityType, entityID, limit, offset)
}

// GetDataSource returns a single data source by id and tenant.
func (s *Service) GetDataSource(ctx context.Context, id, tenantID pgtype.UUID) (DataSource, error) {
	if !s.featureEnabled(ctx, tenantID, dataSourcesFeatureFlag) {
		return DataSource{}, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, dataSourcesFeatureFlag)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID, tenantID)(dataSourceViewPerm) {
		return DataSource{}, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, dataSourceViewPerm)
	}

	ds, err := s.dataSources.get(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DataSource{}, fmt.Errorf("%w: data source not found", errpkg.ErrNotFound)
		}
		return DataSource{}, fmt.Errorf("getting data source: %w", err)
	}
	return ds, nil
}

// UpdateDataSource updates an existing data source.
func (s *Service) UpdateDataSource(ctx context.Context, id, tenantID pgtype.UUID, in DataSourceInput) (DataSource, error) {
	if !s.featureEnabled(ctx, tenantID, dataSourcesFeatureFlag) {
		return DataSource{}, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, dataSourcesFeatureFlag)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID, tenantID)(dataSourceManagePerm) {
		return DataSource{}, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, dataSourceManagePerm)
	}

	ds, err := s.dataSources.update(ctx, id, tenantID, in)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DataSource{}, fmt.Errorf("%w: data source not found", errpkg.ErrNotFound)
		}
		return DataSource{}, fmt.Errorf("updating data source: %w", err)
	}
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:freshness:%s:*", tenantID.String()))
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:advisor:%s:*", tenantID.String()))
	return ds, nil
}

// DeleteDataSource removes a data source by id and tenant.
func (s *Service) DeleteDataSource(ctx context.Context, id, tenantID pgtype.UUID) error {
	if !s.featureEnabled(ctx, tenantID, dataSourcesFeatureFlag) {
		return fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, dataSourcesFeatureFlag)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID, tenantID)(dataSourceManagePerm) {
		return fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, dataSourceManagePerm)
	}

	if _, err := s.dataSources.get(ctx, id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: data source not found", errpkg.ErrNotFound)
		}
		return fmt.Errorf("getting data source for delete: %w", err)
	}

	if err := s.dataSources.delete(ctx, id, tenantID); err != nil {
		return fmt.Errorf("deleting data source: %w", err)
	}
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:freshness:%s:*", tenantID.String()))
	s.deleteJceCacheByPattern(ctx, fmt.Sprintf("jc:jce:advisor:%s:*", tenantID.String()))
	return nil
}

// MatchDataSource finds the most specific data source for the given scope,
// falling back to more general scopes. It returns nil when no registered source matches.
func (s *Service) MatchDataSource(ctx context.Context, tenantID pgtype.UUID, entityType *string, entityID *pgtype.UUID, dimensionSlug, sourceName string, sourceKind *string) (*DataSource, error) {
	return s.dataSources.match(ctx, tenantID, entityType, entityID, dimensionSlug, sourceName, sourceKind)
}
