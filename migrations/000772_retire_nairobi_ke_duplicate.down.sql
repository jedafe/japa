-- Reverts 000772: restore the soft-deleted 'nairobi-ke' city space.
UPDATE spaces
SET deleted_at = NULL
WHERE id = '0dbae971-ae7e-45e9-bc56-1d3a9d5b7049'
  AND slug = 'nairobi-ke';
