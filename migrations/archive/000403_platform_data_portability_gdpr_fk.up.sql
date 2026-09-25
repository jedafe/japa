-- Platform Data Portability — GDPR erasure FK fix (BUG-010).
-- platform_data_exports.created_by and platform_data_imports.created_by
-- currently reference users(id) with no ON DELETE clause (defaults to NO
-- ACTION), which blocks GDPR hard-delete for any admin who has created an
-- export or import. Make both columns nullable and switch the FKs to
-- ON DELETE SET NULL so the export/import audit trail survives erasure
-- with created_by cleared, matching the platform-wide GDPR erasure model
-- (Section 19 / Section 67; ADR-014).
--
-- The erasure sweep in internal/gdpr/service.go reassigns created_by to the
-- deleted-user placeholder before the final HardDeleteUser, so the FK
-- change is defense-in-depth: even if the sweep is bypassed, the user row
-- can still be physically deleted.

-- platform_data_exports
ALTER TABLE platform_data_exports ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE platform_data_exports DROP CONSTRAINT platform_data_exports_created_by_fkey;
ALTER TABLE platform_data_exports
    ADD CONSTRAINT platform_data_exports_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

-- platform_data_imports
ALTER TABLE platform_data_imports ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE platform_data_imports DROP CONSTRAINT platform_data_imports_created_by_fkey;
ALTER TABLE platform_data_imports
    ADD CONSTRAINT platform_data_imports_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
