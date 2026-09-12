package literals

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
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
	// CatalogMaxLimit is the upper bound on active literals loaded for a tenant.
	// It is exported so fallback loaders can use the same cap.
	CatalogMaxLimit = 10000
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

		Search:     "",
		IsActive:   &active,
		Kind:       "",
		Sort:       "",
		Dir:        "",
		PageOffset: 0,
		PageLimit:  CatalogMaxLimit,
	})
	if err != nil {
		return nil, err
	}

	lits := make([]TemplateLiteral, len(rows))
	for i, r := range rows {
		lits[i] = TemplateLiteral{
			ID: r.ID.String(),

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
// is configured. Built-in variables are included in the map so the frontend
// resolver can use bare tokens such as {{Current_Year}} without each tenant
// having to redefine them.
func (c *Catalog) ResolveMap(ctx context.Context, tenantID pgtype.UUID, rc RenderContext) (map[string]string, error) {
	tokens, err := c.GetActive(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	env := rc.ToEnv()
	tokenMap := buildTokenMap(tokens)
	out := make(map[string]string, len(tokens)+len(env.Vars))

	for _, lit := range tokens {
		if !lit.IsActive {
			continue
		}

		resolved, ok := resolveLiteral(ctx, lit, env, tokenMap)
		if !ok {
			continue
		}

		out[lit.Key] = resolved
	}

	addBuiltinVars(out, env)

	return out, nil
}

func resolveLiteral(ctx context.Context, lit TemplateLiteral, env *expr.Env, tokenMap map[string]TemplateLiteral) (string, bool) {
	switch lit.Kind {
	case expr.LiteralKindStatic:
		return resolveStaticLiteral(ctx, lit, env, tokenMap)
	case expr.LiteralKindExpression:
		return resolveExpressionWithEnv(ctx, lit.Value, lit.Fallback, env)
	default:
		return "", false
	}
}

func resolveStaticLiteral(ctx context.Context, lit TemplateLiteral, env *expr.Env, tokenMap map[string]TemplateLiteral) (string, bool) {
	resolved := resolveWithMap(ctx, lit.Value, env, tokenMap, nil)
	if strings.Contains(resolved, "{{") {
		return derefFallback(lit.Fallback)
	}

	if resolved == "" {
		return derefFallback(lit.Fallback)
	}

	return resolved, true
}

func derefFallback(fallback *string) (string, bool) {
	if fallback != nil && *fallback != "" {
		return *fallback, true
	}

	return "", false
}

func addBuiltinVars(out map[string]string, env *expr.Env) {
	for k, v := range env.Vars {
		if _, exists := out[k]; exists {
			continue
		}

		s, ok := expr.ToString(v)
		// Skip empty values — an unset geo var (e.g. Source_Country when no
		// origin is known) must not shadow the consumer's own fallback.
		if !ok || s == "" {
			continue
		}

		out[k] = s
	}
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

func resolveExpressionWithEnv(ctx context.Context, expression string, fallback *string, env *expr.Env) (string, bool) {
	parsed, err := expr.Parse(expression)
	if err != nil {
		if fallback != nil && *fallback != "" {
			return *fallback, true
		}

		return "", false
	}

	v, err := expr.EvalWithTimeout(ctx, parsed, env, 50*time.Millisecond)
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

	// Empty result = the expression referenced an unset context variable
	// (e.g. no detected destination country) — apply the configured fallback,
	// matching resolveStaticLiteral's handling of an empty resolved value.
	if s == "" {
		return derefFallback(fallback)
	}

	return s, true
}
