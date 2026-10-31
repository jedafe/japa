-- Revert to opt-in semantics: column default false. Cannot distinguish a
-- user who opted out under the new regime from a backfilled row, so only
-- the default is reverted.
ALTER TABLE public.user_privacy_settings
    ALTER COLUMN show_points_publicly SET DEFAULT false;
