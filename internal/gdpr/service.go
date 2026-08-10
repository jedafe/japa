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
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string
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

// ConsentConfigResponse is the API payload returned by GET /consent/config.
// It surfaces the platform's valid consent types and the current privacy
// policy version so the frontend does not need to hardcode them
// (BUG-015, BUG-016).
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
type ConsentConfigResponse struct {
	ConsentTypes  []string `json:"consent_types"`
	PolicyVersion string   `json:"policy_version"`
}

// validConsentTypes is the canonical list of consent types recognized by the
// platform. Mirrors the values documented in Section 19 and enforced by the
// consent_records.consent_type column. These are platform constants — they
// are NOT admin-configurable, but surfacing them via the API keeps the
// frontend in sync if the set changes (BUG-015).
var validConsentTypes = []string{
	"marketing_email",
	"analytics_tracking",
	"third_party_sharing",
	"ai_processing",
}

// GetConsentConfig returns the valid consent types and the current privacy
// policy version for the tenant. The policy version is sourced from
// system_settings key "gdpr.policy_version" (default "1.0") so admins can
// bump it without a code change (BUG-016).
// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
func (s *Service) GetConsentConfig(ctx context.Context, tenantID pgtype.UUID) (*ConsentConfigResponse, error) {
	policyVersion := "1.0"
	if s.settings != nil {
		// Use GetStringOr — system_settings values are stored as JSON-encoded
		// strings (e.g. '"1.0"'), so GetSetting returns the raw quoted value.
		// GetStringOr unquotes it to '1.0'. Ref: §6.2 nil/quote handling.
		policyVersion = s.settings.GetStringOr(ctx, tenantID, "gdpr.policy_version", "1.0")
	}

	return &ConsentConfigResponse{
		ConsentTypes:  validConsentTypes,
		PolicyVersion: policyVersion,
	}, nil
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
func (s *Service) ListConsentsByTenant(ctx context.Context, tenantID pgtype.UUID, consentType string, limit, offset int32) ([]gen.ListConsentsByTenantRow, error) {
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

// CancelDataExportRequest cancels a pending data export request. Only
// requests with status "pending" or "processing" can be cancelled; once
// completed the data has already been generated and the download link
// should be allowed to expire naturally.
func (s *Service) CancelDataExportRequest(ctx context.Context, userID, tenantID pgtype.UUID) (gen.DataExportRequest, error) {
	request, found, err := s.store.getLatestDataExportRequestByUser(ctx, userID, tenantID)
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("getting data export request: %w", err)
	}
	if !found {
		return gen.DataExportRequest{}, errors.New("no data export request to cancel")
	}
	if request.Status != string(ExportPending) && request.Status != string(ExportProcessing) {
		return gen.DataExportRequest{}, fmt.Errorf("cannot cancel export with status: %s", request.Status)
	}

	updated, err := s.store.updateDataExportRequestStatus(ctx, gen.UpdateDataExportRequestStatusParams{
		Status:      string(ExportCancelled),
		DownloadUrl: nil,
		ExpiresAt:   pgtype.Timestamptz{},
		CompletedAt: pgtype.Timestamptz{},
		ID:          request.ID,
		TenantID:    tenantID,
	})
	if err != nil {
		return gen.DataExportRequest{}, fmt.Errorf("cancelling data export request: %w", err)
	}
	return updated, nil
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
	// AUDIT-FRESH-SETTING-008: export expiry is configurable via system_settings.
	exportExpiryDays := 1
	if s.settings != nil {
		exportExpiryDays = s.settings.GetIntOr(ctx, request.TenantID, "gdpr.export_expiry_days", exportExpiryDays)
	}
	expiresAt := time.Now().Add(time.Duration(exportExpiryDays) * 24 * time.Hour)
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
//
// Resumability (AUDIT-FRESH-GDPR-003): each phase checkpoints a progress_step
// on the request row. If the job crashes or is retried by Asynq, the next run
// reads progress_step and skips phases already completed, so partial work is
// not re-executed from the top. The step map is mirrored in migration
// 000173_account_deletion_progress_step.up.sql.
//
//	retention_wait flow (AUDIT-FRESH-GDPR-006): when gdpr.hard_delete_enabled
//	is true but the gdpr.retention_days window has not elapsed since the
//	request was created, the request is marked retention_wait instead of
//	completed. The anonymization + token/session/Honcho erasure has already
//	happened (steps 1-4), so the user is functionally erased from the
//	platform; only the physical row deletion is deferred. A periodic sweep
//	(ListRetentionWaitElapsed, run by the maintenance scheduler) re-invokes
//	this handler once the window elapses, at which point step 5 performs the
//	hard delete and step 6 marks the request completed.
func (s *Service) ProcessAccountDeletion(ctx context.Context, requestID pgtype.UUID) error {
	request, err := s.store.getAccountDeletionRequest(ctx, requestID, pgtype.UUID{})
	if err != nil {
		return fmt.Errorf("fetching deletion request: %w", err)
	}

	// Mark as processing. A retried job re-enters here; progress_step is
	// preserved across retries because UpdateAccountDeletionRequestStatus
	// does not touch progress_step.
	_, err = s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:     requestID,
		Status: string(DeletionProcessing),
	})
	if err != nil {
		return fmt.Errorf("marking deletion as processing: %w", err)
	}

	step := request.ProgressStep

	// 1. Anonymize user row + user_profile PII.
	if step < deletionStepAnonymized {
		if err := s.store.anonymizeUser(ctx, gen.AnonymizeUserParams{
			UserID:   request.UserID,
			TenantID: request.TenantID,
		}); err != nil {
			s.markDeletionFailed(ctx, requestID)
			return fmt.Errorf("anonymizing user: %w", err)
		}
		// Clear PII in user_profiles (display_name, bio, timezone, business
		// info, content_country_preference, preferred_currency, avatar/cover)
		// so it is scrubbed during the retention window, not just at hard
		// delete (ADR-014 / ADR-057). Best-effort: a failure logs a warning
		// but does not abort — the user row is already anonymized.
		if err := s.store.anonymizeUserProfile(ctx, gen.AnonymizeUserProfileParams{
			UserID:   request.UserID,
			TenantID: request.TenantID,
		}); err != nil {
			slog.Warn("failed to anonymize user profile during deletion",
				"user_id", request.UserID.String(), "error", err)
		}
		// Scrub PII (ip_hash, user_agent_hash) from affiliate_link_events
		// owned by the user's affiliate links (Section 56, ADR-014). The
		// events are retained for commission audit; only tracking hashes are
		// cleared. Best-effort: a failure logs a warning but does not abort.
		if err := s.store.anonymizeAffiliateLinkEvents(ctx, gen.AnonymizeAffiliateLinkEventsParams{
			UserID:   request.UserID,
			TenantID: request.TenantID,
		}); err != nil {
			slog.Warn("failed to anonymize affiliate link events during deletion",
				"user_id", request.UserID.String(), "error", err)
		}
		// Scrub PII from onboarding_state.step_data (name, current_location,
		// destination) during the anonymization phase (Section 09, ADR-014).
		// The row has ON DELETE CASCADE from users(id) so it is removed at
		// hard-delete, but step_data PII must be cleared now so it is not
		// recoverable during the retention window. Best-effort.
		if err := s.store.write.AnonymizeOnboardingState(ctx, gen.AnonymizeOnboardingStateParams{
			UserID:   request.UserID,
			TenantID: request.TenantID,
		}); err != nil {
			slog.Warn("failed to anonymize onboarding state during deletion",
				"user_id", request.UserID.String(), "error", err)
		}
		// Scrub auth-related PII tables during the anonymization phase so
		// sensitive data (TOTP secrets, OAuth provider IDs, email addresses,
		// password hashes, reset/verification tokens) is not recoverable
		// during the retention window. These tables have ON DELETE CASCADE
		// from users(id), so they would be cleaned at hard-delete, but the
		// anonymization phase runs first and the data must be scrubbed now.
		// Ref: sections/02-authentication-sessions.md, ADR-014.
		// Best-effort: failures log warnings but do not abort.
		authPIITables := []struct {
			name string
			fn   func() error
		}{
			{"totp_credentials", func() error {
				return s.store.deleteUserTOTPCredentials(ctx, gen.DeleteUserTOTPCredentialsParams{
					UserID: request.UserID, TenantID: request.TenantID,
				})
			}},
			{"oauth_accounts", func() error {
				return s.store.deleteUserOAuthAccounts(ctx, gen.DeleteUserOAuthAccountsParams{
					UserID: request.UserID, TenantID: request.TenantID,
				})
			}},
			{"password_reset_tokens", func() error {
				return s.store.deleteUserPasswordResetTokens(ctx, gen.DeleteUserPasswordResetTokensParams{
					UserID: request.UserID, TenantID: request.TenantID,
				})
			}},
			{"email_verifications", func() error {
				return s.store.deleteUserEmailVerifications(ctx, gen.DeleteUserEmailVerificationsParams{
					UserID: request.UserID, TenantID: request.TenantID,
				})
			}},
			{"password_history", func() error {
				return s.store.deleteUserPasswordHistory(ctx, gen.DeleteUserPasswordHistoryParams{
					UserID: request.UserID, TenantID: request.TenantID,
				})
			}},
		}
		for _, t := range authPIITables {
			if err := t.fn(); err != nil {
				slog.Warn("failed to scrub auth PII table during deletion",
					"table", t.name,
					"user_id", request.UserID.String(), "error", err)
			}
		}
		if err := s.checkpoint(ctx, requestID, request.TenantID, deletionStepAnonymized); err != nil {
			return err
		}
		step = deletionStepAnonymized
	}

	// 1b. Scrub PII from saved_scholarships.notes during the anonymization
	// phase (Section 73, ADR-073, ADR-014). The table has ON DELETE CASCADE
	// from users(id) so rows are removed at hard-delete, but the user's
	// free-text application notes must be cleared now so they are not
	// recoverable during the retention window. Best-effort.
	if step == deletionStepAnonymized {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE saved_scholarships
			SET notes = NULL
			WHERE user_id = $1 AND tenant_id = $2 AND notes IS NOT NULL
		`, request.UserID, request.TenantID); err != nil {
			slog.Warn("failed to scrub saved_scholarships notes during deletion",
				"user_id", request.UserID.String(), "error", err)
		}
	}

	// 2. Revoke all refresh tokens (best-effort).
	if step < deletionStepTokensRevoked {
		if err := s.store.deleteUserTokens(ctx, gen.DeleteUserTokensParams{
			UserID:   request.UserID,
			TenantID: request.TenantID,
		}); err != nil {
			slog.Warn("failed to revoke user tokens during deletion",
				"user_id", request.UserID.String(), "error", err)
		}
		if err := s.checkpoint(ctx, requestID, request.TenantID, deletionStepTokensRevoked); err != nil {
			return err
		}
		step = deletionStepTokensRevoked
	}

	// 3. Delete Valkey session keys (best-effort).
	if step < deletionStepSessionsDeleted {
		if s.sessionDeleter != nil {
			if err := s.sessionDeleter.DeleteSessions(ctx, request.UserID.String()); err != nil {
				slog.Warn("failed to delete valkey sessions",
					"user_id", request.UserID.String(), "error", err)
			}
		}
		if err := s.checkpoint(ctx, requestID, request.TenantID, deletionStepSessionsDeleted); err != nil {
			return err
		}
		step = deletionStepSessionsDeleted
	}

	// 4. Delete Honcho AI context — mandatory, not best-effort. If the Honcho
	//    client is configured and the call fails, mark the deletion as failed
	//    and return the error so Asynq retries from this step.
	if step < deletionStepHonchoDeleted {
		if s.honchoEraser != nil {
			if err := s.honchoEraser.DeleteUser(ctx, request.UserID.String()); err != nil {
				s.markDeletionFailed(ctx, requestID)
				return fmt.Errorf("deleting honcho context for user %s: %w", request.UserID, err)
			}
		} else if strings.EqualFold(os.Getenv("JC_ENV"), "production") {
			// AUDIT-BE-019: In production, Honcho erasure is mandatory.
			// A nil eraser means the service was misconfigured — fail the
			// entire deletion job so it retries after correct wiring.
			s.markDeletionFailed(ctx, requestID)
			return fmt.Errorf("honcho eraser not configured — required in production for user %s", request.UserID)
		} else {
			slog.Warn("honcho eraser not configured, skipping AI context deletion",
				"request_id", requestID, "user_id", request.UserID)
		}
		if err := s.checkpoint(ctx, requestID, request.TenantID, deletionStepHonchoDeleted); err != nil {
			return err
		}
		step = deletionStepHonchoDeleted
	}

	// 5. Hard delete — optional physical erasure phase (ADR-057).
	//    See the retention_wait flow documented in the function comment above.
	retentionWait := false
	if step < deletionStepHardDeleted {
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
						s.markDeletionFailed(ctx, requestID)
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
		if err := s.checkpoint(ctx, requestID, request.TenantID, deletionStepHardDeleted); err != nil {
			return err
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

	// AUDIT-FRESH-GDPR-004: record an audit trail entry for the erasure.
	// GDPR requires an audit log of deletions; the actor is the system when
	// the worker drives the flow, or the user when self-service triggered it.
	s.logDeletionAudit(ctx, request, retentionWait)

	return nil
}

// deletion progress_step constants (AUDIT-FRESH-GDPR-003). Mirrored in
// migration 000173_account_deletion_progress_step.up.sql.
const (
	deletionStepNotStarted      int32 = 0
	deletionStepAnonymized      int32 = 1
	deletionStepTokensRevoked   int32 = 2
	deletionStepSessionsDeleted int32 = 3
	deletionStepHonchoDeleted   int32 = 4
	deletionStepHardDeleted     int32 = 5
)

// checkpoint persists the completed step so a retried job resumes after it.
func (s *Service) checkpoint(ctx context.Context, requestID, tenantID pgtype.UUID, step int32) error {
	if err := s.store.updateAccountDeletionRequestProgress(ctx, requestID, tenantID, step); err != nil {
		return fmt.Errorf("checkpointing deletion step %d: %w", step, err)
	}
	return nil
}

// markDeletionFailed flips the request status to failed and logs the secondary error.
func (s *Service) markDeletionFailed(ctx context.Context, requestID pgtype.UUID) {
	if _, statusErr := s.store.updateAccountDeletionRequestStatus(ctx, gen.UpdateAccountDeletionRequestStatusParams{
		ID:     requestID,
		Status: string(DeletionFailed),
	}); statusErr != nil {
		slog.Error("update deletion status to failed", "error", statusErr)
	}
}

// logDeletionAudit writes the GDPR-required audit trail entry for an erasure.
// Best-effort: a logging failure is logged but does not fail the deletion.
func (s *Service) logDeletionAudit(ctx context.Context, request gen.AccountDeletionRequest, retentionWait bool) {
	action := "gdpr.account_deleted"
	if retentionWait {
		action = "gdpr.account_retention_wait"
	}
	if err := logAuditEvent(ctx, s.store, request.TenantID, LogAuditParams{
		ActorUserID: request.UserID,
		ActionType:  action,
		TargetType:  "user",
		TargetID:    request.UserID,
		Metadata: map[string]any{
			"request_id":     request.ID.String(),
			"retention_wait": retentionWait,
		},
	}); err != nil {
		slog.Warn("gdpr: failed to write deletion audit log",
			"request_id", request.ID.String(), "error", err)
	}
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

		// C7: reassign blog_series authorship to preserve curated multi-part
		// series. Without this, ON DELETE CASCADE on blog_series.author_user_id
		// destroys all user-authored series and cascades to blog_series_posts.
		if err := q.ReassignBlogSeriesAuthor(ctx, gen.ReassignBlogSeriesAuthorParams{
			UserID:       userID,
			TenantID:     tenantID,
			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning blog series authorship: %w", err)
		}

		// C7: reassign content_reviews reviewer to preserve editorial audit
		// history. Without this, ON DELETE CASCADE on
		// content_reviews.reviewer_user_id destroys all review records.
		if err := q.ReassignContentReviewReviewer(ctx, gen.ReassignContentReviewReviewerParams{
			UserID:       userID,
			TenantID:     tenantID,
			TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning content review reviewer: %w", err)
		}

		// 2. Nullify payment user references (financial audit trail survives).
		//    Errors are fatal inside the tx — a leftover reference would block
		//    the final DELETE on its foreign key anyway.
		if err := q.NullifyPaymentUserRefs(ctx, gen.NullifyPaymentUserRefsParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying payment user refs: %w", err)
		}

		// 2a. Nullify subscription user references (BUG-020).
		if err := q.NullifySubscriptionUserRefs(ctx, gen.NullifySubscriptionUserRefsParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying subscription user refs: %w", err)
		}

		// 2b. Nullify discount code creator references (BUG-020).
		if err := q.NullifyDiscountCodeCreatorRefs(ctx, gen.NullifyDiscountCodeCreatorRefsParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying discount code creator refs: %w", err)
		}

		// 2c. Nullify discount code use user references (BUG-020).
		// Uses a sentinel UUID because the PRIMARY KEY (code_id, user_id) does
		// not allow NULL user_id.
		if err := q.NullifyDiscountCodeUseUserRefs(ctx, userID); err != nil {
			return fmt.Errorf("nullifying discount code use user refs: %w", err)
		}

		// 3. Nullify ticket submitter references.
		if err := q.NullifyTicketSubmitter(ctx, gen.NullifyTicketSubmitterParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying ticket submitter: %w", err)
		}

		// 3b. Anonymize comparison_items authored by the user (BUG-029).
		// comparison_items carry title/description/image_id that can re-identify
		// a user even after their content is soft-deleted.
		if _, err := q.AnonymizeComparisonItemsByAuthor(ctx, gen.AnonymizeComparisonItemsByAuthorParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("anonymizing comparison items: %w", err)
		}

		// BUG-018: delete custom_field_values for the user's profile entity
		// so PII stored in custom fields (Japa Status, phone, address, etc.)
		// does not survive account deletion. The custom_field_values table
		// is polymorphic on (entity_type, entity_id); we delete rows where
		// entity_type is 'user_profile' or 'user' and entity_id is the user.
		if _, err := q.DeleteCustomFieldValuesByEntity(ctx, userID); err != nil {
			return fmt.Errorf("deleting custom field values for user entity: %w", err)
		}

		// BUG-019: anonymize form_submissions authored by the deleted user.
		// The submission row is preserved for analytics/audit (form
		// submission counts, trigger audit trail), but the user_id reference
		// is severed and the PII fields (ip_address, field_values jsonb) are
		// scrubbed to '{}' so submitted answers cannot re-identify the user.
		if _, err := q.AnonymizeFormSubmissionsByUser(ctx, userID); err != nil {
			return fmt.Errorf("anonymizing form submissions: %w", err)
		}

		// 4. AUDIT-FRESH-GDPR-001: explicit cleanup for tables whose FK to
		//    users(id) has no ON DELETE CASCADE/SET NULL. Without these the
		//    final HardDeleteUser would block on a residual foreign key.

		// content_revisions.edited_by has no CASCADE; drop the user's edit history.
		if err := q.DeleteContentRevisionsByEditor(ctx, userID); err != nil {
			return fmt.Errorf("deleting content revisions: %w", err)
		}

		// user_content_views.user_id has ON DELETE CASCADE, but we delete
		// explicitly to document the erasure path and ensure PII is
		// removed before the final HardDeleteUser (Section 14, ADR-014).
		if err := q.GdprDeleteUserContentViews(ctx, userID); err != nil {
			return fmt.Errorf("deleting user content views: %w", err)
		}

		// GDPR: delete ad_events (impression/click tracking) for the user.
		// ad_events.user_id has ON DELETE CASCADE but we delete explicitly to
		// document the erasure path and remove the user's ad-interaction PII
		// before the final HardDeleteUser. Ref: Section 25/61, GDPR erasure.
		if err := q.GdprDeleteAdEventsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user ad events: %w", err)
		}

		// GDPR: nullify ad_campaigns.created_by for campaigns created by the
		// user. The FK has no CASCADE/SET NULL, so without this step the final
		// HardDeleteUser would block on the residual foreign key. The campaign
		// row is preserved (it may belong to an organization). Ref: Section 25/61.
		if err := q.GdprNullifyAdCampaignCreator(ctx, userID); err != nil {
			return fmt.Errorf("nullifying ad campaign creator: %w", err)
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

		// tag_moderators.user_id links the user to tags they moderate (Section 08/49).
		// The FK has ON DELETE CASCADE, but we delete explicitly to document the
		// erasure path and clear the moderation history before the final HardDeleteUser.
		if err := q.DeleteTagModeratorsByUser(ctx, gen.DeleteTagModeratorsByUserParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting tag moderators: %w", err)
		}

		// BUG-022: forum_thread_tags for threads authored by the user are
		// junction rows that reference the user's threads. Delete them so the
		// final HardDeleteUser (which cascades to forum_threads →
		// forum_thread_tags) doesn't leave orphaned junction rows if the
		// thread is retained for soft-delete. Ref: Section 08 + GDPR.
		if err := q.DeleteForumThreadTagsByAuthor(ctx, gen.DeleteForumThreadTagsByAuthorParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting forum thread tags by author: %w", err)
		}

		// ticket_comments.user_id has no CASCADE; delete the user's ticket comments.
		if err := q.DeleteTicketCommentsByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting ticket comments: %w", err)
		}

		// BUG-024: DM erasure — anonymize DM content and reassign participant
		// references to the placeholder so the conversation survives for the
		// other participant. Without these steps, the CASCADE on users(id)
		// would hard-delete the entire conversation (including the other
		// participant's copy) during HardDeleteUser, violating ADR-014's
		// "never hard delete; soft-delete-and-anonymize" principle.
		// Ref: Section 19 + ADR-014.

		// Anonymize dm_messages authored by the user (clear body, reassign sender).
		if err := q.AnonymizeDMMessagesBySender(ctx, gen.AnonymizeDMMessagesBySenderParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing dm messages: %w", err)
		}

		// Reassign dm_conversations participant references to placeholder.
		if err := q.ReassignDMConversationParticipant1(ctx, gen.ReassignDMConversationParticipant1Params{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning dm conversation participant 1: %w", err)
		}

		if err := q.ReassignDMConversationParticipant2(ctx, gen.ReassignDMConversationParticipant2Params{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning dm conversation participant 2: %w", err)
		}

		// Anonymize dm_requests (clear preview, reassign requester/recipient).
		if err := q.AnonymizeDMRequestsByUser(ctx, gen.AnonymizeDMRequestsByUserParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing dm requests: %w", err)
		}

		// Delete user_dm_settings (user-owned config, no PII in the row).
		if err := q.DeleteUserDMSettingsByUser(ctx, gen.DeleteUserDMSettingsByUserParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting user dm settings: %w", err)
		}

		// Delete user_blocks (user-owned config).
		if err := q.DeleteUserBlocksByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting user blocks: %w", err)
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

		// ownership_claims.reviewed_by is nullable; clear reviewer reference when an admin is erased.
		if err := q.NullifyOwnershipClaimReviewer(ctx, gen.NullifyOwnershipClaimReviewerParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("nullifying ownership claim reviewer: %w", err)
		}

		// organization_members.user_id is NOT NULL; remove the erased user's memberships
		// to prevent re-identification via the member roster.
		if err := q.DeleteOrganizationMembersByUser(ctx, userID); err != nil {
			return fmt.Errorf("deleting organization memberships: %w", err)
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

		// ai_personas.created_by is NOT NULL; reassign to placeholder so persona
		// config rows survive when the admin who created them is erased (BUG-016,
		// ADR-068 compliance).
		if err := q.ReassignAIPersonaCreatedBy(ctx, gen.ReassignAIPersonaCreatedByParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("reassigning ai_personas created_by: %w", err)
		}

		// audit_logs must be preserved for compliance; repoint actor/impersonated refs to the placeholder "Deleted User".
		if err := q.AnonymizeAuditLogActor(ctx, gen.AnonymizeAuditLogActorParams{
			UserID: userID, TenantID: tenantID, TargetUserID: deletedUserPlaceholder,
		}); err != nil {
			return fmt.Errorf("anonymizing audit log actor: %w", err)
		}

		// Japa University GDPR cascade (ADR-063 / audit C2):
		//   - Clear sensitive payout_method jsonb on the user's instructor
		//     profile(s) before the profile is cascade-deleted.
		//   - Anonymize revenue_splits and instructor_payouts: sever the
		//     instructor_id reference (SET NULL) so the financial audit
		//     records survive for regulatory retention. The FKs were changed
		//     from ON DELETE CASCADE to ON DELETE SET NULL in migration
		//     000181 to enforce this at the schema level.
		if _, err := q.ClearInstructorPayoutMethod(ctx, gen.ClearInstructorPayoutMethodParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("clearing instructor payout method: %w", err)
		}
		if _, err := q.AnonymizeRevenueSplitsForInstructor(ctx, gen.AnonymizeRevenueSplitsForInstructorParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("anonymizing revenue splits: %w", err)
		}
		if _, err := q.AnonymizeInstructorPayoutsForInstructor(ctx, gen.AnonymizeInstructorPayoutsForInstructorParams{
			UserID: userID, TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("anonymizing instructor payouts: %w", err)
		}

		// C7: follows where the deleted user is the TARGET (entity_id with
		// entity_type='user') have no FK (polymorphic entity_id) and are not
		// CASCADE-deleted. Remove them explicitly so other users' "following"
		// lists don't contain dead entries pointing to the deleted user.
		// Follows where the deleted user is the FOLLOWER are CASCADE-deleted
		// by the follower_id FK and need no explicit cleanup here.
		// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
		if err := q.DeleteFollowsByEntity(ctx, gen.DeleteFollowsByEntityParams{
			EntityType: "user",
			EntityID:   userID,
		}); err != nil {
			return fmt.Errorf("deleting follows targeting deleted user: %w", err)
		}

		// BUG-010: search_analytics.user_id references users(id) but has no
		// ON DELETE clause. Nullify the user_id on the user's search history
		// rows so the search analytics (zero-result detection, trending
		// queries) survive for platform insights while severing the PII link.
		// Ref: Section 53; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE search_analytics SET user_id = NULL
			WHERE user_id = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying search_analytics user_id: %w", err)
		}

		// Nullify feature_flags.updated_by references. The feature_flags table
		// has updated_by uuid REFERENCES users(id) with no ON DELETE clause
		// (defaults to NO ACTION). Without this nullification, the final
		// HardDeleteUser DELETE fails with a FK violation for any admin who
		// has toggled a feature flag. Ref: ADR-057; Section 42.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE feature_flags SET updated_by = NULL
			WHERE updated_by = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying feature_flags updated_by: %w", err)
		}

		// Nullify doc_pages.last_reviewed_by references. The doc_pages table
		// has last_reviewed_by uuid REFERENCES users(id) with no ON DELETE clause.
		// Without this, the HardDeleteUser DELETE fails with a FK violation for
		// any admin who has approved/published doc pages. The doc pages themselves
		// are retained (they are tenant content, not user PII). Ref: §44; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE doc_pages SET last_reviewed_by = NULL
			WHERE last_reviewed_by = $1
		`, userID); err != nil {
			return fmt.Errorf("nullifying doc_pages last_reviewed_by: %w", err)
		}

		// BUG-011: trending_snapshots with entity_type='user' reference the
		// user via entity_id (no FK — polymorphic). Delete them so the user's
		// trending presence does not survive erasure.
		// Ref: Section 53; ADR-014.
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM trending_snapshots
			WHERE entity_type = 'user' AND entity_id = $1
		`, userID); err != nil {
			return fmt.Errorf("deleting trending_snapshots for user: %w", err)
		}

		// BUG-018: webhook_endpoints.created_by references users(id) with ON
		// DELETE SET NULL (migration 000361), so the FK is handled automatically.
		// However, webhook_deliveries.payload may contain user PII (e.g. event
		// payloads referencing the user). Delete deliveries whose payload
		// contains the user's UUID so PII does not survive erasure. The
		// webhook_endpoints themselves are tenant config and survive with
		// created_by nullified. Ref: §12; §15 GDPR erasure.
		if _, err := s.db.Writer.Exec(ctx, `
			DELETE FROM webhook_deliveries
			WHERE tenant_id = $2
			  AND payload::text LIKE '%' || $1::text || '%'
		`, userID, tenantID); err != nil {
			return fmt.Errorf("deleting webhook_deliveries with user PII: %w", err)
		}

		// 5. Physical deletion — ON DELETE CASCADE handles all child tables.
		// The following feed-related tables are cascade-purged here (no
		// explicit step needed, documented for GDPR traceability — BUG-014):
		//   - feeds (ON DELETE CASCADE from feeds.user_id → users(id) and
		//     feeds.content_id → contents(id); the user's feed rows and any
		//     feed rows pointing to their reassigned content are removed)
		//   - content_scores (ON DELETE CASCADE from
		//     content_scores.content_id → contents(id); scores for the
		//     user's content are removed when the content is cascade-deleted
		//     or retained when content is reassigned to the placeholder)
		// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
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
func (s *Service) ListPendingDeletionRequests(ctx context.Context, limit int32) ([]gen.ListPendingDeletionRequestsRow, error) {
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
	UserProfile          map[string]interface{}   `json:"user_profile"`
	Consents             []map[string]interface{} `json:"consents"`
	Content              []map[string]interface{} `json:"content"`
	Comments             []map[string]interface{} `json:"comments"`
	Spaces               []map[string]interface{} `json:"spaces"`
	Payments             []map[string]interface{} `json:"payments"`
	Notifications        []map[string]interface{} `json:"notifications"`
	UserProfiles         []map[string]interface{} `json:"user_profiles"`
	Reactions            []map[string]interface{} `json:"reactions"`
	Bookmarks            []map[string]interface{} `json:"bookmarks"`
	BookmarkFolders      []map[string]interface{} `json:"bookmark_folders"` // BL-005
	ContentShares        []map[string]interface{} `json:"content_shares"`
	ContentViews         []map[string]interface{} `json:"content_views"`
	Follows              []map[string]interface{} `json:"follows"`
	ForumThreads         []map[string]interface{} `json:"forum_threads"`
	ForumReplies         []map[string]interface{} `json:"forum_replies"`
	ForumVotes           []map[string]interface{} `json:"forum_votes"`
	ForumBookmarks       []map[string]interface{} `json:"forum_bookmarks"`
	ForumPollVotes       []map[string]interface{} `json:"forum_poll_votes"`
	ForumThreadTags      []map[string]interface{} `json:"forum_thread_tags"`
	ReplyViews           []map[string]interface{} `json:"reply_views"`
	Media                []map[string]interface{} `json:"media"`
	Reviews              []map[string]interface{} `json:"reviews"`
	SpaceMembers         []map[string]interface{} `json:"space_members"`
	SpaceModerators      []map[string]interface{} `json:"space_moderators"`
	TagModerators        []map[string]interface{} `json:"tag_moderators"`
	ConsentRecords       []map[string]interface{} `json:"consent_records"`
	WalletLedger         []map[string]interface{} `json:"wallet_ledger"`
	ActivityEvents       []map[string]interface{} `json:"activity_events"`
	LessonProgress       []map[string]interface{} `json:"lesson_progress"`
	LearningCertificates []map[string]interface{} `json:"learning_certificates"`
	MentorshipMatches    []map[string]interface{} `json:"mentorship_matches"`
	MentorshipConnections []map[string]interface{} `json:"mentorship_connections"` // §102 ADR-110
	MentorProfiles       []map[string]interface{} `json:"mentor_profiles"`        // §102 ADR-110
	MentorSessions       []map[string]interface{} `json:"mentor_sessions"`        // §102 ADR-110
	MentorReviews        []map[string]interface{} `json:"mentor_reviews"`         // §102 ADR-110
	ProfessionalProfiles []map[string]interface{} `json:"professional_profiles"`
	ProfessionalRelocations []map[string]interface{} `json:"professional_relocations"` // §33 child table
	ProfessionalDocuments   []map[string]interface{} `json:"professional_documents"`   // §33 child table
	ProfessionalVerifiers   []map[string]interface{} `json:"professional_verifiers"`   // §33 child table
	Organizations        []map[string]interface{} `json:"organizations"`
	ApiKeys              []map[string]interface{} `json:"api_keys"`
	UserPreferences      []map[string]interface{} `json:"user_preferences"`
	UserMutes            []map[string]interface{} `json:"user_mutes"`
	SpaceMutes           []map[string]interface{} `json:"space_mutes"`
	TicketComments       []map[string]interface{} `json:"ticket_comments"`
	InstructorProfiles   []map[string]interface{} `json:"instructor_profiles"`
	RevenueSplits        []map[string]interface{} `json:"revenue_splits"`
	InstructorPayouts    []map[string]interface{} `json:"instructor_payouts"`
	CustomFieldValues    []map[string]interface{} `json:"custom_field_values"` // BUG-020
	PushTokens           []map[string]interface{} `json:"push_tokens"`         // BUG-014
	DMConversations      []map[string]interface{} `json:"dm_conversations"`    // BUG-022
	DMMessages           []map[string]interface{} `json:"dm_messages"`         // BUG-022
	DMRequests           []map[string]interface{} `json:"dm_requests"`         // BUG-022
	DMSettings           []map[string]interface{} `json:"dm_settings"`         // BUG-022
	UserBlocks           []map[string]interface{} `json:"user_blocks"`         // BUG-022
	ExportMetadata       map[string]interface{}   `json:"export_metadata"`
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

	// Bookmark folders the user created (BL-005).
	if rows, err := s.store.gdprExportUserBookmarkFolders(ctx, userID, tenantID); err == nil {
		export.BookmarkFolders = rows
	} else {
		slog.Warn("failed to collect bookmark_folders for export", "user_id", userID.String(), "error", err)
	}

	// Content shares the user created.
	if rows, err := s.store.gdprExportUserContentShares(ctx, userID, tenantID); err == nil {
		export.ContentShares = rows
	} else {
		slog.Warn("failed to collect content_shares for export", "user_id", userID.String(), "error", err)
	}

	// Content views the user recorded (Section 14 — user_content_views PII).
	if rows, err := s.store.gdprExportUserContentViews(ctx, userID); err == nil {
		export.ContentViews = rows
	} else {
		slog.Warn("failed to collect content_views for export", "user_id", userID.String(), "error", err)
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

	// BUG-022: forum_poll_votes, forum_thread_tags, and reply_views were
	// missing from the original GDPR export. These are user-owned rows that
	// must be included in a data export. Ref: Section 08 + GDPR erasure model.
	if rows, err := s.store.gdprExportUserForumPollVotes(ctx, userID); err == nil {
		export.ForumPollVotes = rows
	} else {
		slog.Warn("failed to collect forum_poll_votes for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserForumThreadTags(ctx, userID, tenantID); err == nil {
		export.ForumThreadTags = rows
	} else {
		slog.Warn("failed to collect forum_thread_tags for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserReplyViews(ctx, userID); err == nil {
		export.ReplyViews = rows
	} else {
		slog.Warn("failed to collect reply_views for export", "user_id", userID.String(), "error", err)
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

	// Space moderator assignments (space_moderators table).
	if rows, err := s.store.gdprExportUserSpaceModerators(ctx, userID, tenantID); err == nil {
		export.SpaceModerators = rows
	} else {
		slog.Warn("failed to collect space_moderators for export", "user_id", userID.String(), "error", err)
	}

	// Tag moderator assignments (tag_moderators table — Section 08/49).
	if rows, err := s.store.gdprExportUserTagModerators(ctx, userID, tenantID); err == nil {
		export.TagModerators = rows
	} else {
		slog.Warn("failed to collect tag_moderators for export", "user_id", userID.String(), "error", err)
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

	// Activity events for the user (BUG-008 — previously missing from export).
	if rows, err := s.store.gdprExportUserActivityEvents(ctx, userID, tenantID); err == nil {
		export.ActivityEvents = rows
	} else {
		slog.Warn("failed to collect activity_events for export", "user_id", userID.String(), "error", err)
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

	// §102 Mentorship connections where the user is mentor or mentee (ADR-110).
	if rows, err := s.store.gdprExportUserMentorshipConnections(ctx, userID, tenantID); err == nil {
		export.MentorshipConnections = rows
	} else {
		slog.Warn("failed to collect mentorship_connections for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor profiles owned by the user (ADR-110).
	if rows, err := s.store.gdprExportUserMentorProfiles(ctx, userID, tenantID); err == nil {
		export.MentorProfiles = rows
	} else {
		slog.Warn("failed to collect mentor_profiles for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor sessions where the user is the mentee (ADR-110).
	if rows, err := s.store.gdprExportUserMentorSessions(ctx, userID, tenantID); err == nil {
		export.MentorSessions = rows
	} else {
		slog.Warn("failed to collect mentor_sessions for export", "user_id", userID.String(), "error", err)
	}

	// §102 Mentor reviews written by the user (ADR-110).
	if rows, err := s.store.gdprExportUserMentorReviews(ctx, userID); err == nil {
		export.MentorReviews = rows
	} else {
		slog.Warn("failed to collect mentor_reviews for export", "user_id", userID.String(), "error", err)
	}

	// Professional profiles owned by the user.
	if rows, err := s.store.gdprExportUserProfessionalProfiles(ctx, userID, tenantID); err == nil {
		export.ProfessionalProfiles = rows
	} else {
		slog.Warn("failed to collect professional_profiles for export", "user_id", userID.String(), "error", err)
	}

	// Professional profile child tables (relocations, documents, verifiers).
	if rows, err := s.store.gdprExportUserProfessionalRelocations(ctx, userID, tenantID); err == nil {
		export.ProfessionalRelocations = rows
	} else {
		slog.Warn("failed to collect professional_relocations for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserProfessionalDocuments(ctx, userID, tenantID); err == nil {
		export.ProfessionalDocuments = rows
	} else {
		slog.Warn("failed to collect professional_documents for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserProfessionalVerifiers(ctx, userID, tenantID); err == nil {
		export.ProfessionalVerifiers = rows
	} else {
		slog.Warn("failed to collect professional_verifiers for export", "user_id", userID.String(), "error", err)
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

	// Japa University creator-economy data (ADR-063 / audit C2).
	if rows, err := s.store.gdprExportInstructorProfiles(ctx, userID, tenantID); err == nil {
		export.InstructorProfiles = rows
	} else {
		slog.Warn("failed to collect instructor_profiles for export", "user_id", userID.String(), "error", err)
	}
	if rows, err := s.store.gdprExportRevenueSplits(ctx, userID, tenantID); err == nil {
		export.RevenueSplits = rows
	} else {
		slog.Warn("failed to collect revenue_splits for export", "user_id", userID.String(), "error", err)
	}
	if rows, err := s.store.gdprExportInstructorPayouts(ctx, userID, tenantID); err == nil {
		export.InstructorPayouts = rows
	} else {
		slog.Warn("failed to collect instructor_payouts for export", "user_id", userID.String(), "error", err)
	}

	// BUG-020: export custom_field_values for the user's profile entity so
	// the GDPR data export includes PII stored in custom fields.
	if rows, err := s.store.gdprExportCustomFieldValues(ctx, userID); err == nil {
		export.CustomFieldValues = rows
	} else {
		slog.Warn("failed to collect custom_field_values for export", "user_id", userID.String(), "error", err)
	}

	// BUG-014: export push_tokens (device push notification tokens) so the
	// GDPR data export includes PII-adjacent device identifiers.
	// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
	if rows, err := s.store.gdprExportPushTokens(ctx, userID, tenantID); err == nil {
		export.PushTokens = rows
	} else {
		slog.Warn("failed to collect push_tokens for export", "user_id", userID.String(), "error", err)
	}

	// BUG-022: export DM tables so the GDPR data export includes DM
	// conversations, messages, requests, privacy settings, and block
	// relationships. Ref: Section 19 + ADR-014.
	if rows, err := s.store.gdprExportUserDMConversations(ctx, userID, tenantID); err == nil {
		export.DMConversations = rows
	} else {
		slog.Warn("failed to collect dm_conversations for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMMessages(ctx, userID, tenantID); err == nil {
		export.DMMessages = rows
	} else {
		slog.Warn("failed to collect dm_messages for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMRequests(ctx, userID, tenantID); err == nil {
		export.DMRequests = rows
	} else {
		slog.Warn("failed to collect dm_requests for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserDMSettings(ctx, userID, tenantID); err == nil {
		export.DMSettings = rows
	} else {
		slog.Warn("failed to collect user_dm_settings for export", "user_id", userID.String(), "error", err)
	}

	if rows, err := s.store.gdprExportUserBlocks(ctx, userID); err == nil {
		export.UserBlocks = rows
	} else {
		slog.Warn("failed to collect user_blocks for export", "user_id", userID.String(), "error", err)
	}

	return export, nil
}
