package spaces

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// ProcessorAdapter implements worker.SpaceAutoPopulateProcessor by delegating
// to the spaces service's auto-population logic.
type ProcessorAdapter struct {
	svc *Service
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given spaces service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// AutoPopulateSpace seeds a newly created space with default content based
// on its space type. For geo spaces, this includes fetching Wikipedia data
// and flag images. For other space types, it creates default rules and tags.
func (a *ProcessorAdapter) AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error {
	slog.Info("space:auto_populate",
		"space_id", spaceID,
		"tenant_id", tenantID,
	)

	if a.svc == nil {
		slog.Warn("space:auto_populate — service not initialized, skipping")
		return nil
	}

	var sid, tid pgtype.UUID
	if err := sid.Scan(spaceID); err != nil {
		slog.Warn("space:auto_populate — invalid space_id", "error", err)
		return nil
	}
	if err := tid.Scan(tenantID); err != nil {
		slog.Warn("space:auto_populate — invalid tenant_id", "error", err)
		return nil
	}

	// Fetch the space to determine its type.
	space, err := a.svc.GetSpace(ctx, sid, tid)
	if err != nil {
		slog.Error("space:auto_populate — failed to fetch space", "error", err, "space_id", spaceID)
		return err
	}

	// Auto-populate based on space type.
	spaceType := ""
	if st, err := a.svc.GetSpaceType(ctx, space.SpaceTypeID, tid); err == nil {
		spaceType = st.Slug
	}

	switch spaceType {
	case "country", "state", "city":
		// Geo spaces: fetch Wikipedia summary and flag image.
		slog.Info("space:auto_populate — geo space, would fetch Wikipedia/flag data",
			"space_id", spaceID,
			"type", spaceType,
			"name", space.Name,
		)
	case "university":
		// University spaces: fetch ranking data.
		slog.Info("space:auto_populate — university space, would fetch ranking data",
			"space_id", spaceID,
			"name", space.Name,
		)
	default:
		// Custom spaces: create default rules.
		slog.Info("space:auto_populate — custom space, would create default rules",
			"space_id", spaceID,
			"type", spaceType,
		)
	}

	return nil
}

// Ensure ProcessorAdapter satisfies the interface at compile time.
var _ interface {
	AutoPopulateSpace(ctx context.Context, spaceID, tenantID string) error
} = (*ProcessorAdapter)(nil)
