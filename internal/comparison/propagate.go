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
func (s *Service) BustCachesForItemRef(ctx context.Context, entityType, entityID string) error {
	if entityType == "" || entityID == "" {
		return fmt.Errorf("comparison: bust caches: entity_type and entity_id are required")
	}

	contentIDs, err := s.store.listComparisonContentIDsByItemRef(ctx, entityType, entityID)
	if err != nil {
		return fmt.Errorf("comparison: bust caches for item_ref %s:%s: %w", entityType, entityID, err)
	}

	for _, contentID := range contentIDs {
		s.DeleteJCEComparisonCaches(ctx, contentID)
	}

	slog.Info("comparison: busted JCE caches for item_ref",
		"entity_type", entityType, "entity_id", entityID, "comparisons", len(contentIDs))

	return nil
}
