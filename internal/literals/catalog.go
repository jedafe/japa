package literals

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals/expr"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	catalogCacheTTL    = 5 * time.Minute
	catalogKeyTemplate = "literals:%s"
)

// Catalog caches and resolves active tenant template literals.
type Catalog struct {
	queries *gen.Queries
	valkey  valkeycompat.Cmdable
}

// NewCatalog creates a catalog backed by the given DB queries and Valkey client.
func NewCatalog(queries *gen.Queries, valkey valkeycompat.Cmdable) *Catalog {
	return &Catalog{
		queries: queries,
		valkey:  valkey,
	}
}

// GetActive returns all active template literals for a tenant, preferring the
// Valkey cache. Cache failures are best-effort and never return an error.
func (c *Catalog) GetActive(ctx context.Context, tenantID pgtype.UUID) ([]TemplateLiteral, error) {
	if c.valkey != nil {
		cached, err := c.valkey.Get(ctx, c.cacheKey(tenantID)).Result()
		if err == nil && cached != "" {
			var lits []TemplateLiteral
			if err := json.Unmarshal([]byte(cached), &lits); err == nil {
				return lits, nil
			}

			slog.Warn(
				"literals: failed to unmarshal cached active literals",
				"tenant_id", tenantID.String(),
				"error", err,
			)
		}
	}

	active := true

	rows, err := c.queries.ListTemplateLiterals(ctx, gen.ListTemplateLiteralsParams{
		TenantID:   tenantID,
		Search:     "",
		IsActive:   &active,
		Kind:       "",
		Sort:       "",
		Dir:        "",
		PageOffset: 0,
		PageLimit:  1000,
	})
	if err != nil {
		return nil, err
	}

	lits := make([]TemplateLiteral, len(rows))
	for i, r := range rows {
		lits[i] = TemplateLiteral{
			ID:          r.ID.String(),
			TenantID:    r.TenantID.String(),
			Key:         r.Key,
			Kind:        r.Kind,
			Value:       r.Value,
			Fallback:    r.Fallback,
			Label:       r.Label,
			Description: r.Description,
			Category:    r.Category,
			IsSystem:    r.IsSystem,
			IsActive:    r.IsActive,
		}
	}

	if c.valkey != nil {
		encoded, err := json.Marshal(lits)
		if err == nil {
			if sErr := c.valkey.Set(ctx, c.cacheKey(tenantID), string(encoded), catalogCacheTTL).Err(); sErr != nil {
				slog.Warn(
					"literals: failed to cache active literals",
					"tenant_id", tenantID.String(),
					"error", sErr,
				)
			}
		}
	}

	return lits, nil
}

// ResolveMap resolves every active template literal for a tenant into a flat
// map. Static literals can reference other active literals. Expression failures
// fall back to the literal's fallback value, or omit the key when no fallback
// is configured.
func (c *Catalog) ResolveMap(ctx context.Context, tenantID pgtype.UUID, rc RenderContext) (map[string]string, error) {
	tokens, err := c.GetActive(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, len(tokens))

	for _, lit := range tokens {
		if !lit.IsActive {
			continue
		}

		switch lit.Kind {
		case expr.LiteralKindStatic:
			resolved, err := Resolve(ctx, lit.Value, rc, tokens)
			if err != nil {
				if lit.Fallback != nil && *lit.Fallback != "" {
					out[lit.Key] = *lit.Fallback
				}

				continue
			}

			out[lit.Key] = resolved

		case expr.LiteralKindExpression:
			resolved, ok := resolveExpression(ctx, lit.Value, lit.Fallback, rc)
			if !ok {
				continue
			}

			out[lit.Key] = resolved
		}
	}

	return out, nil
}

// Invalidate removes the cached active literal list for a tenant.
func (c *Catalog) Invalidate(ctx context.Context, tenantID pgtype.UUID) error {
	if c.valkey == nil {
		return nil
	}

	return c.valkey.Del(ctx, c.cacheKey(tenantID)).Err()
}

func (c *Catalog) cacheKey(tenantID pgtype.UUID) string {
	return valkey.MustKey(catalogKeyTemplate, tenantID.String())
}

func resolveExpression(ctx context.Context, expression string, fallback *string, rc RenderContext) (string, bool) {
	parsed, err := expr.Parse(expression)
	if err != nil {
		if fallback != nil && *fallback != "" {
			return *fallback, true
		}

		return "", false
	}

	v, err := expr.EvalWithTimeout(ctx, parsed, rc.ToEnv(), 50*time.Millisecond)
	if err != nil {
		if fallback != nil && *fallback != "" {
			return *fallback, true
		}

		return "", false
	}

	s, ok := expr.ToString(v)
	if !ok {
		if fallback != nil && *fallback != "" {
			return *fallback, true
		}

		return "", false
	}

	return s, true
}
