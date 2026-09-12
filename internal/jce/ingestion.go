// Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §7.5, §8, §9
package jce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

const (
	refreshTaskType    = "jce:refresh_values"
	refreshFeatureFlag = "feature.jce.refresh"
	refreshPerm        = "jce.refresh.trigger"
)

// refreshValuesPayload is the JSON body enqueued for a refresh task.
// It mirrors the payload struct in internal/worker/jce_refresh.go.
type refreshValuesPayload struct {
	TenantID       string  `json:"tenant_id"`
	ContentID      string  `json:"content_id"`
	SourceKind     *string `json:"source_kind,omitempty"`
	IngestionJobID string  `json:"ingestion_job_id"`
	RequestedBy    string  `json:"requested_by_user_id"`
}

// ingestionStore wraps the generated comparison_data_ingestion_jobs queries.
type ingestionStore struct {
	read  *gen.Queries
	write *gen.Queries
}

func newIngestionStore(database *db.DB) *ingestionStore {
	return &ingestionStore{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

func (st *ingestionStore) create(ctx context.Context, tenantID, contentID pgtype.UUID, sourceKind *string) (IngestionJob, error) {
	row, err := st.write.CreateIngestionJob(ctx, gen.CreateIngestionJobParams{

		ContentID:  contentID,
		SourceKind: sourceKind,
	})
	if err != nil {
		return IngestionJob{}, fmt.Errorf("creating ingestion job: %w", err)
	}

	return toIngestionJob(row)
}

func (st *ingestionStore) get(ctx context.Context, id, tenantID pgtype.UUID) (IngestionJob, error) {
	row, err := st.read.GetIngestionJob(ctx,
		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IngestionJob{}, fmt.Errorf("%w: ingestion job not found", errpkg.ErrNotFound)
		}

		return IngestionJob{}, fmt.Errorf("getting ingestion job: %w", err)
	}

	return toIngestionJob(row)
}

func (st *ingestionStore) listByContent(ctx context.Context, tenantID, contentID pgtype.UUID, limit, offset int32) ([]IngestionJob, error) {
	rows, err := st.read.ListIngestionJobsByContent(ctx, gen.ListIngestionJobsByContentParams{

		ContentID: contentID,
		OffsetVal: offset,
		LimitVal:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing ingestion jobs: %w", err)
	}

	out := make([]IngestionJob, 0, len(rows))

	for _, row := range rows {
		job, err := toIngestionJob(row)
		if err != nil {
			return nil, err
		}

		out = append(out, job)
	}

	return out, nil
}

func (st *ingestionStore) findLatestPendingByContent(ctx context.Context, tenantID, contentID pgtype.UUID) (IngestionJob, bool, error) {
	// Limit is intentionally high: in normal operation only one pending job exists
	// at a time, but this avoids missing it if a few historical rows are present.
	const lookbackLimit = 1000

	rows, err := st.read.ListIngestionJobsByContent(ctx, gen.ListIngestionJobsByContentParams{

		ContentID: contentID,
		OffsetVal: 0,
		LimitVal:  lookbackLimit,
	})
	if err != nil {
		return IngestionJob{}, false, fmt.Errorf("finding pending ingestion job: %w", err)
	}

	for _, row := range rows {
		if row.Status == "pending" {
			job, err := toIngestionJob(row)
			if err != nil {
				return IngestionJob{}, false, err
			}

			return job, true, nil
		}
	}

	return IngestionJob{}, false, nil
}

func (st *ingestionStore) update(ctx context.Context, arg gen.UpdateIngestionJobParams) (IngestionJob, error) {
	row, err := st.write.UpdateIngestionJob(ctx, arg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IngestionJob{}, fmt.Errorf("%w: ingestion job not found", errpkg.ErrNotFound)
		}

		return IngestionJob{}, fmt.Errorf("updating ingestion job: %w", err)
	}

	return toIngestionJob(row)
}

func toIngestionJob(row gen.ComparisonDataIngestionJob) (IngestionJob, error) {
	job := IngestionJob{
		ID: row.ID.String(),

		ContentID:      row.ContentID.String(),
		Status:         row.Status,
		SourceKind:     row.SourceKind,
		RefreshedCount: int(row.RefreshedCount),
		Error:          row.Error,
	}

	if row.StartedAt.Valid {
		s := row.StartedAt.Time.UTC().Format(time.RFC3339)
		job.StartedAt = &s
	}

	if row.CompletedAt.Valid {
		s := row.CompletedAt.Time.UTC().Format(time.RFC3339)
		job.CompletedAt = &s
	}

	if row.CreatedAt.Valid {
		job.CreatedAt = row.CreatedAt.Time.UTC().Format(time.RFC3339)
	}

	if row.UpdatedAt.Valid {
		job.UpdatedAt = row.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}

	return job, nil
}

// GetIngestionJob returns a single ingestion job by id and tenant.
func (s *Service) GetIngestionJob(ctx context.Context, id, tenantID pgtype.UUID) (IngestionJob, error) {
	return s.ingestion.get(ctx, id, tenantID)
}

// ListIngestionJobs returns the refresh history for a comparison.
func (s *Service) ListIngestionJobs(ctx context.Context, contentID, tenantID pgtype.UUID, limit, offset int) ([]IngestionJob, error) {
	return s.ingestion.listByContent(ctx, tenantID, contentID, safeInt32(limit), safeInt32(offset))
}

// RefreshComparison starts a refresh job for the given comparison.
// It acquires a distributed lock, creates a pending ingestion job, enqueues the
// worker task, and returns the pending job. If another refresh is already in
// progress it returns the existing pending job instead.
func (s *Service) RefreshComparison(ctx context.Context, contentID, tenantID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error) {
	if !contentID.Valid || !tenantID.Valid {
		return IngestionJob{}, fmt.Errorf("%w: content and tenant ids are required", errpkg.ErrInvalid)
	}

	if !s.featureEnabled(ctx, tenantID, refreshFeatureFlag) {
		return IngestionJob{}, fmt.Errorf("%w: %s disabled", errpkg.ErrFeatureDisabled, refreshFeatureFlag)
	}

	check := s.permissionChecker(ctx, userID, tenantID)
	if !check(refreshPerm) {
		return IngestionJob{}, fmt.Errorf("%w: missing %s", errpkg.ErrForbidden, refreshPerm)
	}

	// Load the comparison and enforce visibility before enqueuing refresh work.
	cmp, err := s.compSvc.GetJCEComparison(ctx, contentID.String(), tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IngestionJob{}, fmt.Errorf("%w: comparison not found", errpkg.ErrNotFound)
		}

		return IngestionJob{}, fmt.Errorf("loading comparison: %w", err)
	}

	if !s.canViewJCEComparison(ctx, cmp, userID) {
		return IngestionJob{}, fmt.Errorf("%w: cannot access comparison", errpkg.ErrForbidden)
	}

	if s.enqueuer == nil {
		return IngestionJob{}, errors.New("jce refresh enqueuer not configured")
	}

	return s.enqueueRefresh(ctx, tenantID, contentID, userID, sourceKind, "default")
}

