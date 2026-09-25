-- Make ai_hitl_reviews.content_id nullable so admin-generated AI output can
-- be sent for human review without an existing content row (Section 20 HITL).
-- The admin "Send to Review" feature submits generated text directly; there
-- is no content row to reference until the review is approved and published.
ALTER TABLE ai_hitl_reviews ALTER COLUMN content_id DROP NOT NULL;
