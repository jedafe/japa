-- Down migration is a no-op: the deleted rows were duplicates and cannot be
-- meaningfully restored. The upsert behavior in CreateSecurityScanResult
-- prevents them from being recreated.
SELECT 1;