// enqueueRefresh is the internal refresh orchestration used by RefreshComparison
// and ScheduleStaleRefresh. It returns the existing pending job when a lock is
// already held, preventing duplicate work.
func (s *Service) enqueueRefresh(ctx context.Context, tenantID, contentID, userID pgtype.UUID, sourceKind *string, queue string) (IngestionJob, error) {
	lockKey := refreshLockKey(contentID)
	lockDur := s.refreshLockDuration(ctx, tenantID)

	if s.vk != nil {
		locked, err := s.vk.SetNX(ctx, lockKey, "1", lockDur).Result()
		if err != nil {
			return IngestionJob{}, fmt.Errorf("acquiring refresh lock: %w", err)
		}

		if !locked {
			if job, ok, err := s.ingestion.findLatestPendingByContent(ctx, tenantID, contentID); err == nil && ok {
				return job, nil
			}

			jobs, err := s.ingestion.listByContent(ctx, tenantID, contentID, 1, 0)
			if err == nil && len(jobs) > 0 {
				return jobs[0], nil
			}

			return IngestionJob{}, errors.New("refresh already in progress")
		}
	}

	job, err := s.ingestion.create(ctx, tenantID, contentID, sourceKind)
	if err != nil {
		s.releaseRefreshLock(ctx, contentID)
		return IngestionJob{}, fmt.Errorf("creating ingestion job: %w", err)
	}

	payload, err := json.Marshal(refreshValuesPayload{

		ContentID:      contentID.String(),
		SourceKind:     sourceKind,
		IngestionJobID: job.ID,
		RequestedBy:    userID.String(),
	})
	if err != nil {
		_, _ = s.ingestion.update(ctx, gen.UpdateIngestionJobParams{
			Status:         "failed",
			RefreshedCount: 0,
			Error:          ptrTo("payload marshal failed"),
			StartedAt:      pgtype.Timestamptz{},
			CompletedAt:    pgtype.Timestamptz{},
			ID:             stringToUUID(job.ID),
		})

		s.releaseRefreshLock(ctx, contentID)

		return IngestionJob{}, fmt.Errorf("marshaling refresh payload: %w", err)
	}

	if err := s.enqueuer.Enqueue(refreshTaskType, payload, asynq.Queue(queue), asynq.MaxRetry(3)); err != nil {
		_, _ = s.ingestion.update(ctx, gen.UpdateIngestionJobParams{
			Status:         "failed",
			RefreshedCount: 0,
			Error:          ptrTo("enqueue failed"),
			StartedAt:      pgtype.Timestamptz{},
			CompletedAt:    pgtype.Timestamptz{},
			ID:             stringToUUID(job.ID),
		})

		s.releaseRefreshLock(ctx, contentID)

		return IngestionJob{}, fmt.Errorf("enqueuing refresh task: %w", err)
	}

	return job, nil
}

