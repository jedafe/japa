package stablecoin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// webhookEnvelope is the provider event wire shape — tolerant: Crossmint
// activity payloads carry the transaction signature under a few keys.
type webhookEnvelope struct {
	Type        string          `json:"type"`
	WalletAddr  string          `json:"wallet_address"`
	TxSignature string          `json:"tx_signature"`
	Signature   string          `json:"signature"` // alt sig field
	TxHash      string          `json:"tx_hash"`   // EVM-style alt
	Slot        int64           `json:"slot"`
	Asset       string          `json:"asset"`
	AmountMinor json.RawMessage `json:"amount"`
	CounterAddr string          `json:"counterparty"`
	OccurredAt  int64           `json:"occurred_at"`
}

// IngestWebhook authenticates and queues one provider event. Deduped by
// tx signature (jc:stablecoin:webhook:dedupe:{sig}) then handed to
// stablecoin:sync_activity — fan-out, never blocking the webhook response
// (Rule 8).
func (s *Service) IngestWebhook(ctx context.Context, payload []byte, signature string) error {
	if err := s.provider.VerifyWebhook(payload, signature); err != nil {
		return fmt.Errorf("stablecoin: webhook auth: %w", err)
	}

	var env webhookEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("stablecoin: webhook parse: %w", err)
	}
	sig := env.TxSignature
	if sig == "" {
		sig = env.Signature
	}
	if sig == "" {
		sig = env.TxHash
	}
	if sig == "" {
		// Verified but not a transaction event (wallet.created, signer
		// approvals, ...) — acknowledge and drop; never 401 a valid event.
		slog.Info("stablecoin: webhook event without tx signature dropped", "type", env.Type)
		return nil
	}

	// Fast-path dedupe: SET NX on the signature. DB unique on tx_signature
	// is the authoritative backstop (InsertActivity ON CONFLICT DO NOTHING).
	ok, err := s.vk.SetNX(ctx, "jc:stablecoin:webhook:dedupe:"+sig, "1", 86400)
	if err != nil {
		return fmt.Errorf("stablecoin: webhook dedupe: %w", err)
	}
	if !ok {
		return nil // replay — already ingested
	}

	if s.enq != nil {
		wrapped, _ := json.Marshal(SyncActivityPayload{Event: payload})
		if err := s.enq.Enqueue(TaskSyncActivity, wrapped); err != nil {
			slog.Warn("stablecoin: sync enqueue failed, processing inline", "sig", sig, "error", err)
			return s.SyncActivity(ctx, payload)
		}
		return nil
	}
	return s.SyncActivity(ctx, payload)
}

// SyncActivity is the stablecoin:sync_activity entry — parses the provider
// payload, resolves the member wallet, mirrors the event.
func (s *Service) SyncActivity(ctx context.Context, payload []byte) error {
	var env webhookEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("stablecoin: sync parse: %w", err)
	}
	ev := env.toProviderEvent()

	w, err := s.store.GetWalletByAddress(ctx, env.WalletAddr)
	if err != nil {
		// Unknown wallet — external address interacting with our rails.
		// Log and drop; the reconcile sweep is the safety net.
		slog.Warn("stablecoin: activity for unknown wallet", "addr", env.WalletAddr, "type", env.Type)
		return nil
	}
	return s.MirrorActivity(ctx, ev, w.ID)
}

