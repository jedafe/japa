-- Content, revision, slug history, pinned posts, JSON-LD templates, and scores.

-- =====================
-- Contents
-- =====================

-- name: CreateContent :one
INSERT INTO contents (
    tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
    field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
    canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
    status, visibility, read_time_minutes, publish_at, published_at, guide_category,
    ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at,
    journey_stage, visa_route, source_country, target_country
) VALUES (
    @tenant_id, @author_id, sqlc.narg(organization_id), sqlc.narg(space_id), @content_type, @title, @slug, @body, @body_html,
    @field_values, sqlc.narg(content_hash), sqlc.narg(geo_space_id),
    sqlc.narg(header_image_id), sqlc.narg(featured_image_id),
    sqlc.narg(canonical_url), sqlc.narg(meta_title), sqlc.narg(meta_description),
    sqlc.narg(og_image_url), @seo_indexable,
    @status, @visibility, @read_time_minutes, sqlc.narg(publish_at), sqlc.narg(published_at),
    sqlc.narg(guide_category),
    COALESCE(sqlc.narg(ai_generated), false), sqlc.narg(ai_model), sqlc.narg(ai_generated_at),
    sqlc.narg(hitl_reviewed_by), sqlc.narg(hitl_reviewed_at),
    sqlc.narg(journey_stage), sqlc.narg(visa_route), sqlc.narg(source_country), sqlc.narg(target_country)
)
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category,
          ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at,
          journey_stage, visa_route, source_country, target_country;

-- name: GetContentByID :one
SELECT id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category,
       ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at,
       journey_stage, visa_route, source_country, target_country
FROM contents
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetContentByHash :one
-- Duplicate-detection check (Rule 12 / BL-006). Returns a conflicting row
-- with the same (tenant_id, content_hash), excluding the row being updated.
SELECT id, tenant_id, author_id, content_hash
FROM contents
WHERE tenant_id = @tenant_id
  AND content_hash = @content_hash
  AND deleted_at IS NULL
  AND id <> @exclude_id;

-- name: GetContentBySlug :one
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count,
       (SELECT COUNT(*)
        FROM comments
        WHERE content_id = c.id
          AND tenant_id = c.tenant_id
          AND deleted_at IS NULL
          AND status = 'published') AS comment_count,
       c.share_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country
FROM contents c
WHERE c.slug = @slug AND c.tenant_id = @tenant_id AND c.deleted_at IS NULL;

-- name: ContentSlugExists :one
-- Returns true if any contents row (including soft-deleted) in the tenant
-- uses the given slug. The unique index on (tenant_id, slug) is not partial,
-- so a slug held by a deleted row is still unavailable.
SELECT EXISTS(SELECT 1 FROM contents WHERE tenant_id = @tenant_id AND slug = @slug);

-- name: ListContentsByTenant :many
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.field_values->>'purpose' AS purpose,
       u.username AS author_username, up.display_name AS author_display_name,
       s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id AND u.tenant_id = c.tenant_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
