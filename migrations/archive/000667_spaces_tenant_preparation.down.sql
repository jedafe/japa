-- Revert Phase 1.3 spaces subsystem preparation.
DROP INDEX IF EXISTS idx_spaces_slug_global;
DROP INDEX IF EXISTS idx_space_types_slug_global;
DROP INDEX IF EXISTS idx_space_permission_presets_name_global;
DROP INDEX IF EXISTS idx_space_moderators_space_user_global;
