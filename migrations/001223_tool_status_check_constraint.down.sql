ALTER TABLE eca_guides DROP CONSTRAINT IF EXISTS chk_eca_guides_status;
ALTER TABLE credential_pathways DROP CONSTRAINT IF EXISTS chk_credential_pathways_status;
ALTER TABLE attestation_pathways DROP CONSTRAINT IF EXISTS chk_attestation_pathways_status;
ALTER TABLE housing_guides DROP CONSTRAINT IF EXISTS chk_housing_guides_status;
