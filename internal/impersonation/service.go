package impersonation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTargetIsAdmin is returned by StartImpersonation when the target user
// holds any admin-level permission, violating the no-admin-on-admin safety
// rule from Section 46. Handlers should map it to HTTP 403.
var ErrTargetIsAdmin = errors.New("cannot impersonate admin-level users")

// ErrNoActiveSession is returned by EndImpersonation when no impersonation
// session is active for the admin. Handlers should map it to HTTP 404.
var ErrNoActiveSession = errors.New("no active impersonation session")

// AdminCapabilityChecker is the RBAC resolver interface used to detect
// admin-level users. Implemented by *rbac.RBAC.
type AdminCapabilityChecker interface {
	HasAdminCapability(ctx context.Context, userID, tenantID pgtype.UUID) bool
}

// Service provides business logic for admin impersonation.
type Service struct {
	store  *Store
	dbPool *pgxpool.Pool
	rbac   AdminCapabilityChecker // nil disables the admin-target check (test-only)
}

// NewService creates a Service with the given store and database pool.
// The RBAC resolver may be wired via SetRBAC after construction.
func NewService(store *Store, dbPool *pgxpool.Pool) *Service {
	return &Service{store: store, dbPool: dbPool}
}

// SetRBAC wires the RBAC resolver used for the no-admin-on-admin safety
// check (Section 46). Required in production; tests may omit it.
func (s *Service) SetRBAC(rbac AdminCapabilityChecker) { s.rbac = rbac }

// StartImpersonation begins impersonating a target user.
// It enforces the no-admin-on-admin safety rule (Section 46), terminates
// and logs any existing session (Section 46 "Valkey Session Key"), logs
// the action to audit_logs FIRST for compliance (Rule 10), then sets the
// Valkey key with a 30-minute TTL. A failed audit log write aborts the
// impersonation — we never proceed without an audit trail.
func (s *Service) StartImpersonation(ctx context.Context, tenantID, adminID, targetUserID pgtype.UUID, ipAddress string) error {
	// Safety: refuse to impersonate any user with admin-level permissions.
	// Section 46: "A user with admin.impersonate cannot impersonate another
	// user who holds any admin-level role."
	if s.rbac != nil && s.rbac.HasAdminCapability(ctx, targetUserID, tenantID) {
		slog.Warn("impersonation refused: target is admin-level",
			"admin_id", adminID.String(), "target_id", targetUserID.String())
		return ErrTargetIsAdmin
	}

	// Terminate any existing session (Section 46: "Starting a new session
	// while one is active silently terminates the previous session and logs
	// the termination."). The audit entry is written before overwriting.
	existing, _ := s.store.GetImpersonation(ctx, adminID)
	if existing != "" {
		if err := s.auditLog(ctx, tenantID, adminID, &existing, "impersonate.terminate_previous", ipAddress, map[string]string{
			"admin_id":        adminID.String(),
			"previous_target": existing,
			"new_target":      targetUserID.String(),
		}); err != nil {
			return fmt.Errorf("abort impersonation: terminate_previous audit log failed: %w", err)
		}
	}

	// Log to audit_logs FIRST — Rule 10 requires this before setting state.
	targetIDStr := targetUserID.String()
	if err := s.auditLog(ctx, tenantID, adminID, &targetIDStr, "impersonate.start", ipAddress, map[string]string{
		"admin_id":    adminID.String(),
		"target_id":   targetUserID.String(),
		"ttl_seconds": fmt.Sprintf("%d", int64(DefaultImpersonationTTL.Seconds())),
	}); err != nil {
		slog.Error("impersonation: audit log INSERT failed", "error", err, "tenant_id", tenantID.String(), "admin_id", adminID.String(), "target_id", targetUserID.String())
		return fmt.Errorf("abort impersonation: audit log failed: %w", err)
	}

	if err := s.store.SetImpersonation(ctx, adminID, targetUserID, DefaultImpersonationTTL); err != nil {
		slog.Error("impersonation: Valkey SET failed", "error", err, "admin_id", adminID.String(), "target_id", targetUserID.String())
		return fmt.Errorf("set impersonation: %w", err)
	}

	slog.Info("impersonation started", "admin", adminID.String(), "target", targetUserID.String())

	return nil
}