LEFT JOIN spaces s ON s.id = c.space_id AND s.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND (@q::text IS NULL OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%' OR c.body ILIKE '%' || @q || '%')
  AND (c.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: ListContentsByIDs :many
-- Fetches content rows by a list of IDs with the same JOINed enrichment
-- as ListContentsByTenant. Used by the Meilisearch search integration:
-- when ?q= is present, the handler queries Meilisearch for matching IDs,
-- then uses this query to fetch the full rows with author/space names.
-- Ref: docs/PERFORMANCE_AUDIT.md §4 (Search Offload)
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.field_values->>'purpose' AS purpose,
       u.username AS author_username, up.display_name AS author_display_name,
       s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id AND u.tenant_id = c.tenant_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
LEFT JOIN spaces s ON s.id = c.space_id AND s.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.id = ANY(@ids::uuid[])
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: ListContentsBySlugs :many
-- Fetches published content rows by a list of slugs with the same JOINed
-- enrichment as the other feed queries. Used by the journey-aware roadmap feed
-- to render the next steps from a readiness assessment in slug order, paginated.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.field_values->>'purpose' AS purpose,
       u.username AS author_username,
       up.display_name AS author_display_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       s.name AS space_name,
       COALESCE(s.slug, '') AS space_slug,
       COALESCE(cs.vote_score, 0) AS vote_score,
       COALESCE(cs.bookmark_count, 0) AS bookmark_count,
       COALESCE(cs.velocity_score, 0) AS velocity_score,
       LEFT(c.body, 200)::text AS excerpt,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path AS cover_storage_path,
       COALESCE((SELECT string_agg(t.name, ',' ORDER BY t.name)
                 FROM content_tags ct
                 JOIN tags t ON t.id = ct.tag_id
                 WHERE ct.content_id = c.id), '') AS tags
FROM contents c
LEFT JOIN users u ON u.id = c.author_id AND u.tenant_id = c.tenant_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
LEFT JOIN spaces s ON s.id = c.space_id AND s.tenant_id = c.tenant_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id) AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id
  AND c.slug = ANY(@slugs::text[])
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
ORDER BY array_position(@slugs::text[], c.slug)
LIMIT @page_limit OFFSET @page_offset;

-- name: ListContentsByType :many
-- Lists content filtered by content_type with optional guide_category.
-- Supports composite cursor (created_at, id) so pages with identical timestamps
-- are not skipped by timestamp-only cursor pagination.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.field_values->>'purpose' AS purpose,
       u.username AS author_username, up.display_name AS author_display_name,
       s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id AND u.tenant_id = c.tenant_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
LEFT JOIN spaces s ON s.id = c.space_id AND s.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id AND c.content_type = @content_type
  AND (@guide_category::text = '' OR c.guide_category = @guide_category::text)
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND (
      (@cursor_created_at::timestamptz IS NULL)
      OR (c.created_at < @cursor_created_at::timestamptz)
      OR (c.created_at = @cursor_created_at::timestamptz AND (@cursor_id::uuid IS NULL OR c.id < @cursor_id))
  )
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: ListContentsBySpace :many
-- Lists content assigned to a given space via the authoritative
-- content_spaces table (BUG-010), with visibility gating. The space name
-- returned is the requested space, regardless of whether it is the primary.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
       c.field_values->>'purpose' AS purpose,
       u.username AS author_username, up.display_name AS author_display_name,
       s.name AS space_name
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
LEFT JOIN users u ON u.id = c.author_id AND u.tenant_id = c.tenant_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id AND up.tenant_id = c.tenant_id
LEFT JOIN spaces s ON s.id = cs.space_id AND s.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND (c.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: SearchContentsBySpace :many
-- Searches content assigned to a given space via the authoritative
-- content_spaces table (BUG-010), with visibility gating.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
WHERE c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND c.title ILIKE '%' || @query || '%'
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: SearchContentsByTypeAndSpace :many
-- Searches content of a given content_type assigned to a space via the
-- authoritative content_spaces table (BUG-010), with visibility gating.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
WHERE c.tenant_id = @tenant_id
  AND c.content_type = @content_type
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND c.title ILIKE '%' || @query || '%'
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContent :one
UPDATE contents
SET title = @title, slug = @slug, body = @body, body_html = @body_html,
    field_values = @field_values, content_hash = COALESCE(sqlc.narg(content_hash), content_hash),
    geo_space_id = sqlc.narg(geo_space_id), canonical_url = sqlc.narg(canonical_url),
    header_image_id = sqlc.narg(header_image_id),
    featured_image_id = sqlc.narg(featured_image_id),
    meta_title = sqlc.narg(meta_title),
    meta_description = sqlc.narg(meta_description),
    og_image_url = sqlc.narg(og_image_url),
    seo_indexable = COALESCE(sqlc.narg(seo_indexable), seo_indexable),
    status = @status, visibility = @visibility, read_time_minutes = @read_time_minutes,
    publish_at = sqlc.narg(publish_at), published_at = sqlc.narg(published_at),
    is_locked = @is_locked, is_pinned = @is_pinned,
    organization_id = sqlc.narg(organization_id), guide_category = COALESCE(sqlc.narg(guide_category), guide_category),
    ai_generated_at = COALESCE(sqlc.narg(ai_generated_at), ai_generated_at),
    ai_model = COALESCE(sqlc.narg(ai_model), ai_model),
    journey_stage = COALESCE(sqlc.narg(journey_stage), journey_stage),
    visa_route = COALESCE(sqlc.narg(visa_route), visa_route),
    source_country = COALESCE(sqlc.narg(source_country), source_country),
    target_country = COALESCE(sqlc.narg(target_country), target_country),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category,
          journey_stage, visa_route, source_country, target_country;

-- name: UpdateContentStatus :one
UPDATE contents
SET status = @status, updated_at = now(),
    published_at = CASE WHEN @status = 'published' AND published_at IS NULL THEN now() ELSE published_at END
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category,
          journey_stage, visa_route, source_country, target_country;

-- name: ReassignSingleContentAuthor :one
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Authorship Assignment)
-- Updates a single content row's author_id and field_values in one
-- statement. The caller (agent handler) reads the current field_values,
-- merges the _ai_provenance.author_assigned_by / author_assignment_reason
-- metadata in Go, and passes the full updated JSONB here. Tenant-scoped
-- for defense-in-depth (BUG-014). Distinct from gdpr.sql's
-- ReassignContentAuthor (:exec bulk reassign on user deletion).
UPDATE contents
SET author_id = @author_id, field_values = @field_values, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category,
          journey_stage, visa_route, source_country, target_country;

-- name: ScheduleContent :one
-- Sets status='scheduled' and publish_at without touching title/body/other
-- fields (catalog: POST /content/:id/schedule). The scheduled_publish Asynq
-- task (enqueued by the caller with ProcessAt=publish_at) flips status to
-- 'published' when the time arrives.
UPDATE contents
SET status = 'scheduled', publish_at = @publish_at, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category,
          journey_stage, visa_route, source_country, target_country;

-- name: IncrementContentView :exec
UPDATE contents SET view_count = view_count + 1 WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: RecordContentView :execresult
-- Inserts a deduplicated view record into user_content_views (Section 14).
-- The PK deduplicates per user/session per day; ON CONFLICT DO NOTHING.
-- country_code is resolved server-side from the client IP via GeoIP.
-- Returns the command tag so callers can detect whether a row was actually
-- inserted (RowsAffected == 1) vs. deduplicated (RowsAffected == 0).
INSERT INTO user_content_views (content_id, user_id, session_id, country_code, viewed_at)
VALUES (@content_id, @user_id, @session_id, @country_code, now())
ON CONFLICT DO NOTHING;

-- name: IncrementContentCommentCount :exec
-- Increments the denormalized comment_count on a content row. Called
-- synchronously when a comment is created so feed cards (which read
-- c.comment_count) show accurate counts without waiting for the async
-- scoring pipeline. Ref: Section 32 (feed reads contents.comment_count).
UPDATE contents SET comment_count = comment_count + 1
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: DecrementContentCommentCount :exec
-- Decrements comment_count when a comment is soft-deleted/removed.
UPDATE contents SET comment_count = GREATEST(comment_count - 1, 0)
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: DeleteContent :exec
UPDATE contents SET deleted_at = now(), updated_at = now() WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Content Revisions
-- =====================

-- name: CreateContentRevision :one
INSERT INTO content_revisions (content_id, field_values, title, body, body_html, edited_by)
VALUES (@content_id, @field_values, @title, @body, @body_html, @edited_by)
RETURNING id, content_id, field_values, title, body, body_html, edited_by, created_at;

-- name: ListContentRevisions :many
SELECT cr.id, cr.content_id, cr.field_values, cr.title, cr.body, cr.body_html, cr.edited_by, cr.created_at
FROM content_revisions cr
JOIN contents c ON c.id = cr.content_id
WHERE cr.content_id = @content_id
  AND c.tenant_id = @tenant_id
ORDER BY cr.created_at DESC, cr.id DESC;

-- name: GetContentRevision :one
SELECT cr.id, cr.content_id, cr.field_values, cr.title, cr.body, cr.body_html, cr.edited_by, cr.created_at
FROM content_revisions cr
JOIN contents c ON c.id = cr.content_id
WHERE cr.id = @id
  AND cr.content_id = @content_id
  AND c.tenant_id = @tenant_id;

-- =====================
-- Slug History
-- =====================

-- name: CreateSlugHistory :one
INSERT INTO slug_history (entity_type, entity_id, old_slug, new_slug)
VALUES (@entity_type, @entity_id, @old_slug, @new_slug)
RETURNING id, entity_type, entity_id, old_slug, new_slug, created_at;

-- name: GetSlugRedirect :one
SELECT id, entity_type, entity_id, old_slug, new_slug, created_at
FROM slug_history
WHERE entity_type = @entity_type AND old_slug = @old_slug
ORDER BY created_at DESC
LIMIT 1;

-- =====================
-- Space Pinned Posts
-- =====================

-- name: PinContentToSpace :one
INSERT INTO space_pinned_posts (space_id, content_id, pinned_by, position)
VALUES (@space_id, @content_id, @pinned_by, @position)
RETURNING id, space_id, content_id, pinned_by, position, created_at;

-- name: UnpinContentFromSpace :exec
DELETE FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id;

-- name: ListPinnedContent :many
SELECT spp.id, spp.space_id, spp.content_id, spp.pinned_by, spp.position, spp.created_at
FROM space_pinned_posts spp
WHERE spp.space_id = @space_id
ORDER BY spp.position ASC, spp.created_at ASC;

-- name: IsContentPinned :one
SELECT EXISTS(SELECT 1 FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id) AS exists;

-- =====================
-- JSON-LD Templates
-- =====================

-- name: GetJSONLDTemplate :one
SELECT id, tenant_id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE tenant_id = @tenant_id AND content_type = @content_type AND is_active = true;

-- name: ListJSONLDTemplates :many
SELECT id, tenant_id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR content_type ILIKE '%' || @q || '%' OR template::text ILIKE '%' || @q || '%')
ORDER BY created_at ASC, id ASC;

