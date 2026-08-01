-- Revert AhrefsBot/SemrushBot to the (incorrect) pre-fix seed values.
UPDATE known_bots
SET bot_type = 'monitoring', policy = 'allow', updated_at = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('AhrefsBot', 'SemrushBot')
  AND is_system = true;
