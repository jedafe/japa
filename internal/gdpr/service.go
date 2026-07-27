package gdpr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
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
	siteName       string
	settings       SettingsProvider
}

// SettingsProvider abstracts system settings access for hard-delete configuration.
type SettingsProvider interface {
	GetSetting(ctx context.Context, tenantID pgtype.UUID, key string) (string, error)
	GetSettingBool(ctx context.Context, tenantID pgtype.UUID, key string) (bool, error)
	GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error)
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, siteName string) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:       database,
		enqueuer: &noopEnqueuer{},
		siteName: siteName,
	}
}

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer, siteName string) *Service {
	svc := NewService(database, siteName)
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

// SetSettings injects the system settings provider for hard-delete configuration.
func (s *Service) SetSettings(sp SettingsProvider) {
	s.settings = sp
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

// GetLatestDataExportRequest returns the caller's most recent export request.
// The bool is false if they have never requested one (catalog: GET
// /me/data-export/status).
func (s *Service) GetLatestDataExportRequest(ctx context.Context, userID, tenantID pgtype.UUID) (gen.DataExportRequest, bool, error) {
	return s.store.getLatestDataExportRequestByUser(ctx, userID, tenantID)
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
	} else if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
		// AUDIT-BE-019: In production, Honcho erasure is mandatory.
		// A nil eraser means the service was misconfigured — fail the
		// entire deletion job so it retries after correct wiring.
		if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:     requestID,
			Status: string(DeletionFailed),
		}); statusErr != nil {
			slog.Error("update deletion status to failed", "error", statusErr)
		}

		return fmt.Errorf("honcho eraser not configured — required in production for user %s", request.UserID)
	} else {
		slog.Warn("honcho eraser not configured, skipping AI context deletion",
			"request_id", requestID, "user_id", request.UserID)
	}

	// 5. Hard delete — optional physical erasure phase (ADR-057).
	//    Only runs if gdpr.hard_delete_enabled is true and the retention window
	//    (gdpr.retention_days) has elapsed since the request was created.
	//    If retention hasn't elapsed, mark as retention_wait so a periodic
	//    sweep can re-process it later. 
	retentionWait := false
	if s.settings != nil {
		hardDeleteEnabled, settingsErr := s.settings.GetSettingBool(ctx, request.TenantID, "gdpr.hard_delete_enabled")
		if settingsErr == nil && hardDeleteEnabled {
			retentionDays, _ := s.settings.GetSettingInt(ctx, request.TenantID, "gdpr.retention_days")
			retentionDeadline := request.CreatedAt.Time.Add(time.Duration(retentionDays) * 24 * time.Hour)
			if time.Now().After(retentionDeadline) {
				slog.Info("gdpr: performing physical erasure",
					"user_id", request.UserID.String(),
					"request_id", requestID.String())

				if hardErr := s.hardDeleteUser(ctx, request.UserID, request.TenantID); hardErr != nil {
					if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
						ID:     requestID,
						Status: string(DeletionFailed),
					}); statusErr != nil {
						slog.Error("update deletion status to failed", "error", statusErr)
					}
					return fmt.Errorf("hard deleting user %s: %w", request.UserID, hardErr)
				}
			} else {
				slog.Info("gdpr: retention period not elapsed, entering retention_wait",
					"user_id", request.UserID.String(),
					"retention_days", retentionDays,
					"retention_deadline", retentionDeadline.Format(time.RFC3339))
				retentionWait = true
			}
		}
	}

	// 6. Mark deletion as completed or retention_wait.
	if retentionWait {
		_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:     requestID,
			Status: string(DeletionRetentionWait),
		})
		if err != nil {
			return fmt.Errorf("marking deletion as retention_wait: %w", err)
		}
	} else {
		_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
			ID:          requestID,
			Status:      string(DeletionCompleted),
			CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("marking deletion as completed: %w", err)
		}
	}

	return nil
}

