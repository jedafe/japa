-- 001121 down — drop email_providers.
DROP INDEX IF EXISTS idx_email_providers_enabled;
DROP TABLE IF EXISTS email_providers;
