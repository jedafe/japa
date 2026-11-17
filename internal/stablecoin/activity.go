package stablecoin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/wallet"

	"github.com/jackc/pgx/v5/pgtype"
)

// webhookEnvelope is the provider event wire shape — tolerant: Crossmint
// activity payloads carry the transaction signature under a few keys.
type webhookEnvelope struct {
	Type            string          `json:"type"`
	WalletAddr      string          `json:"wallet_address"`
	WalletAddrCamel string          `json:"walletAddress"` // camelCase variant
	TxSignature     string          `json:"tx_signature"`
	Signature       string          `json:"signature"` // alt sig field
	TxHash          string          `json:"tx_hash"`   // EVM-style alt
	Slot            int64           `json:"slot"`
	Asset           string          `json:"asset"`
	AmountMinor     json.RawMessage `json:"amount"`
	CounterAddr     string          `json:"counterparty"`
	OccurredAt      int64           `json:"occurred_at"`
}

// walletRef returns the first non-empty wallet identifier — the provider's
// key naming isn't stable across event types.
func (e webhookEnvelope) walletRef() string {
	if e.WalletAddr != "" {
		return e.WalletAddr
	}
	return e.WalletAddrCamel
}

// IngestWebhook authenticates and queues one provider event. Deduped by
// tx signature (jc:stablecoin:webhook:dedupe:{sig}) then handed to
// stablecoin:sync_activity — fan-out, never blocking the webhook response
// (Rule 8).
// ErrWebhookAuth marks signature-verification failures — the handler maps
// it to 401; internal errors stay 500 so the provider retries them.
var ErrWebhookAuth = errors.New("stablecoin: webhook auth failed")

func (s *Service) IngestWebhook(ctx context.Context, payload []byte, signature string) error {
	if err := s.provider.VerifyWebhook(payload, signature); err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookAuth, err)
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

	// Fast-path dedupe: SET NX on the body hash — NOT the tx signature.
	// One member→member tx fires a per-wallet event for each participant
	// (tip_out + tip_in under the same signature); a sig-only key would
	// drop the second wallet's event at ingest. Identical bodies are true
	// replays; any distinct delivery processes. The (wallet_id, signature)
	// DB unique is the authoritative backstop.
	ok, err := s.vk.SetNX(ctx, "jc:stablecoin:webhook:dedupe:"+fmt.Sprintf("%x", sha256.Sum256(payload)), "1", 86400)
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

	w, err := s.store.GetWalletByAddress(ctx, env.walletRef())
	if err != nil {
		if !errors.Is(err, ErrWalletNotFound) {
			// A transient lookup failure must not drop a chain event —
			// the webhook already ACKed, so return the error and let the
			// task retry. Only a genuine not-found falls through.
			return fmt.Errorf("stablecoin: wallet lookup: %w", err)
		}
		// Not a member wallet — check the treasury registry: hot→cold
		// sweep executions and fee-leg landings are provider events too.
		chain := s.settings.GetStringOr(ctx, "stablecoin.network", "solana")
		if terr := s.mirrorTreasuryEvent(ctx, chain, env.walletRef(), ev); terr == nil {
			return nil
		}
		// Unknown wallet — external address interacting with our rails.
		// Log and drop; the reconcile sweep is the safety net.
		slog.Warn("stablecoin: activity for unknown wallet", "addr", env.walletRef(), "type", env.Type)
		return nil
	}
	if w == nil {
		return nil // nil,nil — unresolvable address, nothing to mirror
	}
	return s.MirrorActivity(ctx, ev, w.ID)
}

// eventStatus maps a provider event type to the mirror status — the
// shared mapping for fresh inserts and dedupe-hit transitions.
func eventStatus(ev ProviderEvent) string {
	switch ev.Type {
	case "pending", "submitted":
		return "pending"
	case "failed":
		return "failed"
	default:
		return "confirmed"
	}
}

