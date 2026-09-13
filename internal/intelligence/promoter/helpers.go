// Package promoter — shared extraction field accessors and comparison
// helpers used by all domain promoters. Extracted from scholarship.go so
// each domain promoter (job, visa, housing, remittance, cost_of_living,
// and the 7 guide content types) can reuse them without duplication.
package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// PersonaResolver is the minimal interface a promoter store needs to resolve
// an AI persona's backing user_id. All promoter Store interfaces embed this
// so the createPath can resolve the source's persona → author_id.
type PersonaResolver interface {
	GetPersonaUserID(ctx context.Context, tenantID, personaID pgtype.UUID) (pgtype.UUID, error)
}

// resolveAuthorID resolves the author_id for a promoted content row. If the
// source has a persona_id, it looks up the persona's backing user_id and
// uses that as the author. If the persona lookup fails or the source has no
// persona, it falls back to the promoter's default authorID. This fixes the
// FK violation where defaultTenantID was used as contents.author_id.
func resolveAuthorID(ctx context.Context, resolver PersonaResolver, source gen.IntelSource, tenantID, fallbackAuthorID pgtype.UUID) pgtype.UUID {
	if !source.PersonaID.Valid {
		return fallbackAuthorID
	}

	userID, err := resolver.GetPersonaUserID(ctx, tenantID, source.PersonaID)
	if err != nil || !userID.Valid {
		slog.Warn("promoter: persona user_id lookup failed, using fallback author",
			"persona_id", source.PersonaID.String(),
			"error", err)

		return fallbackAuthorID
	}

	return userID
}

// parseExtractionShared decodes the raw extracted JSON blob into an
// extractor.Extraction map. Shared across all domain promoters.
// Handles both the current format ({field: {value, confidence, evidence}})
// and the legacy format ({field: "plain string value"}) by wrapping bare
// values into FieldExtraction{Value: v}.
func parseExtractionShared(raw []byte) (extractor.Extraction, error) {
	if len(raw) == 0 {
		return extractor.Extraction{}, nil
	}

	var ext extractor.Extraction
	if err := json.Unmarshal(raw, &ext); err == nil {
		return ext, nil
	}
	// Fallback: try legacy format where values are plain strings/scalars
	// instead of {value, confidence, evidence} objects.
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("parse extraction: %w", err)
	}

	ext = make(extractor.Extraction, len(legacy))

	for k, rawVal := range legacy {
		var fe extractor.FieldExtraction
		if err := json.Unmarshal(rawVal, &fe); err == nil && fe.Value != nil {
			ext[k] = fe
			continue
		}
		// Bare scalar — wrap it.
		var scalar any
		if err := json.Unmarshal(rawVal, &scalar); err != nil {
			continue
		}

		ext[k] = extractor.FieldExtraction{Value: scalar, Confidence: 0.5}
	}

	return ext, nil
}

// ---------------------------------------------------------------------------
// Comparison helpers (shared)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Editorial overrides + review log helpers (shared)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Misc helpers (shared)
// ---------------------------------------------------------------------------

// intelReader is the minimal read interface every domain promoter store
// exposes for the Promote skeleton: loading the intel record and its source.
// The concrete per-domain Store interfaces all embed these methods.
type intelReader interface {
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
}

// domainPromoteOps carries the per-domain closures and labels for the shared
// Promote skeleton. E is the domain projection row type.
type domainPromoteOps[E any] struct {
	entityType  string
	notErr      error
	loadLabel   string
	getExisting func(ctx context.Context, tenantID, pgID pgtype.UUID) (E, error)
	createPath  func(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error)
	updatePath  func(ctx context.Context, record gen.IntelRecord, existing E, ext extractor.Extraction) (PromoteResult, error)
}

// promoteDomain is the shared Promote skeleton for the domain promoters:
// load the intel record, verify the entity type, parse the extraction, load
// the source, then dispatch to the create or update path depending on whether
// a projection row already exists for this intel record.
func promoteDomain[E any](ctx context.Context, store intelReader, tenantID pgtype.UUID, recordID uuid.UUID, ops domainPromoteOps[E]) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := store.GetIntelRecord(ctx, tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}

	if record.EntityType != ops.entityType {
		return PromoteResult{}, ops.notErr
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	source, err := store.GetIntelSource(ctx, tenantID, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	// Does a projection row already exist for this intel record? A not-found
	// error means we take the create path; any other error propagates.
	existing, err := ops.getExisting(ctx, tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return ops.createPath(ctx, record, source, ext)
		}

		return PromoteResult{}, fmt.Errorf("promoter: load existing %s: %w", ops.loadLabel, err)
	}

	return ops.updatePath(ctx, record, existing, ext)
}

// ---------------------------------------------------------------------------
// Shared update-path transaction (shared)
// ---------------------------------------------------------------------------

// reviewLogWriter is the subset of every domain TxStore used by the shared
// update-path transaction: writing the auto_update review log.
type reviewLogWriter interface {
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// domainUpdateTx runs the shared update-path transaction for the domain
// promoters. The caller has already merged the extraction over the existing
// row (respecting editorial overrides), re-stamped freshness on params, and
// produced the changed-field list + JSON for the review log. This helper then
// applies the update atomically: a bare re-stamp when nothing changed, or
// update + auto_update review log in one transaction (BUG-02, §6.2).
// // P is the domain's UpdateXIntelParams type and T its TxStore; update is the
// domain's UpdateXIntel method expression and withTx wraps p.store.WithTx.
func domainUpdateTx[P any, T reviewLogWriter](
	ctx context.Context,
	record gen.IntelRecord,
	authorID, tenantID pgtype.UUID,
	label string,
	domainID, contentID uuid.UUID,
	params P,
	changed []string,
	changesJSON []byte,
	withTx func(ctx context.Context, fn func(T) error) error,
	update func(tx T, ctx context.Context, tenantID pgtype.UUID, p P) error,
) (PromoteResult, error) {
	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just re-stamp
		// the row at full freshness.
		if err := withTx(ctx, func(tx T) error { return update(tx, ctx, tenantID, params) }); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump %s freshness: %w", label, err)
		}

		return PromoteResult{DomainID: domainID, ContentID: contentID, Action: ActionVerified}, nil
	}

	note := noteAutoRecrawl
	reviewLog := gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		ActorUserID:   authorID,
		Action:        "auto_update",
		FieldChanges:  changesJSON,
		Note:          &note,
	}

	err := withTx(ctx, func(tx T) error {
		if err := update(tx, ctx, tenantID, params); err != nil {
			return fmt.Errorf("promoter: update %s: %w", label, err)
		}

		if err := tx.CreateIntelReviewLog(ctx, tenantID, reviewLog); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}

		return nil
	})
	if err != nil {
		return PromoteResult{}, err
	}

	return PromoteResult{
		DomainID:      domainID,
		ContentID:     contentID,
		Action:        ActionUpdated,
		ChangedFields: changed,
	}, nil
}
