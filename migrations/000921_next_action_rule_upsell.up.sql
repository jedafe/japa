-- 000921_next_action_rule_upsell.up.sql — §122: explicit upsell flag on rules.
-- Replaces the completion_signal-predicate heuristic for the
-- nextactions.max_upsell_per_day cap — an upsell rule authored without the
-- account_tier/user_class convention previously escaped the cap entirely.
-- Ref: .jeos/audit/nextactions-audit.md (B8), sections/122-next-action-suggestions.md

BEGIN;

ALTER TABLE public.next_action_rules
    ADD COLUMN upsell boolean NOT NULL DEFAULT false;

-- Flag the two seeded upgrade rules; earlier upsell semantics are preserved.
UPDATE public.next_action_rules
SET upsell = true
WHERE slug IN ('free-upgrade-gate-hit', 'free-upgrade-mature');

COMMIT;
