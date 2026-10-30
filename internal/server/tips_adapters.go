package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/opencollective"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tips"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// tipsPermAdapter adapts *rbac.RBAC to tips.PermChecker.
type tipsPermAdapter struct {
	rbac *rbac.RBAC
}

// Can delegates to HasPermission.
func (a tipsPermAdapter) Can(ctx context.Context, userID pgtype.UUID, node string) bool {
	if a.rbac == nil {
		return false
	}
	return a.rbac.HasPermission(ctx, userID, node)
}

// tipsOrgAdapter adapts *organizations.Service to tips.OrgRoleChecker.
type tipsOrgAdapter struct {
	svc *organizations.Service
}

// MemberRole delegates to the organizations service (roles: admin|editor|member).
func (a tipsOrgAdapter) MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error) {
	return a.svc.MemberRole(ctx, orgID, userID)
}

// OrganizationOwner resolves organizations.owner_user_id.
func (a tipsOrgAdapter) OrganizationOwner(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error) {
	org, err := a.svc.GetOrganization(ctx, orgID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return org.OwnerUserID, nil
}

// tipsOCAdapter adapts *opencollective.Client to tips.OCClient.
type tipsOCAdapter struct {
	client *opencollective.Client
}

// VerifyToken delegates to the OC client.
func (a tipsOCAdapter) VerifyToken(ctx context.Context, token string) (string, error) {
	return a.client.VerifyToken(ctx, token)
}

// VerifyCollectiveAccess delegates to the OC client.
func (a tipsOCAdapter) VerifyCollectiveAccess(ctx context.Context, token, slug string) (bool, error) {
	return a.client.VerifyCollectiveAccess(ctx, token, slug)
}

// ListContributions delegates and maps onto the tips-side struct.
func (a tipsOCAdapter) ListContributions(ctx context.Context, slug, token string, limit, offset int) ([]tips.OCContribution, error) {
	rows, err := a.client.ListContributions(ctx, slug, token, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]tips.OCContribution, 0, len(rows))
	for _, c := range rows {
		out = append(out, tips.OCContribution{
			ID: c.ID, AmountCents: c.AmountCents, Currency: c.Currency,
			FromName: c.FromName, FromSlug: c.FromSlug, FromEmail: c.FromEmail,
		})
	}
	return out, nil
}

// tipsPaymentAdapter adapts *payments.Service to tips.PaymentCreator — fiat
// tips ride §17 with purpose_type='donation'.
type tipsPaymentAdapter struct {
	svc *payments.Service
}

// CreatePayment maps the tips input onto §17 and flattens the result.
func (a tipsPaymentAdapter) CreatePayment(ctx context.Context, userID pgtype.UUID, in tips.PaymentCreateInput) (tips.PaymentCreateResult, error) {
	res, err := a.svc.CreatePayment(ctx, userID, payments.CreatePaymentInput{
		Provider:    in.Provider,
		PurposeType: in.PurposeType,
		PurposeID:   in.PurposeID,
		Amount:      in.Amount,
		Currency:    in.Currency,
		Email:       in.Email,
		Metadata:    in.Metadata,
	})
	if err != nil {
		return tips.PaymentCreateResult{}, err
	}
	var providerRef string
	if res.Payment.ProviderRef != nil {
		providerRef = *res.Payment.ProviderRef
	}
	return tips.PaymentCreateResult{
		ID:          res.Payment.ID.String(),
		Provider:    res.Payment.Provider,
		ProviderRef: providerRef,
		CheckoutURL: res.CheckoutURL,
		Status:      res.Payment.Status,
		Amount:      res.Payment.Amount,
		Currency:    res.Payment.Currency,
	}, nil
}

// tipsNotifierAdapter satisfies tips.Notifier — enqueues the
// tips:notify_received task (Rule 8 fire-and-forget).
type tipsNotifierAdapter struct {
	enq *worker.Enqueuer
}

// NotifyTipReceived enqueues the fan-out task.
func (a tipsNotifierAdapter) NotifyTipReceived(ctx context.Context, tipID pgtype.UUID) error {
	if a.enq == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"tip_id": tipID.String()})
	return a.enq.Enqueue(worker.TypeTipsNotifyReceived, payload)
}

