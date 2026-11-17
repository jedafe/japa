package stablecoin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// WalletSummary is the member-facing wallet view (GET /me/stablecoin-wallet).
// Credits-denominated — members never see USDC/USDT/Solana terminology.
type WalletSummary struct {
	Address             string         `json:"address"`
	Status              string         `json:"status"`
	RecoveryMethods     []string       `json:"recovery_methods"`
	Balance             BalanceSummary `json:"balance"`
	NeedsSignerRotation bool           `json:"needs_signer_rotation"`
	MinDepositMinor     int64          `json:"min_deposit_minor"`
}

// GetWalletSummary composes the member wallet view: backed tier from the
// onchain mirror (reconcile-maintained Valkey cache, activity-sum
// fallback), bonus tier from the §28 credits ledger seam.
func (s *Service) GetWalletSummary(ctx context.Context, userID pgtype.UUID) (*WalletSummary, error) {
	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: wallet: %w", err)
	}
	if w == nil {
		return nil, nil // not provisioned yet — client triggers lazy provisioning
	}

	sum := &WalletSummary{
		Address:         w.Address,
		Status:          w.Status,
		RecoveryMethods: w.RecoveryMethods,
	}

	// Backed tier: the reconcile task maintains
	// jc:stablecoin:mirror:{user_id} = backed-credits balance. Cache miss →
	// derive from confirmed mirror rows (chain is truth; both are render).
	// A cached zero is a real balance, not a miss — only fall back when the
	// key is absent or unparseable.
	cacheHit := false
	if v, err := s.vk.Get(ctx, "jc:stablecoin:mirror:"+userID.String()); err == nil && v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			sum.Balance.BackedCredits = n
			cacheHit = true
		}
	}
	if !cacheHit {
		if wid, perr := pgUUID(w.ID); perr == nil {
			// SQL aggregates — a wallet past the old 500-row fetch cap
			// derived a truncated, wrong balance.
			if n, aerr := s.store.SumBackedCredits(ctx, wid); aerr == nil {
				sum.Balance.BackedCredits = n
			}
			// Pending means incoming-unconfirmed — an outbound pending
			// row is still the member's money, not a pending credit.
			if n, aerr := s.store.SumPendingInCredits(ctx, wid); aerr == nil {
				sum.Balance.Pending = n
			}
		}
	}
	if sum.Balance.BackedCredits < 0 {
		sum.Balance.BackedCredits = 0
	}
	if sum.Balance.Pending < 0 {
		sum.Balance.Pending = 0
	}

	// Bonus tier: latest credits-ledger balance via the wallet seam.
	if s.bonusLedger != nil {
		if rows, err := s.bonusLedger.ListCreditEntries(ctx, userID, 1, 0); err == nil && len(rows) > 0 {
			sum.Balance.BonusCredits = rows[0].Balance
		}
	}
	// Rotation cue: a scope inside the renew window means the member must
	// re-run the signer ceremony before delegation halts (§128.2 — scopes
	// are immutable; renewal is rotation, not extension).
	window := time.Duration(s.settings.GetIntOr(ctx, "stablecoin.signer_renew_window_days", 30)) * 24 * time.Hour
	sum.NeedsSignerRotation = signerExpiringWithin(w, time.Now().Add(window))
	sum.MinDepositMinor = int64(s.settings.GetIntOr(ctx, "stablecoin.min_deposit_minor", 1_000_000))
	return sum, nil
}

// RequestProvision enqueues stablecoin:provision_wallet — the lazy
// bootstrap path used by GET /me/stablecoin-wallet (first wallet-surface
// visit) and POST /me/stablecoin-wallet/provision. Fire-and-forget (Rule
// 8): the client re-reads the summary until a wallet appears. MemberAuth
// is optional — the worker provision creates the counterfactual wallet;
// the delegated signer only registers when the member runs the explicit
// activation ceremony (POST /me/stablecoin-wallet/signer).
func (s *Service) RequestProvision(ctx context.Context, userID pgtype.UUID, memberAuth string) error {
	if s.enq == nil {
		return s.provisionInline(ctx, userID, memberAuth)
	}
	payload, _ := json.Marshal(ProvisionWalletPayload{UserID: userID.String(), MemberAuth: memberAuth})
	if err := s.enq.Enqueue(TaskProvisionWallet, payload); err != nil {
		slog.Warn("stablecoin: provision enqueue failed, provisioning inline", "error", err)
		return s.provisionInline(ctx, userID, memberAuth)
	}
	return nil
}

// provisionInline is the enqueue-failure fallback — synchronous so the
// caller still gets a wallet rather than a silent drop.
func (s *Service) provisionInline(ctx context.Context, userID pgtype.UUID, memberAuth string) error {
	_, err := s.ProvisionWallet(ctx, userID, memberAuth)
	return err
}

