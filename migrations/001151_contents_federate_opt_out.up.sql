-- §127 A4 / §126: per-content "don't federate" flag — one flag gates both
-- projections (GetAtprotoProjectableContent, GetFederatableContentByID),
-- so a flagged row is neither a feed.post nor a fetchable AP object.
ALTER TABLE contents
    ADD COLUMN federate_opt_out boolean NOT NULL DEFAULT false;

CREATE INDEX idx_contents_federate_opt_out ON contents (federate_opt_out)
    WHERE federate_opt_out;