// activityFromEvent normalizes one provider event into the mirror row
// shape — shared by fresh inserts and the pending→terminal confirm path
// so both write identical canonical data.
func (s *Service) activityFromEvent(ctx context.Context, ev ProviderEvent, walletID string) Activity {
	kind, direction := classifyEvent(ev)
	creditsPer := s.creditsPerUSD(ctx)
	credits := int64(math.Round(float64(ev.AmountMinor) * creditsPer / 1_000_000)) // USD minor → display credits

	// Counterparty resolution: member wallet ↔ internal transfer, else NULL.
	// Re-resolved at confirm — a counterparty provisioned between the
	// pending insert and the confirm (auto-provisioned recipient) must be
	// linked, or internal transfers look external and get withdrawal-fee'd.
	var cp *string
	if cw, err := s.store.GetWalletByAddress(ctx, ev.CounterAddr); err == nil && cw != nil {
		cp = &cw.UserID
	}

	occurred := time.Unix(ev.OccurredAt, 0).UTC()
	if ev.OccurredAt <= 0 {
		occurred = time.Now().UTC() // provider omitted the timestamp — don't sort to 1970
	}
	raw, _ := json.Marshal(ev)
	return Activity{
		WalletID:           walletID,
		Direction:          direction,
		Kind:               kind,
		Asset:              normalizeAsset(ev.Asset),
		AmountMinor:        ev.AmountMinor,
		CreditsAmount:      credits,
		CounterpartyUserID: cp,
		TxSignature:        ev.TxSignature,
		Slot:               ev.Slot,
		Status:             eventStatus(ev),
		OccurredAt:         occurred,
		Raw:                raw,
	}
}

// confirmActivity lands a terminal status on a pending mirror row, writing
// the confirming event's canonical fields (a 'submitted' insert is a
// placeholder — its kind/direction/counterparty are provisional). The
// guarded WHERE status='pending' means concurrent confirms can't
// double-fire hooks: only the first transition returns true.
func (s *Service) confirmActivity(ctx context.Context, id pgtype.UUID, ev ProviderEvent, walletID string) {
	a := s.activityFromEvent(ctx, ev, walletID)
	ok, err := s.store.ConfirmActivity(ctx, id, a)
	if err != nil {
		slog.Warn("stablecoin: confirm activity failed", "id", id.String(), "error", err)
		return
	}
	if ok {
		s.postConfirmHooks(ctx, ev, walletID, a.Direction, a.Kind, a.Status)
	}
}

// MirrorActivity persists one provider event into stablecoin_activity —
// the render cache for the unified history feed. Called by
// stablecoin:sync_activity. Idempotent on (wallet_id, tx_signature).
func (s *Service) MirrorActivity(ctx context.Context, ev ProviderEvent, walletID string) error {
	if existing, err := s.store.GetActivityBySig(ctx, walletID, ev.TxSignature); err == nil {
		// Already mirrored — but a terminal event for a pending row is a
		// status transition, not a replay: a webhook that arrives after a
		// 'submitted' insert (or the offramp sweep) must still confirm it —
		// and the confirming event's kind/direction/counterparty replace the
		// placeholder values, or hooks fire on a stale kind and never run.
		if existing.Status == "pending" {
			if st := eventStatus(ev); st == "confirmed" || st == "failed" {
				var id pgtype.UUID
				if id.Scan(existing.ID) == nil {
					s.confirmActivity(ctx, id, ev, walletID)
				}
			}
		}
		return nil
	} else if !errors.Is(err, ErrActivityNotFound) {
		return fmt.Errorf("stablecoin: activity lookup: %w", err)
	}

	a := s.activityFromEvent(ctx, ev, walletID)
	inserted, err := s.store.InsertActivity(ctx, a)
	if err != nil {
		return err
	}
	if inserted == nil {
		return nil // (wallet_id, tx_signature) dedupe hit — replay, nothing to do
	}
	s.postConfirmHooks(ctx, ev, walletID, a.Direction, a.Kind, a.Status)
	return nil
}