// hardDeleteUser performs physical deletion of the user row after handling
// non-CASCADE foreign key references. Called only when gdpr.hard_delete_enabled
// is true and the retention period has elapsed.
func (s *Service) hardDeleteUser(ctx context.Context, userID, tenantID pgtype.UUID) error {
	// Resolve the deleted-user placeholder UUID from settings, or use zero UUID.
	deletedUserPlaceholder := pgtype.UUID{}
	_ = deletedUserPlaceholder.Scan("00000000-0000-0000-0000-000000000000")

	if s.settings != nil {
		if placeholderStr, err := s.settings.GetSetting(ctx, tenantID, "gdpr.deleted_user_placeholder_uuid"); err == nil && placeholderStr != "" {
			var p pgtype.UUID
			if scanErr := p.Scan(placeholderStr); scanErr == nil {
				deletedUserPlaceholder = p
			}
		}
	}

	// All four steps run in one transaction so a failure cannot leave the user
	// half-erased (e.g. authorship reassigned but the user row still present).
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// 1. Reassign content authorship to preserve thread continuity.
		if err := q.ReassignContentAuthor(ctx, gen.ReassignContentAuthorParams{
			UserID:       userID,
			TenantID:     tenantID,
			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning content authorship: %w", err)
		}

		// 2. Nullify payment user references (financial audit trail survives).
		//    Errors are fatal inside the tx — a leftover reference would block
		//    the final DELETE on its foreign key anyway.
		if err := q.NullifyPaymentUserRefs(ctx, gen.NullifyPaymentUserRefsParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying payment user refs: %w", err)
		}

		// 3. Nullify ticket submitter references.
		if err := q.NullifyTicketSubmitter(ctx, gen.NullifyTicketSubmitterParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying ticket submitter: %w", err)
		}

		// 4. AUDIT-FRESH-GDPR-001: explicit cleanup for tables whose FK to
		//    users(id) has no ON DELETE CASCADE/SET NULL. Without these the
		//    final HardDeleteUser would block on a residual foreign key.

		// content_revisions.edited_by has no CASCADE; drop the user's edit history.
		if err := q.DeleteContentRevisionsByEditor(ctx, userID); err != nil {
			return fmt.Errorf("deleting content revisions: %w", err)
		}

		// space_pinned_posts.pinned_by has no CASCADE; remove pins made by the user.
		if err := q.DeleteSpacePinnedPostsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting space pinned posts: %w", err)
		}

		// forum_tags.moderator_user_id is nullable; clear it so the tag survives.
		if err := q.NullifyForumTagModerator(ctx, gen.NullifyForumTagModeratorParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying forum tag moderator: %w", err)
		}

		// ticket_comments.user_id has no CASCADE; delete the user's ticket comments.
		if err := q.DeleteTicketCommentsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting ticket comments: %w", err)
		}

		// ticket_events.user_id is nullable audit trail; nullify the actor, keep the event.
		if err := q.NullifyTicketEventsUser(ctx, userID); err != nil {
			return fmt.Errorf("nullifying ticket events user: %w", err)
		}

		// user_preferences is user-owned config; remove it explicitly.
		if err := q.DeleteUserPreferences(ctx, gen.DeleteUserPreferencesParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting user preferences: %w", err)
		}

		// professional_profiles.owner_user_id has no CASCADE; delete owned profiles (cascades members/invitations).
		if err := q.DeleteProfessionalProfilesByOwner(ctx, gen.DeleteProfessionalProfilesByOwnerParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting professional profiles: %w", err)
		}

		// professional_profile_invitations.invited_by is nullable; clear inviter on others' profiles.
		if err := q.NullifyProfessionalProfileInvitationInviter(ctx, userID); err != nil {
			return fmt.Errorf("nullifying professional profile invitation inviter: %w", err)
		}

		// organizations.owner_user_id is NOT NULL; reassign to placeholder to preserve the org for remaining members.
		if err := q.ReassignOrganizationOwner(ctx, gen.ReassignOrganizationOwnerParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning organization owner: %w", err)
		}

		// organization_invitations.invited_by is nullable; clear inviter reference.
		if err := q.NullifyOrganizationInvitationInviter(ctx, userID); err != nil {
			return fmt.Errorf("nullifying organization invitation inviter: %w", err)
		}

		// ownership_claims.claimant_user_id is NOT NULL; delete the user's submitted claims.
		if err := q.DeleteOwnershipClaimsByClaimant(ctx, gen.DeleteOwnershipClaimsByClaimantParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting ownership claims: %w", err)
		}

		// consent_records.user_id is nullable; preserve the compliance record, detach the user.
		if err := q.NullifyConsentRecordUser(ctx, gen.NullifyConsentRecordUserParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying consent record user: %w", err)
		}

		// data_export_requests.user_id is NOT NULL; reassign to placeholder to preserve export audit trail.
		if err := q.ReassignDataExportRequestUser(ctx, gen.ReassignDataExportRequestUserParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning data export request user: %w", err)
		}

		// account_deletion_requests.user_id is NOT NULL; reassign to placeholder so the in-flight request row survives.
		if err := q.ReassignAccountDeletionRequestUser(ctx, gen.ReassignAccountDeletionRequestUserParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning account deletion request user: %w", err)
		}

		// audit_logs must be preserved for compliance; repoint actor/impersonated refs to the placeholder "Deleted User".
		if err := q.AnonymizeAuditLogActor(ctx, gen.AnonymizeAuditLogActorParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing audit log actor: %w", err)
		}

		// 5. Physical deletion — ON DELETE CASCADE handles all child tables.
		if err := q.HardDeleteUser(ctx, gen.HardDeleteUserParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("physically deleting user row: %w", err)
		}

		return nil
	}); err != nil {
		slog.Warn("gdpr: hard delete transaction failed", "user_id", userID.String(), "error", err)
		return err
	}

	slog.Info("gdpr: user physically deleted",
		"user_id", userID.String(),
		"tenant_id", tenantID.String())

	return nil
}

