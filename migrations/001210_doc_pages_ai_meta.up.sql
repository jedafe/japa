-- ADR-088: AI provenance + HITL review columns on doc_pages.
-- ai_generated already exists as a boolean flag; these columns record which
-- model produced the draft, when it was generated, and who reviewed it.
ALTER TABLE doc_pages
	ADD COLUMN ai_model text,
	ADD COLUMN ai_generated_at timestamptz,
	ADD COLUMN hitl_reviewed_by uuid REFERENCES users(id),
	ADD COLUMN hitl_reviewed_at timestamptz;

-- Backfill: existing AI pages get their creation timestamp as the generation
-- time and their first reviewer as the HITL reviewer.
UPDATE doc_pages SET ai_generated_at = created_at
WHERE ai_generated AND ai_generated_at IS NULL;

UPDATE doc_pages
SET hitl_reviewed_by = last_reviewed_by,
    hitl_reviewed_at = last_reviewed_at
WHERE ai_generated AND last_reviewed_by IS NOT NULL AND hitl_reviewed_at IS NULL;