// tipsNotifyDispatchAdapter adapts *notifications.Service to
// tips.NotificationCreator — the actual dispatch call inside the
// tips:notify_received task handler.
type tipsNotifyDispatchAdapter struct {
	svc *notifications.Service
}

// CreateNotification delegates to the notifications service.
func (a tipsNotifyDispatchAdapter) CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
	return a.svc.CreateNotification(ctx, recipientID, nType, actorType, actorID, subjectType, subjectID, title, body, actionURL)
}

// tipsProviderAdapter reports which §17 providers were configured at boot —
// public tip-configs only advertise usable fiat methods.
func (a tipsPaymentAdapter) ProviderNames() []string {
	names := a.svc.ProviderNames()
	return names
}

// tipsSyncLockAdapter implements tips.SyncLocker over Valkey SET NX EX
// (jc:tips:oc_sync_lock:{owner_id} per the keys catalog).
type tipsSyncLockAdapter struct {
	rdb valkeycompat.Cmdable
}

// TryLock returns true when the caller holds the new lock.
func (a tipsSyncLockAdapter) TryLock(ctx context.Context, key string, ttlSeconds int) (bool, error) {
	if a.rdb == nil {
		return true, nil
	}
	return a.rdb.SetNX(ctx, valkey.MustKey("%s", key), "1", time.Duration(ttlSeconds)*time.Second).Result()
}

// Unlock deletes the lock key — only call after TryLock returned true.
func (a tipsSyncLockAdapter) Unlock(ctx context.Context, key string) error {
	if a.rdb == nil {
		return nil
	}
	return a.rdb.Del(ctx, valkey.MustKey("%s", key)).Err()
}

// tipsSettlementAdapter adapts *tips.Service to worker.TipSettlement.
type tipsSettlementAdapter struct {
	svc    *tips.Service
	notify *notifications.Service
}

// SettleFiatTip delegates.
func (a tipsSettlementAdapter) SettleFiatTip(ctx context.Context, tipID, paymentID pgtype.UUID) error {
	return a.svc.SettleFiatTip(ctx, tipID, paymentID)
}

// FailTip delegates.
func (a tipsSettlementAdapter) FailTip(ctx context.Context, tipID pgtype.UUID) error {
	return a.svc.FailTip(ctx, tipID)
}

// ClawbackTip delegates.
func (a tipsSettlementAdapter) ClawbackTip(ctx context.Context, tipID pgtype.UUID) error {
	return a.svc.ClawbackTip(ctx, tipID)
}

// NotifyTipReceived delegates, injecting the notifications dispatcher.
func (a tipsSettlementAdapter) NotifyTipReceived(ctx context.Context, tipID pgtype.UUID) error {
	return a.svc.NotifyTipReceived(ctx, tipID, tipsNotifyDispatchAdapter{svc: a.notify})
}

// SyncOC delegates to SyncAll.
func (a tipsSettlementAdapter) SyncOC(ctx context.Context) error {
	return a.svc.SyncAll(ctx)
}

// tipsFeatureAdapter adapts *features.Checker to tips.FeatureChecker — the
// flags live in feature_flags, toggled via admin → Extensions.
type tipsFeatureAdapter struct {
	checker *features.Checker
}

// IsEnabled delegates to the feature-flag checker.
func (a tipsFeatureAdapter) IsEnabled(ctx context.Context, key string) bool {
	if a.checker == nil {
		return false
	}
	return a.checker.IsEnabled(ctx, key)
}

// tipsRefundAdapter adapts *payments.Service to tips.PaymentRefunder — admin
// tip refunds issue a real provider-side refund for fiat tips.
type tipsRefundAdapter struct {
	svc *payments.Service
}

// RefundPayment delegates to CreateRefund with the tip's full fiat amount.
func (a tipsRefundAdapter) RefundPayment(ctx context.Context, paymentID string, amount int64, reason string) error {
	_, err := a.svc.CreateRefund(ctx, payments.CreateRefundInput{
		PaymentID: paymentID,
		Amount:    amount,
		Reason:    reason,
	})
	return err
}
