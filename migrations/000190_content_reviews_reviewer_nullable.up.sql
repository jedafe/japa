-- Section 24: Blog Extension — make content_reviews.reviewer_user_id nullable.
-- BUG-003: SubmitForReview creates a pending review row before a reviewer is
-- assigned. The NOT NULL constraint forced the submitting author to be
-- recorded as the reviewer of their own post. Making the column nullable
-- allows NULL on submit and the actual reviewer's ID on approve/reject.
ALTER TABLE content_reviews ALTER COLUMN reviewer_user_id DROP NOT NULL;
