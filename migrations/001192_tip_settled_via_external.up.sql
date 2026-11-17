-- §125: OC/KoFi-imported contributions insert directly as completed —
-- they settled off-platform, so 'credits' would be a false provenance.
-- 'external' marks rows where no platform rail moved anything.
ALTER TABLE tips DROP CONSTRAINT tips_settled_via_check;
ALTER TABLE tips ADD CONSTRAINT tips_settled_via_check
    CHECK (settled_via IN ('credits', 'backed', 'external'));

UPDATE tips SET settled_via = 'external'
WHERE method IN ('opencollective', 'kofi', 'buymeacoffee') AND status = 'completed';
