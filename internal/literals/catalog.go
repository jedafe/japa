package literals

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals/expr"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	catalogCacheTTL    = 5 * time.Minute
	catalogKeyTemplate = "literals:%s"
	// CatalogMaxLimit is the upper bound on active literals loaded for a platform.
	// It is exported so fallback loaders can use the same cap.
	CatalogMaxLimit = 10000
)

// Catalog caches and resolves active platform template literals.
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

// GetActive returns all active template literals for a platform, preferring the
// Valkey cache. Cache failures are best-effort and never return an error.
func (c *Catalog) GetActive(ctx context.Context) ([]TemplateLiteral, error) {
	if c.valkey != nil {
		cached, err := c.valkey.Get(ctx, c.cacheKey()).Result()
		if err == nil && cached != "" {
			var lits []TemplateLiteral
			if err := json.Unmarshal([]byte(cached), &lits); err == nil {
				return lits, nil
			}

			slog.Warn("literals: failed to unmarshal cached active literals", "error", err)
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
			if sErr := c.valkey.Set(ctx, c.cacheKey(), string(encoded), catalogCacheTTL).Err(); sErr != nil {
				slog.Warn("literals: failed to cache active literals", "error", sErr)
			}
		}
	}

	return lits, nil
}

// ResolveMap resolves every active template literal for a platform into a flat
// map. Static literals can reference other active literals. Expression failures
// fall back to the literal's fallback value, or omit the key when no fallback
// is configured. Built-in variables are included in the map so the frontend
// resolver can use bare tokens such as {{Current_Year}} without each platform
// having to redefine them.
func (c *Catalog) ResolveMap(ctx context.Context, rc RenderContext) (map[string]string, error) {
	tokens, err := c.GetActive(ctx)
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

// ResolveText resolves {{...}} placeholders in a single string using the
// catalog's active literals. Load failures log a warning and return the
// input unchanged — literal resolution is best-effort and must never break
// a response.
func (c *Catalog) ResolveText(ctx context.Context, s string, rc RenderContext) string {
	if c == nil || rc.raw || !strings.Contains(s, "{{") {
		return s
	}

	tokens, err := c.GetActive(ctx)
	if err != nil {
		slog.Warn("literals: failed to load literals for ResolveText", "error", err)

		return s
	}

	out, _ := Resolve(ctx, s, rc, tokens)

	return out
}

// ResolveFields resolves the named text fields (Title, Body, Excerpt,
// Content, Description, and friends) on any content-like struct or slice of
// structs using the catalog's active literals. Best-effort: failures log a
// warning and leave the input unchanged.
func (c *Catalog) ResolveFields(ctx context.Context, v any, rc RenderContext) {
	if c == nil || v == nil || rc.raw {
		return
	}

	tokens, err := c.GetActive(ctx)
	if err != nil {
		slog.Warn("literals: failed to load literals for ResolveFields", "error", err)

		return
	}

	_ = ResolveFields(ctx, v, rc, tokens)
}

// ResolveData resolves every string inside a JSON-shaped payload (maps,
// slices, nested values) plus named fields on any structs it contains.
// Used for rendered widget data and response envelopes. Best-effort.
func (c *Catalog) ResolveData(ctx context.Context, v any, rc RenderContext) {
	if c == nil || v == nil || rc.raw {
		return
	}

	tokens, err := c.GetActive(ctx)
	if err != nil {
		slog.Warn("literals: failed to load literals for ResolveData", "error", err)

		return
	}

	ResolveData(ctx, v, rc, tokens)
}

// ResolveJSON resolves every string inside a raw JSON payload (rendered
// widget data, page-builder block props) and returns the re-marshaled JSON.
// When the payload contains no "{{" marker or fails to unmarshal, the input
// is returned unchanged. Best-effort — never errors.
func (c *Catalog) ResolveJSON(ctx context.Context, raw json.RawMessage, rc RenderContext) json.RawMessage {
	if c == nil || rc.raw || len(raw) == 0 || !strings.Contains(string(raw), "{{") {
		return raw
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}

	c.ResolveData(ctx, &v, rc)

	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}

	return out
}

// Invalidate removes the cached active literal list for a platform.
func (c *Catalog) Invalidate(ctx context.Context) error {
	if c.valkey == nil {
		return nil
	}

	return c.valkey.Del(ctx, c.cacheKey()).Err()
}

func (c *Catalog) cacheKey() string {
	return valkey.MustKey(catalogKeyTemplate, "")
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
