// Package dedup checks (source_id, external_key) uniqueness and flags
// conflicts when the same external_key appears from different sources.
//
// The dedup stage sits between extraction and promotion in the intelligence
// spine. For every freshly extracted intel record it answers one question:
// has this upstream entity already been seen? The answer drives the write
// path — insert (new), update (same source re-extraction), or hold for
// human review (cross-source conflict).
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package dedup

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// RecordLister is the subset of stores.Store that the dedup service needs.
// Declared as an interface so the service can be unit-tested with a mock
// instead of a live *db.DB connection.
type RecordLister interface {
	// GetIntelRecordByExternalKey fetches a single intel record by the
	// (source_id, external_key) natural key. It returns pgx.ErrNoRows
	// (wrapped) when no record exists for that key pair.
	GetIntelRecordByExternalKey(ctx context.Context, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error)
	// ListIntelRecordsByExternalKey fetches all intel records matching the
	// given external_key across ALL sources. Used by CheckAcrossSources to
	// detect cross-source conflicts (BUG-20 fix).
	ListIntelRecordsByExternalKey(ctx context.Context, externalKey string) ([]gen.IntelRecord, error)
}

// Service wraps a RecordLister and exposes the dedup decision logic. It is
// safe for concurrent use because it holds no mutable state.
type Service struct {
	store RecordLister
}

// NewService creates a dedup service backed by the given record lister. Any
// *stores.Store satisfies RecordLister, so callers can pass the shared
// intelligence store directly. The tenantID scopes all queries per ADR-112
// Amendment 2.
func NewService(store RecordLister) *Service {
	return &Service{store: store}
}

// CheckResult describes the dedup outcome for a candidate intel record.
type CheckResult int

const (
	// CheckResultNew means no existing record matches (source_id,
	// external_key); the caller should INSERT a new intel record.
	CheckResultNew CheckResult = iota
	// CheckResultUpdate means an existing record matches the same
	// source_id and external_key; the caller should UPDATE that row.
	CheckResultUpdate
	// CheckResultConflict means a different source_id already claims the
	// same external_key; the caller should hold the record for human
	// review rather than silently overwriting it.
	CheckResultConflict
)

// String returns a human-readable label for a CheckResult, useful in logs
// and review-log entries.
func (r CheckResult) String() string {
	switch r {
	case CheckResultNew:
		return "new"
	case CheckResultUpdate:
		return "update"
	case CheckResultConflict:
		return "conflict"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// Check looks up an existing intel record by (source_id, external_key) and
// returns the dedup decision:
//
//   - CheckResultNew when no record exists for that key pair.
//   - CheckResultUpdate when the same source already has a record for the
//     external_key (a re-extraction of the same upstream entity).
//
// Cross-source conflict detection (same external_key, different source_id)
// is handled by CheckAcrossSources. Any error other than "no rows" is
// returned to the caller unchanged.
func (s *Service) Check(ctx context.Context, sourceID pgtype.UUID, externalKey string) (CheckResult, error) {
	_, err := s.store.GetIntelRecordByExternalKey(ctx, gen.GetIntelRecordByExternalKeyParams{
		SourceID:    sourceID,
		ExternalKey: externalKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckResultNew, nil
		}

		return 0, fmt.Errorf("dedup: check by external key: %w", err)
	}
	// A row exists for this (source_id, external_key) pair — it is a
	// re-extraction of an entity this source already tracks.
	return CheckResultUpdate, nil
}

// CheckAcrossSources reports whether a different source_id already claims
// the given external_key, which indicates a cross-source conflict that
// should be held for human review rather than silently creating a duplicate
// projection row (BUG-20 fix).
//
// The query returns all intel records matching external_key regardless of
// source. If any record has a different source_id than the candidate, it is
// a cross-source conflict.
func (s *Service) CheckAcrossSources(ctx context.Context, sourceID pgtype.UUID, externalKey string) (CheckResult, error) {
	records, err := s.store.ListIntelRecordsByExternalKey(ctx, externalKey)
	if err != nil {
		return 0, fmt.Errorf("dedup: cross-source check: %w", err)
	}

	for _, rec := range records {
		// A record from a DIFFERENT source with the same external_key is a
		// cross-source conflict. Same-source records are handled by Check.
		if rec.SourceID != sourceID {
			return CheckResultConflict, nil
		}
	}

	return CheckResultNew, nil
}
