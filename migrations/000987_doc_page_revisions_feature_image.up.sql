-- §44 amendment 5: per-page feature image (media library) + revision
-- snapshots written on every content-changing page write.
ALTER TABLE doc_pages
    ADD COLUMN feature_image_id uuid REFERENCES media(id);

CREATE TABLE doc_page_revisions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_page_id      uuid NOT NULL REFERENCES doc_pages(id) ON DELETE CASCADE,
    version          int NOT NULL,
    title            text NOT NULL,
    content          jsonb NOT NULL,
    feature_image_id uuid REFERENCES media(id),
    summary          text,
    created_by       uuid REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT doc_page_revisions_page_version_key UNIQUE (doc_page_id, version)
);
CREATE INDEX idx_doc_page_revisions_page
    ON doc_page_revisions (doc_page_id, version DESC);
