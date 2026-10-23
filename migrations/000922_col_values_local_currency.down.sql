-- 000922_col_values_local_currency.down.sql
-- Re-denominates the up migration's rows back to USD by dividing by the
-- latest fx rate (base USD). Restores the pre-migration magnitudes for
-- the four curated cities' comparison_values and Lagos's rent_1br cfv.

BEGIN;

CREATE TEMP TABLE _col_fx AS
SELECT DISTINCT ON (quote_currency) quote_currency, rate
FROM fx_rates
WHERE base_currency = 'USD'
ORDER BY quote_currency, fetched_at DESC;

CREATE TEMP TABLE _col_city_ccy AS
WITH RECURSIVE walk AS (
    SELECT i.id AS item_id, i.geo_space_id AS node, 0 AS depth
    FROM comparison_items i
    JOIN comparison_type_definitions t ON t.id = i.type_id AND t.slug = 'cost_of_living'
    WHERE i.deleted_at IS NULL AND i.geo_space_id IS NOT NULL
    UNION ALL
    SELECT w.item_id, par.id, w.depth + 1
    FROM walk w
    JOIN (
        SELECT j.space_id AS child, j.parent_space_id AS parent
        FROM space_parent_spaces j
        UNION
        SELECT s2.id, s2.parent_id
        FROM spaces s2
        WHERE s2.parent_id IS NOT NULL
    ) e ON e.child = w.node
    JOIN spaces par ON par.id = e.parent
    WHERE w.depth < 3
)
SELECT DISTINCT ON (w.item_id) w.item_id, c.currency_code
FROM walk w
JOIN spaces c ON c.id = w.node AND c.currency_code IS NOT NULL AND c.currency_code <> ''
ORDER BY w.item_id, w.depth;

UPDATE comparison_values v
SET value = trim(both '.' from to_char(v.value::numeric / r.rate, 'FM9999999999990.999'))
FROM _col_city_ccy cc
JOIN _col_fx r ON r.quote_currency = cc.currency_code AND r.rate > 0
WHERE v.item_id = cc.item_id
  AND v.item_id IN (
      '00000000-0000-0000-0000-000000000b01',
      '00000000-0000-0000-0000-000000000b02',
      '00000000-0000-0000-0000-000000000b03',
      '00000000-0000-0000-0000-000000000b04'
  )
  AND v.value ~ '^\d+(\.\d+)?$';

UPDATE custom_field_values cfv
SET value_json = (
    SELECT jsonb_object_agg(k.key, to_jsonb(ROUND(k.value::text::numeric / r.rate)))
    FROM jsonb_each(cfv.value_json) AS k(key, value)
    CROSS JOIN _col_fx r
    WHERE r.quote_currency = 'NGN' AND r.rate > 0
      AND k.value::text ~ '^\d+(\.\d+)?$'
)
FROM custom_fields cf
JOIN custom_field_groups g ON g.id = cf.group_id AND g.slug = 'wiki_city_housing'
WHERE cfv.field_id = cf.id
  AND cf.key = 'rent_1br'
  AND cfv.entity_type = 'space'
  AND EXISTS (SELECT 1 FROM spaces s WHERE s.id = cfv.entity_id AND s.slug = 'lagos')
  AND cfv.value_json IS NOT NULL;

DROP TABLE _col_city_ccy;
DROP TABLE _col_fx;

COMMIT;
