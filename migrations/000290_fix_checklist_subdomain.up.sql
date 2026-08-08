-- 000290_fix_checklist_subdomain.up.sql
-- Correct the checklist.subdomain system_setting from "visa" to "visa-checklist".
--
-- The JEOS spec (§70, ADR-070) and the system-settings-catalog define the
-- default subdomain as "visa-checklist". Migration 000262 seeded it correctly
-- with ON CONFLICT DO NOTHING, but an earlier incorrect value ("visa") was
-- already present and was never corrected. The origin middleware
-- (internal/middleware/origin.go) and the tools app DEFAULT_SUBDOMAINS map
-- both hardcode "visa-checklist", so the stale "visa" value breaks host
-- detection for the visa-checklist single-tool surface (ADR-086, §78).
--
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md
-- Ref: japaconnect-engineering/adr/ADR-086-tools-web-surface.md
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md
--
-- Guarded by the current value so the migration is idempotent and will not
-- override a future intentional admin change away from "visa".
UPDATE system_settings
SET value = '"visa-checklist"'::jsonb,
    updated_at = now()
WHERE key = 'checklist.subdomain'
  AND value = '"visa"'::jsonb;
