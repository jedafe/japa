package worker

import (
	"context"
)

// WalletProcessor defines the interface for wallet background task processing.
// The wallet service implements this interface and registers with the worker.
type WalletProcessor interface {
	ProcessActivityEventTask(ctx context.Context, payload []byte) error
	ProcessCreditPurchaseTask(ctx context.Context, payload []byte) error
	AwardPointsTask(ctx context.Context, payload []byte) error
	DeductCreditsTask(ctx context.Context, payload []byte) error
	ReconcilePayoutsTask(ctx context.Context, payload []byte) error
}

var walletProcessor WalletProcessor

// RegisterWalletProcessor registers the wallet service processor for background tasks.
func RegisterWalletProcessor(p WalletProcessor) {
	walletProcessor = p
}