// ProcessRefreshValues is the worker-side refresh implementation. It loads the
// comparison matrix, finds missing or stale values, resolves the best data
// source, fills values with provenance metadata, and finalizes the job.
func (s *Service) ProcessRefreshValues(ctx context.Context, tenantID, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) (err error) {
	_ = userID

	job, err := s.ingestion.get(ctx, jobID, tenantID)
	if err != nil {
		return err
	}

	if job.Status != "pending" {
		return fmt.Errorf("ingestion job %s is not pending (status=%s)", job.ID, job.Status)
	}

	startedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	if _, err := s.ingestion.update(ctx, gen.UpdateIngestionJobParams{
		Status:         "running",
		RefreshedCount: 0,
		Error:          nil,
		StartedAt:      startedAt,
		CompletedAt:    pgtype.Timestamptz{},
		ID:             jobID,
	}); err != nil {
		return fmt.Errorf("marking ingestion job running: %w", err)
	}

	var refreshedCount int

	var procErrs []string

	usedSources := make(map[string]DataSource)

	defer func() {
		finalErr := s.finalizeRefresh(ctx, tenantID, contentID, jobID, startedAt, refreshedCount, usedSources, procErrs)
		if finalErr != nil && err == nil {
			err = finalErr
		}
	}()

	matrix, err := s.compSvc.GetJCEComparisonMatrix(ctx, contentID, tenantID)
	if err != nil {
		return fmt.Errorf("loading comparison matrix: %w", err)
	}

	values, err := gen.New(s.db.Reader).GetComparisonValuesByContent(ctx,
		contentID,
	)
	if err != nil {
		return fmt.Errorf("loading comparison values: %w", err)
	}

	type valueKey struct{ itemID, dimensionID [16]byte }

	valueIndex := make(map[valueKey]gen.ComparisonValue, len(values))
	for _, v := range values {
		valueIndex[valueKey{v.ItemID.Bytes, v.DimensionID.Bytes}] = v
	}

	sources, _, err := s.dataSources.list(ctx, tenantID, nil, nil, 0, 0)
	if err != nil {
		return fmt.Errorf("loading data sources: %w", err)
	}

	if len(sources) == 0 {
		procErrs = append(procErrs, "no data sources registered for tenant")
		return nil
	}

	sourceByID := make(map[string]*DataSource, len(sources))
	for i := range sources {
		sourceByID[sources[i].ID] = &sources[i]
	}

	settings := s.loadFreshnessSettings(ctx, tenantID)
	now := time.Now().UTC()

	for _, item := range matrix {
		itemRef, _ := parseItemRef(item.Item.ItemRef)
		entityType, entityID, _ := resolveItemEntityFromRef(item.Item.ItemRef)

		for _, dim := range item.Dimensions {
			key := valueKey{item.Item.ID.Bytes, dim.DimensionID.Bytes}
			v := valueIndex[key]

			source, isExisting := s.resolveRefreshSource(ctx, tenantID, entityType, entityID, dim.Label, v, sourceByID, sourceKind)
			if source == nil {
				continue
			}

			shouldRefresh := false
			if !hasValue(v) {
				shouldRefresh = true
			} else if isExisting {
				_, isStale, ferr := s.cellFreshness(ctx, v, source, settings)
				if ferr != nil {
					procErrs = append(procErrs, fmt.Sprintf("freshness for %s/%s: %v", item.Item.ID.String(), dim.DimensionID.String(), ferr))
					continue
				}

				if isStale {
					shouldRefresh = true
				}
			} else {
				// New source selected for a missing cell: always fill.
				shouldRefresh = true
			}

			if !shouldRefresh {
				continue
			}

			val, ok, rerr := s.resolveValue(ctx, tenantID, item, dim, itemRef, source, now)
			if rerr != nil {
				procErrs = append(procErrs, fmt.Sprintf("resolve %s/%s: %v", item.Item.ID.String(), dim.DimensionID.String(), rerr))
				continue
			}

			if !ok {
				procErrs = append(procErrs, fmt.Sprintf("no value resolved for %s/%s", item.Item.ID.String(), dim.DimensionID.String()))
				continue
			}

			dsID := stringToUUID(source.ID)
			input := comparison.UpsertComparisonValueInput{
				DimensionID:  dim.DimensionID,
				ItemID:       item.Item.ID,
				DataSourceID: &dsID,
			}
			setInputValue(&input, dim.Type, val)

			md, err := s.buildProvenance(source, val, now)
			if err != nil {
				procErrs = append(procErrs, fmt.Sprintf("provenance %s/%s: %v", item.Item.ID.String(), dim.DimensionID.String(), err))
				continue
			}

			metadata, err := json.Marshal(md)
			if err != nil {
				procErrs = append(procErrs, fmt.Sprintf("marshal provenance %s/%s: %v", item.Item.ID.String(), dim.DimensionID.String(), err))
				continue
			}

			if _, err := s.compSvc.UpsertComparisonValueWithMetadata(ctx, tenantID, input, metadata); err != nil {
				procErrs = append(procErrs, fmt.Sprintf("upsert %s/%s: %v", item.Item.ID.String(), dim.DimensionID.String(), err))
				continue
			}

			usedSources[source.ID] = *source
			refreshedCount++
		}
	}

	return nil
}

