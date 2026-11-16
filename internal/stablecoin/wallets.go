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
	Address         string         `json:"address"`
	Status          string         `json:"status"`
	RecoveryMethods []string       `json:"recovery_methods"`
	Balance         BalanceSummary `json:"balance"`
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
	if v, err := s.vk.Get(ctx, "jc:stablecoin:mirror:"+userID.String()); err == nil && v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			sum.Balance.BackedCredits = n
		}
	}
	if sum.Balance.BackedCredits == 0 {
		if wid, perr := pgUUID(w.ID); perr == nil {
			if rows, aerr := s.store.ListActivityByWallet(ctx, wid, 500, 0); aerr == nil {
				for _, a := range rows {
					switch {
					case a.Status == "pending":
						sum.Balance.Pending += a.CreditsAmount
					case a.Status != "confirmed":
						continue
					case a.Direction == "in":
						sum.Balance.BackedCredits += a.CreditsAmount
					default:
						sum.Balance.BackedCredits -= a.CreditsAmount
					}
				}
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
	return sum, nil
}

// ProvisionWallet lazily creates a member's smart wallet — called on signup
// completion or first wallet-surface visit (stablecoin:provision_wallet).
// The address is counterfactual: assigned now, deployed on first tx, so
// dormant wallets cost nothing.
func (s *Service) ProvisionWallet(ctx context.Context, userID pgtype.UUID, memberAuth string) (*Wallet, error) {
	if existing, err := s.store.GetWalletByUser(ctx, userID); err != nil && !errors.Is(err, ErrWalletNotFound) {
		return nil, fmt.Errorf("stablecoin: wallet lookup: %w", err)
	} else if existing != nil {
		// Idempotent — but a wallet stuck at provisioned with no signer
		// retries registration here (the deferred path below).
		if existing.Status == WalletStatusProvisioned && existing.DelegatedSigner == nil {
			if rerr := s.registerDelegatedSigner(ctx, existing); rerr != nil {
				slog.Warn("stablecoin: delegated signer retry deferred", "wallet_id", existing.ID, "error", rerr)
			}
		}
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

	// Register the scoped delegated signer — powers in-app spending without
	// per-action signatures. Scope values are the blast-radius ceiling.
	// Registration itself needs member recovery approval (OTP ceremony);
	// scopes are immutable — expiry forces rotation, not renewal (§128.2).
	if err := s.registerDelegatedSigner(ctx, w); err != nil {
		// Wallet exists; signer registration retries on the next provision
		// pass — non-fatal, the member can still deposit/withdraw by passkey.
		slog.Warn("stablecoin: delegated signer registration deferred",
			"wallet_id", w.ID, "error", err)
	}
	return w, nil
}

// registerDelegatedSigner registers Japa.to's scope-capped operational
// signer and persists the registration state on the wallet row.
func (s *Service) registerDelegatedSigner(ctx context.Context, w *Wallet) error {
	signerRef := s.settings.GetStringOr(ctx, "stablecoin.delegated_signer_ref", "")
	if signerRef == "" {
		return fmt.Errorf("stablecoin.delegated_signer_ref unset")
	}
	dailyCap := s.settings.GetIntOr(ctx, "stablecoin.signer_daily_cap_usd_minor", 5000)
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
		SignerRef: signerRef,
		Scopes:    ds.Scopes,
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
