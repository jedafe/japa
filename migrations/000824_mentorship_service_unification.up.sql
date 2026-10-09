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

-- 5. Shadow listings. Two populations must get one:
--    a) every is_mentor user → status 'published' (discoverable in the
--       mentorship tab, linked via user_profiles.mentor_listing_content_id);
--    b) every DISTINCT mentor_id present in mentorship_connections that
--       population (a) didn't cover — mentors who flipped is_mentor off or
--       never had a user_profiles row → status 'draft' (kept out of
--       discovery, but their bookings still have a listing_meta_id to
--       reference; without this the connection rows would be dropped
--       unrestorably with the table).
--    _mentor_shadow records exactly the content ids THIS migration
--    created (RETURNING), so the meta INSERT and the bookings backfill
--    can never attach to a pre-existing 'mentor-*' slug row.
--    content_hash: sha256 hex of the normalized bio PLUS a per-user
--    distinguisher ('|mentor:<user_id>') — empty/duplicate bios would
--    otherwise collide on idx_contents_content_hash (Rule 12 parity:
--    the app path uses content.ComputeContentHash; the distinguisher
--    matches ComputeContentHashWithDistinguisher semantics).
--    ON CONFLICT DO NOTHING (unqualified): idx_contents_slug and
--    idx_contents_content_hash are partial uniques (000745,
--    WHERE deleted_at IS NULL), so a column-list arbiter would not
--    infer them — and a slug or hash collision must skip, not fail.
CREATE TEMPORARY TABLE _mentor_shadow (
    content_id uuid PRIMARY KEY,
    author_id  uuid NOT NULL,
    listed     boolean NOT NULL    -- true when the author is an is_mentor profile
) ON COMMIT DROP;

WITH ins AS (
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
    UNION ALL
    SELECT
        u.id,
        'service_listing',
        COALESCE(NULLIF(up.display_name, ''), u.username) || ' — Mentorship',
        'mentor-' || u.username,
        COALESCE(up.mentor_bio, ''),
        '',
        'draft',
        NULL,
        encode(sha256(convert_to(
            lower(regexp_replace(btrim(COALESCE(up.mentor_bio,'')), '\s+', ' ', 'g'))
            || '|mentor:' || u.id::text,
            'UTF8')), 'hex')
    FROM users u
    LEFT JOIN user_profiles up ON up.user_id = u.id
    WHERE u.id IN (SELECT DISTINCT mentor_id FROM mentorship_connections)
      AND NOT EXISTS (
          SELECT 1 FROM user_profiles up2
          WHERE up2.user_id = u.id AND up2.is_mentor = true
      )
    ON CONFLICT DO NOTHING
    RETURNING id, author_id
)
INSERT INTO _mentor_shadow (content_id, author_id, listed)
SELECT i.id, i.author_id, COALESCE(up.is_mentor, false)
FROM ins i
LEFT JOIN user_profiles up ON up.user_id = i.author_id;

INSERT INTO service_listing_meta (content_id, org_id, service_category, is_remote, listing_kind, purchase_mode)
SELECT s.content_id, NULL, 'mentorship', true, 'mentorship', 'inquiry'
FROM _mentor_shadow s
WHERE NOT EXISTS (SELECT 1 FROM service_listing_meta m WHERE m.content_id = s.content_id);

-- Link only live mentors (draft fallback shadows stay unlinked).
UPDATE user_profiles up
SET mentor_listing_content_id = s.content_id
FROM _mentor_shadow s
WHERE s.author_id = up.user_id
  AND s.listed
  AND up.is_mentor = true;

-- 6. Migrate connections → bookings (mentor resolved via the shadow
--    listing created above — covers draft shadows too, so EVERY
--    connection row lands). meta preserves the source row id and
--    responded_at for the down migration. mentorship_connections has
--    no tenant_id column.
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
JOIN _mentor_shadow s ON s.author_id = mc.mentor_id
JOIN service_listing_meta m ON m.content_id = s.content_id;

-- Fail loudly rather than drop a connection row unrecoverably: a mentor
-- whose shadow INSERT was skipped by a slug/hash collision would
-- otherwise lose every one of their connections on DROP TABLE.
DO $$
DECLARE
    expected int;
    actual   int;
BEGIN
    SELECT count(*) INTO expected FROM mentorship_connections;
    SELECT count(*) INTO actual
    FROM service_bookings
    WHERE meta->>'source' = 'mentorship_connection';
    IF expected <> actual THEN
        RAISE EXCEPTION 'mentorship migration mismatch: % connection rows but % migrated bookings — aborting before DROP TABLE',
            expected, actual;
    END IF;
END $$;

-- 7. Mentee-count trigger moves to service_bookings (confirmed
--    mentorship-kind bookings only). Fires for both NEW and OLD provider
--    so a provider reassignment can't strand the count; listing_meta_id
--    is in the UPDATE column list so re-pointing a booking between a
--    mentorship and non-mentorship listing can't strand it either;
--    DELETE included for parity with the original trg_mentee_count
--    semantics.
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
AFTER INSERT OR DELETE OR UPDATE OF status, provider_user_id, listing_meta_id ON service_bookings
FOR EACH ROW EXECUTE FUNCTION trg_update_active_mentee_count();

-- Backfill rows were inserted before the trigger existed; recompute
-- counts once for every shadowed provider (includes non-is_mentor
-- providers whose connections were preserved as draft-shadow bookings)
-- so migrated 'confirmed' bookings are reflected.
UPDATE user_profiles up
SET active_mentee_count = (
    SELECT COUNT(*)::smallint
    FROM service_bookings sb
    JOIN service_listing_meta m ON m.id = sb.listing_meta_id
    WHERE sb.provider_user_id = up.user_id
      AND m.listing_kind = 'mentorship'
      AND sb.status = 'confirmed'
)
WHERE EXISTS (
    SELECT 1 FROM _mentor_shadow s WHERE s.author_id = up.user_id
);

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
