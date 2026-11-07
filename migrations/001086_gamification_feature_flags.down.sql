-- 001086_gamification_feature_flags.down.sql — revert the feature_flag columns.

ALTER TABLE public.challenge_definitions
    DROP COLUMN IF EXISTS feature_flag;

ALTER TABLE public.badge_definitions
    DROP COLUMN IF EXISTS feature_flag;