-- name: UpsertJSONLDTemplate :one
INSERT INTO content_type_jsonld_templates (tenant_id, content_type, template, is_active)
VALUES (@tenant_id, @content_type, @template, @is_active)
ON CONFLICT (tenant_id, content_type) DO UPDATE
SET template = EXCLUDED.template, is_active = EXCLUDED.is_active, updated_at = now()
RETURNING id, tenant_id, content_type, template, is_active, created_at, updated_at;

-- name: DeleteJSONLDTemplate :exec
DELETE FROM content_type_jsonld_templates WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Content Scores
-- =====================

-- name: UpsertContentScore :one
INSERT INTO content_scores (content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score)
VALUES (@content_id, @vote_score, @comment_count, @view_count, @bookmark_count, @velocity_score)
ON CONFLICT (content_id) DO UPDATE
SET vote_score = EXCLUDED.vote_score, comment_count = EXCLUDED.comment_count,
    view_count = EXCLUDED.view_count, bookmark_count = EXCLUDED.bookmark_count,
    velocity_score = EXCLUDED.velocity_score, last_scored_at = now()
RETURNING content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, relevance_embedding, last_scored_at;

-- name: EnsureContentScore :exec
-- Creates a zero-default content_scores row if one does not yet exist.
-- Called on content publish and by the engagement worker so that INNER JOIN
-- feed queries (explore/top/rising/space/profile) never exclude content for
-- lacking a score row, and so the worker can increment counts in place.
-- ON CONFLICT DO NOTHING preserves any existing scores.
INSERT INTO content_scores (content_id)
VALUES (@content_id)
ON CONFLICT (content_id) DO NOTHING;