// postConfirmHooks runs the side-effects a confirmed mirror row triggers —
// balance-cache invalidation, withdrawal fee collection, and onchain
// escrow transitions. Called on fresh inserts and on pending→confirmed
// transitions so the outcome is identical regardless of which path
// confirmed the row.
func (s *Service) postConfirmHooks(ctx context.Context, ev ProviderEvent, walletID, direction, kind, status string) {
	if status != "confirmed" {
		return
	}
	// A confirmed event changes the rendered balance — bust the mirror
	// cache or deposits/tips stay invisible until the reconcile TTL.
	var w *Wallet
	if wid, perr := pgUUID(walletID); perr == nil {
		w, _ = s.store.GetWalletByID(ctx, wid)
	}
	if w != nil {
		_ = s.vk.Del(ctx, "jc:stablecoin:mirror:"+w.UserID)
	}
	// Confirmed member-signed withdrawal → collect the platform fee via
	// the delegated signer (the send couldn't carry a fee leg).
	if direction == "out" && kind == ActivityWithdrawal && w != nil {
		s.collectWithdrawalFee(ctx, w, ev.AmountMinor, normalizeAsset(ev.Asset), ev.TxSignature, ev.CounterAddr)
	}
	// Onchain escrow transitions — the chain event is the authoritative
	// proof; the client-reported calls are the optimistic path. The event's
	// counterparty is the Squads vault — match it before falling back to
	// the wallet heuristic (multi-escrow wallets make wallet-only lookup
	// ambiguous).
	switch kind {
	case ActivityEscrowFunding:
		s.completeEscrowFunding(ctx, walletID, ev.TxSignature, ev.CounterAddr, ev.AmountMinor)
	case ActivityEscrowRelease:
		s.completeEscrowRelease(ctx, walletID, ev.TxSignature, ev.CounterAddr)
	}
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
	a = strings.ToLower(strings.TrimSpace(a))
	if a == "" {
		return AssetUSDC
	}
	// Strip a "chain:" prefix ("solana:usdc" → "usdc") but keep unknown
	// assets labeled as themselves — defaulting to usdc would mislabel
	// foreign tokens and miscount them in balances/feeds.
	if i := strings.Index(a, ":"); i >= 0 && i+1 < len(a) {
		return a[i+1:]
	}
	return a
}

