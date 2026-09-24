package resolver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// DBConfig describes a simple table that can be resolved and searched with a
// small set of known columns. It is intended for tool tables that carry a
// human-readable title/name.
type DBConfig struct {
	Entity         string // human name for error messages
	Table          string
	IDColumn       string
	TitleColumn    string
	SlugColumn     string // optional; empty means no slug search
	DescColumn     string // optional; empty means no description
	PlatformColumn string // optional; empty means no platform filter
	PlatformJoin   string // optional JOIN clause (e.g. "JOIN contents c ON c.id = t.guide_id")
	PlatformWhere  string // optional predicate when PlatformJoin is used
	ActiveWhere    string // optional SQL predicate (e.g. "t.is_active = true" or "t.status = 'published'")

	// Optional personalization metadata columns for Sub-spec 2 Smart Compare.
	// Leave empty to omit from the resolver response.
	GeoSpaceIDColumn  string // uuid FK to spaces(id) representing the item's geo
	CountryColumn     string // ISO country code or country name
	TagsColumn        string // text[] of tag-like labels (e.g. skills)
	FieldValuesColumn string // jsonb column containing score_inputs data (e.g. field_values)
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
	db      DBTX
	cfg     DBConfig
	queries *gen.Queries // optional: enables custom_field_values hydration
}

// NewDBResolver creates a resolver backed by a single table. Pass
// WithQueries to also merge custom_field_values rows (the canonical store)
// into the item's field_values metadata on Resolve.
func NewDBResolver(db DBTX, cfg DBConfig, opts ...Option) *DBResolver {
	o := newResolverOptions(opts...)

	return &DBResolver{db: db, cfg: cfg, queries: o.queries}
}

// EntityType returns the configured entity type for this resolver.
func (r *DBResolver) EntityType() string { return r.cfg.Entity }

// Resolve looks up a single item by the id or slug in ref.
func (r *DBResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		// Most id columns are uuid; use the typed value when possible.
		if id, err := ParseUUID(idStr); err == nil {
			return r.resolveByColumn(ctx, r.cfg.IDColumn, id)
		}

		return r.resolveByColumn(ctx, r.cfg.IDColumn, idStr)
	}

	if r.cfg.SlugColumn != "" {
		if slug, ok := ref["slug"].(string); ok && slug != "" {
			return r.resolveByColumn(ctx, r.cfg.SlugColumn, slug)
		}
	}

	return ResolvedItem{}, false, errors.New("missing id/slug in item_ref")
}

type dbRowValues struct {
	id, title, slug, description string
	geoSpaceID                   pgtype.UUID
	country                      string
	tags                         []string
	fieldValues                  []byte
}

// selectColumnNames returns the SELECT expressions for the configured columns.
// The order matches the scan arguments produced by scanArgs.
func (r *DBResolver) selectColumnNames() ([]string, error) {
	idCol, err := quoteIdent(r.cfg.IDColumn)
	if err != nil {
		return nil, err
	}

	titleCol, err := quoteIdent(r.cfg.TitleColumn)
	if err != nil {
		return nil, err
	}

	slugCol := "''::text"

	if r.cfg.SlugColumn != "" {
		c, err := quoteIdent(r.cfg.SlugColumn)
		if err != nil {
			return nil, err
		}

		slugCol = "t." + c
	}

	descCol := "''::text"

	if r.cfg.DescColumn != "" {
		c, err := quoteIdent(r.cfg.DescColumn)
		if err != nil {
			return nil, err
		}

		descCol = fmt.Sprintf("COALESCE(t.%s, '')", c)
	}

	cols := []string{
		"t." + idCol,
		"t." + titleCol,
		slugCol,
		descCol,
	}

	if r.cfg.GeoSpaceIDColumn != "" {
		c, err := quoteIdent(r.cfg.GeoSpaceIDColumn)
		if err != nil {
			return nil, err
		}

		cols = append(cols, "t."+c)
	}

	if r.cfg.CountryColumn != "" {
		c, err := quoteIdent(r.cfg.CountryColumn)
		if err != nil {
			return nil, err
		}

		cols = append(cols, fmt.Sprintf("COALESCE(t.%s, '')", c))
	}

	if r.cfg.TagsColumn != "" {
		c, err := quoteIdent(r.cfg.TagsColumn)
		if err != nil {
			return nil, err
		}

		cols = append(cols, "t."+c)
	}

	if r.cfg.FieldValuesColumn != "" {
		c, err := quoteIdent(r.cfg.FieldValuesColumn)
		if err != nil {
			return nil, err
		}

		cols = append(cols, "t."+c)
	}

	return cols, nil
}

