package impersonation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrTargetIsAdmin is returned by StartImpersonation when the target user
// holds any admin-level permission, violating the no-admin-on-admin safety
// rule from Section 46. Handlers should map it to HTTP 403.
var ErrTargetIsAdmin = errors.New("cannot impersonate admin-level users")

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
// It enforces the no-admin-on-admin safety rule (Section 46), logs the
// action to audit_logs FIRST for compliance (Rule 10), then sets the
// Valkey key with a 30-minute TTL. A failed audit log write aborts the
// impersonation — we never proceed without an audit trail.
func (s *Service) StartImpersonation(ctx context.Context, tenantID, adminID, targetUserID pgtype.UUID) error {
	// Safety: refuse to impersonate any user with admin-level permissions.
	// Section 46: "A user with admin.impersonate cannot impersonate another
	// user who holds any admin-level role."
	if s.rbac != nil && s.rbac.HasAdminCapability(ctx, targetUserID, tenantID) {
		slog.Warn("impersonation refused: target is admin-level",
			"admin_id", adminID.String(), "target_id", targetUserID.String())
		return ErrTargetIsAdmin
	}

	// Log to audit_logs FIRST — Rule 10 requires this before setting state.
	if _, err := s.dbPool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, impersonated_user_id, action_type, target_type, target_id, metadata)
		VALUES ($1, $2, $3, 'impersonation.start', 'user', $3, $4::jsonb)
	`, tenantID, adminID, targetUserID, fmt.Sprintf(`{"admin_id":"%s","target_id":"%s"}`, adminID.String(), targetUserID.String())); err != nil {
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

// EndImpersonation clears the impersonation state for an admin.
func (s *Service) EndImpersonation(ctx context.Context, tenantID, adminID pgtype.UUID) error {
	if err := s.store.ClearImpersonation(ctx, adminID); err != nil {
		return fmt.Errorf("clear impersonation: %w", err)
	}

	// Log the end of impersonation (Rule 10 — audit trail required).
	if _, err := s.dbPool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, action_type, target_type, target_id, metadata)
		VALUES ($1, $2, 'impersonation.end', 'user', $2, $3::jsonb)
	`, tenantID, adminID, fmt.Sprintf(`{"admin_id":"%s"}`, adminID.String())); err != nil {
		// Rule 10: audit log failure is a compliance issue — return error so handler returns HTTP 500.
		return fmt.Errorf("audit log for impersonation end: %w", err)
	}

	slog.Info("impersonation ended", "admin", adminID.String())

	return nil
}

// GetImpersonationStatus returns the impersonated user ID if active.
func (s *Service) GetImpersonationStatus(ctx context.Context, adminID pgtype.UUID) (string, bool, error) {
	targetID, err := s.store.GetImpersonation(ctx, adminID)
	if err != nil {
		return "", false, fmt.Errorf("get impersonation status: %w", err)
	}

	return targetID, targetID != "", nil
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
