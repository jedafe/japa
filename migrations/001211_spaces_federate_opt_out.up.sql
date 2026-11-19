-- §126: member-side "don't federate this space" flag. false (default)
-- = public member-created spaces federate as Group actors
-- (auto-provisioned on entry into the public/active envelope).
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS federate_opt_out boolean NOT NULL DEFAULT false;
