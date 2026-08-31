-- JCE Sub-spec 2/3: profiles, score factors, scores, and recommendations.

-- =====================
-- Comparison profiles
-- =====================

-- name: CreateComparisonProfile :one
INSERT INTO comparison_profiles (
    tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template
) VALUES (
    @tenant_id, @user_id, @title, @source_space_id, @target_space_ids,
    @goals, @preferences, @demographics, @is_default, @is_template
)
RETURNING id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at;

-- name: ListComparisonProfilesByUser :many
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND deleted_at IS NULL
ORDER BY is_default DESC, updated_at DESC;

-- name: ListComparisonProfilesByTenant :many
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
ORDER BY is_default DESC, updated_at DESC;

-- name: ListComparisonProfileTemplates :many
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND is_template = true
  AND deleted_at IS NULL
ORDER BY title ASC;

-- name: GetComparisonProfile :one
SELECT id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at
FROM comparison_profiles
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UpdateComparisonProfile :one
UPDATE comparison_profiles
SET title = @title,
    source_space_id = @source_space_id,
    target_space_ids = @target_space_ids,
    goals = @goals,
    preferences = @preferences,
    demographics = @demographics,
    is_default = @is_default,
    is_template = @is_template,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, user_id, title, source_space_id, target_space_ids,
    goals, preferences, demographics, is_default, is_template, deleted_at, created_at, updated_at;

-- name: DeleteComparisonProfile :exec
UPDATE comparison_profiles
SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ClearDefaultProfileForUser :exec
UPDATE comparison_profiles
SET is_default = false, updated_at = now()
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND is_default = true
  AND deleted_at IS NULL
  AND id <> @exclude_id;

-- name: CountComparisonProfilesBySourceSpace :many
SELECT source_space_id, count(*)::int AS profile_count
FROM comparison_profiles
WHERE tenant_id = @tenant_id
  AND (user_id = @user_id OR @user_id IS NULL)
  AND is_template = false
  AND deleted_at IS NULL
GROUP BY source_space_id;

-- =====================
-- Score factors
-- =====================

-- name: CreateScoreFactor :one
INSERT INTO comparison_score_factors (
    tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, is_system
) VALUES (
    @tenant_id, @comparison_type_id, @factor_key, @label, @description,
    @dimension_slug, @data_source, @default_weight, @config, @is_system
)
RETURNING id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at;

-- name: ListScoreFactorsByType :many
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE tenant_id = @tenant_id
  AND (
      comparison_type_id = @comparison_type_id
      OR (sqlc.narg(include_global)::boolean = true AND comparison_type_id IS NULL)
  )
  AND (sqlc.narg(data_source)::text IS NULL OR data_source = sqlc.narg(data_source)::text)
ORDER BY is_system DESC, factor_key ASC;

-- name: GetScoreFactor :one
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateScoreFactor :one
UPDATE comparison_score_factors
SET comparison_type_id = @comparison_type_id,
    factor_key = @factor_key,
    label = @label,
    description = @description,
    dimension_slug = @dimension_slug,
    data_source = @data_source,
    default_weight = @default_weight,
    config = @config,
    is_system = @is_system,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at;

-- name: DeleteScoreFactor :exec
DELETE FROM comparison_score_factors
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetScoreFactorByKey :one
SELECT id, tenant_id, comparison_type_id, factor_key, label, description,
    dimension_slug, data_source, default_weight, config, version, is_system, created_at, updated_at
FROM comparison_score_factors
WHERE tenant_id = @tenant_id
  AND comparison_type_id = @comparison_type_id
  AND factor_key = @factor_key;

-- =====================
-- Scores
-- =====================

-- name: UpsertComparisonScore :one
INSERT INTO comparison_scores (
    tenant_id, content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, version
) VALUES (
    @tenant_id, @content_id, @item_id, @factor_id, @dimension_id, @profile_id,
    @score_value, @max_value, @explanation, @eligible_status, @version
)
ON CONFLICT (tenant_id, content_id, item_id, factor_id, profile_id) DO UPDATE
SET score_value = EXCLUDED.score_value,
    max_value = EXCLUDED.max_value,
    explanation = EXCLUDED.explanation,
    eligible_status = EXCLUDED.eligible_status,
    version = EXCLUDED.version,
    calculated_at = now()
RETURNING id, tenant_id, content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, calculated_at, version;

-- name: ListComparisonScoresByContent :many
SELECT id, tenant_id, content_id, item_id, factor_id, dimension_id, profile_id,
    score_value, max_value, explanation, eligible_status, calculated_at, version
FROM comparison_scores
WHERE tenant_id = @tenant_id
  AND content_id = @content_id
  AND (profile_id IS NOT DISTINCT FROM @profile_id)
ORDER BY item_id, factor_id;

-- name: DeleteComparisonScoresByContent :exec
DELETE FROM comparison_scores
WHERE tenant_id = @tenant_id
  AND content_id = @content_id
  AND (profile_id IS NOT DISTINCT FROM @profile_id);

-- =====================
-- Recommendations
-- =====================

