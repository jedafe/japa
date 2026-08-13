// Package promoter maps intel_records to domain-specific tables.
//
// The promoter is the ONLY domain-specific step in the ingestion spine: it
// turns a validated, deduped intel_record into (or updates) a concrete
// projection row — for the scholarship domain, a scholarships row plus the
// contents guide entry that backs it.
//
// Its single most important invariant is editorial-override respect: on the
// update path, any field present in scholarships.editorial_overrides is
// skipped so a human edit always wins over a re-crawl. This is the regression
// guard that prevents the automation from silently reverting editorial work.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package promoter

import (
	"context"

	"github.com/google/uuid"
)

// Promoter maps an intel_record to a domain-specific row.
// Each domain (scholarship, …) implements its own create + update paths.
type Promoter interface {
	// Promote promotes the intel record identified by recordID, creating the
	// domain projection if it does not yet exist or updating it (respecting
	// editorial overrides) if it does.
	Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error)
	// Entity returns the entity_type this promoter handles ("scholarship").
	Entity() string
}

// PromoteResult is the outcome of a Promote call.
type PromoteResult struct {
	// DomainID is the id of the created/updated domain projection row
	// (e.g. scholarships.id).
	DomainID uuid.UUID
	// ContentID is the id of the contents guide entry backing the projection,
	// or the zero uuid on the update path when no new content is created.
	ContentID uuid.UUID
	// Action is one of "created", "updated", or "verified".
	//   - "created": a new projection row was inserted.
	//   - "updated": an existing row was modified (auto_update logged).
	//   - "verified": the row was re-verified with no field changes; only
	//     last_verified_at + freshness_score were bumped.
	Action string
	// ChangedFields lists the domain column names that were modified on the
	// update path (empty for "created" and "verified").
	ChangedFields []string
}

// Action constants returned in PromoteResult.Action.
const (
	ActionCreated  = "created"
	ActionUpdated  = "updated"
	ActionVerified = "verified"
)
