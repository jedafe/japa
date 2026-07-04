package gdpr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// HonchoEraser deletes a user's AI context from the Honcho agent-memory
// service. The honcho.Client implements this interface. The erasure step in
// ProcessAccountDeletion calls DeleteUser and propagates the error so the
// Asynq task retries — Honcho deletion is mandatory, not best-effort.
type HonchoEraser interface {
	DeleteUser(ctx context.Context, userID string) error
}

// SessionDeleter deletes user sessions from Valkey/Redis.
type SessionDeleter interface {
	DeleteSessions(ctx context.Context, userID string) error
}

// Service provides GDPR operations with reader/writer pool separation.
type Service struct {
	store          *Store
	db             *db.DB
	enqueuer       Enqueuer
	honchoEraser   HonchoEraser
	sessionDeleter SessionDeleter
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

// SetHonchoEraser injects the Honcho client used for AI context erasure
// during account deletion. When set, the erasure step must succeed or the
// deletion job retries. When nil (development), the step is skipped.
func (s *Service) SetHonchoEraser(h HonchoEraser) {
	s.honchoEraser = h
}

// SetSessionDeleter injects the session deleter for clearing Valkey sessions
// during account deletion. When nil (development), the step is skipped.
func (s *Service) SetSessionDeleter(d SessionDeleter) {
	s.sessionDeleter = d
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
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
		return gen.ConsentRecord{}, errors.New("consent_type is required")
	}

	if !ValidConsentType(input.ConsentType) {
		return gen.ConsentRecord{}, fmt.Errorf("invalid consent_type: %s", input.ConsentType)
	}

	if input.PolicyVersion == "" {
		return gen.ConsentRecord{}, errors.New("policy_version is required")
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
func (s *Service) RevokeConsent(ctx context.Context, tenantID, userID pgtype.UUID, consentType, policyVersion string) error {
	if consentType == "" {
		return errors.New("consent_type is required")
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
		if err := s.enqueuer.Enqueue(worker.TypeDataExport, payload); err != nil {
			slog.Error("enqueue data export", "error", err)
		}
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
		ID:     requestID,
		Status: string(ExportProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking export as processing: %w", err)
	}

	// Collect user data from all relevant tables.
	userData, err := s.collectUserData(ctx, request.UserID, request.TenantID)
	if err != nil {
		if _, statusErr := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
			ID:     requestID,
			Status: string(ExportFailed),
		}); statusErr != nil {
			slog.Error("update data export status to failed", "error", statusErr)
		}

		return fmt.Errorf("collecting user data: %w", err)
	}

	// Generate JSON export.
	exportJSON, err := json.MarshalIndent(userData, "", "  ")
	if err != nil {
		if _, statusErr := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
			ID:     requestID,
			Status: string(ExportFailed),
		}); statusErr != nil {
			slog.Error("update data export status to failed", "error", statusErr)
		}

		return fmt.Errorf("marshaling export data: %w", err)
	}

	// In production, upload to S3 and create signed URL.
	// For now, store locally and create a reference URL.
	expiresAt := time.Now().Add(24 * time.Hour)
	exportURL := fmt.Sprintf("/exports/%s/data.json", request.UserID.String())
	slog.Info("data export generated",
		"user_id", request.UserID.String(),
		"export_size", len(exportJSON),
		"url", exportURL)

	_, err = s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		ID:          requestID,
		Status:      string(ExportCompleted),
		DownloadUrl: &exportURL,
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
		if err := s.enqueuer.Enqueue(worker.TypeAccountDeletionCleanup, payload); err != nil {
			slog.Error("enqueue account deletion", "error", err)
		}
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
		if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:     requestID,
			Status: string(DeletionFailed),
		}); statusErr != nil {
			slog.Error("update deletion status to failed", "error", statusErr)
		}

		return fmt.Errorf("anonymizing user: %w", err)
	}

	// 2. Revoke all refresh tokens.
	err = s.store.deleteUserTokens(ctx, gen.DeleteUserTokensParams{
		UserID:   request.UserID,
		TenantID: request.TenantID,
	})
	if err != nil {
		slog.Warn("failed to revoke user tokens during deletion",
			"user_id", request.UserID.String(), "error", err)
	}

	// 3. Delete Valkey session keys.
	if s.sessionDeleter != nil {
		if err := s.sessionDeleter.DeleteSessions(ctx, request.UserID.String()); err != nil {
			slog.Warn("failed to delete valkey sessions",
				"user_id", request.UserID.String(), "error", err)
		}
	}

	// 4. Delete Honcho AI context — mandatory, not best-effort. If the Honcho
	//    client is configured and the call fails, mark the deletion as failed
	//    and return the error so Asynq retries the whole job.
	if s.honchoEraser != nil {
		if err := s.honchoEraser.DeleteUser(ctx, request.UserID.String()); err != nil {
			if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
				ID:     requestID,
				Status: string(DeletionFailed),
			}); statusErr != nil {
				slog.Error("update deletion status to failed", "error", statusErr)
			}

			return fmt.Errorf("deleting honcho context for user %s: %w", request.UserID, err)
		}
	} else {
		slog.Warn("honcho eraser not configured, skipping AI context deletion",
			"request_id", requestID, "user_id", request.UserID)
	}

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
		return errors.New("action_type is required")
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

// UserDataExport represents the complete user data export.
type UserDataExport struct {
	UserProfile    map[string]interface{}   `json:"user_profile"`
	Consents       []map[string]interface{} `json:"consents"`
	Content        []map[string]interface{} `json:"content"`
	Comments       []map[string]interface{} `json:"comments"`
	Spaces         []map[string]interface{} `json:"spaces"`
	Payments       []map[string]interface{} `json:"payments"`
	Notifications  []map[string]interface{} `json:"notifications"`
	ExportMetadata map[string]interface{}   `json:"export_metadata"`
}

// collectUserData gathers all user data from relevant tables.
func (s *Service) collectUserData(ctx context.Context, userID, tenantID pgtype.UUID) (*UserDataExport, error) {
	export := &UserDataExport{
		ExportMetadata: map[string]interface{}{
			"user_id":   userID.String(),
			"tenant_id": tenantID.String(),
			"generated": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0",
			"platform":  "JapaConnect",
		},
	}

	// Collect user profile (anonymized in export since user is being deleted).
	export.UserProfile = map[string]interface{}{
		"user_id":    userID.String(),
		"exported":   true,
		"anonymized": true,
	}

	// Collect consents
	consents, err := s.store.getUserConsents(ctx, userID, "")
	if err != nil {
		slog.Warn("failed to collect consents for export", "user_id", userID.String(), "error", err)
	} else {
		for _, c := range consents {
			export.Consents = append(export.Consents, map[string]interface{}{
				"type":        c.ConsentType,
				"granted":     c.Granted,
				"policy":      c.PolicyVersion,
				"recorded_at": c.CreatedAt.Time,
			})
		}
	}

	// Note: In production, collect from additional tables (content, comments,
	// spaces, payments, notifications) using the database reader.
	// For now, we collect what's available through the GDPR store.

	return export, nil
}
