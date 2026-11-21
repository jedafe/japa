-- Constrain tool record status columns to the documented lifecycle enum.
-- The Go layer validates before writing, but the column DEFAULT 'published'
-- plus raw inserts previously allowed out-of-enum values to bypass HITL.
-- Enum mirrors the shared lifecycle: draft/ai_draft/pending are pre-publish,
-- published only via approve endpoints, rejected via reject endpoints.

ALTER TABLE eca_guides
    ADD CONSTRAINT chk_eca_guides_status
    CHECK (status IN ('draft', 'ai_draft', 'pending', 'published', 'rejected'));

ALTER TABLE credential_pathways
    ADD CONSTRAINT chk_credential_pathways_status
    CHECK (status IN ('draft', 'ai_draft', 'pending', 'published', 'rejected'));

ALTER TABLE attestation_pathways
    ADD CONSTRAINT chk_attestation_pathways_status
    CHECK (status IN ('draft', 'ai_draft', 'pending', 'published', 'rejected'));

ALTER TABLE housing_guides
    ADD CONSTRAINT chk_housing_guides_status
    CHECK (status IN ('draft', 'ai_draft', 'pending', 'published', 'rejected'));
