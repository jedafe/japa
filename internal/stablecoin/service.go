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
	ListWalletsByStatus(ctx context.Context, status string, limit int32) ([]Wallet, error)
	ListWallets(ctx context.Context, status, q string, limit, offset int32) ([]Wallet, error)
	InsertActivity(ctx context.Context, a Activity) (*Activity, error)
	GetActivityBySig(ctx context.Context, sig string) (*Activity, error)
	ListActivityByWallet(ctx context.Context, walletID pgtype.UUID, limit, offset int32) ([]Activity, error)
	GetWalletByID(ctx context.Context, id pgtype.UUID) (*Wallet, error)
	ListPendingActivity(ctx context.Context, limit int32) ([]Activity, error)
	SetActivityStatus(ctx context.Context, id pgtype.UUID, status string) error
	SumOutByAssetSince(ctx context.Context, walletID pgtype.UUID, asset string, since time.Time) (int64, error)
	GetEscrow(ctx context.Context, id pgtype.UUID) (*Escrow, error)
	GetEscrowByBooking(ctx context.Context, bookingID pgtype.UUID) (*Escrow, error)
	InsertEscrow(ctx context.Context, e Escrow) (*Escrow, error)
	SetEscrowState(ctx context.Context, id pgtype.UUID, state, fundedSig, releaseSig string, decision *ArbiterDecision, deadline *time.Time) error
	SetEscrowVaultAddress(ctx context.Context, id pgtype.UUID, addr string) error
	ListExpiredFundedEscrows(ctx context.Context, limit int32) ([]Escrow, error)
	GetServiceBooking(ctx context.Context, id pgtype.UUID) (*ServiceBooking, error)
	ListTreasuryWallets(ctx context.Context) ([]TreasuryWallet, error)
	GetTreasuryWallet(ctx context.Context, chain, role string) (*TreasuryWallet, error)
	InsertTreasurySweep(ctx context.Context, sw TreasurySweep) (*TreasurySweep, error)
	GetTreasurySweep(ctx context.Context, id pgtype.UUID) (*TreasurySweep, error)
	DecideTreasurySweep(ctx context.Context, id pgtype.UUID, state string, approvedBy pgtype.UUID, squadsProposalID string) error
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

// tipReader is the read seam into §125 tips for tips:settle_stablecoin —
// the stablecoin module resolves the tip's parties and net amount, then
// settles via delegated signer (tip−fee → recipient, fee → treasury hot).
type tipReader interface {
	GetTip(ctx context.Context, id pgtype.UUID) (*Tip, error)
}

// Tip is the settlement-relevant slice of a §125 tip row.
type Tip struct {
	ID              pgtype.UUID
	SenderUserID    pgtype.UUID
	RecipientUserID pgtype.UUID
	NetCredits      int64 // tip − platform fee → recipient leg
	FeeAmount       int64 // platform fee → treasury hot leg
	Status          string
}

// creditRater resolves the USD→credits display rate from the §58
// currencies table — credits_per_unit lives per-currency there, not in
// system_settings.
type creditRater interface {
	CreditsPerUSD(ctx context.Context) (int64, error)
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
	TotalPrice     int64
	FeeAmount      int64
	Status         string
}

// TreasurySweep is one hot→cold sweep proposal (treasury_sweeps).
type TreasurySweep struct {
	ID               string
	Chain            string
	Asset            string
	AmountMinor      int64
	HotWalletID      string
	ColdAddress      string
	State            string // proposed | approved | executed | rejected | expired
	ProposedBy       string
	ApprovedBy       string
	SquadsProposalID string
	ExecutedSig      string
}

// Service is the stablecoin domain entry point.
type Service struct {
	provider    Provider
	store       store
	settings    settings
	vk          valkey
	enq         enqueuer
	bonusLedger bonusLedger
	tipReader   tipReader
	creditRater creditRater
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

// killswitch reports whether delegated-signer ops are halted platform-wide
// (jc:stablecoin:signer_killswitch — incident response for §128.10).
func (s *Service) killswitch(ctx context.Context) (bool, error) {
	return s.vk.Exists(ctx, "jc:stablecoin:signer_killswitch")
}

// SetKillswitch flips the delegated-signer halt flag (super_admin only,
// api-routes-catalog). It never blocks self-custody — member passkey-signed
// ops bypass the delegated path entirely.
func (s *Service) SetKillswitch(ctx context.Context, engaged bool) error {
	if engaged {
		return s.vk.Set(ctx, "jc:stablecoin:signer_killswitch", "1", 0)
	}
	return s.vk.Del(ctx, "jc:stablecoin:signer_killswitch")
}
