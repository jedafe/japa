-- Section 33 (extension): Professional Profile relocations, documents, and verifiers.
--
-- These three child tables extend professional_profiles with relocation history,
-- credential documents, and third-party verifiers. Every table carries
-- tenant_id for multi-tenancy and cascades on profile deletion.
--
-- Field names mirror the frontend API contract in apps/main/src/lib/api.ts:
--   * Relocation uses `year` (int) not moved_at; status is planned|in_progress|completed.
--   * Document uses `file_url` and `description`, not media_id.
--   * Verifier stores a user_id reference; user_name is resolved at render time
--     via JOIN users (render-time over save-time, per AGENTS.md principle #4).

-- =====================
-- Professional Profile Relocations
-- =====================
CREATE TABLE IF NOT EXISTS professional_relocations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    country     text NOT NULL,
    city        text,
    status      text NOT NULL DEFAULT 'planned', -- planned, in_progress, completed
    year        int,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_professional_relocations_profile ON professional_relocations(profile_id);
CREATE INDEX idx_professional_relocations_tenant ON professional_relocations(tenant_id);

-- =====================
-- Professional Profile Documents
-- =====================
CREATE TABLE IF NOT EXISTS professional_documents (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_id    uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    title         text NOT NULL,
    document_type text NOT NULL, -- credential, certificate, resume, portfolio
    description   text,
    file_url      text,
    verified      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_professional_documents_profile ON professional_documents(profile_id);
CREATE INDEX idx_professional_documents_tenant ON professional_documents(tenant_id);

-- =====================
-- Professional Profile Verifiers
-- =====================
-- user_name is resolved at render time via JOIN users; only the user_id
-- reference is stored, so renaming a user never leaves a stale baked-in name.
CREATE TABLE IF NOT EXISTS professional_verifiers (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_id  uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text,
    status      text NOT NULL DEFAULT 'pending', -- pending, verified, rejected
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(profile_id, user_id)
);
CREATE INDEX idx_professional_verifiers_profile ON professional_verifiers(profile_id);
CREATE INDEX idx_professional_verifiers_tenant ON professional_verifiers(tenant_id);