// ActivateSigner runs the member's delegated-signer ceremony — registers
// Japa.to's scope-capped operational signer (memberAuth carries the
// member's recovery-approval proof), or rotates it when a prior
// registration exists. Member-initiated only: the platform can never
// install a spend signer unilaterally (§128.2).
func (s *Service) ActivateSigner(ctx context.Context, userID pgtype.UUID, memberAuth string) (*Wallet, error) {
	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: wallet: %w", err)
	}
	if w == nil {
		return nil, ErrWalletNotFound
	}
	if w.Status == WalletStatusSuspended {
		return nil, ErrWalletSuspended
	}
	if w.DelegatedSigner != nil && w.DelegatedSigner.SignerRef != "" {
		return w, s.RotateSignerScopes(ctx, w, memberAuth)
	}
	return w, s.registerDelegatedSigner(ctx, w, memberAuth)
}

// ProvisionWallet lazily creates a member's smart wallet — called on signup
// completion or first wallet-surface visit (stablecoin:provision_wallet).
// The address is counterfactual: assigned now, deployed on first tx, so
// dormant wallets cost nothing.
func (s *Service) ProvisionWallet(ctx context.Context, userID pgtype.UUID, memberAuth string) (*Wallet, error) {
	if existing, err := s.store.GetWalletByUser(ctx, userID); err != nil && !errors.Is(err, ErrWalletNotFound) {
		return nil, fmt.Errorf("stablecoin: wallet lookup: %w", err)
	} else if existing != nil {
		return existing, nil
	}

	pw, err := s.provider.CreateWallet(ctx, CreateWalletInput{
		UserID:          userID.String(),
		Chain:           s.settings.GetStringOr(ctx, "stablecoin.network", "solana"),
		RecoveryMethods: []string{"email", "phone"},
		MemberAuth:      memberAuth,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: create wallet: %w", err)
	}

	w, err := s.store.InsertWallet(ctx, Wallet{
		UserID:  userID.String(),
		Chain:   pw.Chain,
		Address: pw.Address,
		Locator: pw.Locator,
		Status:  WalletStatusProvisioned,
	})
	if err != nil {
		// Concurrent provision for the same user — the other caller won the
		// insert. Re-read and return the persisted row rather than failing;
		// the duplicate provider wallet stays linked to this member and is
		// unused (counterfactual addresses cost nothing until deployed).
		if existing, lerr := s.store.GetWalletByUser(ctx, userID); lerr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("stablecoin: persist wallet: %w", err)
	}

	// The delegated signer is NOT registered here — installation requires
	// the member's recovery-approval ceremony, which only the member can
	// initiate (POST /me/stablecoin-wallet/signer → ActivateSigner). The
	// wallet stays `provisioned` until then; passkey-signed deposits and
	// withdrawals work without it.
	return w, nil
}

// registerDelegatedSigner registers Japa.to's scope-capped operational
// signer and persists the registration state on the wallet row. memberAuth
// carries the member's approval proof for the provider ceremony — an
// empty value defers to the provider's own approval flow.
func (s *Service) registerDelegatedSigner(ctx context.Context, w *Wallet, memberAuth string) error {
	signerRef := s.settings.GetStringOr(ctx, "stablecoin.delegated_signer_ref", "")
	if signerRef == "" {
		return fmt.Errorf("stablecoin.delegated_signer_ref unset")
	}
	dailyCap := s.settings.GetIntOr(ctx, "stablecoin.signer_daily_cap_usd_minor", 50_000_000)
	scopeDays := s.settings.GetIntOr(ctx, "stablecoin.signer_scope_expiry_days", 365)
	expiresAt := time.Now().Add(time.Duration(scopeDays) * 24 * time.Hour)

	ds := &DelegatedSigner{
		SignerRef:    signerRef,
		RegisteredAt: time.Now(),
	}
	for _, asset := range s.assetList(ctx) {
		ds.Scopes = append(ds.Scopes, SignerScope{
			Asset:         asset,
			DailyCapMinor: int64(dailyCap),
			ExpiresAt:     expiresAt,
		})
	}

	if err := s.provider.RegisterDelegatedSigner(ctx, w.Locator, DelegatedSignerInput{
		SignerRef:  signerRef,
		Scopes:     ds.Scopes,
		MemberAuth: memberAuth,
	}); err != nil {
		return err
	}

	wid, err := pgUUID(w.ID)
	if err != nil {
		return err
	}
	if err := s.store.UpdateDelegatedSigner(ctx, wid, ds); err != nil {
		return fmt.Errorf("stablecoin: persist signer: %w", err)
	}
	// Signer registered → wallet is fully operational.
	if err := s.store.UpdateWalletStatus(ctx, wid, WalletStatusActive); err != nil {
		return fmt.Errorf("stablecoin: activate wallet: %w", err)
	}
	return nil
}

// assetList reads stablecoin.assets — the settlement SPL mints. Catalog
// type is json (["usdc","usdt"]); the settings seam returns the raw value,
// so decode JSON first and tolerate a CSV-encoded value.
func (s *Service) assetList(ctx context.Context) []string {
	raw := s.settings.GetStringOr(ctx, "stablecoin.assets", "")
	out := []string{}
	if raw != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			parsed = strings.Split(raw, ",")
		}
		for _, a := range parsed {
			a = strings.ToLower(strings.TrimSpace(strings.Trim(a, `"`)))
			if a == AssetUSDC || a == AssetUSDT {
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		out = []string{AssetUSDC, AssetUSDT}
	}
	return out
}
