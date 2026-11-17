UPDATE tips SET settled_via = 'credits' WHERE settled_via = 'external';
ALTER TABLE tips DROP CONSTRAINT tips_settled_via_check;
ALTER TABLE tips ADD CONSTRAINT tips_settled_via_check
    CHECK (settled_via IN ('credits', 'backed'));
