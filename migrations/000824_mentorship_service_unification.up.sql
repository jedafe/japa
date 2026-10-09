-- 000824_mentorship_service_unification.up.sql
-- Mentorship merges into the service layer. Mentorship offerings are
-- service_listing_meta rows with listing_kind='mentorship'; mentorship
-- requests are service_bookings (intent='inquiry', provider_user_id = mentor).
-- mentorship_connections is dropped after its rows are migrated into
-- service_bookings, and the mentee-count trigger is re-pointed.
--
-- Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md

BEGIN;

-- 1. New listing kind + category applies_to value.
ALTER TABLE service_listing_meta
    DROP CONSTRAINT service_listing_meta_listing_kind_check,
    ADD CONSTRAINT service_listing_meta_listing_kind_check
        CHECK (listing_kind IN ('service','package','mentorship'));

ALTER TABLE service_categories
    DROP CONSTRAINT service_categories_applies_to_check,
    ADD CONSTRAINT service_categories_applies_to_check
        CHECK (applies_to IN ('service','package','mentorship','both'));

-- 2. Bookings gain a user-owned provider dimension (mentorship listings
--    are authored by the mentor user, not an org) plus request context.
--    The OR anonymized_at clause lets a GDPR-scrubbed row survive after
--    both owner pointers are nulled.
ALTER TABLE service_bookings
    ALTER COLUMN org_id DROP NOT NULL,
    ADD COLUMN provider_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN destination_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    ADD COLUMN meta jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT service_bookings_provider_check
        CHECK ((org_id IS NOT NULL) <> (provider_user_id IS NOT NULL)
               OR anonymized_at IS NOT NULL);

CREATE INDEX idx_svc_bookings_provider_user
    ON service_bookings (provider_user_id, status);

-- 3. Mentor → shadow listing link (upsert target for PATCH
--    /me/mentor-profile; NULL until the mentor opts in).
ALTER TABLE user_profiles
    ADD COLUMN mentor_listing_content_id uuid REFERENCES contents(id) ON DELETE SET NULL;

-- 4. Seed the mentorship taxonomy category. applies_to='mentorship'
--    keeps it out of the service/package pickers.
INSERT INTO service_categories (slug, label, description, icon, accent_color, applies_to, position)
VALUES ('mentorship', 'Mentorship', 'Guidance from members who already made the move', 'graduation-cap', 'violet', 'mentorship', 115)
ON CONFLICT (slug) DO NOTHING;

-- 5. Shadow listings for every existing mentor.
--    content_hash: sha256 hex of the normalized bio PLUS a per-user
--    distinguisher ('|mentor:<user_id>') — empty/duplicate bios would
--    otherwise collide on idx_contents_content_hash (Rule 12 parity:
--    the app path uses content.ComputeContentHash; the distinguisher
--    matches ComputeContentHashWithDistinguisher semantics).
--    ON CONFLICT DO NOTHING (unqualified): idx_contents_slug and
--    idx_contents_content_hash are partial uniques (000745,
--    WHERE deleted_at IS NULL), so a column-list arbiter would not
--    infer them — and a slug or hash collision must skip, not fail.
INSERT INTO contents (author_id, content_type, title, slug, body, body_html, status, published_at, content_hash)
SELECT
    up.user_id,
    'service_listing',
    COALESCE(NULLIF(up.display_name, ''), u.username) || ' — Mentorship',
    'mentor-' || u.username,
    COALESCE(up.mentor_bio, ''),
    '',
    'published',
    now(),
    encode(sha256(convert_to(
        lower(regexp_replace(btrim(COALESCE(up.mentor_bio,'')), '\s+', ' ', 'g'))
        || '|mentor:' || up.user_id::text,
        'UTF8')), 'hex')
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.is_mentor = true
ON CONFLICT DO NOTHING;

INSERT INTO service_listing_meta (content_id, org_id, service_category, is_remote, listing_kind, purchase_mode)
SELECT c.id, NULL, 'mentorship', true, 'mentorship', 'inquiry'
FROM contents c
WHERE c.slug LIKE 'mentor-%' AND c.content_type = 'service_listing'
  AND NOT EXISTS (SELECT 1 FROM service_listing_meta m WHERE m.content_id = c.id);

