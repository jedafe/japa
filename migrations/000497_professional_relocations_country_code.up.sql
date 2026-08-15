-- ADR-064a: Add country_code (ISO 3166-1 alpha-2) to professional_relocations
-- so relocation country selections (sourced from the spaces country table)
-- can feed the geo-aware personalization feature. The legacy `country` text
-- column is retained for backward compatibility (display name).
ALTER TABLE professional_relocations
    ADD COLUMN IF NOT EXISTS country_code text;
