-- 000758_wiki_page_feedback
-- §80 Reader UX & Page Feedback amendment: per-user helpful votes on
-- materialized wiki pages ("Was this page helpful?"). Per-user rows — not
-- aggregate counters — so a member can toggle or switch their vote; counts
-- are aggregated at read. Lazy template/category pages have no wiki_pages
-- row and therefore cannot receive votes.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

CREATE TABLE wiki_page_feedback (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    wiki_page_id uuid NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vote         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT wiki_page_feedback_vote_check CHECK (vote IN ('up', 'down')),
    CONSTRAINT wiki_page_feedback_unique UNIQUE (wiki_page_id, user_id)
);

-- Read path is always per-page aggregation.
CREATE INDEX idx_wiki_page_feedback_page ON wiki_page_feedback (wiki_page_id);

COMMIT;