-- name: CreateComparisonRecommendation :one
INSERT INTO comparison_recommendations (
    tenant_id, content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority
) VALUES (
    @tenant_id, @content_id, @item_id, @profile_id, @factor_id, @text, @action_type, @action_url, @priority
)
RETURNING id, tenant_id, content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority, created_at, updated_at;

-- name: ListComparisonRecommendationsByContent :many
SELECT id, tenant_id, content_id, item_id, profile_id, factor_id, text, action_type, action_url, priority, created_at, updated_at
FROM comparison_recommendations
WHERE tenant_id = @tenant_id
  AND content_id = @content_id
  AND (profile_id IS NOT DISTINCT FROM @profile_id)
ORDER BY priority DESC, created_at ASC;

-- name: DeleteComparisonRecommendationsByContent :exec
DELETE FROM comparison_recommendations
WHERE tenant_id = @tenant_id
  AND content_id = @content_id
  AND (profile_id IS NOT DISTINCT FROM @profile_id);

-- =====================
-- Scoring helpers
-- =====================

-- name: GetComparisonItemsForScoring :many
SELECT id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at
FROM comparison_items
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY position ASC;

-- name: GetComparisonValuesForScoring :many
SELECT cv.dimension_id, cv.item_id, cv.value, cv.value_rating, cv.value_boolean, cd.slug AS dimension_slug
FROM comparison_values cv
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id AND cd.tenant_id = cv.tenant_id
WHERE cv.tenant_id = @tenant_id
  AND cd.content_id = @content_id
  AND cv.item_id = ANY(@item_ids::uuid[]);

-- =====================
-- Data sources (JCE Sub-spec 4)
-- =====================

-- name: CreateDataSource :one
INSERT INTO comparison_data_sources (
    tenant_id, entity_type, entity_id, dimension_slug, source_name, source_url, source_kind, trust_level, config
) VALUES (
    @tenant_id, sqlc.narg(entity_type), sqlc.narg(entity_id), sqlc.narg(dimension_slug), @source_name, sqlc.narg(source_url), @source_kind, @trust_level, @config
)
RETURNING *;

-- name: ListDataSources :many
SELECT * FROM comparison_data_sources
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
  AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid)
ORDER BY source_name ASC
LIMIT NULLIF(@page_limit, 0) OFFSET @page_offset;

-- name: CountDataSources :one
SELECT count(*)::int FROM comparison_data_sources
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
  AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid);

-- name: GetDataSource :one
SELECT * FROM comparison_data_sources
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateDataSource :one
UPDATE comparison_data_sources
SET entity_type = sqlc.narg(entity_type),
    entity_id = sqlc.narg(entity_id),
    dimension_slug = sqlc.narg(dimension_slug),
    source_name = @source_name,
    source_url = sqlc.narg(source_url),
    source_kind = @source_kind,
    trust_level = @trust_level,
    freshness_score = COALESCE(sqlc.narg(freshness_score), freshness_score),
    last_verified_at = sqlc.narg(last_verified_at),
    config = @config,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: DeleteDataSource :exec
DELETE FROM comparison_data_sources
WHERE id = @id AND tenant_id = @tenant_id;

-- name: TouchDataSource :exec
-- Updates the freshness bookkeeping columns for a data source after it has
-- contributed to a successful refresh.
UPDATE comparison_data_sources
SET last_verified_at = COALESCE(sqlc.narg(last_verified_at), last_verified_at),
    freshness_score = COALESCE(sqlc.narg(freshness_score), freshness_score),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: MatchDataSource :one
-- Finds the most specific data source for the given scope, falling back to more general scopes.
-- An empty @source_name matches any source_name. A NULL @source_kind matches any source_kind.
SELECT * FROM comparison_data_sources
WHERE tenant_id = @tenant_id
  AND (source_name = @source_name OR @source_name = '')
  AND (entity_type IS NOT DISTINCT FROM sqlc.narg(entity_type)::text)
  AND (entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::uuid)
  AND (dimension_slug IS NOT DISTINCT FROM sqlc.narg(dimension_slug)::text)
  AND (sqlc.narg(source_kind)::text IS NULL OR source_kind = sqlc.narg(source_kind)::text)
ORDER BY
  (entity_id IS NOT NULL)::int + (dimension_slug IS NOT NULL)::int DESC,
  trust_level DESC,
  freshness_score DESC,
  created_at ASC
LIMIT 1;

-- =====================
-- Ingestion jobs (JCE Sub-spec 4)
-- =====================

-- name: CreateIngestionJob :one
INSERT INTO comparison_data_ingestion_jobs (
    tenant_id, content_id, source_kind
) VALUES (
    @tenant_id, @content_id, sqlc.narg(source_kind)
)
RETURNING *;

-- name: GetIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateIngestionJob :one
UPDATE comparison_data_ingestion_jobs
SET status = @status,
    refreshed_count = @refreshed_count,
    error = sqlc.narg(error),
    started_at = sqlc.narg(started_at),
    completed_at = sqlc.narg(completed_at),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: ListIngestionJobsByContent :many
SELECT * FROM comparison_data_ingestion_jobs
WHERE tenant_id = @tenant_id AND content_id = @content_id
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetLatestCompletedIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE tenant_id = @tenant_id AND content_id = @content_id AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1;