// creditsPerUnit resolves credits-per-whole-unit for a currency from the
// currencies table (§58/ADR-065) — nil-safe: returns 0 when the rate seam
// is unwired or the row is absent.
func (s *Service) creditsPerUnit(ctx context.Context, code string) float64 {
	if s.creditRater == nil {
		return 0
	}
	n, err := s.creditRater.CreditsPerUnit(ctx, code)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// creditsPerUSD resolves credits-per-USD — falls back to a neutral default
// when the rate is unavailable (display math only; settlement callers must
// not rely on the fallback).
func (s *Service) creditsPerUSD(ctx context.Context) float64 {
	if n := s.creditsPerUnit(ctx, "usd"); n > 0 {
		return n
	}
	return 100 // neutral display default (real rate comes from the USD currencies row)
}

// creditsToUsdMinor converts platform credits to 6-decimal USD-token minor —
// tips are credits-denominated; onchain ops are minor-denominated.
func (s *Service) creditsToUsdMinor(ctx context.Context, credits int64) int64 {
	return int64(math.Round(float64(credits) * 1_000_000 / s.creditsPerUSD(ctx)))
}

// fiatToUsdMinor converts fiat minor (2-decimal convention, matching
// payments.amount) to USD-token minor. USD-priced bookings are an exact
// 2dp→6dp unit conversion; other currencies convert via the two-hop
// credits rate (fiat minor→units→credits→USD minor) and fail closed when
// the rate is missing — never settle escrow on a guessed rate.
func (s *Service) fiatToUsdMinor(ctx context.Context, fiatMinor int64, currency string) (int64, error) {
	code := strings.ToLower(strings.TrimSpace(currency))
	switch code {
	case "":
		return 0, fmt.Errorf("stablecoin: booking has no price currency")
	case "usd":
		return fiatMinor * 10_000, nil
	}
	// Both remaining paths convert through the USD credits rate — fail
	// closed when it is missing (escrow settlement must never guess).
	usdPer := s.creditsPerUnit(ctx, "usd")
	if usdPer <= 0 {
		return 0, fmt.Errorf("stablecoin: USD credits rate missing — cannot price escrow")
	}
	if code == "credits" {
		return int64(math.Round(float64(fiatMinor) * 1_000_000 / usdPer)), nil
	}
	fiatPer := s.creditsPerUnit(ctx, code)
	if fiatPer <= 0 {
		return 0, fmt.Errorf("stablecoin: unsupported booking currency %q", currency)
	}
	// fiat minor → units → credits → USD-token minor; single round at the end
	// keeps fractional weak-currency rates (NGN) from truncating twice. The
	// divisor is the ISO minor-unit exponent (JPY 0, KWD 3) — not a fixed 100.
	div := math.Pow10(wallet.CurrencyExponent(code))
	credits := float64(fiatMinor) * fiatPer / div
	return int64(math.Round(credits * 1_000_000 / usdPer)), nil
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
	// Page the active set — a single 500-row fetch would silently drop
	// every wallet past the cap on each cron tick. Unbounded: the page loop
	// ends on a short page, never on a wallet-count cap.
	const pageSize = 500
	var all []Wallet
	for off := int32(0); ; off += pageSize {
		page, err := s.store.ListWalletsByStatus(ctx, WalletStatusActive, pageSize, off)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
	}
	return all, nil
}

// reconcileWallet pulls provider activity for one wallet (paged), mirrors
// gaps, and rewrites the balance mirror cache from chain-reported balances.
func (s *Service) reconcileWallet(ctx context.Context, w Wallet) error {
	cursor := ""
	for page := 0; page < 10; page++ { // bounded — a runaway cursor loop can't wedge the cron tick
		events, next, err := s.provider.Activity(ctx, w.Locator, cursor)
		if err != nil {
			return fmt.Errorf("provider activity: %w", err)
		}
		for _, ev := range events {
			if err := s.MirrorActivity(ctx, ev, w.ID); err != nil {
				slog.Warn("stablecoin: reconcile mirror failed", "wallet_id", w.ID, "sig", ev.TxSignature, "error", err)
			}
		}
		if next == "" || len(events) == 0 {
			break
		}
		cursor = next
	}

	// Balance mirror: chain balances are truth (the activity mirror can
	// lag). Convert raw minor to display credits via the currencies rate.
	if pw, err := s.provider.GetWallet(ctx, w.Locator); err == nil && len(pw.Balances) > 0 {
		creditsPer := s.creditsPerUSD(ctx)
		var bal int64
		for asset, minor := range pw.Balances {
			if asset != "usdc" && asset != "usdt" {
				continue
			}
			bal += int64(math.Round(float64(minor) * creditsPer / 1_000_000))
		}
		if err := s.vk.Set(ctx, "jc:stablecoin:mirror:"+w.UserID, strconv.FormatInt(bal, 10), 3600); err != nil {
			return fmt.Errorf("mirror cache: %w", err)
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
	ts := e.OccurredAt
	if ts > 1_000_000_000_000 { // provider may send ms — same guard as the poll path
		ts /= 1000
	}
	return ProviderEvent{
		Type:        e.Type,
		TxSignature: sig,
		Slot:        e.Slot,
		Asset:       e.Asset,
		AmountMinor: amount,
		CounterAddr: e.CounterAddr,
		OccurredAt:  ts,
		Raw:         nil,
	}
}
