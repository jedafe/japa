-- Deduplicate existing unresolved security_scan_results rows.
--
-- Before this migration, every scan run inserted a brand-new row for each
-- finding even when an unresolved row of the same scan_type already existed,
-- so the control panel /security page showed the same finding repeated on
-- every scan. The CreateSecurityScanResult query is now an upsert that
-- refreshes the existing unresolved row in place. This migration collapses
-- the duplicate rows already accumulated: for each (tenant_id, scan_type)
-- group of unresolved findings, keep the most recent one and delete the
-- older duplicates. Resolved findings are left untouched (they are the
-- audit history).

DELETE FROM security_scan_results r
USING (
    SELECT id
    FROM (
        SELECT id,
               tenant_id,
               scan_type,
               ROW_NUMBER() OVER (
                   PARTITION BY tenant_id, scan_type
                   ORDER BY created_at DESC, id DESC
               ) AS rn
        FROM security_scan_results
        WHERE resolved_at IS NULL
    ) ranked
    WHERE rn > 1
) dup
WHERE r.id = dup.id;
