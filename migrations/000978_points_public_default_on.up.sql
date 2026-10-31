-- show_points_publicly becomes opt-out (product decision): members rank on
-- the leaderboard and show points on their profile unless they explicitly
-- turn it off. Absent privacy rows mean opted-in (COALESCE ... , true in
-- the leaderboard/profile queries); rows created later inherit the new
-- column default; existing rows are backfilled since the consent gate was
-- only just enforced and no one opted out under it.
ALTER TABLE public.user_privacy_settings
    ALTER COLUMN show_points_publicly SET DEFAULT true;

UPDATE public.user_privacy_settings
SET show_points_publicly = true
WHERE show_points_publicly = false;