// MirrorActivity persists one provider event into stablecoin_activity —
// the render cache for the unified history feed. Called by
// stablecoin:sync_activity. Idempotent on tx_signature.
func (s *Service) MirrorActivity(ctx context.Context, ev ProviderEvent, walletID string) error {
	if _, err := s.store.GetActivityBySig(ctx, ev.TxSignature); err == nil {
		return nil // already mirrored — idempotent
	} else if !errors.Is(err, ErrActivityNotFound) {
		return fmt.Errorf("stablecoin: activity lookup: %w", err)
	}

	kind, direction := classifyEvent(ev)
	creditsPer := s.creditsPerUSD(ctx)
	credits := ev.AmountMinor * creditsPer / 1_000_000 // USD minor → display credits

	// Counterparty resolution: member wallet ↔ internal transfer, else NULL.
	var cp *string
	if cw, err := s.store.GetWalletByAddress(ctx, ev.CounterAddr); err == nil && cw != nil {
		cp = &cw.UserID
	}

	status := "confirmed"
	if ev.Type == "pending" || ev.Type == "submitted" {
		status = "pending"
	} else if ev.Type == "failed" {
		status = "failed"
	}

	occurred := time.Unix(ev.OccurredAt, 0).UTC()
	if ev.OccurredAt <= 0 {
		occurred = time.Now().UTC() // provider omitted the timestamp — don't sort to 1970
	}
	_, err := s.store.InsertActivity(ctx, Activity{
		WalletID:           walletID,
		Direction:          direction,
		Kind:               kind,
		Asset:              normalizeAsset(ev.Asset),
		AmountMinor:        ev.AmountMinor,
		CreditsAmount:      credits,
		CounterpartyUserID: cp,
		TxSignature:        ev.TxSignature,
		Slot:               ev.Slot,
		Status:             status,
		OccurredAt:         occurred,
	})
	return err
}

// classifyEvent maps provider event types to mirror kind/direction.
func classifyEvent(ev ProviderEvent) (kind, direction string) {
	switch ev.Type {
	case "transfer_in", "deposit":
		return ActivityDeposit, "in"
	case "tip_in":
		return ActivityTipIn, "in"
	case "tip_out":
		return ActivityTipOut, "out"
	case "transfer_out", "send":
		return ActivityWithdrawal, "out"
	case "purchase":
		return ActivityPurchase, "out"
	case "fee":
		return ActivityFee, "out"
	case "offramp":
		return ActivityOfframp, "out"
	case "escrow_funding":
		return ActivityEscrowFunding, "out"
	case "escrow_release":
		return ActivityEscrowRelease, "in"
	case "failed":
		return ActivityWithdrawal, "out"
	default:
		return ev.Type, "out"
	}
}

func normalizeAsset(a string) string {
	switch a {
	case AssetUSDC, AssetUSDT, "solana:usdc", "solana:usdt":
		if len(a) > 7 {
			return a[7:]
		}
		return a
	default:
		return AssetUSDC
	}
}

// creditsPerUSD resolves the display ratio from the currencies table
// (credits_per_unit on the USD row, §58/ADR-065) — falls back to 1:1 in
// minor units when the rate seam is unwired or the row is absent.
func (s *Service) creditsPerUSD(ctx context.Context) int64 {
	if s.creditRater != nil {
		if n, err := s.creditRater.CreditsPerUSD(ctx); err == nil && n > 0 {
			return n
		}
	}
	return 100 // 1 credit per USD minor-unit display unit — neutral default
}

// FeedEntry is one row of the unified member activity feed —
// credits-denominated, merges onchain mirror + bonus ledger (§128.9).
type FeedEntry struct {
	Kind      string    `json:"kind"`      // deposit | tip_in | tip_out | purchase | fee | withdrawal | offramp | escrow_* | bonus_earn | bonus_spend | bonus_refund
	Direction string    `json:"direction"` // in | out
	Credits   int64     `json:"credits"`
	Asset     string    `json:"asset,omitempty"`
	Status    string    `json:"status"`
	TxSig     string    `json:"tx_signature,omitempty"`
	Source    string    `json:"source,omitempty"`
	Tier      string    `json:"tier"` // backed | bonus
	At        time.Time `json:"at"`
}

