-- Indexes supporting the /me/moderation/* scoped queue (spec
-- .jeos/specs/2026-09-23-studio-moderation-tab.md).
--
-- The de-tenant migration (000668) dropped content_reports.tenant_id CASCADE,
-- which also dropped idx_reports_status and idx_reports_reporter. This adds
-- the indexes the moderator queue actually needs:
--
--   * reviewed_by partial — CountAssignedReports + `assigned=me` filter +
--     the scope_assigned flag on every queue row.
--   * (status, created_at DESC) — open-only counts, per-status filter tabs,
--     and the queue's created_at ordering.

CREATE INDEX IF NOT EXISTS idx_reports_reviewed_by
    ON content_reports(reviewed_by)
    WHERE reviewed_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_reports_status_created
    ON content_reports(status, created_at DESC);