UPDATE user_profiles up
SET mentor_listing_content_id = c.id
FROM users u, contents c
WHERE u.id = up.user_id
  AND up.is_mentor = true
  AND c.slug = 'mentor-' || u.username
  AND c.content_type = 'service_listing'
  AND c.deleted_at IS NULL;

-- 6. Migrate connections → bookings (mentor resolved via shadow listing).
--    meta preserves the source row id and responded_at for the down
--    migration. mentorship_connections has no tenant_id column.
INSERT INTO service_bookings (
    listing_meta_id, booker_user_id, provider_user_id,
    intent, party_size, note, destination_space_id, status, purchase_mode,
    confirmed_at, fulfilled_at, cancelled_at, cancel_reason, meta,
    created_at, updated_at
)
SELECT
    m.id,
    mc.mentee_id,
    mc.mentor_id,
    'inquiry',
    1,
    mc.goals,
    mc.destination_space_id,
    CASE mc.status
        WHEN 'active' THEN 'confirmed'
        WHEN 'completed' THEN 'fulfilled'
        ELSE mc.status END,
    'inquiry',
    CASE WHEN mc.status IN ('active','completed') THEN mc.responded_at END,
    CASE WHEN mc.status = 'completed' THEN mc.completed_at END,
    mc.cancelled_at,
    mc.cancel_reason,
    jsonb_build_object(
        'source','mentorship_connection',
        'connection_id', mc.id,
        'responded_at', mc.responded_at),
    mc.created_at,
    mc.updated_at
FROM mentorship_connections mc
JOIN user_profiles up ON up.user_id = mc.mentor_id
JOIN service_listing_meta m ON m.content_id = up.mentor_listing_content_id;

-- 7. Mentee-count trigger moves to service_bookings (confirmed
--    mentorship-kind bookings only). Fires for both NEW and OLD provider
--    so a provider reassignment can't strand the count; DELETE included
--    for parity with the original trg_mentee_count semantics.
DROP TRIGGER IF EXISTS trg_mentee_count ON mentorship_connections;

CREATE OR REPLACE FUNCTION trg_update_active_mentee_count()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE user_profiles up
    SET active_mentee_count = (
        SELECT COUNT(*)::smallint
        FROM service_bookings sb
        JOIN service_listing_meta m ON m.id = sb.listing_meta_id
        WHERE sb.provider_user_id = up.user_id
          AND m.listing_kind = 'mentorship'
          AND sb.status = 'confirmed'
    )
    WHERE up.user_id = COALESCE(NEW.provider_user_id, OLD.provider_user_id)
       OR up.user_id = OLD.provider_user_id;
    RETURN NULL;
END; $$;

DROP TABLE mentorship_connections;

CREATE TRIGGER trg_service_booking_mentee_count
AFTER INSERT OR DELETE OR UPDATE OF status, provider_user_id ON service_bookings
FOR EACH ROW EXECUTE FUNCTION trg_update_active_mentee_count();

-- Backfill rows were inserted before the trigger existed; recompute
-- mentor counts once so migrated 'confirmed' bookings are reflected.
UPDATE user_profiles up
SET active_mentee_count = (
    SELECT COUNT(*)::smallint
    FROM service_bookings sb
    JOIN service_listing_meta m ON m.id = sb.listing_meta_id
    WHERE sb.provider_user_id = up.user_id
      AND m.listing_kind = 'mentorship'
      AND sb.status = 'confirmed'
)
WHERE up.is_mentor = true;

-- Kind filter parity with idx_service_listing_meta_kind (000765): the
-- mentorship tab of GET /services?kind=... filters on listing_kind.
CREATE INDEX idx_service_listing_meta_kind_mentorship
    ON service_listing_meta (listing_kind)
    WHERE listing_kind = 'mentorship';

-- 8. Nav seed repoint — Mentors item now lands on the mentorship kind
--    tab of the service directory.
UPDATE menu_items SET url = '/services?kind=mentorship'
WHERE url = '/mentors';

COMMIT;