-- name: IncrementScoreVote :exec
-- Atomically increments vote_score by delta. Used by the engagement worker
-- to avoid the read-modify-write race that loses deltas when concurrent
-- events (vote, comment, bookmark, view) overwrite each other's columns.
UPDATE content_scores
SET vote_score = vote_score + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreComment :exec
-- Atomically adjusts comment_count by delta (clamped at 0).
UPDATE content_scores
SET comment_count = GREATEST(0, comment_count + @delta), last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreView :exec
-- Atomically increments view_count by delta.
UPDATE content_scores
SET view_count = view_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreBookmark :exec
-- Atomically increments bookmark_count by delta.
UPDATE content_scores
SET bookmark_count = bookmark_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: GetContentScore :one
SELECT content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, relevance_embedding, last_scored_at
FROM content_scores
WHERE content_id = @content_id;

-- name: ListContentIDsScoredSince :many
-- Returns content IDs whose content_scores.last_scored_at is newer than the
-- given cursor, joined to contents for tenant scoping. Used by the §13
-- batched engagement-score re-index cron to find content whose search index
-- document needs refreshing without re-indexing on every engagement event.
-- Limits to @batch_size rows ordered by last_scored_at ascending so the
-- cursor advances monotonically.
SELECT c.id, c.tenant_id
FROM content_scores cs
JOIN contents c ON c.id = cs.content_id
WHERE cs.last_scored_at > @cursor AND c.deleted_at IS NULL
ORDER BY cs.last_scored_at ASC
LIMIT @batch_size;

