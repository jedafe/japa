-- 000922_col_values_local_currency.up.sql
-- §72 currency-normalization follow-up (2026-10-15 amendment).
--
-- The currency-dimension contract for the Cost of Living matrix is that
-- money values are denominated in each city's local currency via the §58
-- country→currency linkage — the same convention the JCE matrix and the
-- §80 wiki CoL fields already apply (the API then normalizes every cell
-- to fx.default_base_currency for comparison). The seeded/admin
-- comparison_values for the four curated cities and Lagos's
-- wiki_city_housing.rent_1br community value were authored in USD under
-- the pre-amendment contract (ADR-072 §1), so the normalization divided
-- already-USD values by the local rate and produced absurd results —
-- e.g. Lagos rent $0.28, Accra overall index 102×.
--
-- Re-denominate those rows to each city's local currency using the
-- latest fx_rates row (base USD). Rows are multiplied by the local-units-
-- per-USD rate; the down migration divides by the same rate.
--
-- Ref: .jeos/sections/72-cost-of-living.md (currency normalization amendment)
--      .jeos/sections/80-destination-wiki.md (per-space denomination)

BEGIN;

-- Latest fx rate per quote currency (USD base) — mirrors
-- fx.Service.GetMatrix's DISTINCT ON read.
CREATE TEMP TABLE _col_fx AS
SELECT DISTINCT ON (quote_currency) quote_currency, rate
FROM fx_rates
WHERE base_currency = 'USD'
ORDER BY quote_currency, fetched_at DESC;

-- Resolve each curated city's country currency by walking the parent
-- chain (parent_id ∪ space_parent_spaces, ≤3 hops — mirrors
-- GetSpaceCountryCurrencyCode).
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

-- 1) comparison_values: USD-authored seeded/admin values → local.
--    Restricted to the four curated seeded items whose values are known
--    to be USD-denominated (Lagos, Abuja, Accra, Nairobi). Only numeric
--    text values convert; non-numeric cells are left alone.
UPDATE comparison_values v
SET value = trim(both '.' from to_char(v.value::numeric * r.rate, 'FM9999999999990.999'))
FROM _col_city_ccy cc
JOIN _col_fx r ON r.quote_currency = cc.currency_code AND r.rate > 0
WHERE v.item_id = cc.item_id
  AND v.item_id IN (
      '00000000-0000-0000-0000-000000000b01', -- Lagos
      '00000000-0000-0000-0000-000000000b02', -- Abuja
      '00000000-0000-0000-0000-000000000b03', -- Accra
      '00000000-0000-0000-0000-000000000b04'  -- Nairobi
  )
  AND v.value ~ '^\d+(\.\d+)?$';

-- 2) Lagos wiki_city_housing.rent_1br price_range — authored in USD but
--    the field is space-denominated (NGN) per the §80 amendment.
UPDATE custom_field_values cfv
SET value_json = (
    SELECT jsonb_object_agg(k.key, to_jsonb(ROUND(k.value::text::numeric * r.rate)))
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
