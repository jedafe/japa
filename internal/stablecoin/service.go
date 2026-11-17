package stablecoin

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// settings is the hot-reloadable system_settings reader (Rule 3 — every
// tunable comes from here, never hardcoded).
type settings interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
}

// settingsWriter is the optional write half — used only by the killswitch
// so the halt flag survives a Valkey flush (incident controls must not
// silently disengage on a cache restart). Nil in tests.
type settingsWriter interface {
	SetSetting(ctx context.Context, key, value string) error
}

// auditor records privileged money-movement control actions to
// audit_logs (admin killswitch toggles). Nil in tests — logging is
// best-effort and never blocks the control itself.
type auditor interface {
	LogAction(ctx context.Context, actorUserID pgtype.UUID, action, targetType string, metadata map[string]interface{})
}

// valkey is the cache/flag client. Keys used (all in the catalog):
//
//	jc:stablecoin:mirror:{user_id}          — display-balance cache
//	jc:stablecoin:webhook:dedupe:{sig}      — provider-event dedup
//	jc:stablecoin:idempotency:{key}         — op idempotency (SET NX, Rule 9)
//	jc:stablecoin:signer_killswitch         — halts all delegated-signer ops
type valkey interface {
	Exists(ctx context.Context, key string) (bool, error)
	SetNX(ctx context.Context, key string, val string, ttlSeconds int) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val string, ttlSeconds int) error
	Del(ctx context.Context, key string) error
}

// store is the persistence seam — implemented over sqlc queries on the
// §128 tables (stablecoin_wallets, stablecoin_activity, service_escrows,
// treasury_wallets, treasury_sweeps).
type store interface {
	GetWalletByUser(ctx context.Context, userID pgtype.UUID) (*Wallet, error)
	GetWalletByAddress(ctx context.Context, address string) (*Wallet, error)
	InsertWallet(ctx context.Context, w Wallet) (*Wallet, error)
	UpdateWalletStatus(ctx context.Context, id pgtype.UUID, status string) error
	UpdateDelegatedSigner(ctx context.Context, id pgtype.UUID, ds *DelegatedSigner) error
	ListWalletsByStatus(ctx context.Context, status string, limit, offset int32) ([]Wallet, error)
	ListWallets(ctx context.Context, status, q string, limit, offset int32) ([]Wallet, error)
	InsertActivity(ctx context.Context, a Activity) (*Activity, error)
	GetActivityBySig(ctx context.Context, walletID, sig string) (*Activity, error)
	ListActivityByWallet(ctx context.Context, walletID pgtype.UUID, limit, offset int32) ([]Activity, error)
	GetWalletByID(ctx context.Context, id pgtype.UUID) (*Wallet, error)
	ListPendingActivity(ctx context.Context, limit int32) ([]Activity, error)
	ConfirmActivity(ctx context.Context, id pgtype.UUID, a Activity) (bool, error)
	SumOutByAssetSince(ctx context.Context, walletID pgtype.UUID, asset string, since time.Time) (int64, error)
	SumBackedCredits(ctx context.Context, walletID pgtype.UUID) (int64, error)
	SumPendingInCredits(ctx context.Context, walletID pgtype.UUID) (int64, error)
	GetEscrow(ctx context.Context, id pgtype.UUID) (*Escrow, error)
	GetEscrowByBooking(ctx context.Context, bookingID pgtype.UUID) (*Escrow, error)
	GetUnambiguousLiveEscrowByWallet(ctx context.Context, walletID pgtype.UUID) (*Escrow, error)
	GetEscrowByVault(ctx context.Context, vaultAddr string) (*Escrow, error)
	InsertEscrow(ctx context.Context, e Escrow) (*Escrow, error)
	SetEscrowState(ctx context.Context, id pgtype.UUID, state, fundedSig, releaseSig string, decision *ArbiterDecision, deadline *time.Time) error
	SetEscrowVaultAddress(ctx context.Context, id pgtype.UUID, addr string) error
	ListExpiredFundedEscrows(ctx context.Context, limit int32) ([]Escrow, error)
	GetServiceBooking(ctx context.Context, id pgtype.UUID) (*ServiceBooking, error)
	ListTreasuryWallets(ctx context.Context) ([]TreasuryWallet, error)
	GetTreasuryWallet(ctx context.Context, chain, role string) (*TreasuryWallet, error)
	GetTreasuryWalletByAddress(ctx context.Context, chain, address string) (*TreasuryWallet, error)
	InsertTreasurySweep(ctx context.Context, sw TreasurySweep) (*TreasurySweep, error)
	GetTreasurySweep(ctx context.Context, id pgtype.UUID) (*TreasurySweep, error)
	GetApprovedSweepForTransfer(ctx context.Context, chain, asset, hotWalletID string, amountMinor int64) (*TreasurySweep, error)
	DecideTreasurySweep(ctx context.Context, id pgtype.UUID, state string, approvedBy pgtype.UUID, squadsProposalID string) error
	ExpireStaleSweeps(ctx context.Context, cutoff time.Time) (int64, error)
	MarkSweepExecuted(ctx context.Context, id pgtype.UUID, sig string) error
	ListSweepsByState(ctx context.Context, state string, limit, offset int32) ([]TreasurySweep, error)
}