// HardDeleteUser is the public entry point for admin-triggered hard deletes.
// It performs the full GDPR cascade (reassign content, nullify refs, physical delete)
// in a single transaction. This is the same logic as the internal hardDeleteUser
// but exposed for admin use on already-soft-deleted users.
func (s *Service) HardDeleteUser(ctx context.Context, userID, tenantID pgtype.UUID) error {
	return s.hardDeleteUser(ctx, userID, tenantID)
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
	UserProfile            map[string]interface{}   `json:"user_profile"`
	Consents               []map[string]interface{} `json:"consents"`
	Content                []map[string]interface{} `json:"content"`
	Comments               []map[string]interface{} `json:"comments"`
	Spaces                 []map[string]interface{} `json:"spaces"`
	Payments               []map[string]interface{} `json:"payments"`
	Notifications          []map[string]interface{} `json:"notifications"`
	UserProfiles           []map[string]interface{} `json:"user_profiles"`
	Reactions              []map[string]interface{} `json:"reactions"`
	Bookmarks              []map[string]interface{} `json:"bookmarks"`
	ContentShares          []map[string]interface{} `json:"content_shares"`
	Follows                []map[string]interface{} `json:"follows"`
	ForumThreads           []map[string]interface{} `json:"forum_threads"`
	ForumReplies           []map[string]interface{} `json:"forum_replies"`
	ForumVotes             []map[string]interface{} `json:"forum_votes"`
	ForumBookmarks         []map[string]interface{} `json:"forum_bookmarks"`
	Media                  []map[string]interface{} `json:"media"`
	Reviews                []map[string]interface{} `json:"reviews"`
	SpaceMembers           []map[string]interface{} `json:"space_members"`
	ConsentRecords         []map[string]interface{} `json:"consent_records"`
	WalletLedger           []map[string]interface{} `json:"wallet_ledger"`
	LessonProgress         []map[string]interface{} `json:"lesson_progress"`
	LearningCertificates   []map[string]interface{} `json:"learning_certificates"`
	MentorshipMatches      []map[string]interface{} `json:"mentorship_matches"`
	ProfessionalProfiles   []map[string]interface{} `json:"professional_profiles"`
	Organizations          []map[string]interface{} `json:"organizations"`
	ApiKeys                []map[string]interface{} `json:"api_keys"`
	UserPreferences        []map[string]interface{} `json:"user_preferences"`
	UserMutes              []map[string]interface{} `json:"user_mutes"`
	SpaceMutes             []map[string]interface{} `json:"space_mutes"`
	TicketComments         []map[string]interface{} `json:"ticket_comments"`
	ExportMetadata         map[string]interface{}   `json:"export_metadata"`
}

