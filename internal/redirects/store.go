package redirects

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store is an exported type.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore is an exported function.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

func (s *Store) listRedirects(ctx context.Context, tenantID pgtype.UUID, isActive *bool, search, sort, dir string, limit, offset int32) ([]gen.Redirect, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListRedirects(ctx, gen.ListRedirectsParams{
		IsActive: isActive, Search: search,
		Sort: sort, Dir: dir,
		PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing redirects: %w", err)
	}

	return rows, nil
}

func (s *Store) countRedirects(ctx context.Context, tenantID pgtype.UUID, isActive *bool, search string) (int32, error) {
	count, err := s.read.CountRedirects(ctx, gen.CountRedirectsParams{
		IsActive: isActive, Search: search,
	})
	if err != nil {
		return 0, fmt.Errorf("counting redirects: %w", err)
	}

	return count, nil
}

func (s *Store) getRedirect(ctx context.Context, id, tenantID pgtype.UUID) (gen.Redirect, error) {
	row, err := s.read.GetRedirect(ctx, id)
	if err != nil {
		return gen.Redirect{}, fmt.Errorf("getting redirect: %w", err)
	}

	return row, nil
}

func (s *Store) getRedirectByPath(ctx context.Context, tenantID pgtype.UUID, sourcePath string) (gen.Redirect, error) {
	row, err := s.read.GetRedirectByPath(ctx,
		sourcePath,
	)
	if err != nil {
		return gen.Redirect{}, fmt.Errorf("getting redirect by path: %w", err)
	}

	return row, nil
}

func (s *Store) createRedirect(ctx context.Context, p gen.CreateRedirectParams) (gen.Redirect, error) {
	row, err := s.write.CreateRedirect(ctx, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Redirect{}, fmt.Errorf("creating redirect: %w", ErrDuplicateSourcePath)
		}

		return gen.Redirect{}, fmt.Errorf("creating redirect: %w", err)
	}

	return row, nil
}

func (s *Store) updateRedirect(ctx context.Context, p gen.UpdateRedirectParams) (gen.Redirect, error) {
	row, err := s.write.UpdateRedirect(ctx, p)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Redirect{}, fmt.Errorf("updating redirect: %w", ErrDuplicateSourcePath)
		}

		return gen.Redirect{}, fmt.Errorf("updating redirect: %w", err)
	}

	return row, nil
}

func (s *Store) deleteRedirect(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteRedirect(ctx, id); err != nil {
		return fmt.Errorf("deleting redirect: %w", err)
	}

	return nil
}

func (s *Store) bulkUpdateRedirectStatus(ctx context.Context, tenantID pgtype.UUID, isActive bool, ids []pgtype.UUID) error {
	if err := s.write.BulkUpdateRedirectStatus(ctx, gen.BulkUpdateRedirectStatusParams{
		IsActive: isActive, Ids: ids,
	}); err != nil {
		return fmt.Errorf("bulk updating redirect status: %w", err)
	}

	return nil
}

func (s *Store) bulkDeleteRedirects(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID) error {
	if err := s.write.BulkDeleteRedirects(ctx,
		ids,
	); err != nil {
		return fmt.Errorf("bulk deleting redirects: %w", err)
	}

	return nil
}

func (s *Store) getRedirectsByIDs(ctx context.Context, tenantID pgtype.UUID, ids []pgtype.UUID) ([]gen.Redirect, error) {
	rows, err := s.read.GetRedirectsByIDs(ctx,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("getting redirects by ids: %w", err)
	}

	return rows, nil
}
