-- 000809_sop_fields.up.sql
-- AI SOP Generator (§110, ADR-121)
-- — sop_fields: curated field-of-study dictionary feeding the public
--   /tools/sop-generator "Field of study" select. sop_samples.field_of_study
--   stays a text column; the dictionary is the managed option list its
--   values are drawn from.
--
-- Ref: japaconnect-engineering/sections/110-sop-generator.md

BEGIN;

CREATE TABLE IF NOT EXISTS sop_fields (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    is_active  boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Case-insensitive uniqueness so "Computer Science" and "computer science"
-- can't produce duplicate dropdown options.
CREATE UNIQUE INDEX IF NOT EXISTS sop_fields_name_key
    ON sop_fields (lower(name));

-- Seed the managed list from field values already in use on samples so the
-- public select keeps today's options.
INSERT INTO sop_fields (name)
SELECT DISTINCT btrim(field_of_study)
FROM sop_samples
WHERE field_of_study IS NOT NULL AND btrim(field_of_study) <> ''
ON CONFLICT DO NOTHING;

-- Seed the MCP agent-tool rows for sop_field create/update/delete
-- (mirrors the sop_template/sop_sample rows in 000788). No assign_geo —
-- sop_fields is a global dictionary with no geo-scope field.
INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('sop_field.create', 'Create a new SOP Fields of Study record.', 'agent_immigration', '{"function_name":"sop_field.create","description":"Create a new SOP Fields of Study record.","handler":"sop_field.create","parameters":{"type":"object","description":"Fields for the new SOP Fields of Study record","properties":{"payload":{"type":"object","description":"Tool record fields","additionalProperties":true}},"additionalProperties":true},"writes_user_data":true,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'sop_generator.manage') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;
INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('sop_field.update', 'Update an existing SOP Fields of Study record.', 'agent_immigration', '{"function_name":"sop_field.update","description":"Update an existing SOP Fields of Study record.","handler":"sop_field.update","parameters":{"type":"object","properties":{"tool_id":{"type":"string","description":"UUID of the record"},"payload":{"type":"object","description":"Fields to update","additionalProperties":true}},"required":["tool_id"],"additionalProperties":true},"writes_user_data":true,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'sop_generator.manage') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;
INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('sop_field.delete', 'Delete an existing SOP Fields of Study record.', 'agent_immigration', '{"function_name":"sop_field.delete","description":"Delete an existing SOP Fields of Study record.","handler":"sop_field.delete","parameters":{"type":"object","properties":{"tool_id":{"type":"string","description":"UUID of the record"},"payload":{"type":"object","description":"Fields to update","additionalProperties":true}},"required":["tool_id"],"additionalProperties":true},"writes_user_data":true,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'sop_generator.manage') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

COMMIT;