// enqueuer is the Asynq seam (Rule 8 — webhook ingestion is fan-out).
type enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// bonusLedger is the read seam into the §28 credits ledger — the unified
// feed merges onchain mirror rows with bonus-credit entries.
type bonusLedger interface {
	ListCreditEntries(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]BonusEntry, error)
}

// tipReader is the read+write seam into §125 tips for
// tips:settle_stablecoin — the stablecoin module resolves the tip's
// parties and net amount, settles via delegated signer (tip−fee →
// recipient, fee → treasury hot), then closes the tip's lifecycle so a
// settled tip can't be double-processed or swept as stale.
type tipReader interface {
	GetTip(ctx context.Context, id pgtype.UUID) (*Tip, error)
	// ClaimTipSettle transitions pending/failed → settling before money
	// moves; false means the tip already reached a terminal state.
	ClaimTipSettle(ctx context.Context, id pgtype.UUID) (bool, error)
	MarkSettled(ctx context.Context, id pgtype.UUID, txSig string) error
}

// Tip is the settlement-relevant slice of a §125 tip row.
type Tip struct {
	ID              pgtype.UUID
	SenderUserID    pgtype.UUID
	RecipientUserID pgtype.UUID
	NetCredits      int64 // tip − platform fee → recipient leg
	FeeAmount       int64 // platform fee → treasury hot leg
	Method          string
	Status          string
}

// creditRater resolves the USD→credits display rate from the §58
// currencies table — credits_per_unit lives per-currency there, not in
// system_settings.
type creditRater interface {
	// CreditsPerUnit returns credits per ONE WHOLE unit of the currency —
	// numeric: weak currencies may price below one credit per unit (§58/001185).
	// (§58: USD row 100 → $1 = 100 credits = $0.01/credit.)
	CreditsPerUnit(ctx context.Context, currencyCode string) (float64, error)
}

// BonusEntry is one bonus-credit ledger row for the unified feed.
type BonusEntry struct {
	ID         string
	Operation  string // earn | spend | refund | adjust
	Amount     int64
	Balance    int64
	Source     string
	Reference  string
	OccurredAt time.Time
}

// ServiceBooking is the escrow-relevant slice of service_bookings (§111).
type ServiceBooking struct {
	ID             string
	BookerUserID   string
	ProviderUserID string
	TotalPrice     int64 // fiat minor units (payments.amount convention)
	FeeAmount      int64
	FeeCurrency    string // ISO code or 'credits'
	Status         string
}

