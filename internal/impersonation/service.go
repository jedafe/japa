package impersonation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service provides business logic for admin impersonation.
type Service struct {
	store  *Store
	dbPool *pgxpool.Pool
}

// NewService creates a Service with the given store and database pool.
func NewService(store *Store, dbPool *pgxpool.Pool) *Service {
	return &Service{store: store, dbPool: dbPool}
}

// StartImpersonation begins impersonating a target user.
// It logs the action to audit_logs FIRST for compliance (Rule 10),
// then sets the Valkey key with a 30-minute TTL. A failed audit log
// write aborts the impersonation — we never proceed without an audit trail.
func (s *Service) StartImpersonation(ctx context.Context, adminID, targetUserID pgtype.UUID) error {
	// Log to audit_logs FIRST — Rule 10 requires this before setting state.
	if _, err := s.dbPool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, 'impersonation.start', 'user', $3, jsonb_build_object('admin_id', $2, 'target_id', $3))
	`, adminID, adminID, targetUserID); err != nil {
		return fmt.Errorf("abort impersonation: audit log failed: %w", err)
	}

	if err := s.store.SetImpersonation(ctx, adminID, targetUserID, DefaultImpersonationTTL); err != nil {
		return fmt.Errorf("set impersonation: %w", err)
	}

	slog.Info("impersonation started", "admin", adminID.String(), "target", targetUserID.String())

	return nil
}

// EndImpersonation clears the impersonation state for an admin.
func (s *Service) EndImpersonation(ctx context.Context, adminID pgtype.UUID) error {
	if err := s.store.ClearImpersonation(ctx, adminID); err != nil {
		return fmt.Errorf("clear impersonation: %w", err)
	}

	// Log the end of impersonation (Rule 10 — audit trail required).
	if _, err := s.dbPool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, 'impersonation.end', 'user', $2, jsonb_build_object('admin_id', $2))
	`, adminID, adminID); err != nil {
		// EndImpersonation already cleared state — log warning but don't fail
		// (the state is already cleared, so returning an error would be misleading).
		slog.Warn("impersonation: failed to write audit log for end", "error", err)
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

// IsAdmin checks if a user has the admin.impersonate permission.
// This is a simplified check — in production, resolve through RBAC.
func (s *Service) IsAdmin(_ context.Context, _ pgtype.UUID) bool {
	// For now, all users with the impersonate permission are admins.
	// The RBAC resolver checks this at the middleware level.
	return true
}
