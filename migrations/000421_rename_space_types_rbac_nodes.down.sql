-- Revert BUG-006: rename spaces.types.* back to space_types.*.
UPDATE permissions SET key = 'space_types.create' WHERE key = 'spaces.types.create';
UPDATE permissions SET key = 'space_types.edit'   WHERE key = 'spaces.types.edit';
UPDATE permissions SET key = 'space_types.delete' WHERE key = 'spaces.types.delete';
