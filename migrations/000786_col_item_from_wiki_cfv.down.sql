-- 000786 down: drop the wiki cfv -> comparison_item materialization trigger.
-- Materialized items are data, not schema — intentionally kept.

DROP TRIGGER IF EXISTS trg_col_item_from_wiki_cfv ON custom_field_values;
DROP FUNCTION IF EXISTS materialize_col_item_for_wiki_space();