-- =====================
-- Content Tags
-- =====================

-- name: AddContentTag :exec
INSERT INTO content_tags (content_id, tag_id)
VALUES (@content_id, @tag_id)
ON CONFLICT (content_id, tag_id) DO NOTHING;

-- name: RemoveContentTag :exec
DELETE FROM content_tags WHERE content_id = @content_id AND tag_id = @tag_id;

-- name: ListContentTags :many
SELECT t.id, t.name, t.slug, t.description, t.color, t.follower_count
FROM content_tags ct
JOIN tags t ON t.id = ct.tag_id
WHERE ct.content_id = @content_id
ORDER BY t.name;

-- name: GetContentSpaces :many
-- Returns all space assignments for a content item (Section 14: content_spaces).
-- Used by feed fan-out (Section 32) to distribute a post to every assigned space.
SELECT cs.space_id, cs.is_primary
FROM content_spaces cs
WHERE cs.content_id = @content_id
ORDER BY cs.is_primary DESC, cs.created_at ASC;

-- name: DeleteContentSpaces :exec
-- Removes all space assignments for a content item (Section 14).
-- Used by UpdateContentSpaces to replace the full assignment set.
DELETE FROM content_spaces WHERE content_id = @content_id;

-- name: AddContentSpace :exec
-- Inserts a single space assignment for a content item (Section 14).
-- is_primary must be true for exactly one row per content item.
INSERT INTO content_spaces (content_id, space_id, is_primary)
VALUES (@content_id, @space_id, @is_primary);

-- name: SetContentSpaceID :exec
-- Updates the primary space columns (space_id and geo_space_id) on a content
-- item to keep contents and content_spaces synchronized. Used by the
-- auto-assign feature and UpdateContentSpaces. Ref: Section 14.
UPDATE contents SET space_id = @space_id, geo_space_id = @space_id, updated_at = now()
WHERE id = @content_id;

-- name: SetContentPrimarySpace :exec
-- Sets is_primary=false on all existing content_spaces for this content,
-- then ensures the requested space is present with is_primary=true. Used
-- when UpdateContent changes the primary space without losing additional
-- assignments. Ref: Section 14.
WITH reassigned AS (
    UPDATE content_spaces
    SET is_primary = false
    WHERE content_id = @content_id
    RETURNING space_id
)
INSERT INTO content_spaces (content_id, space_id, is_primary)
SELECT @content_id, @space_id, true
FROM reassigned
ON CONFLICT (content_id, space_id) DO UPDATE SET is_primary = true;

