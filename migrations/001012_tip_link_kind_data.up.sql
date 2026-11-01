-- Normalize any 'affiliate' rows written before the kind was renamed to
-- 'product' (001011 changed the CHECK but not existing data).
UPDATE tip_external_links SET kind = 'product' WHERE kind = 'affiliate';
