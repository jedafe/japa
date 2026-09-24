package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/webhooks"
)

// publicWebhookAdapter satisfies publicapi.WebhookSubscriptionService over
// *webhooks.Service — the personal-endpoint methods are already owner-scoped
// at the store layer (scope='user' AND created_by=@owner).
//
// Ref: spec 2026-09-24-agent-data-api-phase3 (Phase 3 personal webhooks).
type publicWebhookAdapter struct {
	svc *webhooks.Service
}

func (a *publicWebhookAdapter) CreateUserEndpoint(ctx context.Context, ownerID pgtype.UUID, name, url string, eventTypes []string) (publicapi.PublicWebhookEndpoint, error) {
	ep, err := a.svc.CreateUserEndpoint(ctx, ownerID, webhooks.CreateWebhookRequest{
		Name:       name,
		URL:        url,
		EventTypes: eventTypes,
	})
	if err != nil {
		return publicapi.PublicWebhookEndpoint{}, err
	}
	out := toPublicWebhookEndpoint(ep)
	out.Secret = ep.Secret // plaintext, once — §12 "secret shown once"
	return out, nil
}

func (a *publicWebhookAdapter) ListUserEndpoints(ctx context.Context, ownerID pgtype.UUID) ([]publicapi.PublicWebhookEndpoint, error) {
	rows, err := a.svc.ListUserEndpoints(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]publicapi.PublicWebhookEndpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicapi.PublicWebhookEndpoint{
			ID:         r.ID,
			Name:       r.Name,
			URL:        r.URL,
			EventTypes: r.EventTypes,
			Status:     r.Status,
			IsActive:   r.IsActive,
			VerifiedAt: r.VerifiedAt,
			CreatedAt:  r.CreatedAt,
		})
	}
	return out, nil
}

func (a *publicWebhookAdapter) CountUserEndpoints(ctx context.Context, ownerID pgtype.UUID) (int64, error) {
	return a.svc.CountUserEndpoints(ctx, ownerID)
}

func (a *publicWebhookAdapter) VerifyUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error {
	return a.svc.VerifyUserEndpoint(ctx, ownerID, id)
}

func (a *publicWebhookAdapter) DeleteUserEndpoint(ctx context.Context, ownerID, id pgtype.UUID) error {
	return a.svc.DeleteUserEndpoint(ctx, ownerID, id)
}

func (a *publicWebhookAdapter) ListUserDeliveries(ctx context.Context, ownerID, endpointID pgtype.UUID, limit, offset int32) ([]publicapi.PublicWebhookDelivery, error) {
	rows, err := a.svc.ListUserDeliveries(ctx, ownerID, endpointID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]publicapi.PublicWebhookDelivery, 0, len(rows))
	for _, d := range rows {
		out = append(out, publicapi.PublicWebhookDelivery{
			ID:               d.ID,
			EventID:          d.EventID,
			EventType:        d.EventType,
			Status:           d.Status,
			Attempts:         d.Attempts,
			LastError:        d.LastError,
			LastResponseCode: d.LastResponseCode,
			NextAttemptAt:    d.NextAttemptAt,
			DeliveredAt:      d.DeliveredAt,
			CreatedAt:        d.CreatedAt,
		})
	}
	return out, nil
}

func toPublicWebhookEndpoint(ep webhooks.WebhookEndpointWithSecret) publicapi.PublicWebhookEndpoint {
	out := publicapi.PublicWebhookEndpoint{
		ID:         ep.ID.String(),
		Name:       ep.Name,
		URL:        ep.URL,
		EventTypes: ep.EventTypes,
		Status:     ep.Status,
		IsActive:   ep.IsActive,
		CreatedAt:  ep.CreatedAt.Time,
	}
	if ep.VerifiedAt.Valid {
		t := ep.VerifiedAt.Time
		out.VerifiedAt = &t
	}
	return out
}

var _ publicapi.WebhookSubscriptionService = (*publicWebhookAdapter)(nil)
