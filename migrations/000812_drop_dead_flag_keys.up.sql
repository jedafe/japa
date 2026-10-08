-- 000812_drop_dead_flag_keys.up.sql
-- Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §7
--
-- Migration 000807 minted feature_flags rows named after the legacy
-- system_settings keys 'newsletter.enabled' and 'referral.enabled', but the
-- services read 'feature.newsletter' and 'feature.referral' (the rename
-- mapping landed in 000811). Those two rows are inert — no code path reads
-- them — so they surface as phantom entries in Control → Extensions and the
-- public flag map.

-- Unlink any roadmap item first: deleting the row bypasses the config-change
-- hook (SyncFlagPhase) that would normally clear the link, leaving the item
-- pointing at a nonexistent flag with a frozen derived status.
UPDATE roadmap_items
SET feature_flag_key = NULL
WHERE feature_flag_key IN ('newsletter.enabled', 'referral.enabled');

-- Drop them; the real keys are unaffected.
DELETE FROM feature_flags
WHERE key IN ('newsletter.enabled', 'referral.enabled');
