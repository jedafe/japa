-- Reverse the org_type custom field seed.
DELETE FROM custom_fields WHERE key = 'org_type' AND is_system = true;