// resolveRefreshSource finds the data source to use for a cell, respecting the
// optional source_kind filter. It returns nil when the cell should be skipped.
// The bool reports whether the returned source is the one already attached to
// the cell (true) or a newly selected one (false).
func (s *Service) resolveRefreshSource(
	ctx context.Context,
	tenantID pgtype.UUID,
	entityType *string,
	entityID *pgtype.UUID,
	dimensionSlug string,
	v gen.ComparisonValue,
	sourceByID map[string]*DataSource,
	sourceKind *string,
) (*DataSource, bool) {
	var md map[string]any
	if len(v.Metadata) > 0 && string(v.Metadata) != "null" {
		_ = json.Unmarshal(v.Metadata, &md)
	}

	// First, try to preserve the existing data source by ID.
	if v.DataSourceID.Valid {
		if ds, ok := sourceByID[v.DataSourceID.String()]; ok {
			if sourceKind == nil || ds.SourceKind == ptrString(sourceKind) {
				return ds, true
			}

			return nil, false
		}
	}

	// Fall back to the source named in the cell metadata, then any scoped source.
	sourceName := ""

	if md != nil {
		if sn, ok := md["source"].(string); ok {
			sourceName = sn
		}
	}

	// Try the exact source name (if known), then wildcard by scope. The query
	// returns the most trusted/freshest match for the requested scope and kind.
	names := []string{sourceName}
	if sourceName != "" {
		names = append(names, "")
	}

	for _, sn := range names {
		matched, err := s.MatchDataSource(ctx, tenantID, entityType, entityID, dimensionSlug, sn, sourceKind)
		if err != nil {
			continue
		}

		if matched == nil {
			continue
		}

		if sourceKind == nil || matched.SourceKind == ptrString(sourceKind) {
			// Treat a source matched by the cell's own provenance as existing.
			return matched, sn == sourceName
		}
	}

	return nil, false
}

