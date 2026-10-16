-- 000874_rag_spec_corpus.up.sql — Engineering Spec Corpus (§121, ADR-131).
--
-- Adds the rag_spec_sources registry table: one row per indexed .jeos file,
-- with a deterministic UUIDv5 id (uuid5('jeos:' + path)) shared by the file's
-- content_embeddings rows (entity_type='spec_doc', visibility='private').
-- file_sha256 lets the reindex sweep skip unchanged files; the path UNIQUE
-- constraint makes registry rows the citation join target (title/slug).
--
-- Also seeds the agent.spec_search permission node (granted to the admin-level
-- roles following the 000826 convention) — it gates the staff-only spec.search
-- agent tool. Other roles can receive it via the RBAC admin UI.

CREATE TABLE rag_spec_sources (
  id           uuid PRIMARY KEY,
  path         text NOT NULL UNIQUE,
  title        text NOT NULL,
  file_sha256  text NOT NULL,
  chunk_count  int  NOT NULL DEFAULT 0,
  indexed_at   timestamptz NOT NULL DEFAULT now()
);

INSERT INTO permissions (key, description) VALUES
    ('agent.spec_search', 'Retrieve engineering-spec corpus chunks via the spec.search agent tool')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'agent.spec_search'
ON CONFLICT (role_id, permission_id) DO NOTHING;
