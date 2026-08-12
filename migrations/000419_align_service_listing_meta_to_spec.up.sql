-- 000419_align_service_listing_meta_to_spec.up.sql
-- Service Directory (§98, ADR-106) — align service_listing_meta to the spec:
--   * origin_communities: text → text[]  (§98 schema: TEXT[])
--   * claim_status default: 'none' → 'unclaimed'  (§98 schema)
--   * migrate existing 'none' rows to 'unclaimed'
--   * add verified_by / verified_at for audit trail (ADR-088 HITL)
-- Ref: japaconnect-engineering/sections/98-service-directory.md
--      japaconnect-engineering/adr/ADR-106-service-directory.md

BEGIN;

-- origin_communities: convert text (JSON blob) → text[].
-- Existing rows hold either NULL or a JSON array string like '["Nigerian","Ghanaian"]'.
-- Use jsonb_array_elements_text to extract elements into a native text[].
ALTER TABLE service_listing_meta
    ALTER COLUMN origin_communities DROP DEFAULT;

ALTER TABLE service_listing_meta
    ALTER COLUMN origin_communities TYPE text[]
    USING CASE
        WHEN origin_communities IS NULL OR origin_communities::text = '[]' THEN ARRAY[]::text[]
        WHEN origin_communities::text ~ '^\[' THEN
            ARRAY(SELECT value::text
                  FROM jsonb_array_elements_text(origin_communities::jsonb) AS value)
        ELSE ARRAY[origin_communities]  -- legacy scalar free-text → single-element array
    END;

ALTER TABLE service_listing_meta
    ALTER COLUMN origin_communities SET DEFAULT ARRAY[]::text[];

-- claim_status: fix default and migrate 'none' → 'unclaimed'.
ALTER TABLE service_listing_meta
    ALTER COLUMN claim_status SET DEFAULT 'unclaimed';

UPDATE service_listing_meta
    SET claim_status = 'unclaimed'
    WHERE claim_status = 'none';

-- Audit trail for verification (ADR-088 HITL).
ALTER TABLE service_listing_meta
    ADD COLUMN IF NOT EXISTS verified_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS verified_at timestamptz;

-- Backfill verified_at for already-verified rows using updated_at as best-effort.
UPDATE service_listing_meta
    SET verified_at = updated_at
    WHERE is_verified = true AND verified_at IS NULL;

COMMIT;
