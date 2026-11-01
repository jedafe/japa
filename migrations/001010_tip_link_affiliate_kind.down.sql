DELETE FROM tip_external_links WHERE kind = 'affiliate';
ALTER TABLE tip_external_links DROP CONSTRAINT tip_external_links_kind_check;
ALTER TABLE tip_external_links ADD CONSTRAINT tip_external_links_kind_check
    CHECK (kind IN ('paypal','kofi','buymeacoffee','opencollective','other'));
