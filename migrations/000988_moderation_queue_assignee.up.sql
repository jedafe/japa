-- Unified moderation queue (§22): assignee columns for every queue source
-- table that lacked one, so PATCH /admin/moderation-queue/{id}/assign works
-- for all rows. reviews.reviewer_user_id is the review's author, NOT an
-- assignee — hence the separate column. FK mirrors tickets.assigned_to
-- (nullable, no ON DELETE clause = default NO ACTION, matching convention).
ALTER TABLE reviews          ADD COLUMN assigned_to uuid REFERENCES users(id);
ALTER TABLE crs_rulesets     ADD COLUMN assigned_to uuid REFERENCES users(id);
ALTER TABLE pof_requirements ADD COLUMN assigned_to uuid REFERENCES users(id);
ALTER TABLE eca_guides       ADD COLUMN assigned_to uuid REFERENCES users(id);
ALTER TABLE sop_templates    ADD COLUMN assigned_to uuid REFERENCES users(id);
ALTER TABLE sop_samples      ADD COLUMN assigned_to uuid REFERENCES users(id);
