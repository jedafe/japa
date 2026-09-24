// Package comparison — propagate.go implements BustCachesForItemRef, the
// wiki:fields_propagate collaborator that invalidates every JCE comparison
// cache containing a given item_ref entity after an approved wiki community
// fields revision changes that entity's structured values.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
//
//	japaconnect-engineering/sections/30-generic-comparison-system.md (Sub-spec 5)
package comparison

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// BustCachesForItemRef finds every comparison content that contains an item
// whose item_ref matches {type: entityType, id: entityID} and deletes its JCE
// caches (freshness + advisor keys). Called by wiki.Service.PropagateFields —
// the body of the wiki:fields_propagate Asynq task — after a fields/mixed
// revision with value-affecting ops is approved.
//
// Idempotent: an entity referenced by no comparison produces an empty list and
// returns nil; cache deletion itself is best-effort (DeleteJCEComparisonCaches
// logs Valkey errors rather than returning them), so only the item_ref lookup
// can fail this method.
//
// Space-backed entities share one row ID across item_ref types: a university
// space may be edited as entity_type 'space' while comparisons reference it
// as {type:'university'}. For those types the lookup fans out across every
// space-backed alias so no comparison cache survives a type mismatch.
func (s *Service) BustCachesForItemRef(ctx context.Context, entityType, entityID string) error {
	if entityType == "" || entityID == "" {
		return fmt.Errorf("comparison: bust caches: entity_type and entity_id are required")
	}

	seen := map[pgtype.UUID]bool{}
	total := 0

	for _, t := range itemRefTypes(entityType) {
		contentIDs, err := s.store.listComparisonContentIDsByItemRef(ctx, t, entityID)
		if err != nil {
			return fmt.Errorf("comparison: bust caches for item_ref %s:%s: %w", t, entityID, err)
		}

		for _, contentID := range contentIDs {
			if seen[contentID] {
				continue
			}

			seen[contentID] = true
			s.DeleteJCEComparisonCaches(ctx, contentID)
			total++
		}
	}

	slog.Info("comparison: busted JCE caches for item_ref",
		"entity_type", entityType, "entity_id", entityID, "comparisons", total)

	return nil
}

// spaceBackedItemRefTypes are the item_ref entity types that resolve rows in
// the spaces table — the same set customfields.ValueEntityType collapses for
// value storage, expanded here for propagation because item_ref keeps the
// specific type.
var spaceBackedItemRefTypes = []string{"space", "university", "country", "state", "city"}

// itemRefTypes returns the item_ref types a propagation lookup must cover:
// the submitted type plus, for space-backed entities, every space alias.
func itemRefTypes(entityType string) []string {
	for _, t := range spaceBackedItemRefTypes {
		if t == entityType {
			return spaceBackedItemRefTypes
		}
	}

	return []string{entityType}
}