// ListActivity returns the unified member feed: onchain mirror rows plus
// bonus-ledger entries, newest-first (§128.9 — chain is truth, DB renders).
func (s *Service) ListActivity(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]FeedEntry, error) {
	feed := []FeedEntry{}

	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil && !errors.Is(err, ErrWalletNotFound) {
		return nil, fmt.Errorf("stablecoin: activity wallet: %w", err)
	}
	if w != nil {
		if wid, perr := pgUUID(w.ID); perr == nil {
			if rows, aerr := s.store.ListActivityByWallet(ctx, wid, limit+offset, 0); aerr == nil {
				for _, a := range rows {
					feed = append(feed, FeedEntry{
						Kind:      a.Kind,
						Direction: a.Direction,
						Credits:   a.CreditsAmount,
						Asset:     a.Asset,
						Status:    a.Status,
						TxSig:     a.TxSignature,
						Tier:      "backed",
						At:        a.OccurredAt,
					})
				}
			}
		}
	}

	if s.bonusLedger != nil {
		if rows, berr := s.bonusLedger.ListCreditEntries(ctx, userID, limit+offset, 0); berr == nil {
			for _, b := range rows {
				kind := "bonus_" + b.Operation
				dir := "out"
				if b.Amount > 0 {
					dir = "in"
				}
				feed = append(feed, FeedEntry{
					Kind:      kind,
					Direction: dir,
					Credits:   abs64(b.Amount),
					Status:    "confirmed",
					Source:    b.Source,
					Tier:      "bonus",
					At:        b.OccurredAt,
				})
			}
		}
	}

	sort.Slice(feed, func(i, j int) bool { return feed[i].At.After(feed[j].At) })
	if int(offset) >= len(feed) {
		return []FeedEntry{}, nil
	}
	feed = feed[offset:]
	if int(limit) < len(feed) {
		feed = feed[:limit]
	}
	return feed, nil
}

// Reconcile is the periodic mirror-vs-chain sweep
// (stablecoin:reconcile_wallets) — closes webhook gaps, refreshes the
// mirror cache, and flags divergent balances. Chain is truth; the mirror
// is a render cache.
func (s *Service) Reconcile(ctx context.Context) error {
	// Refresh every active wallet's mirror cache from provider activity.
	wallets, err := s.listActiveWallets(ctx)
	if err != nil {
		return fmt.Errorf("stablecoin: reconcile wallets: %w", err)
	}
	for _, w := range wallets {
		if err := s.reconcileWallet(ctx, w); err != nil {
			slog.Warn("stablecoin: reconcile wallet failed", "wallet_id", w.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) listActiveWallets(ctx context.Context) ([]Wallet, error) {
	return s.store.ListWalletsByStatus(ctx, WalletStatusActive, 500)
}

// reconcileWallet pulls provider activity for one wallet, mirrors gaps,
// and rewrites the balance mirror cache.
func (s *Service) reconcileWallet(ctx context.Context, w Wallet) error {
	events, err := s.provider.Activity(ctx, w.Locator, "")
	if err != nil {
		return fmt.Errorf("provider activity: %w", err)
	}
	for _, ev := range events {
		if err := s.MirrorActivity(ctx, ev, w.ID); err != nil {
			slog.Warn("stablecoin: reconcile mirror failed", "wallet_id", w.ID, "sig", ev.TxSignature, "error", err)
		}
	}

	// Recompute the backed-credits mirror from confirmed rows.
	if wid, perr := pgUUID(w.ID); perr == nil {
		if rows, aerr := s.store.ListActivityByWallet(ctx, wid, 1000, 0); aerr == nil {
			var bal int64
			for _, a := range rows {
				if a.Status != "confirmed" {
					continue
				}
				if a.Direction == "in" {
					bal += a.CreditsAmount
				} else {
					bal -= a.CreditsAmount
				}
			}
			if bal < 0 {
				bal = 0
			}
			if err := s.vk.Set(ctx, "jc:stablecoin:mirror:"+w.UserID, strconv.FormatInt(bal, 10), 3600); err != nil {
				return fmt.Errorf("mirror cache: %w", err)
			}
		}
	}
	return nil
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// toProviderEvent normalizes the webhook envelope into a ProviderEvent.
func (e webhookEnvelope) toProviderEvent() ProviderEvent {
	sig := e.TxSignature
	if sig == "" {
		sig = e.Signature
	}
	if sig == "" {
		sig = e.TxHash
	}
	var amount int64
	if len(e.AmountMinor) > 0 {
		// Tolerant: provider may send "1000000" or 1000000.
		_ = json.Unmarshal(e.AmountMinor, &amount)
		if amount == 0 {
			var s string
			if json.Unmarshal(e.AmountMinor, &s) == nil {
				amount, _ = strconv.ParseInt(s, 10, 64)
			}
		}
	}
	return ProviderEvent{
		Type:        e.Type,
		TxSignature: sig,
		Slot:        e.Slot,
		Asset:       e.Asset,
		AmountMinor: amount,
		CounterAddr: e.CounterAddr,
		OccurredAt:  e.OccurredAt,
		Raw:         nil,
	}
}