// resolveValue extracts the new value for a cell from a data source.
// The priority is: static value in Config, metadata path via the resolver
// registry, then an AI estimate when the dispatcher is wired.
func (s *Service) resolveValue(
	ctx context.Context,
	tenantID pgtype.UUID,
	item comparison.ComparisonMatrixItem,
	dim comparison.ComparisonMatrixDimension,
	itemRef map[string]any,
	source *DataSource,
	now time.Time,
) (any, bool, error) {
	// Static value in data source config.
	if raw, ok := source.Config["value"]; ok && raw != nil {
		return raw, true, nil
	}

	// Resolver-backed value extracted from resolved item metadata.
	if path, ok := source.Config["metadata_path"].(string); ok && path != "" {
		if s.r == nil || len(itemRef) == 0 {
			return nil, false, nil
		}

		resolved, err := s.r.Resolve(ctx, itemRef, tenantID)
		if err != nil {
			return nil, false, err
		}

		if resolved.Metadata == nil {
			return nil, false, nil
		}

		v, ok := resolved.Metadata[path]
		if !ok || v == nil {
			return nil, false, nil
		}

		return v, true, nil
	}

	// AI estimate path (only when dispatcher is wired and source kind is ai_estimate).
	if source.SourceKind == "ai_estimate" && s.dispatcher != nil {
		prompt := fmt.Sprintf("Provide the %s for %q as a short, factual value.", dim.Label, item.Item.Title)
		ctxStr := "You are a precise comparison-data assistant. Return only the requested value with no extra prose."

		v, err := s.dispatcher.Generate(ctx, "chat", prompt, ctxStr, "", 0)
		if err != nil {
			return nil, false, err
		}

		v = strings.TrimSpace(v)
		if v == "" {
			return nil, false, nil
		}

		return v, true, nil
	}

	return nil, false, nil
}

// buildProvenance assembles the metadata JSON for a freshly filled value.
func (s *Service) buildProvenance(source *DataSource, value any, now time.Time) (map[string]any, error) {
	confidence := "high"

	switch source.SourceKind {
	case "third_party":
		confidence = "medium"
	case "ai_estimate":
		confidence = "low"
	}

	if c, ok := source.Config["confidence"].(string); ok && c != "" {
		confidence = c
	}

	sourceRef := ""
	if u, ok := source.Config["source_url"].(string); ok && u != "" {
		sourceRef = u
	} else if source.SourceURL != nil {
		sourceRef = *source.SourceURL
	}

	return map[string]any{
		"source":         source.SourceName,
		"source_kind":    source.SourceKind,
		"source_ref":     sourceRef,
		"confidence":     confidence,
		"retrieved_at":   now.Format(time.RFC3339),
		"is_estimate":    source.SourceKind == "ai_estimate",
		"data_source_id": source.ID,
		"value":          value,
	}, nil
}

