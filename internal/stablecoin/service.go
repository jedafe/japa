package stablecoin

import (
	"context"

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
}

// store is the persistence seam — implemented over sqlc queries once the
// §128 migration lands (stablecoin_wallets, stablecoin_activity,
// service_escrows, treasury_wallets).
type store interface {
	GetWalletByUser(ctx context.Context, userID pgtype.UUID) (*Wallet, error)
	InsertWallet(ctx context.Context, w Wallet) (*Wallet, error)
	UpdateWalletStatus(ctx context.Context, id pgtype.UUID, status string) error
	InsertActivity(ctx context.Context, a Activity) (*Activity, error)
	GetActivityBySig(ctx context.Context, sig string) (*Activity, error)
	GetEscrow(ctx context.Context, id pgtype.UUID) (*Escrow, error)
	ListTreasuryWallets(ctx context.Context) ([]TreasuryWallet, error)
}

// Service is the stablecoin domain entry point.
type Service struct {
	provider Provider
	store    store
	settings settings
	vk       valkey
}

// NewService wires the stablecoin module.
func NewService(p Provider, s store, st settings, vk valkey) *Service {
	return &Service{provider: p, store: s, settings: st, vk: vk}
}

// killswitch reports whether delegated-signer ops are halted platform-wide
// (jc:stablecoin:signer_killswitch — incident response for §128.10).
func (s *Service) killswitch(ctx context.Context) (bool, error) {
	return s.vk.Exists(ctx, "jc:stablecoin:signer_killswitch")
}