-- name: ListContentByTag :many
-- Composite cursor (created_at, id) avoids skipping items that share the
-- same created_at timestamp as the last item on a page. Ref: §4.1.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path   AS cover_storage_path
FROM contents c
JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id) AND m.deleted_at IS NULL
WHERE ct.tag_id = @tag_id AND c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (
      (@can_see_unpublished::boolean AND @can_see_private::boolean)
      OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
      OR (
          c.status = 'published'
          AND (
              c.visibility = 'public'
              OR (c.visibility = 'members' AND @can_see_members::boolean)
              OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
          )
      )
  )
  AND (
    @cursor_created_at::timestamptz IS NULL
    OR c.created_at < @cursor_created_at::timestamptz
    OR (c.created_at = @cursor_created_at::timestamptz AND c.id < @cursor_id::uuid)
  )
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- =====================
-- Content Shares
-- =====================

-- name: CreateContentShare :one
INSERT INTO content_shares (tenant_id, content_id, user_id, platform)
VALUES (@tenant_id, @content_id, @user_id, @platform)
RETURNING id, tenant_id, content_id, user_id, platform, created_at;

-- name: CountContentShares :one
SELECT COUNT(*)::int AS count
FROM content_shares
WHERE tenant_id = @tenant_id AND content_id = @content_id;

-- =====================
-- Bookmarks (polymorphic)
-- =====================

-- name: CreateBookmark :one
INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder, folder_id)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @folder, @folder_id)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE SET folder = EXCLUDED.folder, folder_id = EXCLUDED.folder_id
RETURNING id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at;

-- name: SetBookmarkFolder :exec
UPDATE bookmarks SET folder_id = @folder_id
WHERE tenant_id = @tenant_id AND user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks
WHERE tenant_id = @tenant_id AND user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteBookmarkByID :execrows
-- Deletes a single bookmark by its row ID, scoped to tenant + owner.
-- Used by DELETE /me/bookmarks/{id} — the polymorphic remove endpoint that
-- works regardless of entity_type (content, forum_thread, space, comment, etc.).
DELETE FROM bookmarks
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;

