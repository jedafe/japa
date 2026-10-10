// Package academy — processoradapter.go implements the worker.AcademyProcessor
// interface by delegating to the learning store for track completion recalculation
// and certificate issuance.
package academy

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ProcessorAdapter implements worker.AcademyProcessor by delegating to the
// learning store. It is registered with the worker at startup so background
// tasks (academy:update_track_completion, academy:issue_certificate) invoke
// real service logic instead of the noop stub.
type ProcessorAdapter struct {
	store *Store
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given store.
func NewProcessorAdapter(store *Store) *ProcessorAdapter {
	return &ProcessorAdapter{store: store}
}

// UpdateTrackCompletion recalculates a user's progress in a learning track
// by counting completed lessons vs total published lessons, then persists
// the percentage to the enrollment record.
func (a *ProcessorAdapter) UpdateTrackCompletion(ctx context.Context, userID, trackID string) error {
	var uid, tkid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return fmt.Errorf("invalid user_id %q: %w", userID, err)
	}

	if err := tkid.Scan(trackID); err != nil {
		return fmt.Errorf("invalid track_id %q: %w", trackID, err)
	}

	completed, total, err := a.store.GetUserTrackProgress(ctx, uid, tkid)
	if err != nil {
		return fmt.Errorf("get track progress: %w", err)
	}

	if total == 0 {
		slog.Info("academy:track_completion — no lessons in track, skipping",
			"track_id", trackID)
		return nil
	}

	progressPct := calculatePercent(completed, total)

	if err := a.store.UpdateEnrollmentProgress(ctx, uid, tkid, progressPct); err != nil {
		// Enrollment may not exist yet — log but don't fail the task.
		slog.Warn("academy:track_completion — enrollment progress update skipped",
			"user_id", userID, "track_id", trackID, "error", err)
	}

	slog.Info("academy:track_completion — progress updated",
		"user_id", userID, "track_id", trackID,
		"completed", completed, "total", total, "pct", progressPct)

	return nil
}

// IssueCertificate generates and stores a certificate for a user who has
// completed all lessons in a track. Idempotent — returns early if a
// certificate already exists for this user/track pair.
func (a *ProcessorAdapter) IssueCertificate(ctx context.Context, userID, trackID, courseName string) error {
	var uid, tkid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return fmt.Errorf("invalid user_id %q: %w", userID, err)
	}

	if err := tkid.Scan(trackID); err != nil {
		return fmt.Errorf("invalid track_id %q: %w", trackID, err)
	}

	// Idempotent: skip if certificate already exists.
	existing, _ := a.store.GetCertificate(ctx, uid, tkid)
	if existing != nil {
		slog.Info("academy:issue_certificate — certificate already exists",
			"user_id", userID, "track_id", trackID,
			"certificate_number", existing.CertificateNumber)

		return nil
	}

	// Use hex-encoded UUID bytes (not raw) — raw bytes contain invalid UTF-8
	// sequences that PostgreSQL's text column rejects (SQLSTATE 22021).
	certNumber := fmt.Sprintf("JC-%d-%s", time.Now().Unix(), hex.EncodeToString(uid.Bytes[:8]))

	if _, err := a.store.IssueCertificate(ctx, uid, tkid, certNumber); err != nil {
		return fmt.Errorf("issue certificate: %w", err)
	}

	slog.Info("academy:issue_certificate — issued",
		"user_id", userID, "track_id", trackID,
		"course_name", courseName, "certificate_number", certNumber)

	return nil
}
