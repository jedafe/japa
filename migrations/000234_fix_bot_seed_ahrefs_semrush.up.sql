-- Fix seed data for AhrefsBot and SemrushBot per Section 43.
-- Section 43 specifies: AhrefsBot, SemrushBot (scraper → limit).
-- Migration 000028 incorrectly seeded them as monitoring/allow.

UPDATE known_bots
SET bot_type = 'scraper', policy = 'limit', updated_at = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('AhrefsBot', 'SemrushBot')
  AND is_system = true;
