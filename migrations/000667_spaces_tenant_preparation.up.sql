-- Phase 1.3 — spaces subsystem single-tenant preparation.
-- Adds global unique constraints/indexes needed before tenant_id filters are
-- removed from spaces.sql and tenant_id columns can eventually be dropped.
-- All data is already verified to belong to the default tenant.

DO $$
BEGIN
    IF EXISTS (SELECT slug FROM spaces GROUP BY slug HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate space slugs found; cannot make slug globally unique';
    END IF;
    IF EXISTS (SELECT slug FROM space_types GROUP BY slug HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate space_type slugs found';
    END IF;
    IF EXISTS (SELECT name FROM space_permission_presets GROUP BY name HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate permission-preset names found';
    END IF;
    IF EXISTS (SELECT space_id, user_id FROM space_moderators GROUP BY space_id, user_id HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate space_moderator rows found';
    END IF;
END
$$;

CREATE UNIQUE INDEX idx_spaces_slug_global ON spaces(slug);
CREATE UNIQUE INDEX idx_space_types_slug_global ON space_types(slug);
CREATE UNIQUE INDEX idx_space_permission_presets_name_global ON space_permission_presets(name);
CREATE UNIQUE INDEX idx_space_moderators_space_user_global ON space_moderators(space_id, user_id);

-- The existing unique index on space_members is already (space_id, user_id),
-- which is the global form. No change needed.
