# Section 19 -- GDPR/NDPR Compliance: Implementation Complete

**Date:** 2026-06-27
**Section:** 19
**Phase:** 6
**Status:** Complete

## Overview

Implemented GDPR and NDPR compliance for JapaConnect. Covers consent
management (append-only records), data export requests (async via Asynq),
account deletion (anonymize, not hard-delete), and audit logging. All tables
are tenant-scoped. Honcho erasure integration is deferred.

## Files Created (11)

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000027_gdpr_compliance.sql` | -- | 4 tables: consent_records, data_export_requests, account_deletion_requests, audit_logs |
| 2 | `internal/db/queries/gdpr.sql` | 159 | 20 sqlc queries (consent, export, deletion, audit) |
| 3 | `internal/db/gen/gdpr.sql.go` | 643 | sqlc-generated Go code |
| 4 | `internal/gdpr/types.go` | -- | Consent types, export/deletion status enums, input structs |
| 5 | `internal/gdpr/store.go` | 193 | Store wrapping sqlc queries |
| 6 | `internal/gdpr/service.go` | 401 | Business logic: consent, export, deletion, audit |
| 7 | `internal/gdpr/handlers.go` | 393 | 10 HTTP handlers |
| 8 | `internal/gdpr/worker.go` | 30 | Asynq processor interfaces (DataExportProcessor, AccountDeletionProcessor) |
| 9 | `internal/gdpr/audit.go` | 53 | Audit log helper for recording data access events |
| 10. | `internal/gdpr/service_test.go` | -- | 7 tests (consent/export/deletion status, noop enqueuer) |
| 11 | `internal/gdpr/handlers_test.go` | -- | 7 tests (write helpers, parsing, auth, error paths) |

## Database Schema

Migration `000027_gdpr_compliance.sql` creates four tables:

- **consent_records** -- Append-only consent log. Each row is a timestamped
  opt-in or opt-out event. Enables audit trail and regulatory compliance.
  Unique on (tenant_id, user_id, consent_type) for latest-value queries.

- **data_export_requests** -- User-initiated data export requests. Status
  progresses: pending -> processing -> completed/failed. Asynq task handles
  the actual export asynchronously.

- **account_deletion_requests** -- User-initiated account deletion requests.
  Status progresses: pending -> processing -> completed/failed. Anonymizes
  user data rather than hard-deleting.

- **audit_logs** -- Append-only audit trail for data access and modifications.
  Records actor, action, entity type/id, and IP address.

## Key Decisions

1. **Append-only consent records** -- consent is never updated or deleted.
  Each change creates a new row with timestamp. Enables full history for
  regulatory audits.

2. **Anonymize on delete** -- account deletion replaces PII with anonymized
  placeholders rather than hard-deleting rows. Preserves referential integrity
  across the platform (content, comments, reviews remain but are disconnected
  from the user).

3. **Asynq for async processing** -- data export and account deletion run as
  Asynq tasks. User gets immediate confirmation; background worker handles the
  heavy lifting. Task types: `gdpr:data:export` and `gdpr:account:deletion:cleanup`.

4. **Interface-based processor injection** -- GDPR worker uses
  `DataExportProcessor` and `AccountDeletionProcessor` interfaces so the
  worker package can call GDPR service methods without importing the GDPR
  package directly.

5. **Audit helper** -- `RecordAuditEvent` is a convenience function that
  creates audit log entries. Used across all GDPR operations and available
  for other packages to call.

6. **Honcho erasure deferred** -- Honcho integration requires the Honcho client
  library. Data export and deletion tasks will call Honcho's erasure API when
  available. Currently a TODO placeholder in the worker handlers.

## API Routes (10)

| Method | Path | Handler |
|--------|------|---------|
| POST | /api/v1/gdpr/consent | RecordConsent |
| GET | /api/v1/gdpr/consent | GetConsentHistory |
| GET | /api/v1/gdpr/consent/latest | GetLatestConsent |
| POST | /api/v1/gdpr/export | RequestDataExport |
| GET | /api/v1/gdpr/export | ListDataExportRequests |
| GET | /api/v1/gdpr/export/{id} | GetDataExportRequest |
| POST | /api/v1/gdpr/delete | RequestAccountDeletion |
| GET | /api/v1/gdpr/delete | ListAccountDeletionRequests |
| GET | /api/v1/gdpr/delete/{id} | GetAccountDeletionRequest |
| GET | /api/v1/gdpr/audit | ListAuditLogs |

## Permission Keys (4)

- `gdpr.consent.record` -- Record consent changes
- `gdpr.export.request` -- Request data export
- `gdpr.delete.request` -- Request account deletion
- `gdpr.audit.view` -- View audit logs (admin)

## Test Results

14 tests covering:
- Consent type constants (marketing, analytics, essential, third_party)
- Export status constants (pending, processing, completed, failed)
- Deletion status constants (pending, processing, completed, failed)
- Pagination defaults
- NoopEnqueuer (test double for Asynq client)
- Handler write helpers (writeJSON, writeError)
- ParsePageSize / ParseOffset
- ClientIP extraction
- HandleServiceError mapping
- RecordConsent invalid JSON handling

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/gdpr/...` | ok -- 14 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## Deferrals

- Honcho erasure API integration (requires Honcho client library)
- MinIO upload for data export files
- Signed URL generation for export download
- Valkey session key deletion during account deletion
- Email notification when export/deletion completes
