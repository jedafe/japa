package gdpr

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for GDPR operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Consent Records
// ---------------------------------------------------------------------------

func (s *Store) recordConsent(ctx context.Context, p gen.RecordConsentParams) (gen.ConsentRecord, error) {
	row, err := s.write.RecordConsent(ctx, p)
	if err != nil {
		return gen.ConsentRecord{}, fmt.Errorf("recording consent: %w", err)
	}

	return row, nil
}

func (s *Store) getUserConsents(ctx context.Context, userID pgtype.UUID, consentType string) ([]gen.ConsentRecord, error) {
	rows, err := s.read.GetUserConsents(ctx, gen.GetUserConsentsParams{
		UserID:      userID,
		ConsentType: consentType,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user consents: %w", err)
	}

	return rows, nil
}

func (s *Store) hasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error) {
	granted, err := s.read.HasConsent(ctx, gen.HasConsentParams{
		UserID:      userID,
		ConsentType: consentType,
	})
	if err != nil {
		return false, fmt.Errorf("checking consent: %w", err)
	}

	return granted, nil
}

func (s *Store) revokeConsent(ctx context.Context, p gen.RevokeConsentParams) error {
	if err := s.write.RevokeConsent(ctx, p); err != nil {
		return fmt.Errorf("revoking consent: %w", err)
	}

	return nil
}

func (s *Store) listConsentsByTenant(ctx context.Context, tenantID pgtype.UUID, consentType string, limit, offset int32) ([]gen.ConsentRecord, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListConsentsByTenant(ctx, gen.ListConsentsByTenantParams{
		TenantID:    tenantID,
		ConsentType: consentType,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing consents by tenant: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Data Export Requests
// ---------------------------------------------------------------------------

func (s *Store) createDataExportRequest(ctx context.Context, p gen.CreateDataExportRequestParams) (gen.DataExportRequest, error) {
	row, err := s.write.CreateDataExportRequest(ctx, p)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("creating data export request: %w", err)
	}

	return row, nil
}

func (s *Store) getDataExportRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.DataExportRequest, error) {
	row, err := s.read.GetDataExportRequest(ctx, gen.GetDataExportRequestParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("getting data export request: %w", err)
	}

	return row, nil
}

func (s *Store) updateDataExportRequestStatus(ctx context.Context, p gen.UpdateDataExportRequestStatusParams) (gen.DataExportRequest, error) {
	row, err := s.write.UpdateDataExportRequestStatus(ctx, p)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("updating data export request status: %w", err)
	}

	return row, nil
}

func (s *Store) listPendingDataExportRequests(ctx context.Context, limit int32) ([]gen.DataExportRequest, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingDataExportRequests(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing pending data export requests: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Account Deletion Requests
// ---------------------------------------------------------------------------

func (s *Store) createAccountDeletionRequest(ctx context.Context, p gen.CreateAccountDeletionRequestParams) (gen.AccountDeletionRequest, error) {
	row, err := s.write.CreateAccountDeletionRequest(ctx, p)
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("creating account deletion request: %w", err)
	}

	return row, nil
}

func (s *Store) getAccountDeletionRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.AccountDeletionRequest, error) {
	row, err := s.read.GetAccountDeletionRequest(ctx, gen.GetAccountDeletionRequestParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("getting account deletion request: %w", err)
	}

	return row, nil
}

func (s *Store) updateAccountDeletionRequestStatus(ctx context.Context, p gen.UpdateAccountDeletionRequestStatusParams) (gen.AccountDeletionRequest, error) {
	row, err := s.write.UpdateAccountDeletionRequestStatus(ctx, p)
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("updating account deletion request status: %w", err)
	}

	return row, nil
}

func (s *Store) listPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.AccountDeletionRequest, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingDeletionRequests(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing pending deletion requests: %w", err)
	}

	return rows, nil
}

func (s *Store) anonymizeUser(ctx context.Context, p gen.AnonymizeUserParams) error {
	if err := s.write.AnonymizeUser(ctx, p); err != nil {
		return fmt.Errorf("anonymizing user: %w", err)
	}

	return nil
}

func (s *Store) deleteUserTokens(ctx context.Context, p gen.DeleteUserTokensParams) error {
	if err := s.write.DeleteUserTokens(ctx, p); err != nil {
		return fmt.Errorf("deleting user tokens: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

func (s *Store) createAuditLog(ctx context.Context, p gen.CreateAuditLogParams) (gen.AuditLog, error) {
	row, err := s.write.CreateAuditLog(ctx, p)
	if err != nil {
		return gen.AuditLog{}, fmt.Errorf("creating audit log: %w", err)
	}

	return row, nil
}

func (s *Store) listAuditLogs(ctx context.Context, p gen.ListAuditLogsParams) ([]gen.AuditLog, error) {
	if p.PageLimit <= 0 || p.PageLimit > MaxPageSize {
		p.PageLimit = DefaultPageSize
	}

	rows, err := s.read.ListAuditLogs(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}

	return rows, nil
}
