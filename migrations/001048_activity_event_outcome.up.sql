-- Settle outcome for activity events — makes "processed but paid nothing"
-- visible instead of silent. Values: earned | capped | disabled | no_rule |
-- one_time | floored | reverted | spent | killswitch | duplicate.
-- NULL = predates instrumentation.
ALTER TABLE activity_events ADD COLUMN IF NOT EXISTS outcome text;
