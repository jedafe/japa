package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db/gen"
)

// NewQueries creates a new sqlc Queries instance from a pgxpool.Pool.
func NewQueries(pool *pgxpool.Pool) *gen.Queries {
	return gen.New(pool)
}

// WithTx executes fn within a transaction on the given pool.
// If fn returns an error, the transaction is rolled back.
// If fn returns nil, the transaction is committed.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(queries *gen.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	q := gen.New(tx)

	if err := fn(q); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(
				fmt.Errorf("rolling back transaction: %w", rbErr),
				err,
			)
		}

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// WithTxResult executes fn within a transaction and returns the result.
// Useful when the caller needs a value computed inside the transaction.
func WithTxResult[T any](ctx context.Context, pool *pgxpool.Pool, fn func(queries *gen.Queries) (T, error)) (T, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("beginning transaction: %w", err)
	}

	q := gen.New(tx)

	result, err := fn(q)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			var zero T

			return zero, errors.Join(
				fmt.Errorf("rolling back transaction: %w", rbErr),
				err,
			)
		}

		var zero T

		return zero, err
	}

	if err := tx.Commit(ctx); err != nil {
		var zero T
		return zero, fmt.Errorf("committing transaction: %w", err)
	}

	return result, nil
}
