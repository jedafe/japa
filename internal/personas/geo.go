// Package personas — geo.go implements §68 Geo Space Assignment: resolving a
// geo space ref (UUID or slug), expanding it to its geo ancestor chain, and
// joining a persona's backing user to that chain so the persona profile
// carries a geo space like a real profile (§05, ADR-064a).
// // Ref: japaconnect-engineering/sections/68-ai-personas.md (Geo Space Assignment)
package personas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// isGeoSpaceLevel mirrors users.Store.isValidGeoSpace — only the geo
// hierarchy levels can anchor a persona's location.
func isGeoSpaceLevel(level string) bool {
	switch level {
	case "country", "state", "city":
		return true
	default:
		return false
	}
}

// resolveGeoSpace resolves a space ref (UUID or slug — GetSpaceBySlug accepts
// both and already excludes soft-deleted rows) to an active geo space id.
// Failures wrap ErrInvalid so callers surface 400 validation_error.
func (s *Service) resolveGeoSpace(ctx context.Context, q *gen.Queries, ref string) (pgtype.UUID, error) {
	ref = strings.TrimSpace(ref)

	space, err := q.GetSpaceBySlug(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("geo_space %q: space not found: %w", ref, errpkg.ErrInvalid)
		}
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: lookup: %w", ref, err)
	}

	if space.Status != StatusActive {
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: space is not active: %w", ref, errpkg.ErrInvalid)
	}

	if !isGeoSpaceLevel(space.Level) {
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: not a geo space (country/state/city): %w", ref, errpkg.ErrInvalid)
	}

	return space.ID, nil
}

// geoChain returns spaceID plus the ids of its geo-level ancestors
// (ListSpaceAncestors walks parent_id ∪ space_parent_spaces, ≤3 hops —
// the geo chain never exceeds city → state → country).
func geoChain(ctx context.Context, q *gen.Queries, spaceID pgtype.UUID) ([]pgtype.UUID, error) {
	chain := []pgtype.UUID{spaceID}

	ancestors, err := q.ListSpaceAncestors(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list geo ancestors: %w", err)
	}

	for _, anc := range ancestors {
		if isGeoSpaceLevel(anc.Level) {
			chain = append(chain, anc.ID)
		}
	}

	return chain, nil
}

// applyGeoSpace joins userID to every space in the chain (idempotent via
// AddSpaceMember's ON CONFLICT) and sets users.current_geo_space_id to the
// chain head — the deepest level the admin selected. Mirrors onboarding's
// autoJoinGeoSpaces (§09 space auto-join) minus the form-field resolution.
func (s *Service) applyGeoSpace(ctx context.Context, q *gen.Queries, userID pgtype.UUID, chain []pgtype.UUID) error {
	// Re-verify the chain head inside the tx — it was resolved pre-tx during
	// provisioning and could (theoretically) be soft-deleted or deactivated
	// before commit. Ancestors are already filtered deleted_at IS NULL by
	// ListSpaceAncestors.
	head := chain[0]
	sp, err := q.GetSpaceByID(ctx, head)
	if err != nil {
		return fmt.Errorf("re-check geo space: %w", err)
	}
	if sp.DeletedAt.Valid || sp.Status != StatusActive {
		return fmt.Errorf("geo space %s is not active: %w", head.String(), errpkg.ErrInvalid)
	}

	for _, spaceID := range chain {
		if _, err := q.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
			SpaceID: spaceID,
			UserID:  userID,
		}); err != nil {
			return fmt.Errorf("join geo space %s: %w", spaceID.String(), err)
		}
	}

	if err := q.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
		CurrentGeoSpaceID: chain[0],
		ID:                userID,
	}); err != nil {
		return fmt.Errorf("set current geo space: %w", err)
	}

	return nil
}

// replaceGeoSpace applies PATCH replace semantics: memberships in geo-level
// spaces outside the new chain are deleted, then the new chain is joined and
// current_geo_space_id updated. An empty ref clears geo assignment entirely —
// all geo memberships are dropped and current_geo_space_id is set NULL.
func (s *Service) replaceGeoSpace(ctx context.Context, q *gen.Queries, userID pgtype.UUID, ref string) error {
	// Whitespace-only refs behave like empty — the clear path.
	ref = strings.TrimSpace(ref)

	var chain []pgtype.UUID

	if ref != "" {
		spaceID, err := s.resolveGeoSpace(ctx, q, ref)
		if err != nil {
			return err
		}

		chain, err = geoChain(ctx, q, spaceID)
		if err != nil {
			return err
		}
	}

	keep := chain
	if keep == nil {
		keep = []pgtype.UUID{}
	}

	if err := q.RemoveUserGeoMembershipsExcept(ctx, gen.RemoveUserGeoMembershipsExceptParams{
		UserID:  userID,
		KeepIds: keep,
	}); err != nil {
		return fmt.Errorf("remove stale geo memberships: %w", err)
	}

	if chain == nil {
		// Clear path — NULL current geo space, no memberships to add.
		return q.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
			CurrentGeoSpaceID: pgtype.UUID{},
			ID:                userID,
		})
	}

	return s.applyGeoSpace(ctx, q, userID, chain)
}
