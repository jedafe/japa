-- 000189: Add soft-delete support to spaces (Rule 11, ADR-014).
-- Spaces are user-created entities; hard-deleting them CASCADE-removes
-- space_members, space_moderators, space_rules, and space_parent_spaces,
-- losing the audit/recovery trail. Soft-delete preserves the rows.

ALTER TABLE spaces ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- Partial index: only index non-deleted rows for fast read queries.
CREATE INDEX IF NOT EXISTS idx_spaces_active
    ON spaces (tenant_id, deleted_at)
    WHERE deleted_at IS NULL;
