-- 000811_task13_flag_backfill.down.sql
-- The feature_flags backfill rows are intentionally NOT removed on down —
-- they represent real operator intent and removing them could re-enable
-- features that were deliberately off (same rationale as 000807).
SELECT 1;