// hasValue reports whether a comparison value row actually holds a value.
func hasValue(v gen.ComparisonValue) bool {
	return v.Value != nil || v.ValueRating.Valid || v.ValueBoolean != nil
}

// setInputValue maps a resolved value to the correct comparison value field
// based on the dimension type.
func setInputValue(input *comparison.UpsertComparisonValueInput, dimType string, val any) {
	switch dimType {
	case "boolean", "checkbox":
		if b, ok := val.(bool); ok {
			input.ValueBoolean = &b
			return
		}

		if s, ok := val.(string); ok {
			b, _ := strconv.ParseBool(s)
			input.ValueBoolean = &b

			return
		}
	case "rating", "number", "currency":
		if f, ok := toFloat(val); ok {
			input.ValueRating = &f
			return
		}
	}

	input.Value = fmt.Sprintf("%v", val)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err == nil {
			return f, true
		}
	}

	return 0, false
}

func parseItemRef(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}

	var ref map[string]any
	if err := json.Unmarshal(b, &ref); err != nil {
		return nil, err
	}

	return ref, nil
}

// resolveItemEntityFromRef extracts the resolved entity type and (when it is a
// valid UUID) entity id from a comparison item's item_ref JSON. This is the
// correct scope for data-source matching; the comparison item UUID itself is
// only the local matrix identifier.
func resolveItemEntityFromRef(itemRef []byte) (entityType *string, entityID *pgtype.UUID, err error) {
	ref, err := parseItemRef(itemRef)
	if err != nil {
		return nil, nil, err
	}

	if ref == nil {
		return nil, nil, nil
	}

	if t, ok := ref["type"].(string); ok && t != "" {
		entityType = &t
	}

	if id, ok := ref["id"].(string); ok && id != "" {
		var u pgtype.UUID
		if serr := u.Scan(id); serr == nil {
			entityID = &u
		}
	}

	return entityType, entityID, nil
}

// finalizeRefresh writes the final job status, updates data-source bookkeeping
// for every source that contributed a value, deletes the freshness cache, and
// releases the distributed lock.
func (s *Service) finalizeRefresh(ctx context.Context, tenantID, contentID, jobID pgtype.UUID, startedAt pgtype.Timestamptz, refreshedCount int, usedSources map[string]DataSource, procErrs []string) error {
	// The CHECK constraint only allows completed/failed; record partial errors
	// in the error column with a completed status.
	status := "completed"

	var errMsg *string

	if len(procErrs) > 0 {
		if refreshedCount == 0 {
			status = "failed"
		}

		summary := strings.Join(procErrs, "; ")
		if len(summary) > 1024 {
			summary = summary[:1024]
		}

		errMsg = &summary
	}

	completedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	if _, err := s.ingestion.update(ctx, gen.UpdateIngestionJobParams{
		Status:         status,
		RefreshedCount: safeInt32(refreshedCount),
		Error:          errMsg,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		ID:             jobID,
	}); err != nil {
		return fmt.Errorf("finalizing ingestion job: %w", err)
	}

	// Update bookkeeping for every source that contributed a refreshed value.
	lastVerifiedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	for _, ds := range usedSources {
		score := safeInt32(ds.TrustLevel * 20)
		if score > 100 {
			score = 100
		}

		if err := s.dataSources.touch(ctx, stringToUUID(ds.ID), tenantID, lastVerifiedAt, &score); err != nil {
			slog.Error("jce refresh: failed to touch data source", "error", err, "data_source_id", ds.ID)
		}
	}

	s.deleteFreshnessCache(ctx, tenantID, contentID)
	s.deleteAdvisorCache(ctx, tenantID, contentID)
	s.releaseRefreshLock(ctx, contentID)

	return nil
}