// collectUserData gathers all user data from relevant tables.
func (s *Service) collectUserData(ctx context.Context, userID, tenantID pgtype.UUID) (*UserDataExport, error) {
	export := &UserDataExport{
		ExportMetadata: map[string]interface{}{
			"user_id":   userID.String(),
			"tenant_id": tenantID.String(),
			"generated": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0",
			"platform":  s.siteName,
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

	// Collect user's published content.
	if contentRows, err := s.store.gdprExportUserContent(ctx, userID, tenantID); err == nil {
		for _, c := range contentRows {
			export.Content = append(export.Content, map[string]interface{}{
				"id":           c.ID.String(),
				"title":        c.Title,
				"slug":         c.Slug,
				"content_type": c.ContentType,
				"status":       c.Status,
				"created_at":   c.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect content for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's comments.
	if commentRows, err := s.store.gdprExportUserComments(ctx, userID, tenantID); err == nil {
		for _, c := range commentRows {
			export.Comments = append(export.Comments, map[string]interface{}{
				"id":         c.ID.String(),
				"content_id": c.ContentID.String(),
				"body":       c.Body,
				"created_at": c.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect comments for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's space memberships.
	if spaceRows, err := s.store.gdprExportUserSpaces(ctx, userID, tenantID); err == nil {
		for _, sp := range spaceRows {
			export.Spaces = append(export.Spaces, map[string]interface{}{
				"id":        sp.ID.String(),
				"name":      sp.Name,
				"slug":      sp.Slug,
				"status":    sp.Status,
				"role":      sp.Role,
				"joined_at": sp.JoinedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect spaces for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's payment history.
	if paymentRows, err := s.store.gdprExportUserPayments(ctx, userID, tenantID); err == nil {
		for _, p := range paymentRows {
			export.Payments = append(export.Payments, map[string]interface{}{
				"id":         p.ID.String(),
				"provider":   p.Provider,
				"amount":     p.Amount,
				"currency":   p.Currency,
				"status":     p.Status,
				"created_at": p.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect payments for export", "user_id", userID.String(), "error", err)
	}

	// Collect user's notifications.
	if notifRows, err := s.store.gdprExportUserNotifications(ctx, userID, tenantID); err == nil {
		for _, n := range notifRows {
			export.Notifications = append(export.Notifications, map[string]interface{}{
				"id":                n.ID.String(),
				"notification_type": n.NotificationType,
				"title":             n.Title,
				"body":              n.Body,
				"read_at":           n.ReadAt,
				"created_at":        n.CreatedAt.Time,
			})
		}
	} else {
		slog.Warn("failed to collect notifications for export", "user_id", userID.String(), "error", err)
	}

	// AUDIT-FRESH-GDPR-002: collect the remaining user-owned tables. Each block
	// is best-effort — a failure logs a warning and the export still succeeds
	// with the tables that could be collected, matching the existing pattern.

	// User profile detail row.
	if rows, err := s.store.gdprExportUserProfiles(ctx, userID, tenantID); err == nil {
		export.UserProfiles = rows
	} else {
		slog.Warn("failed to collect user_profiles for export", "user_id", userID.String(), "error", err)
	}

	// Reactions the user placed on content.
	if rows, err := s.store.gdprExportUserReactions(ctx, userID, tenantID); err == nil {
		export.Reactions = rows
	} else {
		slog.Warn("failed to collect reactions for export", "user_id", userID.String(), "error", err)
	}

	// Bookmarks the user saved.
	if rows, err := s.store.gdprExportUserBookmarks(ctx, userID, tenantID); err == nil {
		export.Bookmarks = rows
	} else {
		slog.Warn("failed to collect bookmarks for export", "user_id", userID.String(), "error", err)
	}

	// Content shares the user created.
	if rows, err := s.store.gdprExportUserContentShares(ctx, userID, tenantID); err == nil {
		export.ContentShares = rows
	} else {
		slog.Warn("failed to collect content_shares for export", "user_id", userID.String(), "error", err)
	}

	// Follow relationships initiated by the user.
	if rows, err := s.store.gdprExportUserFollows(ctx, userID, tenantID); err == nil {
		export.Follows = rows
	} else {
		slog.Warn("failed to collect follows for export", "user_id", userID.String(), "error", err)
	}

	// Forum threads authored by the user.
	if rows, err := s.store.gdprExportUserForumThreads(ctx, userID, tenantID); err == nil {
		export.ForumThreads = rows
	} else {
		slog.Warn("failed to collect forum_threads for export", "user_id", userID.String(), "error", err)
	}

	// Forum replies authored by the user.
	if rows, err := s.store.gdprExportUserForumReplies(ctx, userID, tenantID); err == nil {
		export.ForumReplies = rows
	} else {
		slog.Warn("failed to collect forum_replies for export", "user_id", userID.String(), "error", err)
	}

	// Forum votes cast by the user.
	if rows, err := s.store.gdprExportUserForumVotes(ctx, userID, tenantID); err == nil {
		export.ForumVotes = rows
	} else {
		slog.Warn("failed to collect forum_votes for export", "user_id", userID.String(), "error", err)
	}

	// Forum bookmarks the user saved.
	if rows, err := s.store.gdprExportUserForumBookmarks(ctx, userID, tenantID); err == nil {
		export.ForumBookmarks = rows
	} else {
		slog.Warn("failed to collect forum_bookmarks for export", "user_id", userID.String(), "error", err)
	}

	// Media uploaded by the user.
	if rows, err := s.store.gdprExportUserMedia(ctx, userID, tenantID); err == nil {
		export.Media = rows
	} else {
		slog.Warn("failed to collect media for export", "user_id", userID.String(), "error", err)
	}

	// Reviews the user wrote.
	if rows, err := s.store.gdprExportUserReviews(ctx, userID, tenantID); err == nil {
		export.Reviews = rows
	} else {
		slog.Warn("failed to collect reviews for export", "user_id", userID.String(), "error", err)
	}

	// Space membership records.
	if rows, err := s.store.gdprExportUserSpaceMembers(ctx, userID, tenantID); err == nil {
		export.SpaceMembers = rows
	} else {
		slog.Warn("failed to collect space_members for export", "user_id", userID.String(), "error", err)
	}

	// Consent records (full rows, complementing the summarized Consents block).
	if rows, err := s.store.gdprExportUserConsentRecords(ctx, userID, tenantID); err == nil {
		export.ConsentRecords = rows
	} else {
		slog.Warn("failed to collect consent_records for export", "user_id", userID.String(), "error", err)
	}

	// Wallet ledger entries for the user.
	if rows, err := s.store.gdprExportUserWalletLedger(ctx, userID, tenantID); err == nil {
		export.WalletLedger = rows
	} else {
		slog.Warn("failed to collect wallet_ledger for export", "user_id", userID.String(), "error", err)
	}

	// Learning lesson progress.
	if rows, err := s.store.gdprExportUserLessonProgress(ctx, userID, tenantID); err == nil {
		export.LessonProgress = rows
	} else {
		slog.Warn("failed to collect lesson_progress for export", "user_id", userID.String(), "error", err)
	}

	// Issued learning certificates.
	if rows, err := s.store.gdprExportUserLearningCertificates(ctx, userID, tenantID); err == nil {
		export.LearningCertificates = rows
	} else {
		slog.Warn("failed to collect learning_certificates for export", "user_id", userID.String(), "error", err)
	}

	// Mentorship matches where the user is mentor or mentee.
	if rows, err := s.store.gdprExportUserMentorshipMatches(ctx, userID, tenantID); err == nil {
		export.MentorshipMatches = rows
	} else {
		slog.Warn("failed to collect mentorship_matches for export", "user_id", userID.String(), "error", err)
	}

	// Professional profiles owned by the user.
	if rows, err := s.store.gdprExportUserProfessionalProfiles(ctx, userID, tenantID); err == nil {
		export.ProfessionalProfiles = rows
	} else {
		slog.Warn("failed to collect professional_profiles for export", "user_id", userID.String(), "error", err)
	}

	// Organizations owned by the user.
	if rows, err := s.store.gdprExportUserOrganizations(ctx, userID, tenantID); err == nil {
		export.Organizations = rows
	} else {
		slog.Warn("failed to collect organizations for export", "user_id", userID.String(), "error", err)
	}

	// API keys issued by the user.
	if rows, err := s.store.gdprExportUserApiKeys(ctx, userID, tenantID); err == nil {
		export.ApiKeys = rows
	} else {
		slog.Warn("failed to collect api_keys for export", "user_id", userID.String(), "error", err)
	}

	// User preference settings.
	if rows, err := s.store.gdprExportUserPreferences(ctx, userID, tenantID); err == nil {
		export.UserPreferences = rows
	} else {
		slog.Warn("failed to collect user_preferences for export", "user_id", userID.String(), "error", err)
	}

	// User-initiated mutes of other users.
	if rows, err := s.store.gdprExportUserMutes(ctx, userID, tenantID); err == nil {
		export.UserMutes = rows
	} else {
		slog.Warn("failed to collect user_mutes for export", "user_id", userID.String(), "error", err)
	}

	// Spaces the user muted.
	if rows, err := s.store.gdprExportUserSpaceMutes(ctx, userID, tenantID); err == nil {
		export.SpaceMutes = rows
	} else {
		slog.Warn("failed to collect space_mutes for export", "user_id", userID.String(), "error", err)
	}

	// Comments the user posted on support tickets.
	if rows, err := s.store.gdprExportUserTicketComments(ctx, userID, tenantID); err == nil {
		export.TicketComments = rows
	} else {
		slog.Warn("failed to collect ticket_comments for export", "user_id", userID.String(), "error", err)
	}

	return export, nil
}
