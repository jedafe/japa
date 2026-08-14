// Package server — mentorship_adapters.go provides the wiring-layer adapter
// that bridges the mentorship service's EventTracker interface to the
// analytics service's TrackEvent method (§102 analytics events).
//
// The adapter exists because the mentorship package defines its own
// EventTracker interface (accepting primitive params) to avoid importing
// internal/analytics, while analytics.Service.TrackEvent accepts an
// analytics.TrackEventInput struct. The adapter converts between the two.
package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/analytics"
)

// analyticsEventTrackerAdapter adapts *analytics.Service to the mentorship
// EventTracker interface.
type analyticsEventTrackerAdapter struct {
	svc *analytics.Service
}

// TrackEvent delegates to analytics.Service.TrackEvent, converting the
// primitive params into an analytics.TrackEventInput.
func (a analyticsEventTrackerAdapter) TrackEvent(ctx context.Context, tenantID, userID pgtype.UUID, eventType, entityType string, entityID pgtype.UUID, meta map[string]any) error {
	_, err := a.svc.TrackEvent(ctx, tenantID, analytics.TrackEventInput{
		EventType:  analytics.EventType(eventType),
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Meta:       meta,
	})
	return err
}
