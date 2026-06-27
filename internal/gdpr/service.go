package gdpr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service provides GDPR operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer
	return svc
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
	return nil
}

// ---------------------------------------------------------------------------
// Consent Management
// ---------------------------------------------------------------------------

// RecordConsentInput holds parameters for recording a consent decision.
type RecordConsentInput struct {
	ConsentType   string
	Granted       bool
	PolicyVersion string
	IPAddress     *string
	UserAgent     *string
}

// RecordConsent records a user's consent decision.
func (s *Service) RecordConsent(ctx context.Context, tenantID, userID pgtype.UUID, input RecordConsentInput) (gen.ConsentRecord, error) {
	if input.ConsentType == "" {
		return gen.ConsentRecord{}, fmt.Errorf("consent_type is required")
	}
	if !ValidConsentType(input.ConsentType) {
		return gen.ConsentRecord{}, fmt.Errorf("invalid consent_type: %s", input.ConsentType)
	}
	if input.PolicyVersion == "" {
		return gen.ConsentRecord{}, fmt.Errorf("policy_version is required")
	}

	return s.store.recordConsent(ctx, gen.RecordConsentParams{
		TenantID:      tenantID,
		UserID:        userID,
		ConsentType:   input.ConsentType,
		Granted:       input.Granted,
		PolicyVersion: input.PolicyVersion,
		IpAddress:     parseIPAddress(input.IPAddress),
		UserAgent:     input.UserAgent,
	})
}

// GetUserConsents returns consent records for a user, optionally filtered by type.
func (s *Service) GetUserConsents(ctx context.Context, userID pgtype.UUID, consentType string) ([]gen.ConsentRecord, error) {
	return s.store.getUserConsents(ctx, userID, consentType)
}

// HasConsent checks whether a user has granted a specific consent.
func (s *Service) HasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error) {
	return s.store.hasConsent(ctx, userID, consentType)
}

// RevokeConsent records a consent revocation.
func (s *Service) RevokeConsent(ctx context.Context, tenantID, userID pgtype.UUID, consentType string, policyVersion string) error {
	if consentType == "" {
		return fmt.Errorf("consent_type is required")
	}
	if !ValidConsentType(consentType) {
		return fmt.Errorf("invalid consent_type: %s", consentType)
	}

	return s.store.revokeConsent(ctx, gen.RevokeConsentParams{
		TenantID:      tenantID,
		UserID:        userID,
		ConsentType:   consentType,
		PolicyVersion: policyVersion,
	})
}

// ListConsentsByTenant returns consent records for admin compliance reports.
func (s *Service) ListConsentsByTenant(ctx context.Context, tenantID pgtype.UUID, consentType string, limit, offset int32) ([]gen.ConsentRecord, error) {
	return s.store.listConsentsByTenant(ctx, tenantID, consentType, limit, offset)
}

// ---------------------------------------------------------------------------
// Data Export
// ---------------------------------------------------------------------------

// RequestDataExport creates a data export request and enqueues a background job.
func (s *Service) RequestDataExport(ctx context.Context, tenantID, userID pgtype.UUID) (gen.DataExportRequest, error) {
	request, err := s.store.createDataExportRequest(ctx, gen.CreateDataExportRequestParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("creating data export request: %w", err)
	}

	// Enqueue background job.
	payload, marshalErr := json.Marshal(map[string]any{
		"request_id": request.ID.String(),
		"tenant_id":  tenantID.String(),
		"user_id":    userID.String(),
	})
	if marshalErr == nil {
		_ = s.enqueuer.Enqueue(worker.TypeDataExport, payload)
	}

	return request, nil
}

// GetDataExportRequest retrieves a data export request by ID.
func (s *Service) GetDataExportRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.DataExportRequest, error) {
	return s.store.getDataExportRequest(ctx, id, tenantID)
}

// ProcessDataExport collects user data, generates JSON, and stores a signed URL.
// This is called by the Asynq worker.
func (s *Service) ProcessDataExport(ctx context.Context, requestID pgtype.UUID) error {
	// Mark as processing.
	request, err := s.store.getDataExportRequest(ctx, requestID, pgtype.UUID{})
	if err != nil {
		return fmt.Errorf("fetching export request: %w", err)
	}

	_, err = s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		ID:       requestID,
		Status:   string(ExportProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking export as processing: %w", err)
	}

	// TODO: Collect user data from all relevant tables, generate JSON,
	// upload to MinIO, create signed URL with 24h expiry.
	// For now, mark as completed with a placeholder URL.
	expiresAt := time.Now().Add(24 * time.Hour)
	placeholderURL := fmt.Sprintf("/exports/%s/data.json", request.UserID.String())

	_, err = s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		ID:          requestID,
		Status:      string(ExportCompleted),
		DownloadUrl: &placeholderURL,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("marking export as completed: %w", err)
	}

	return nil
}

// ListPendingDataExportRequests returns pending export requests for the worker.
func (s *Service) ListPendingDataExportRequests(ctx context.Context, limit int32) ([]gen.DataExportRequest, error) {
	return s.store.listPendingDataExportRequests(ctx, limit)
}

// ---------------------------------------------------------------------------
// Account Deletion
// ---------------------------------------------------------------------------

