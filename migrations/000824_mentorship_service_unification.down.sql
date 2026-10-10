-- 000824_mentorship_service_unification.down.sql
-- Full reverse of the mentorship → service-layer unification: recreates
-- mentorship_connections exactly as baseline 000735 defined it (columns,
-- PK, UNIQUE, indexes, FKs), backfills rows from the migrated
-- service_bookings, restores the original mentee-count function/trigger,
-- and removes every object the up migration added.
--
-- Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md

BEGIN;

-- 1. Recreate mentorship_connections (baseline 000735 definition).
CREATE TABLE public.mentorship_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    mentor_id uuid NOT NULL,
    mentee_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    goals text,
    destination_space_id uuid,
    responded_at timestamp with time zone,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    cancel_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentor_id_mentee_id_status_key
    UNIQUE (mentor_id, mentee_id, status);

CREATE INDEX idx_mentorship_conn_mentee_status
    ON public.mentorship_connections USING btree (mentee_id, status);
CREATE INDEX idx_mentorship_conn_mentor_status
    ON public.mentorship_connections USING btree (mentor_id, status);
CREATE INDEX idx_mentorship_conn_status
    ON public.mentorship_connections USING btree (status);

ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_destination_space_id_fkey
    FOREIGN KEY (destination_space_id) REFERENCES public.spaces(id) ON DELETE SET NULL;
ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentee_id_fkey
    FOREIGN KEY (mentee_id) REFERENCES public.users(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.mentorship_connections
    ADD CONSTRAINT mentorship_connections_mentor_id_fkey
    FOREIGN KEY (mentor_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- 2. Re-point the mentee-count machinery BEFORE touching bookings: the
--    restored function body reads mentorship_connections, so the new
--    service_bookings trigger must go first (its firing would compile
--    against the wrong row shape).
DROP TRIGGER IF EXISTS trg_service_booking_mentee_count ON service_bookings;

-- Original body verbatim from 000735_baseline_schema.up.sql.
CREATE OR REPLACE FUNCTION public.trg_update_active_mentee_count()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  -- On DELETE, use OLD.mentor_id; on INSERT/UPDATE, use NEW.mentor_id.
  -- COALESCE handles both cases: NEW is NULL on DELETE, OLD is NULL on INSERT.
  UPDATE user_profiles
  SET active_mentee_count = (
    SELECT COUNT(*) FROM mentorship_connections
    WHERE mentor_id = COALESCE(NEW.mentor_id, OLD.mentor_id)
    AND status = 'active'
  )
  WHERE user_id = COALESCE(NEW.mentor_id, OLD.mentor_id);
  RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER trg_mentee_count
AFTER INSERT OR DELETE OR UPDATE OF status ON public.mentorship_connections
FOR EACH ROW EXECUTE FUNCTION public.trg_update_active_mentee_count();

-- 3. Backfill connections from migrated bookings. Original row ids are
--    restored from meta->>'connection_id'; responded_at round-trips via
--    meta (confirmed_at was only set for active/completed rows).
INSERT INTO public.mentorship_connections (
    id, mentor_id, mentee_id, status, goals, destination_space_id,
    responded_at, completed_at, cancelled_at, cancel_reason,
    created_at, updated_at
)
SELECT
    (sb.meta->>'connection_id')::uuid,
    sb.provider_user_id,
    sb.booker_user_id,
    CASE sb.status
        WHEN 'confirmed' THEN 'active'
        WHEN 'fulfilled' THEN 'completed'
        ELSE sb.status END,
    sb.note,
    sb.destination_space_id,
    (sb.meta->>'responded_at')::timestamptz,
    sb.fulfilled_at,
    sb.cancelled_at,
    sb.cancel_reason,
    sb.created_at,
    sb.updated_at
FROM service_bookings sb
WHERE sb.meta->>'source' = 'mentorship_connection'
  -- Skip anonymized shells: the GDPR scrub (AnonymizeServiceBookings)
  -- nulls both owner pointers, leaving no mentor/mentee to restore —
  -- mentorship_connections requires both NOT NULL. These rows are purged
  -- with the other user-provider bookings below.
  AND sb.provider_user_id IS NOT NULL
  AND sb.booker_user_id IS NOT NULL;

-- Mirror of the up assertion: the backfill must have restored every
-- restorable migrated booking before any user-provider rows are deleted.
-- expected uses the same unresolvable-row filter as the INSERT so an
-- anonymized shell does not falsely trip the mismatch.
DO $$
DECLARE
    expected int;
    actual   int;
BEGIN
    SELECT count(*) INTO expected
    FROM service_bookings
    WHERE meta->>'source' = 'mentorship_connection'
      AND provider_user_id IS NOT NULL
      AND booker_user_id IS NOT NULL;
    SELECT count(*) INTO actual FROM mentorship_connections;
    IF expected <> actual THEN
        RAISE EXCEPTION 'mentorship restore mismatch: % restorable bookings but % restored connections — aborting before DELETE',
            expected, actual;
    END IF;
END $$;

-- 4. Remove every user-provider booking before restoring org_id NOT NULL
--    (they all carry org_id = NULL): migrated connection rows, live
--    mentorship bookings created since 000824 (meta = {'goals':...} —
--    not source-tagged), and GDPR-anonymized shells (both pointers
--    nulled). Org-owned bookings keep org_id and survive — including
--    anonymized ones, which retain org_id as the org's audit record.
DELETE FROM service_bookings
WHERE provider_user_id IS NOT NULL OR org_id IS NULL;

-- 5. Revert service_bookings schema.
ALTER TABLE service_bookings
    DROP CONSTRAINT service_bookings_provider_check;

DROP INDEX IF EXISTS idx_svc_bookings_provider_user;

ALTER TABLE service_bookings
    DROP COLUMN provider_user_id,
    DROP COLUMN destination_space_id,
    DROP COLUMN meta,
    ALTER COLUMN org_id SET NOT NULL;

-- 6. Remove the mentor → shadow listing link.
ALTER TABLE user_profiles
    DROP COLUMN mentor_listing_content_id;

-- 7. Delete shadow listings. The meta rows cascade via
--    service_listing_meta_content_id_fkey (ON DELETE CASCADE). Scoped to
--    rows that actually carry a mentorship meta row so an unrelated
--    'mentor-*' service listing is never touched.
DELETE FROM contents c
WHERE c.slug LIKE 'mentor-%'
  AND c.content_type = 'service_listing'
  AND EXISTS (
      SELECT 1 FROM service_listing_meta m
      WHERE m.content_id = c.id AND m.listing_kind = 'mentorship'
  );

DROP INDEX IF EXISTS idx_service_listing_meta_kind_mentorship;

-- 8. Restore the listing_kind CHECK (no 'mentorship' meta rows remain).
ALTER TABLE service_listing_meta
    DROP CONSTRAINT service_listing_meta_listing_kind_check,
    ADD CONSTRAINT service_listing_meta_listing_kind_check
        CHECK (listing_kind IN ('service','package'));

-- 9. Remove the seeded category only if no listing references it, then
--    restore the applies_to CHECK (the row itself would violate it).
DELETE FROM service_categories
WHERE slug = 'mentorship'
  AND NOT EXISTS (
      SELECT 1 FROM service_listing_meta m
      WHERE m.service_category = 'mentorship'
  );

ALTER TABLE service_categories
    DROP CONSTRAINT service_categories_applies_to_check,
    ADD CONSTRAINT service_categories_applies_to_check
        CHECK (applies_to IN ('service','package','both'));

-- 10. Revert the nav item.
UPDATE menu_items SET url = '/mentors'
WHERE url = '/services?kind=mentorship';

COMMIT;
