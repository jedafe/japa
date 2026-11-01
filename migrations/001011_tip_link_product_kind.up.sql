-- §125: 'affiliate' link kind renamed 'product' — a product link that the
-- platform monetizes via tracking injection; the user-facing concept is the
-- member's product (book, course, listing), not an affiliate program.
ALTER TABLE tip_external_links DROP CONSTRAINT tip_external_links_kind_check;
ALTER TABLE tip_external_links ADD CONSTRAINT tip_external_links_kind_check
    CHECK (kind IN ('paypal','kofi','buymeacoffee','opencollective','other','product'));
