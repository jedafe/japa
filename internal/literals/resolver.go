// Package literals resolves template literal placeholders in strings using a
// whitelist-only expression evaluator and a render context.
package literals

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/literals/expr"
)

var (
	// tokenRegex matches bare tokens with an optional inline fallback:
	// {{Token}} or {{Token|fallback value}}.
	tokenRegex = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)(?:\|([^}]+))?\s*\}\}`)

	// exprRegex matches any remaining {{ ... }} block that is not a bare token.
	exprRegex = regexp.MustCompile(`\{\{([\s\S]+?)\}\}`)
)

// Resolve replaces {{...}} placeholders in text using the provided render context
// and tenant token definitions. Unknown tokens are left unchanged.
func Resolve(ctx context.Context, text string, rc RenderContext, tokens []TemplateLiteral) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}

	env := rc.ToEnv()

	tokenMap := make(map[string]TemplateLiteral, len(tokens))
	for _, t := range tokens {
		tokenMap[t.Key] = t
	}

	// First pass: replace bare tokens with optional inline fallback.
	text = tokenRegex.ReplaceAllStringFunc(text, func(match string) string {
		m := tokenRegex.FindStringSubmatch(match)
		key := m[1]

		inlineFallback := ""
		if len(m) > 2 && m[2] != "" {
			inlineFallback = strings.TrimSpace(m[2])
		}

		return resolveToken(ctx, key, inlineFallback, env, tokenMap)
	})

	// Second pass: evaluate general expressions.
	text = exprRegex.ReplaceAllStringFunc(text, func(match string) string {
		if tokenRegex.MatchString(match) {
			return match // leave unresolved bare tokens unchanged
		}

		inner := match[2 : len(match)-2]

		parsed, err := expr.Parse(inner)
		if err != nil {
			return match
		}

		v, err := expr.EvalWithTimeout(ctx, parsed, env, 50*time.Millisecond)
		if err != nil {
			return match
		}

		s, _ := expr.ToString(v)

		return s
	})

	return text, nil
}

func resolveToken(ctx context.Context, key, inlineFallback string, env *expr.Env, tokenMap map[string]TemplateLiteral) string {
	lit, ok := tokenMap[key]
	if !ok {
		if inlineFallback != "" {
			return inlineFallback
		}

		return fmt.Sprintf("{{%s}}", key)
	}

	if lit.Kind == expr.LiteralKindStatic {
		return lit.Value
	}

	parsed, err := expr.Parse(lit.Value)
	if err != nil {
		return resolveFailure(key, inlineFallback, lit.Fallback)
	}

	v, err := expr.EvalWithTimeout(ctx, parsed, env, 50*time.Millisecond)
	if err != nil {
		return resolveFailure(key, inlineFallback, lit.Fallback)
	}

	s, _ := expr.ToString(v)

	return s
}

func resolveFailure(key, inlineFallback string, fallback *string) string {
	if inlineFallback != "" {
		return inlineFallback
	}

	if fallback != nil && *fallback != "" {
		return *fallback
	}

	return fmt.Sprintf("{{%s}}", key)
}
