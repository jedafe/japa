-- Revert category column addition.
-- Note: this does not restore the global space type or re-seed removed types.

ALTER TABLE space_types DROP COLUMN IF EXISTS category;