// EndImpersonation atomically retrieves and clears the impersonation state
// for an admin, then logs the end action with the correct impersonated_user_id
// (Section 46, Rule 10). The target_id is captured via GetDel before the
// audit log is written so the end event is always linked to the right user.
func (s *Service) EndImpersonation(ctx context.Context, tenantID, adminID pgtype.UUID, ipAddress string) (string, error) {
	// Atomically get-and-delete so we capture the target_id for the audit log.
	targetID, err := s.store.GetDelImpersonation(ctx, adminID)
	if err != nil {
		return "", fmt.Errorf("getdel impersonation: %w", err)
	}

	if targetID == "" {
		return "", ErrNoActiveSession
	}

	// Log the end of impersonation with the correct impersonated_user_id
	// (Rule 10 — audit trail required, actor=admin, impersonated=target).
	if err := s.auditLog(ctx, tenantID, adminID, &targetID, "impersonate.end", ipAddress, map[string]string{
		"admin_id":  adminID.String(),
		"target_id": targetID,
	}); err != nil {
		// Rule 10: audit log failure is a compliance issue — return error so handler returns HTTP 500.
		return "", fmt.Errorf("audit log for impersonation end: %w", err)
	}

	slog.Info("impersonation ended", "admin", adminID.String(), "target", targetID)

	return targetID, nil
}

// GetImpersonationStatus returns the impersonated user ID, whether an
// impersonation is active, and the remaining TTL in seconds (for computing
// expires_at per Section 46 status response contract).
func (s *Service) GetImpersonationStatus(ctx context.Context, adminID pgtype.UUID) (string, bool, int64, error) {
	targetID, err := s.store.GetImpersonation(ctx, adminID)
	if err != nil {
		return "", false, 0, fmt.Errorf("get impersonation status: %w", err)
	}

	if targetID == "" {
		return "", false, 0, nil
	}

	ttlSeconds, _ := s.store.TTLImpersonation(ctx, adminID)

	return targetID, true, ttlSeconds, nil
}

// IsAdmin reports whether the given user holds any admin-level capability.
// Resolves through the RBAC resolver when wired; returns false when no
// resolver is configured (defensive — production wiring is required).
//
// Ref: japaconnect-engineering/sections/46-impersonation.md
func (s *Service) IsAdmin(ctx context.Context, userID, tenantID pgtype.UUID) bool {
	if s.rbac == nil {
		return false
	}
	return s.rbac.HasAdminCapability(ctx, userID, tenantID)
}

// auditLog inserts a row into audit_logs for an impersonation action.
// impersonatedUserID may be nil for actions that don't have a target.
// metadata is serialized to JSON. ipAddress is the client IP from
// clientip.FromRequest (Section 46 handler pattern includes IPAddress).
func (s *Service) auditLog(ctx context.Context, tenantID, adminID pgtype.UUID, impersonatedUserID *string, actionType, ipAddress string, metadata map[string]string) error {
	metaJSON := "{}"
	if len(metadata) > 0 {
		parts := make([]string, 0, len(metadata))
		for k, v := range metadata {
			parts = append(parts, fmt.Sprintf(`"%s":"%s"`, k, v))
		}
		metaJSON = "{" + strings.Join(parts, ",") + "}"
	}

	var impersonatedArg any
	if impersonatedUserID != nil {
		impersonatedArg = *impersonatedUserID
	}

	_, err := s.dbPool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, impersonated_user_id, action_type, target_type, target_id, metadata, ip_address)
		VALUES ($1, $2, $3, $4, 'user', $3, $5::jsonb, $6::inet)
	`, tenantID, adminID, impersonatedArg, actionType, metaJSON, ipAddress)
	if err != nil {
		return fmt.Errorf("audit log insert: %w", err)
	}

	return nil
}

// TargetProfile is the minimal public profile returned by
// StartImpersonation per the Section 46 response contract.
type TargetProfile struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// GetTargetProfile fetches the id, username, and email of the target user
// for the StartImpersonation response (Section 46: target: {id, username, email}).
func (s *Service) GetTargetProfile(ctx context.Context, tenantID, targetUserID pgtype.UUID) (*TargetProfile, error) {
	var tp TargetProfile
	err := s.dbPool.QueryRow(ctx, `
		SELECT id::text, username, email FROM users WHERE id = $1 AND tenant_id = $2
	`, targetUserID, tenantID).Scan(&tp.ID, &tp.Username, &tp.Email)
	if err != nil {
		return nil, fmt.Errorf("get target profile: %w", err)
	}
	return &tp, nil
}