-- name: IsBookmarked :one
SELECT EXISTS(
    SELECT 1 FROM bookmarks
    WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: ListBookmarksByUser :many
SELECT b.id, b.tenant_id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListBookmarksByFolder :many
SELECT b.id, b.tenant_id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id AND b.folder_id = @folder_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBookmarksByUser :one
SELECT COUNT(*)::int AS count
FROM bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: ListPublishedContentByAuthor :many
-- Public profile Posts tab (GET /users/{id}/content): a user's PUBLISHED PUBLIC
-- content, newest first, paginated. author_id is the user id (FK users.id).
-- LEFT JOINs organizations so dual-surface posts carry the "on behalf of" badge.
-- Pinned posts (is_pinned = true) are sorted to the top so the profile
-- surfaces the author's featured/top content first.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.is_pinned DESC, c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublishedContentByOrganization :many
-- Org profile Posts tab (GET /organizations/{id}/content): the org's PUBLISHED
-- PUBLIC content, newest first, paginated. Keyed by organization_id (ADR-062).
-- author_id (the human) is projected for accountability; org badge fields via join.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.organization_id = @organization_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: PublishScheduledContent :execrows
UPDATE contents
SET status = 'published',
    published_at = COALESCE(published_at, now()),
    updated_at = now()
WHERE id = @id
  AND tenant_id = @tenant_id
  AND status = 'scheduled'
  AND publish_at <= now();

-- name: ListScheduledContentReady :many
SELECT id, tenant_id
FROM contents
WHERE status = 'scheduled'
  AND publish_at IS NOT NULL
  AND publish_at <= now()
ORDER BY publish_at ASC
LIMIT 100;

-- name: GetContentTitleAndBody :one
SELECT COALESCE(title, '') AS title, COALESCE(body, '') AS body
FROM contents
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListPublishedSlugsForSitemap :many
SELECT c.slug, c.updated_at, c.visibility, c.seo_indexable, c.content_type,
       c.comment_count, c.share_count, c.reshare_count,
       array_length(string_to_array(trim(c.body), ' '), 1) AS word_count,
       COALESCE(ft.reply_count, 0) AS reply_count,
       COALESCE(ft.solved, false) AS solved,
       (ft.id IS NOT NULL AND ft.solved = false) AS is_question,
       COALESCE(r.reaction_count, 0) AS reaction_count
FROM contents c
LEFT JOIN forum_threads ft ON ft.content_id = c.id
LEFT JOIN (
    SELECT entity_id, count(*) AS reaction_count
    FROM reactions
    WHERE entity_type IN ('content', 'blog_post', 'forum_thread', 'discussion', 'guide', 'comparison')
    GROUP BY entity_id
) r ON r.entity_id = c.id
WHERE c.tenant_id = @tenant_id AND c.status = 'published'
  AND c.deleted_at IS NULL AND c.visibility = 'public' AND c.seo_indexable = true
ORDER BY c.updated_at DESC
LIMIT 10000;

-- name: ListPublishedContentForRSS :many
SELECT title, slug, created_at
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published'
ORDER BY created_at DESC
LIMIT 50;

-- name: ListPublishedContentForLLMsTxt :many
SELECT title, slug, content_type
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published' AND visibility = 'public'
ORDER BY created_at DESC
LIMIT 1000;

-- name: ListPublishedContentForDigest :many
SELECT title, slug, created_at
FROM contents
WHERE tenant_id = @tenant_id
  AND status = 'published'
  AND created_at >= now() - interval '7 days'
ORDER BY created_at DESC
LIMIT 10;

-- name: GetPublishedContentByID :one
-- Public API (Section 47): returns a single content item only if it is
-- published, public, and not soft-deleted. Drafts, scheduled, private, and
-- member-only content are never exposed to external API consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE id = @id
  AND tenant_id = @tenant_id
  AND status = 'published'
  AND visibility = 'public'
  AND deleted_at IS NULL;

-- name: ArchiveContent :execrows
UPDATE contents SET status = 'archived', updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND status != 'archived';

-- name: GetPublicAuthorAnalytics :one
-- Public profile Analytics tab (GET /users/{id}/analytics): aggregates
-- published+public content stats for the profile owner. Soft-deleted and
-- non-public/draft content is excluded. total_likes counts reactions on the
-- author's published+public content. Follower/following counts come from the
-- users table / follows subquery so the analytics response is self-contained.
-- Uses subqueries for content aggregates so the user row is always returned
-- even when the user has zero published posts (no GROUP BY needed).
SELECT
    COALESCE((
        SELECT SUM(c.view_count)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_views,
    COALESCE((
        SELECT SUM(c.share_count)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_shares,
    COALESCE((
        SELECT COUNT(c.id)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_posts,
    COALESCE((
        SELECT COUNT(*)::int FROM reactions r
        WHERE r.tenant_id = @tenant_id
          AND r.entity_type = 'content'
          AND r.entity_id IN (
              SELECT c2.id FROM contents c2
              WHERE c2.tenant_id = @tenant_id
                AND c2.author_id = @author_id
                AND c2.status = 'published'
                AND c2.visibility = 'public'
                AND c2.deleted_at IS NULL
          )
    ), 0)::int AS total_likes,
    u.follower_count::int AS total_followers,
    (SELECT COUNT(*)::int FROM follows f
      WHERE f.tenant_id = @tenant_id AND f.follower_id = u.id) AS total_following
FROM users u
WHERE u.id = @author_id AND u.tenant_id = @tenant_id;

-- name: CountPostsByAuthor :one
-- Counts published, non-deleted content by the given author in a tenant.
-- Used by the spam pipeline's new-user link restriction (§41 Layer 3) to
-- determine whether a user has passed the "first N posts" threshold.
SELECT COUNT(*)::int AS count
FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.deleted_at IS NULL;

-- name: FindRoadmapSlugsByJourney :many
-- Selects published journey-relevant content slugs that match a visitor's
-- journey context. Returns destination-specific content first, then global
-- fallback content, with stage/visa/origin/purpose matches used as tie-breakers.
-- Used by the assessment result roadmap endpoint to surface geo-aware guides.
SELECT c.slug
FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND c.status = 'published'
  AND c.visibility IN ('public', 'members')
  AND (
      @destination_country::text = ''
      OR c.target_country = @destination_country
      OR c.target_country IS NULL
      OR c.target_country = ''
  )
  AND (
      @journey_stage::text = ''
      OR c.journey_stage = @journey_stage
      OR c.journey_stage IS NULL
      OR c.journey_stage = ''
  )
  AND (
      @visa_route::text = ''
      OR c.visa_route = @visa_route
      OR c.visa_route IS NULL
      OR c.visa_route = ''
  )
  AND (
      @origin_country::text = ''
      OR c.source_country = @origin_country
      OR c.source_country IS NULL
      OR c.source_country = ''
      OR c.target_country = @destination_country
  )
  AND (
      @purpose::text = ''
      OR c.field_values->>'purpose' = @purpose
      OR c.field_values->>'purpose' IS NULL
      OR c.field_values->>'purpose' = ''
  )
ORDER BY
  CASE WHEN c.target_country = @destination_country THEN 0 ELSE 1 END,
  CASE WHEN c.journey_stage = @journey_stage THEN 0 ELSE 1 END,
  CASE WHEN c.visa_route = @visa_route THEN 0 ELSE 1 END,
  CASE WHEN c.source_country = @origin_country THEN 0 ELSE 1 END,
  CASE WHEN c.field_values->>'purpose' = @purpose THEN 0 ELSE 1 END,
  c.view_count DESC,
  c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: FindRoadmapContentByDimension :many
-- Returns published content rows that match a scoring dimension and the
-- visitor's journey context. Destination-specific content is returned first,
-- then global fallback content, with stage/visa/origin/purpose matches used as
-- tie-breakers. Used by the assessment scoring worker to resolve a generic
-- curated step to the most specific published guide for the visitor's context.
--
-- The dimension is read from the content's field_values JSONB so any guide
-- category can be tagged with the readiness dimension it addresses.
SELECT c.slug, c.title, LEFT(c.body, 200)::text AS description,
       c.target_country, c.journey_stage, c.visa_route, c.source_country,
       c.field_values->>'purpose' AS purpose
FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND c.status = 'published'
  AND c.visibility IN ('public', 'members')
  AND c.field_values->>'dimension' = @dimension
  AND (
      @destination_country::text = ''
      OR c.target_country = @destination_country
      OR c.target_country IS NULL
      OR c.target_country = ''
  )
  AND (
      @journey_stage::text = ''
      OR c.journey_stage = @journey_stage
      OR c.journey_stage IS NULL
      OR c.journey_stage = ''
  )
  AND (
      @visa_route::text = ''
      OR c.visa_route = @visa_route
      OR c.visa_route IS NULL
      OR c.visa_route = ''
  )
  AND (
      @origin_country::text = ''
      OR c.source_country = @origin_country
      OR c.source_country IS NULL
      OR c.source_country = ''
      OR c.target_country = @destination_country
  )
  AND (
      @purpose::text = ''
      OR c.field_values->>'purpose' = @purpose
      OR c.field_values->>'purpose' IS NULL
      OR c.field_values->>'purpose' = ''
  )
ORDER BY
  CASE WHEN c.target_country = @destination_country THEN 0 ELSE 1 END,
  CASE WHEN c.journey_stage = @journey_stage THEN 0 ELSE 1 END,
  CASE WHEN c.visa_route = @visa_route THEN 0 ELSE 1 END,
  CASE WHEN c.source_country = @origin_country THEN 0 ELSE 1 END,
  CASE WHEN c.field_values->>'purpose' = @purpose THEN 0 ELSE 1 END,
  c.view_count DESC,
  c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;