// scanArgs returns the Scan destinations for a dbRowValues in the same order as
// selectColumnNames.
func (r *DBResolver) scanArgs(rv *dbRowValues) []any {
	dst := []any{&rv.id, &rv.title, &rv.slug, &rv.description}
	if r.cfg.GeoSpaceIDColumn != "" {
		dst = append(dst, &rv.geoSpaceID)
	}

	if r.cfg.CountryColumn != "" {
		dst = append(dst, &rv.country)
	}

	if r.cfg.TagsColumn != "" {
		dst = append(dst, &rv.tags)
	}

	if r.cfg.FieldValuesColumn != "" {
		dst = append(dst, &rv.fieldValues)
	}

	return dst
}

func (r *DBResolver) resolveByColumn(ctx context.Context, col string, value any) (ResolvedItem, bool, error) {
	searchCol, err := quoteIdent(col)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	table, err := quoteIdent(r.cfg.Table)
	if err != nil {
		return ResolvedItem{}, false, err
	}

	cols, err := r.selectColumnNames()
	if err != nil {
		return ResolvedItem{}, false, err
	}

	where, args, err := r.whereClause()
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
		"SELECT %s FROM %s t %s WHERE %s LIMIT 1",
		strings.Join(cols, ", "), table, r.cfg.PlatformJoin, where,
	)

	rv := &dbRowValues{}
	row := r.db.QueryRow(ctx, query, args...)

	if err := row.Scan(r.scanArgs(rv)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return ResolvedItem{}, false, nil
		}

		return ResolvedItem{}, false, fmt.Errorf("resolving %s by %s: %w", r.cfg.Entity, col, err)
	}

	item := r.toItem(rv)

	// §30 Sub-spec 5: merge canonical custom_field_values rows into
	// field_values so matrix cells can hydrate even for tables without a
	// field_values column. cfv wins over the raw column (canonical truth).
	if r.queries != nil {
		if id, perr := ParseUUID(rv.id); perr == nil {
			if cfv := resolveFieldValues(ctx, r.queries, r.cfg.Entity, id); len(cfv) > 0 {
				merged, _ := item.Metadata["field_values"].(map[string]any)
				if merged == nil {
					merged = make(map[string]any, len(cfv))
				}

				for k, v := range cfv {
					merged[k] = v
				}

				item.Metadata["field_values"] = merged
			}
		}
	}

	return item, true, nil
}

