package resolver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// DBConfig describes a simple table that can be resolved and searched with a
// small set of known columns. It is intended for tool tables that carry a
// tenant_id and a human-readable title/name.
type DBConfig struct {
	Entity       string // human name for error messages
	Table        string
	IDColumn     string
	TitleColumn  string
	SlugColumn   string // optional; empty means no slug search
	DescColumn   string // optional; empty means no description
	TenantColumn string // optional; empty means no tenant filter
	TenantJoin   string // optional JOIN clause (e.g. "JOIN contents c ON c.id = t.guide_id")
	TenantWhere  string // optional tenant predicate when TenantJoin is used (e.g. "c.tenant_id = $1")
	ActiveWhere  string // optional SQL predicate (e.g. "t.is_active = true" or "t.status = 'published'")
}

var identRe = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")

func quoteIdent(s string) (string, error) {
	if !identRe.MatchString(s) {
		return "", fmt.Errorf("invalid SQL identifier: %q", s)
	}
	return s, nil
}

// DBTX is the subset of a pgx pool that DBResolver needs.
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

// DBResolver resolves items by reading a PostgreSQL table directly.
// All identifiers are whitelisted before use, so dynamic SQL is safe.
type DBResolver struct {
	db  DBTX
	cfg DBConfig
}

// NewDBResolver creates a resolver backed by a single table.
func NewDBResolver(db DBTX, cfg DBConfig) *DBResolver {
	return &DBResolver{db: db, cfg: cfg}
}

// EntityType returns the configured entity type for this resolver.
func (r *DBResolver) EntityType() string { return r.cfg.Entity }

// Resolve looks up a single item by the id or slug in ref.
func (r *DBResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		// Most id columns are uuid; use the typed value when possible.
		if id, err := ParseUUID(idStr); err == nil {
			return r.resolveByColumn(ctx, r.cfg.IDColumn, id, tenantID)
		}
		return r.resolveByColumn(ctx, r.cfg.IDColumn, idStr, tenantID)
	}
	if r.cfg.SlugColumn != "" {
		if slug, ok := ref["slug"].(string); ok && slug != "" {
			return r.resolveByColumn(ctx, r.cfg.SlugColumn, slug, tenantID)
		}
	}
	return ResolvedItem{}, false, errors.New("missing id/slug in item_ref")
}

func (r *DBResolver) resolveByColumn(ctx context.Context, col string, value any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	idCol, err := quoteIdent(r.cfg.IDColumn)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	titleCol, err := quoteIdent(r.cfg.TitleColumn)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	descCol := "''::text"
	if r.cfg.DescColumn != "" {
		c, err := quoteIdent(r.cfg.DescColumn)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		descCol = c
	}
	slugCol := "''::text"
	if r.cfg.SlugColumn != "" {
		c, err := quoteIdent(r.cfg.SlugColumn)
		if err != nil {
			return ResolvedItem{}, false, err
		}
		slugCol = c
	}
	searchCol, err := quoteIdent(col)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	table, err := quoteIdent(r.cfg.Table)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	where, args, err := r.whereClause(tenantID)
	if err != nil {
		return ResolvedItem{}, false, err
	}
	if where != "" {
		where += " AND "
	}
	where += fmt.Sprintf("t.%s = $%d", searchCol, len(args)+1)
	args = append(args, value)
	if r.cfg.ActiveWhere != "" {
		where += " AND " + r.cfg.ActiveWhere
	}

	query := fmt.Sprintf(
		"SELECT t.%s, t.%s, %s, %s FROM %s t %s WHERE %s LIMIT 1",
		idCol, titleCol, slugCol, descCol, table, r.cfg.TenantJoin, where,
	)

	var id, title, slug, description string
	row := r.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&id, &title, &slug, &description); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return ResolvedItem{}, false, nil
		}
		return ResolvedItem{}, false, fmt.Errorf("resolving %s by %s: %w", r.cfg.Entity, col, err)
	}

	return r.toItem(id, title, slug, description), true, nil
}

// Search performs a case-insensitive prefix/substring search against the title
// and optional slug columns for the configured table.
func (r *DBResolver) Search(ctx context.Context, query string, _ map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	idCol, err := quoteIdent(r.cfg.IDColumn)
	if err != nil {
		return nil, err
	}
	titleCol, err := quoteIdent(r.cfg.TitleColumn)
	if err != nil {
		return nil, err
	}
	descCol := "''::text"
	if r.cfg.DescColumn != "" {
		c, err := quoteIdent(r.cfg.DescColumn)
		if err != nil {
			return nil, err
		}
		descCol = c
	}
	table, err := quoteIdent(r.cfg.Table)
	if err != nil {
		return nil, err
	}

	where, args, err := r.whereClause(tenantID)
	if err != nil {
		return nil, err
	}
	if where != "" {
		where += " AND "
	}
	qArg := len(args) + 1

	slugCol := "''::text"
	slugSearch := ""
	if r.cfg.SlugColumn != "" {
		c, err := quoteIdent(r.cfg.SlugColumn)
		if err != nil {
			return nil, err
		}
		slugCol = c
		slugSearch = fmt.Sprintf(" OR t.%s ILIKE $%d", c, qArg)
	}
	where += fmt.Sprintf("(t.%s ILIKE $%d%s)", titleCol, qArg, slugSearch)
	args = append(args, "%"+query+"%")
	if r.cfg.ActiveWhere != "" {
		where += " AND " + r.cfg.ActiveWhere
	}

	querySQL := fmt.Sprintf(
		"SELECT t.%s, t.%s, %s, %s FROM %s t %s WHERE %s ORDER BY t.%s LIMIT $%d OFFSET 0",
		idCol, titleCol, slugCol, descCol, table, r.cfg.TenantJoin, where, titleCol, len(args)+1,
	)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", r.cfg.Entity, err)
	}
	defer rows.Close()

	out := make([]ResolvedItem, 0, limit)
	for rows.Next() {
		var id, title, slug, description string
		if err := rows.Scan(&id, &title, &slug, &description); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", r.cfg.Entity, err)
		}
		out = append(out, r.toItem(id, title, slug, description))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// whereClause returns the tenant predicate and its arguments.
func (r *DBResolver) whereClause(tenantID pgtype.UUID) (string, []any, error) {
	if r.cfg.TenantColumn != "" {
		c, err := quoteIdent(r.cfg.TenantColumn)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("t.%s = $1", c), []any{tenantID}, nil
	}
	if r.cfg.TenantWhere != "" {
		return r.cfg.TenantWhere, []any{tenantID}, nil
	}
	return "", nil, nil
}

func (r *DBResolver) toItem(id, title, slug, description string) ResolvedItem {
	meta := map[string]any{}
	if slug != "" {
		meta["slug"] = slug
	}
	return ResolvedItem{
		Type:        r.cfg.Entity,
		ID:          id,
		Title:       title,
		Description: Excerpt(description, 160),
		Metadata:    meta,
	}
}