// TreasurySweep is one hot→cold sweep proposal (treasury_sweeps).
type TreasurySweep struct {
	ID               string `json:"id"`
	Chain            string `json:"chain"`
	Asset            string `json:"asset"`
	AmountMinor      int64  `json:"amount_minor"`
	HotWalletID      string `json:"hot_wallet_id"`
	ColdAddress      string `json:"cold_address"`
	State            string `json:"state"` // proposed | approved | executed | rejected | expired
	ProposedBy       string `json:"proposed_by"`
	ApprovedBy       string `json:"approved_by"`
	SquadsProposalID string `json:"squads_proposal_id"`
	ExecutedSig      string `json:"executed_sig"`
}

// Service is the stablecoin domain entry point.
type Service struct {
	provider    Provider
	store       store
	settings    settings
	settingsW   settingsWriter
	vk          valkey
	enq         enqueuer
	bonusLedger bonusLedger
	tipReader   tipReader
	creditRater creditRater
	auditor     auditor
}

// NewService wires the stablecoin module.
func NewService(p Provider, s store, st settings, vk valkey) *Service {
	return &Service{provider: p, store: s, settings: st, vk: vk}
}

// SetEnqueuer injects the Asynq enqueuer for webhook fan-out (Rule 8).
func (s *Service) SetEnqueuer(e enqueuer) {
	s.enq = e
}

// SetBonusLedger injects the §28 credits-ledger read seam for the unified
// activity feed.
func (s *Service) SetBonusLedger(b bonusLedger) {
	s.bonusLedger = b
}

// SetTipReader injects the §125 tip read seam for tips:settle_stablecoin.
func (s *Service) SetTipReader(t tipReader) {
	s.tipReader = t
}

// SetCreditRater injects the §58 currencies-table rate seam — display
// conversion for the unified feed and wallet summary.
func (s *Service) SetCreditRater(r creditRater) {
	s.creditRater = r
}

// SetSettingsWriter injects the system_settings write half so the
// killswitch survives a Valkey flush.
func (s *Service) SetSettingsWriter(w settingsWriter) {
	s.settingsW = w
}

// SetAuditor injects the audit_logs sink for privileged control actions.
func (s *Service) SetAuditor(a auditor) {
	s.auditor = a
}

// killswitch reports whether delegated-signer ops are halted platform-wide
// (jc:stablecoin:signer_killswitch — incident response for §128.10). The
// Valkey flag is the fast path; the mirrored system_settings row is the
// durable source consulted on cache miss so a flush can't silently
// disengage a halt.
func (s *Service) killswitch(ctx context.Context) (bool, error) {
	on, err := s.vk.Exists(ctx, "jc:stablecoin:signer_killswitch")
	if err != nil || on {
		return on, err
	}
	return s.settings.GetBoolOr(ctx, "stablecoin.signer_killswitch", false), nil
}

// SetKillswitch flips the delegated-signer halt flag (super_admin only,
// api-routes-catalog). It never blocks self-custody — member passkey-signed
// ops bypass the delegated path entirely. Writes the durable setting first,
// then the Valkey fast flag; audits the toggle when an actor is provided.
func (s *Service) SetKillswitch(ctx context.Context, actor pgtype.UUID, engaged bool) error {
	if s.settingsW != nil {
		v := "false"
		if engaged {
			v = "true"
		}
		if err := s.settingsW.SetSetting(ctx, "stablecoin.signer_killswitch", v); err != nil {
			return err
		}
	}
	var err error
	if engaged {
		err = s.vk.Set(ctx, "jc:stablecoin:signer_killswitch", "1", 0)
	} else {
		err = s.vk.Del(ctx, "jc:stablecoin:signer_killswitch")
	}
	if err != nil {
		return err
	}
	if s.auditor != nil && actor.Valid {
		s.auditor.LogAction(ctx, actor, "stablecoin.signer_killswitch", "system_settings",
			map[string]interface{}{"engaged": engaged})
	}
	return nil
}
