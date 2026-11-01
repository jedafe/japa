-- §125 external link kinds: add 'affiliate' (provider-domain-gated, platform-
-- monetized via /out tracking injection). 'other' stays in the CHECK so
-- legacy rows remain valid — new writes reject it in service code.
ALTER TABLE tip_external_links DROP CONSTRAINT tip_external_links_kind_check;
ALTER TABLE tip_external_links ADD CONSTRAINT tip_external_links_kind_check
    CHECK (kind IN ('paypal','kofi','buymeacoffee','opencollective','other','affiliate'));