// Search performs a case-insensitive prefix/substring search against the title
// and optional slug columns for the configured table. If the /compare scope
// chain provides a geo scope filter, it is applied against DBConfig.GeoSpaceIDColumn
// or DBConfig.CountryColumn when those columns are configured.
func (r *DBResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	titleCol, err := quoteIdent(r.cfg.TitleColumn)
	if err != nil {
		return nil, err
	}

	table, err := quoteIdent(r.cfg.Table)
	if err != nil {
		return nil, err
	}

	cols, err := r.selectColumnNames()
	if err != nil {
		return nil, err
	}

	where, args, err := r.whereClause()
	if err != nil {
		return nil, err
	}

	geoSpaceID := deepestGeoFilter(filters)
	if r.cfg.GeoSpaceIDColumn != "" && geoSpaceID.Valid {
		c, err := quoteIdent(r.cfg.GeoSpaceIDColumn)
		if err != nil {
			return nil, err
		}

		if where != "" {
			where += " AND "
		}

		where += fmt.Sprintf("t.%s = $%d", c, len(args)+1)
		args = append(args, geoSpaceID)
	}

	countryCode := parseStringFilter(filters, "country_code")
	if r.cfg.CountryColumn != "" && countryCode != "" {
		c, err := quoteIdent(r.cfg.CountryColumn)
		if err != nil {
			return nil, err
		}

		if where != "" {
			where += " AND "
		}

		where += fmt.Sprintf("t.%s = $%d", c, len(args)+1)
		args = append(args, countryCode)
	}

	if where != "" {
		where += " AND "
	}

	qArg := len(args) + 1

	slugSearch := ""

	if r.cfg.SlugColumn != "" {
		c, err := quoteIdent(r.cfg.SlugColumn)
		if err != nil {
			return nil, err
		}

		slugSearch = fmt.Sprintf(" OR t.%s ILIKE $%d", c, qArg)
	}

	where += fmt.Sprintf("(t.%s ILIKE $%d%s)", titleCol, qArg, slugSearch)

	args = append(args, "%"+query+"%")

	if r.cfg.ActiveWhere != "" {
		where += " AND " + r.cfg.ActiveWhere
	}

	querySQL := fmt.Sprintf(
		"SELECT %s FROM %s t %s WHERE %s ORDER BY t.%s LIMIT $%d OFFSET $%d",
		strings.Join(cols, ", "), table, r.cfg.PlatformJoin, where, titleCol, len(args)+1, len(args)+2,
	)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", r.cfg.Entity, err)
	}
	defer rows.Close()

	out := make([]ResolvedItem, 0, limit)

	for rows.Next() {
		rv := &dbRowValues{}
		if err := rows.Scan(r.scanArgs(rv)...); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", r.cfg.Entity, err)
		}

		out = append(out, r.toItem(rv))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// whereClause is a no-op now that platform scoping has been removed. It keeps
// the function shape so existing callers still compile.
func (r *DBResolver) whereClause() (string, []any, error) {
	return "", nil, nil
}

func (r *DBResolver) toItem(rv *dbRowValues) ResolvedItem {
	meta := map[string]any{
		"score_inputs": dbResolverScoreInputs(r.cfg.Entity, rv),
	}
	if rv.slug != "" {
		meta["slug"] = rv.slug
	}

	if r.cfg.GeoSpaceIDColumn != "" && rv.geoSpaceID.Valid {
		meta["geo_space_id"] = rv.geoSpaceID.String()
	}

	if r.cfg.CountryColumn != "" && rv.country != "" {
		meta["country"] = rv.country
	}

	if r.cfg.TagsColumn != "" && len(rv.tags) > 0 {
		meta["tags"] = rv.tags
	}

	// §30 Sub-spec 5: expose the raw field_values JSONB map (keyed by field
	// key) so matrix cells can hydrate from entity field values.
	if r.cfg.FieldValuesColumn != "" {
		if fields := parseDBFieldValues(rv.fieldValues); len(fields) > 0 {
			meta["field_values"] = fields
		}
	}

	return ResolvedItem{
		Type:        r.cfg.Entity,
		ID:          rv.id,
		Title:       rv.title,
		Description: Excerpt(rv.description, 160),
		Metadata:    meta,
	}
}

func dbResolverScoreInputs(entity string, rv *dbRowValues) map[string]any {
	fields := parseDBFieldValues(rv.fieldValues)

	switch entity {
	case "organization", "bank":
		cost := fieldValueNumber(fields, "cost", "monthly_fee", "fee")
		income := fieldValueNumber(fields, "income", "interest_rate", "cashback", "cashback_rate")
		speed := fieldValueNumber(fields, "speed", "account_opening_days", "opening_days")
		atmFee := fieldValueNumber(fields, "atm_fee")
		minBalance := fieldValueNumber(fields, "min_balance")

		countries := []string{}
		if c := fieldValueStrings(fields, "countries"); c != nil {
			countries = c
		} else if rv.country != "" {
			countries = []string{rv.country}
		}

		return map[string]any{
			"cost":        cost,
			"income":      income,
			"speed":       speed,
			"atm_fee":     atmFee,
			"min_balance": minBalance,
			"countries":   countries,
		}
	}

	return map[string]any{}
}

// parseDBFieldValues unmarshals a JSONB field-values column into a map. It
// returns an empty map for nil or invalid payloads so callers can safely query
// keys.
func parseDBFieldValues(b []byte) map[string]any {
	if len(b) == 0 {
		return map[string]any{}
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}

	if m == nil {
		return map[string]any{}
	}

	return m
}
