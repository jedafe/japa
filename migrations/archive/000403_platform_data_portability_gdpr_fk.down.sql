-- Reverse BUG-010: restore NOT NULL and NO ACTION on created_by for both
-- platform_data_exports and platform_data_imports.
-- NOTE: this down migration will fail if any row has a NULL created_by
-- (e.g. a user was hard-deleted after the up migration ran). That is
-- expected — the GDPR erasure is irreversible.

ALTER TABLE platform_data_imports DROP CONSTRAINT platform_data_imports_created_by_fkey;
ALTER TABLE platform_data_imports
    ADD CONSTRAINT platform_data_imports_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE platform_data_imports ALTER COLUMN created_by SET NOT NULL;

ALTER TABLE platform_data_exports DROP CONSTRAINT platform_data_exports_created_by_fkey;
ALTER TABLE platform_data_exports
    ADD CONSTRAINT platform_data_exports_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE platform_data_exports ALTER COLUMN created_by SET NOT NULL;
