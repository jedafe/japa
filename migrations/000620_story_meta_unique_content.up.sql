-- 000620_story_meta_unique_content.up.sql
-- Community Story Wall (§79, ADR-087) — enforce one story_meta row per
-- contents row.
--
-- The original table (000298) declared only a non-unique index on
-- content_id. Two write paths (the generic contents pipeline's
-- syncStoryMeta and the dedicated /stories service's CreateStoryMeta) could
-- therefore create parallel rows for the same story, which both duplicated
-- stories in list queries (JOIN story_meta) and made single-row reads
-- non-deterministic.
--
-- 1) Dedupe: keep the most recently updated row per (content_id, tenant_id),
--    preferring rows that carry data when updated_at ties.
-- 2) Add a UNIQUE constraint so concurrent writers upsert instead of
--    duplicating.

DELETE FROM story_meta a
USING story_meta b
WHERE a.tenant_id = b.tenant_id
  AND a.content_id = b.content_id
  AND (
        a.updated_at < b.updated_at
        OR (a.updated_at = b.updated_at AND a.id::text < b.id::text)
      );

ALTER TABLE story_meta
    ADD CONSTRAINT story_meta_content_tenant_key UNIQUE (content_id, tenant_id);
