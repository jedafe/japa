-- Reverts 000111_content_reports.
BEGIN;
DROP TABLE IF EXISTS content_reports;
COMMIT;
