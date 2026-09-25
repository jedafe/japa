-- Down migration for 000027_gdpr_compliance.sql
-- Drops audit logs, account deletion requests, data export requests, and consent records tables.
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS account_deletion_requests;
DROP TABLE IF EXISTS data_export_requests;
DROP TABLE IF EXISTS consent_records;
