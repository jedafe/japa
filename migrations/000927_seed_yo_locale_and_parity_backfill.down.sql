-- 000927_seed_yo_locale_and_parity_backfill (down)
-- The parity backfill in step 3 is not individually reversible (it filled
-- only missing keys per locale); the down migration removes the yo pack that
-- this migration provisioned and leaves any pre-existing partial coverage
-- in other locales as it was.
BEGIN;

DELETE FROM translations WHERE locale = 'yo';

COMMIT;