// ScheduleStaleRefresh scans all JCE comparisons with smart_mode != 'none',
// skipping those with a recent completed refresh or an active job, and enqueues
// refresh tasks for the stale ones. It returns the number of enqueued tasks.
func (s *Service) ScheduleStaleRefresh(ctx context.Context, userID pgtype.UUID) (int, error) {
	if s.enqueuer == nil {
		return 0, errors.New("jce refresh enqueuer not configured")
	}

	// Global/platform defaults for the sweep window and batch size. Individual
	// tenants can override via the same keys if the settings layer supports
	// tenant-specific values, but the sweep itself is run once per instance.
	sweepHours := 24
	sweepLimit := 1000

	if s.settings != nil && s.defaultTenantID.Valid {
		sweepHours = s.settings.GetIntOr(ctx, s.defaultTenantID, "jce.stale_refresh_sweep_hours", sweepHours)
		sweepLimit = s.settings.GetIntOr(ctx, s.defaultTenantID, "jce.stale_refresh_sweep_limit", sweepLimit)
	}

	const staleSweepSQL = `
SELECT ccm.tenant_id, ccm.content_id
FROM comparison_content_metadata ccm
JOIN contents c ON c.id = ccm.content_id AND c.tenant_id = ccm.tenant_id
WHERE ccm.smart_mode IS DISTINCT FROM 'none'
  AND c.deleted_at IS NULL
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND NOT EXISTS (
      SELECT 1 FROM comparison_data_ingestion_jobs j
      WHERE j.content_id = ccm.content_id AND j.tenant_id = ccm.tenant_id
        AND j.status IN ('pending','running')
  )
  AND (
      NOT EXISTS (
          SELECT 1 FROM comparison_data_ingestion_jobs j
          WHERE j.content_id = ccm.content_id AND j.tenant_id = ccm.tenant_id
            AND j.status = 'completed'
      )
      OR (
          SELECT MAX(j.completed_at) FROM comparison_data_ingestion_jobs j
          WHERE j.content_id = ccm.content_id AND j.tenant_id = ccm.tenant_id
            AND j.status = 'completed'
      ) < now() - make_interval(hours => $1)
  )
LIMIT $2
`

	rows, err := s.db.Reader.Query(ctx, staleSweepSQL, sweepHours, sweepLimit)
	if err != nil {
		return 0, fmt.Errorf("querying stale comparisons: %w", err)
	}
	defer rows.Close()

	var enqueued int

	for rows.Next() {
		var tenantID, contentID pgtype.UUID
		if err := rows.Scan(&tenantID, &contentID); err != nil {
			return enqueued, fmt.Errorf("scanning stale comparison: %w", err)
		}

		if !s.featureEnabled(ctx, tenantID, refreshFeatureFlag) {
			continue
		}

		if _, err := s.enqueueRefresh(ctx, tenantID, contentID, userID, nil, "low"); err != nil {
			// Log and continue with the rest of the sweep; a locked comparison
			// is not a fatal error for the batch.
			continue
		}

		enqueued++
	}

	if err := rows.Err(); err != nil {
		return enqueued, fmt.Errorf("stale sweep rows: %w", err)
	}

	return enqueued, nil
}

func refreshLockKey(contentID pgtype.UUID) string {
	return "jc:jce:refresh_lock:" + contentID.String()
}

func (s *Service) refreshLockDuration(ctx context.Context, tenantID pgtype.UUID) time.Duration {
	minutes := 5
	if s.settings != nil {
		minutes = s.settings.GetIntOr(ctx, tenantID, "jce.refresh_lock_ttl_minutes", minutes)
	}

	return time.Duration(minutes) * time.Minute
}

func (s *Service) releaseRefreshLock(ctx context.Context, contentID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	_ = s.vk.Del(ctx, refreshLockKey(contentID)).Err()
}

func ptrTo(s string) *string { return &s }
