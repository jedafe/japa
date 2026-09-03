package tiers

import (
	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated queries for tier operations.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a store bound to the reader and writer pools.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// NewStoreFromTx creates a store bound to a single transaction. Use this for
// transactional tier changes so all reads and writes see the same snapshot.
func NewStoreFromTx(tx pgx.Tx) *Store {
	q := gen.New(tx)
	return &Store{read: q, write: q}
}
