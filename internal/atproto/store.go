package atproto

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// store is the package's data-access seam over sqlc queries (same
// pattern as internal/federation.Store).
type store struct {
	q *gen.Queries
}

func newStore(q *gen.Queries) *store { return &store{q: q} }

func (s *store) accountByHandle(ctx context.Context, handle string) (gen.AtprotoAccount, error) {
	return s.q.GetAtprotoAccountByHandle(ctx, handle)
}

func (s *store) accountByUserID(ctx context.Context, userID pgtype.UUID) (gen.AtprotoAccount, error) {
	return s.q.GetAtprotoAccountByUserID(ctx, userID)
}

func (s *store) createAccount(ctx context.Context, p gen.CreateAtprotoAccountParams) (gen.AtprotoAccount, error) {
	return s.q.CreateAtprotoAccount(ctx, p)
}

func (s *store) setAccountStatus(ctx context.Context, id pgtype.UUID, status string) error {
	return s.q.UpdateAtprotoAccountStatus(ctx, gen.UpdateAtprotoAccountStatusParams{ID: id, Status: status})
}

func (s *store) listAccounts(ctx context.Context, q string, limit, offset int32) ([]gen.ListAtprotoAccountsRow, error) {
	return s.q.ListAtprotoAccounts(ctx, gen.ListAtprotoAccountsParams{
		Q: q, PageLimit: limit, PageOffset: offset,
	})
}

func (s *store) countAccounts(ctx context.Context, q string) (int64, error) {
	return s.q.CountAtprotoAccounts(ctx, q)
}

func (s *store) stats(ctx context.Context) (gen.GetAtprotoStatsRow, error) {
	return s.q.GetAtprotoStats(ctx)
}

func (s *store) userByID(ctx context.Context, userID pgtype.UUID) (gen.GetUserByIDRow, error) {
	return s.q.GetUserByID(ctx, userID)
}
