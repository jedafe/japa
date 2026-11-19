ALTER TABLE doc_pages
	DROP COLUMN IF EXISTS ai_model,
	DROP COLUMN IF EXISTS ai_generated_at,
	DROP COLUMN IF EXISTS hitl_reviewed_by,
	DROP COLUMN IF EXISTS hitl_reviewed_at;
