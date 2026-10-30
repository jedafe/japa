package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ForceDelete permanently removes a row and every row that references it
// through NO ACTION / RESTRICT foreign keys, traversing the FK graph in
// dependency order. Dependents declared SET NULL or SET DEFAULT are
// preserved (the referencing column is nulled or reset); CASCADE
// dependents are left to the database. A visited set breaks FK cycles
// (users ↔ spaces, spaces ↔ orgs).
//
// Must be called inside the caller's transaction — pass the live pgx.Tx so
// every dependent delete commits or rolls back with the parent row.
//
// Used by admin hard-delete paths (contents, users, organizations). A bare
// DELETE fails with 23503 whenever a dependent row exists; the inbound-FK
// set for users alone spans ~40 tables and moves with every migration, so
// enumerating pg_constraint beats hand-maintained delete lists.
func ForceDelete(ctx context.Context, tx gen.DBTX, table, pkColumn string, id pgtype.UUID) error {
	return forceDelete(ctx, tx, table, pkColumn, id, make(map[string]struct{}))
}

type inboundFK struct {
	childTable string
	childCol   string
	parentCol  string
	delAction  string // pg_constraint.confdeltype: 'a','c','n','d','r'
}

// singleColPK returns the table's single-column primary key name. The
// second return is false for composite-key or keyless tables — those rows
// are deleted by their FK column directly (treated as graph leaves).
func singleColPK(ctx context.Context, tx gen.DBTX, table string) (string, bool) {
	var pk string
	err := tx.QueryRow(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
		WHERE i.indrelid = $1::regclass AND i.indisprimary AND i.indnatts = 1`,
		table).Scan(&pk)
	if err != nil {
		return "", false
	}
	return pk, true
}

func forceDelete(ctx context.Context, tx gen.DBTX, table, pkCol string, id any, seen map[string]struct{}) error {
	key := table + "\x00" + fmt.Sprint(id)
	if _, ok := seen[key]; ok {
		return nil
	}
	seen[key] = struct{}{}

	rows, err := tx.Query(ctx, `
		SELECT cl.relname, a.attname, pa.attname, c.confdeltype
		FROM pg_constraint c
		JOIN pg_class cl ON cl.oid = c.conrelid
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		JOIN pg_attribute pa ON pa.attrelid = c.confrelid AND pa.attnum = c.confkey[1]
		WHERE c.confrelid = $1::regclass AND c.contype = 'f' AND array_length(c.conkey, 1) = 1`,
		table)
	if err != nil {
		return fmt.Errorf("listing inbound FKs for %s: %w", table, err)
	}

	var fks []inboundFK
	for rows.Next() {
		var fk inboundFK
		if err := rows.Scan(&fk.childTable, &fk.childCol, &fk.parentCol, &fk.delAction); err != nil {
			rows.Close()
			return fmt.Errorf("scanning inbound FK for %s: %w", table, err)
		}
		fks = append(fks, fk)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating inbound FKs for %s: %w", table, err)
	}

	for _, fk := range fks {
		// Only sweep references aimed at this row's key column — an FK may
		// target a different unique column on the same table.
		if fk.parentCol != pkCol {
			continue
		}

		child := pgx.Identifier{fk.childTable}.Sanitize()
		col := pgx.Identifier{fk.childCol}.Sanitize()

		switch fk.delAction {
		case "c": // ON DELETE CASCADE — the database removes dependents.
		case "n": // SET NULL — preserve the row, sever the reference.
			if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = NULL WHERE %s = $1", child, col, col), id); err != nil {
				return fmt.Errorf("nulling %s.%s: %w", fk.childTable, fk.childCol, err)
			}
		case "d": // SET DEFAULT — preserve the row, reset the reference.
			if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = DEFAULT WHERE %s = $1", child, col, col), id); err != nil {
				return fmt.Errorf("defaulting %s.%s: %w", fk.childTable, fk.childCol, err)
			}
		default: // NO ACTION ('a') / RESTRICT ('r') — dependents must go first.
			if childPK, ok := singleColPK(ctx, tx, fk.childTable); ok {
				depRows, err := tx.Query(ctx,
					fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", pgx.Identifier{childPK}.Sanitize(), child, col), id)
				if err != nil {
					return fmt.Errorf("listing dependents of %s in %s: %w", table, fk.childTable, err)
				}
				var depIDs []any
				for depRows.Next() {
					var d any
					if err := depRows.Scan(&d); err != nil {
						depRows.Close()
						return fmt.Errorf("scanning dependent PK in %s: %w", fk.childTable, err)
					}
					depIDs = append(depIDs, d)
				}
				depRows.Close()
				if err := depRows.Err(); err != nil {
					return err
				}
				for _, dep := range depIDs {
					if err := forceDelete(ctx, tx, fk.childTable, childPK, dep, seen); err != nil {
						return err
					}
				}
			}
			// Covers rows skipped by the visited set, composite-key tables,
			// and anything inserted mid-transaction.
			if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = $1", child, col), id); err != nil {
				return fmt.Errorf("deleting dependents in %s.%s: %w", fk.childTable, fk.childCol, err)
			}
		}
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s = $1", pgx.Identifier{table}.Sanitize(), pgx.Identifier{pkCol}.Sanitize()),
		id); err != nil {
		return fmt.Errorf("deleting %s.%s=%v: %w", table, pkCol, id, err)
	}
	return nil
}
