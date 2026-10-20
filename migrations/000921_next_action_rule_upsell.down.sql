-- 000921_next_action_rule_upsell.down.sql — drop the §122 upsell flag.

BEGIN;

ALTER TABLE public.next_action_rules
    DROP COLUMN upsell;

COMMIT;
