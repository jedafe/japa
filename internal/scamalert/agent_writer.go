// Package scamalert — agent_writer.go is the write surface for the
// scam_report.* agent tools (§103). It wraps Store so agent-driven creates
// and updates go through the same sanitization and geo validation as the
// public submit path, and stamp ADR-088 AI provenance on the row.
//
// The wrapped methods keep the exact Store signatures the tool-writer
// reflection layer expects — wiring stays structural.
package scamalert

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
)

// AgentWriter wraps *Store with agent-safe write methods (BUG-004/BUG-029):
// before this, scam_report.create/update bypassed sanitization, geo
// validation, and never stamped ai_generated.
type AgentWriter struct {
	*Store
}

// NewAgentWriter wraps the scam-alert store for the agent tool writer.
func NewAgentWriter(s *Store) *AgentWriter { return &AgentWriter{Store: s} }

// sanitizeAgentReport applies the plain-text field hygiene the service
// layer enforces on the public path.
func sanitizeAgentReport(r *ScamReport) {
	r.Description = content.SanitizeText(r.Description)
	if r.OriginCommunity != nil {
		t := content.SanitizeText(*r.OriginCommunity)
		r.OriginCommunity = &t
	}
	if r.AmountRange != nil {
		t := content.SanitizeText(*r.AmountRange)
		r.AmountRange = &t
	}
	if r.SubjectLabel != nil {
		t := content.SanitizeText(*r.SubjectLabel)
		r.SubjectLabel = &t
	}
}

// stampAgentProvenance marks the row AI-generated per ADR-088 — agent-tool
// writes are AI-authored by definition.
func stampAgentProvenance(r *ScamReport) {
	r.AiGenerated = true
	model := "agent-tool"
	r.AiModel = &model
	r.AiGeneratedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

// CreateReport validates geo + sanitizes + stamps AI provenance, then
// delegates to the store insert (status forced to under_review there).
func (w *AgentWriter) CreateReport(ctx context.Context, report ScamReport) (pgtype.UUID, error) {
	sanitizeAgentReport(&report)
	if report.DestinationSpaceID.Valid {
		ok, err := w.Store.CountrySpaceExists(ctx, report.DestinationSpaceID)
		if err != nil {
			return pgtype.UUID{}, err
		}
		if !ok {
			return pgtype.UUID{}, ErrInvalidDestination
		}
	}
	if report.OriginSpaceID.Valid {
		ok, err := w.Store.CountrySpaceExists(ctx, report.OriginSpaceID)
		if err != nil {
			return pgtype.UUID{}, err
		}
		if !ok {
			return pgtype.UUID{}, ErrInvalidOrigin
		}
	}
	stampAgentProvenance(&report)
	return w.Store.CreateReport(ctx, report)
}

// UpdateReport sanitizes + stamps AI provenance, then delegates to the
// store update (content fields only — status/moderation untouched).
func (w *AgentWriter) UpdateReport(ctx context.Context, id pgtype.UUID, report ScamReport) (ScamReportRow, error) {
	sanitizeAgentReport(&report)
	if report.DestinationSpaceID.Valid {
		ok, err := w.Store.CountrySpaceExists(ctx, report.DestinationSpaceID)
		if err != nil {
			return ScamReportRow{}, err
		}
		if !ok {
			return ScamReportRow{}, ErrInvalidDestination
		}
	}
	stampAgentProvenance(&report)
	row, err := w.Store.UpdateReport(ctx, id, report)
	if err != nil {
		return ScamReportRow{}, err
	}
	// Persist the provenance stamp — UpdateReport only writes content
	// fields.
	_, uerr := w.pool.Writer.Exec(ctx,
		`UPDATE scam_reports SET ai_generated = true, ai_model = $2, ai_generated_at = now() WHERE id = $1`,
		id, "agent-tool")
	if uerr != nil {
		return row, uerr
	}
	row.AiGenerated = true
	m := "agent-tool"
	row.AiModel = &m
	return row, nil
}
