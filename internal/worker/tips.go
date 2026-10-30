// tips.go — §125 Asynq handlers: settle_fiat credits the recipient after a
// donation payment succeeds, notify_received fans out the tip.received
// notification, oc_sync polls connected Open Collective collectives.
package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// TipSettlement is the worker-side seam into internal/tips.Service — kept
// narrow so worker never imports the domain package directly.
type TipSettlement interface {
	SettleFiatTip(ctx context.Context, tipID, paymentID pgtype.UUID) error
	ClawbackTip(ctx context.Context, tipID pgtype.UUID) error
	FailTip(ctx context.Context, tipID pgtype.UUID) error
	NotifyTipReceived(ctx context.Context, tipID pgtype.UUID) error
	SyncOC(ctx context.Context) error
}

var tipSettlement TipSettlement

// RegisterTipSettlement injects the tips settlement adapter at server boot.
func RegisterTipSettlement(s TipSettlement) {
	tipSettlement = s
}

// HandleTipsSettleFiat processes tips:settle_fiat — payload
// {"tip_id": uuid, "payment_id": uuid}. payment_id repairs tip.payment_id
// when the create-time attach raced/failed.
func HandleTipsSettleFiat(ctx context.Context, t *asynq.Task) error {
	if tipSettlement == nil {
		return fmt.Errorf("tips settlement not registered")
	}
	var p struct {
		TipID     string `json:"tip_id"`
		PaymentID string `json:"payment_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshaling tip_id: %w", err)
	}
	var tipID pgtype.UUID
	if err := tipID.Scan(p.TipID); err != nil {
		return fmt.Errorf("invalid tip_id %q: %w", p.TipID, err)
	}
	var paymentID pgtype.UUID
	if p.PaymentID != "" {
		_ = paymentID.Scan(p.PaymentID)
	}
	if err := tipSettlement.SettleFiatTip(ctx, tipID, paymentID); err != nil {
		return fmt.Errorf("settling fiat tip %s: %w", p.TipID, err)
	}
	return nil
}

// HandleTipsNotifyReceived processes tips:notify_received.
func HandleTipsNotifyReceived(ctx context.Context, t *asynq.Task) error {
	if tipSettlement == nil {
		return nil // notification is best-effort; don't retry without a service
	}
	var p struct {
		TipID string `json:"tip_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshaling tip_id: %w", err)
	}
	var tipID pgtype.UUID
	if err := tipID.Scan(p.TipID); err != nil {
		return fmt.Errorf("invalid tip_id %q: %w", p.TipID, err)
	}
	return tipSettlement.NotifyTipReceived(ctx, tipID)
}

// HandleTipsOCSync processes tips:oc_sync — sweeps every collective with a
// stored token under a Valkey SET-NX lock per owner.
func HandleTipsOCSync(ctx context.Context, t *asynq.Task) error {
	if tipSettlement == nil {
		return nil
	}
	return tipSettlement.SyncOC(ctx)
}

// enqueueTipsSettleFiat fires the fiat settlement task from the payment
// success hook — fire-and-forget (Rule 8).
func enqueueTipsSettleFiat(tipID, paymentID pgtype.UUID) {
	if taskEnqueuer == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"tip_id": tipID.String(), "payment_id": paymentID.String()})
	_ = taskEnqueuer.Enqueue(TypeTipsSettleFiat, payload)
}