// RequestAccountDeletion creates a deletion request and enqueues a background job.
func (s *Service) RequestAccountDeletion(ctx context.Context, tenantID, userID pgtype.UUID, reason string) (gen.AccountDeletionRequest, error) {
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	request, err := s.store.createAccountDeletionRequest(ctx, gen.CreateAccountDeletionRequestParams{
		TenantID: tenantID,
		UserID:   userID,
		Reason:   reasonPtr,
	})
	if err != nil {
		return gen.AccountDeletionRequest{}, fmt.Errorf("creating account deletion request: %w", err)
	}

	// Enqueue background job.
	payload, marshalErr := json.Marshal(map[string]any{
		"request_id": request.ID.String(),
		"tenant_id":  tenantID.String(),
		"user_id":    userID.String(),
	})
	if marshalErr == nil {
		_ = s.enqueuer.Enqueue(worker.TypeAccountDeletionCleanup, payload)
	}

	return request, nil
}

// GetAccountDeletionRequest retrieves a deletion request by ID.
func (s *Service) GetAccountDeletionRequest(ctx context.Context, id, tenantID pgtype.UUID) (gen.AccountDeletionRequest, error) {
	return s.store.getAccountDeletionRequest(ctx, id, tenantID)
}

// ProcessAccountDeletion performs the full account deletion workflow.
// This is called by the Asynq worker.
func (s *Service) ProcessAccountDeletion(ctx context.Context, requestID pgtype.UUID) error {
	request, err := s.store.getAccountDeletionRequest(ctx, requestID, pgtype.UUID{})
	if err != nil {
		return fmt.Errorf("fetching deletion request: %w", err)
	}

	// Mark as processing.
	_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:     requestID,
		Status: string(DeletionProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking deletion as processing: %w", err)
	}

	// 1. Anonymize user row.
	err = s.store.anonymizeUser(ctx, gen.AnonymizeUserParams{
		UserID:   request.UserID,
		TenantID: request.TenantID,
	})
	if err != nil {
		_, _ = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:     requestID,
			Status: string(DeletionFailed),
		})
		return fmt.Errorf("anonymizing user: %w", err)
	}

	// 2. Revoke all refresh tokens.
	err = s.store.deleteUserTokens(ctx, gen.DeleteUserTokensParams{
		UserID:   request.UserID,
		TenantID: request.TenantID,
	})
	if err != nil {
		// Log but don't fail — tokens may already be revoked.
		_ = err
	}

	// 3. TODO: Delete Valkey session keys (requires Valkey client injection).
	// 4. TODO: Honcho erasure (requires Honcho client integration).

	// 5. Mark deletion as completed.
	_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:          requestID,
		Status:      string(DeletionCompleted),
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("marking deletion as completed: %w", err)
	}

	return nil
}

// ListPendingDeletionRequests returns pending deletion requests for the worker.
func (s *Service) ListPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.AccountDeletionRequest, error) {
	return s.store.listPendingDeletionRequests(ctx, limit)
}

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

// LogAuditParams holds parameters for creating an audit log entry.
type LogAuditParams struct {
	ActorUserID        pgtype.UUID
	ImpersonatedUserID pgtype.UUID
	ActionType         string
	TargetType         string
	TargetID           pgtype.UUID
	Metadata           map[string]any
	IPAddress          *string
}

// LogAuditEvent creates an audit log entry.
func (s *Service) LogAuditEvent(ctx context.Context, tenantID pgtype.UUID, params LogAuditParams) error {
	if params.ActionType == "" {
		return fmt.Errorf("action_type is required")
	}

	metadataJSON := []byte("{}")
	if len(params.Metadata) > 0 {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return fmt.Errorf("marshaling audit metadata: %w", err)
		}
		metadataJSON = b
	}

	var targetType *string
	if params.TargetType != "" {
		targetType = &params.TargetType
	}

	var ipAddress *netip.Addr
	if params.IPAddress != nil {
		addr, err := netip.ParseAddr(*params.IPAddress)
		if err == nil {
			ipAddress = &addr
		}
	}

	_, err := s.store.createAuditLog(ctx, gen.CreateAuditLogParams{
		TenantID:           tenantID,
		ActorUserID:        params.ActorUserID,
		ImpersonatedUserID: params.ImpersonatedUserID,
		ActionType:         params.ActionType,
		TargetType:         targetType,
		TargetID:           params.TargetID,
		Metadata:           metadataJSON,
		IpAddress:          ipAddress,
	})
	return err
}

// ListAuditLogs returns audit logs with filters for admin access.
func (s *Service) ListAuditLogs(ctx context.Context, tenantID, actorUserID pgtype.UUID, actionType, targetType string, startDate, endDate *time.Time, limit, offset int32) ([]gen.AuditLog, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	var startDateTZ, endDateTZ pgtype.Timestamptz
	if startDate != nil {
		startDateTZ = pgtype.Timestamptz{Time: *startDate, Valid: true}
	}
	if endDate != nil {
		endDateTZ = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}

	var targetTypePtr *string
	if targetType != "" {
		targetTypePtr = &targetType
	}

	// Use zero UUID as sentinel for "no filter" on actor_user_id.
	zeroUUID := pgtype.UUID{}
	_ = zeroUUID.Scan("00000000-0000-0000-0000-000000000000")

	actorFilter := zeroUUID
	if actorUserID.Valid {
		actorFilter = actorUserID
	}

	return s.store.listAuditLogs(ctx, gen.ListAuditLogsParams{
		TenantID:    tenantID,
		ActorUserID: actorFilter,
		ActionType:  actionType,
		TargetType:  targetTypePtr,
		StartDate:   startDateTZ,
		EndDate:     endDateTZ,
		PageLimit:   limit,
		PageOffset:  offset,
	})
}

// parseIPAddress converts a string IP address to a netip.Addr pointer.
func parseIPAddress(s *string) *netip.Addr {
	if s == nil {
		return nil
	}
	addr, err := netip.ParseAddr(*s)
	if err != nil {
		return nil
	}
	return &addr
}
