-- 000619_fix_roadmap_variant_geo_metadata.up.sql
-- Repair roadmap guide variants whose target_country / source_country do not
-- match the geo pair encoded in their slug suffix "<base>-<dest>-<origin>"
-- (migration 000587 naming convention). A handful of published variants were
-- written with stale geo columns (e.g. slug "-ca-ng" but target_country='GB'),
-- which makes the readiness assessment's geo content resolution mis-resolve
-- them for the wrong journey and never match the intended one. Those rows also
-- carry spurious journey_stage/visa_route values that no other variant uses,
-- so they are cleared on the corrected rows.
--
-- Idempotent: only rows whose stored geo pair differs from the slug-encoded
-- pair are touched.
--
-- Ref: .jeos/adr/ADR-071-japa-readiness-assessment.md
--      .jeos/sections/71-readiness-assessment.md

BEGIN;

UPDATE contents
SET target_country = upper(substring(slug from '-([a-z]{2})-[a-z]{2}$')),
    source_country = upper(substring(slug from '-([a-z]{2})$')),
    journey_stage  = NULL,
    visa_route     = NULL,
    updated_at     = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND deleted_at IS NULL
  AND guide_category = 'roadmap'
  AND slug ~ '-[a-z]{2}-[a-z]{2}$'
  AND (
      target_country IS DISTINCT FROM upper(substring(slug from '-([a-z]{2})-[a-z]{2}$'))
      OR source_country IS DISTINCT FROM upper(substring(slug from '-([a-z]{2})$'))
  );

COMMIT;
