-- Remove draft comparisons left dead by deleted types — orphan-typed
-- items/dimensions (FK SET NULL) mean the draft has no coherent type
-- config to render or resume against. Only drafts containing orphan
-- rows are removed; healthy empty drafts are untouched.

BEGIN;

CREATE TEMP TABLE dead_comparison_drafts AS
SELECT DISTINCT content_id FROM comparison_items WHERE type_id IS NULL
UNION
SELECT DISTINCT content_id FROM comparison_dimensions WHERE type_id IS NULL;

DELETE FROM comparison_values
WHERE dimension_id IN (
        SELECT id FROM comparison_dimensions d
        JOIN dead_comparison_drafts x ON x.content_id = d.content_id)
   OR item_id IN (
        SELECT id FROM comparison_items i
        JOIN dead_comparison_drafts x ON x.content_id = i.content_id);

DELETE FROM comparison_dimensions d
USING dead_comparison_drafts x WHERE x.content_id = d.content_id;

DELETE FROM comparison_items i
USING dead_comparison_drafts x WHERE x.content_id = i.content_id;

-- comparison_content_metadata cascades on the contents delete.
DELETE FROM contents c
USING dead_comparison_drafts x WHERE x.content_id = c.id;

DROP TABLE dead_comparison_drafts;

COMMIT;
