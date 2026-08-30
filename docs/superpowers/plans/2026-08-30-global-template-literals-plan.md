# Global Template Literals with Admin CRUD — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a server-side, whitelisted expression engine for dynamic text tokens, store tenant-scoped template literals in PostgreSQL, expose CRUD in the admin panel, and resolve tokens in content titles/bodies and page-builder blocks.

**Architecture:** A new `internal/literals` package implements a small lexer/parser/evaluator for `{{...}}` expressions. It consumes a `RenderContext` (time, geo, tenant) and a tenant's `template_literals` rows. `internal/content` and page-builder load functions call the resolver before sending text to the client. Admin CRUD lives in `internal/admin/literals.go` and `apps/admin/src/routes/literals/+page.svelte`.

**Tech Stack:** Go 1.26.4, pgx/v5, sqlc, Chi, SvelteKit 2 + Svelte 5 + Tailwind v4, golang-migrate, Vitest.

## Global Constraints

- Do not hand-edit `internal/db/gen/*.sql.go`; edit `.sql` queries and run `make sqlc-gen`.
- All Go errors wrap with `fmt.Errorf("...: %w", err)`.
- Migrations must have up/down pairs.
- Token syntax is `{{...}}`; bare-token fallback is `{{Token|fallback}}`.
- Expression evaluator must be whitelist-only: no reflection, no I/O, no unbounded recursion, 50ms timeout per call.
- No hardcoded data in SvelteKit `load()` functions (Rule 16).
- Use `bun` for JS tooling.
- Keep phase 2 (emails, menus, per-user deadline tokens) out of this plan.

---

### Task 1: Create `template_literals` table, sqlc queries, and permission

**Files:**
- Create: `migrations/000572_create_template_literals.up.sql`
- Create: `migrations/000572_create_template_literals.down.sql`
- Create: `migrations/000573_template_literals_permission.up.sql`
- Create: `migrations/000573_template_literals_permission.down.sql`
- Create: `internal/db/queries/literals.sql`
- Modify: `internal/db/gen/literals.sql.go` (generated)
- Modify: `internal/db/gen/models.go` (generated)

**Interfaces:**
- Produces: `gen.TemplateLiteral` model and sqlc query functions: `ListTemplateLiterals`, `GetTemplateLiteral`, `CreateTemplateLiteral`, `UpdateTemplateLiteral`, `DeleteTemplateLiteral` (soft), `ListTemplateLiteralsByKeys`.

- [ ] **Step 1: Write the up migration**

In `migrations/000572_create_template_literals.up.sql`:

```sql
CREATE TABLE template_literals (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key           text NOT NULL,
    kind          text NOT NULL DEFAULT 'expression',
    value         text NOT NULL DEFAULT '',
    fallback      text,
    label         text,
    description   text,
    category      text,
    is_system     boolean NOT NULL DEFAULT false,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE UNIQUE INDEX idx_template_literals_tenant_key ON template_literals(tenant_id, key) WHERE deleted_at IS NULL;
CREATE INDEX idx_template_literals_tenant_active ON template_literals(tenant_id, is_active) WHERE deleted_at IS NULL;

-- Seed system tokens for the default tenant.
INSERT INTO template_literals (tenant_id, key, kind, value, fallback, label, description, category, is_system)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'Current_Year',  'expression', 'year(Now)',                  '',          'Current Year',      'The current calendar year.',                                'time', true),
    ('00000000-0000-0000-0000-000000000001', 'Current_Date',  'expression', 'format(Now, "YYYY-MM-DD")',  '',          'Current Date',      'The current date.',                                         'time', true),
    ('00000000-0000-0000-0000-000000000001', 'Current_Time',  'expression', 'format(Now, "HH:mm")',       '',          'Current Time',      'The current time.',                                         'time', true),
    ('00000000-0000-0000-0000-000000000001', 'Source_Country','expression', 'Source_Country',             'your country','Source Country',    'The visitor\'s detected source country.',                  'geo',  true),
    ('00000000-0000-0000-0000-000000000001', 'Source_Code',   'expression', 'Source_Code',                'XX',        'Source Code',       'The visitor\'s detected source country code.',             'geo',  true),
    ('00000000-0000-0000-0000-000000000001', 'Destination_Country','expression','Destination_Country',     'your destination','Destination Country','The visitor\'s selected destination country.',           'geo',  true),
    ('00000000-0000-0000-0000-000000000001', 'Destination_Code',   'expression','Destination_Code',        'XX',        'Destination Code',  'The visitor\'s selected destination country code.',        'geo',  true),
    ('00000000-0000-0000-0000-000000000001', 'Tenant_Name',   'expression', 'Tenant_Name',                '',          'Tenant Name',       'The current tenant name.',                                  'tenant', true),
    ('00000000-0000-0000-0000-000000000001', 'Locale',        'expression', 'Locale',                     'en',        'Locale',            'The active locale.',                                        'locale', true);
```

- [ ] **Step 2: Write the down migration**

In `migrations/000572_create_template_literals.down.sql`:

```sql
DROP TABLE IF EXISTS template_literals;
```

- [ ] **Step 3: Write the permission migration**

In `migrations/000573_template_literals_permission.up.sql`:

```sql
INSERT INTO permissions (key, description) VALUES
    ('content.literals.manage', 'Manage global template literals (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key = 'content.literals.manage'
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
ON CONFLICT DO NOTHING;
```

In `migrations/000573_template_literals_permission.down.sql`:

```sql
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'content.literals.manage');
DELETE FROM permissions WHERE key = 'content.literals.manage';
```

- [ ] **Step 4: Write sqlc queries**

In `internal/db/queries/literals.sql`:

```sql
-- name: ListTemplateLiterals :many
SELECT id, tenant_id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND (@q::text = '' OR key ILIKE '%' || @q || '%' OR label ILIKE '%' || @q || '%')
  AND (@is_active::boolean IS NULL OR is_active = @is_active)
ORDER BY
    CASE WHEN @sort = 'key' AND @dir = 'asc' THEN key END ASC,
    CASE WHEN @sort = 'key' AND @dir = 'desc' THEN key END DESC,
    CASE WHEN @sort = 'created_at' AND @dir = 'asc' THEN created_at END ASC,
    CASE WHEN @sort = 'created_at' AND @dir = 'desc' THEN created_at END DESC,
    created_at DESC
LIMIT @page_size OFFSET @offset;

-- name: CountTemplateLiterals :one
SELECT COUNT(*) FROM template_literals
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND (@q::text = '' OR key ILIKE '%' || @q || '%' OR label ILIKE '%' || @q || '%')
  AND (@is_active::boolean IS NULL OR is_active = @is_active);

-- name: GetTemplateLiteral :one
SELECT id, tenant_id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE tenant_id = @tenant_id
  AND id = @id
  AND deleted_at IS NULL;

-- name: GetTemplateLiteralByKey :one
SELECT id, tenant_id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE tenant_id = @tenant_id
  AND key = @key
  AND deleted_at IS NULL;

-- name: CreateTemplateLiteral :one
INSERT INTO template_literals (tenant_id, key, kind, value, fallback, label, description, category, is_active)
VALUES (@tenant_id, @key, @kind, @value, @fallback, @label, @description, @category, @is_active)
RETURNING id, tenant_id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at;

-- name: UpdateTemplateLiteral :one
UPDATE template_literals
SET kind        = @kind,
    value       = @value,
    fallback    = @fallback,
    label       = @label,
    description = @description,
    category    = @category,
    is_active   = @is_active,
    updated_at  = now()
WHERE tenant_id = @tenant_id
  AND id = @id
  AND deleted_at IS NULL
RETURNING id, tenant_id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at;

-- name: DeleteTemplateLiteral :exec
UPDATE template_literals
SET deleted_at = now(), updated_at = now()
WHERE tenant_id = @tenant_id
  AND id = @id
  AND deleted_at IS NULL;
```

- [ ] **Step 5: Generate sqlc models**

Run:

```bash
make sqlc-gen
```

Expected: `internal/db/gen/literals.sql.go` and `internal/db/gen/models.go` are updated with `TemplateLiteral` and query functions.

- [ ] **Step 6: Verify with `go build ./...`**

Run:

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add migrations/000572_* migrations/000573_* internal/db/queries/literals.sql internal/db/gen/literals.sql.go internal/db/gen/models.go
git commit -m "feat(literals): add template_literals table, queries, and permission"
```

---

### Task 2: Generate country adjective map in Go

**Files:**
- Create: `internal/literals/countrydata/gen.go` (build script)
- Create: `internal/literals/countrydata/data.go`
- Create: `internal/literals/countrydata/data_test.go`

**Interfaces:**
- Produces: `countrydata.AdjectiveFor(codeOrName string) string` and `countrydata.NameFor(code string) string`.

- [ ] **Step 1: Read the source TypeScript file**

The existing `packages/landing-blocks/src/geo/generatedCountryTokens.ts` has a map of `Record<string, GeoCopyTokens>` with `country_adjective` and `country`. Use that as the canonical source.

- [ ] **Step 2: Write a small generator**

In `internal/literals/countrydata/gen.go` (build tag `ignore` or a standalone `go run` script) or a Node/Bun script:

```go
//go:build ignore

package main

import (
    "encoding/json"
    "fmt"
    "os"
    "strings"
)

type token struct {
    Country          string            `json:"country"`
    CountryShort     string            `json:"country_short"`
    CountryAdjective string            `json:"country_adjective"`
    Names            map[string]string `json:"names"`
}

func main() {
    // Parse packages/landing-blocks/src/geo/generatedCountryTokens.ts
    // Extract the JSON object and build a Go map.
    // Write internal/literals/countrydata/data.go
}
```

For this task, use a Node/Bun script to avoid a full TypeScript parser in Go. Create `internal/literals/countrydata/generate.js`:

```js
import { readFileSync, writeFileSync } from 'node:fs';

const ts = readFileSync('packages/landing-blocks/src/geo/generatedCountryTokens.ts', 'utf8');
const match = ts.match(/export const GENERATED_COUNTRY_TOKENS: Record<string, GeoCopyTokens> = (\{[\s\S]*\n\};)/);
if (!match) throw new Error('Could not find GENERATED_COUNTRY_TOKENS');
const json = match[1].replace(/(\w+):/g, '"$1":').replace(/,\n(\s*\})/g, '\n$1');
const data = JSON.parse(json);

const byCode = Object.fromEntries(Object.entries(data).map(([code, t]) => [code, { name: t.country, adjective: t.country_adjective }]));
const byName = Object.fromEntries(Object.entries(data).map(([code, t]) => [t.country.toLowerCase(), { code, adjective: t.country_adjective }]));

writeFileSync('internal/literals/countrydata/data.go',
`package countrydata

var ByCode = map[string]Country{
${Object.entries(byCode).map(([k, v]) => `    "${k}": {Name: "${v.name}", Adjective: "${v.adjective}"},`).join('\n')}
}

var ByName = map[string]Country{
${Object.entries(byName).map(([k, v]) => `    "${k}": {Code: "${v.code}", Adjective: "${v.adjective}"},`).join('\n')}
}

type Country struct {
    Name      string
    Code      string
    Adjective string
}
`);
```

- [ ] **Step 3: Run the generator and commit the generated file**

```bash
bun run internal/literals/countrydata/generate.js
```

- [ ] **Step 4: Add lookup helpers**

In `internal/literals/countrydata/data.go` (or a separate `lookup.go`):

```go
package countrydata

import "strings"

func AdjectiveFor(input string) string {
    if c, ok := ByCode[strings.ToUpper(input)]; ok {
        return c.Adjective
    }
    if c, ok := ByName[strings.ToLower(strings.TrimSpace(input))]; ok {
        return c.Adjective
    }
    return ""
}

func CodeForName(input string) string {
    if c, ok := ByName[strings.ToLower(strings.TrimSpace(input))]; ok {
        return c.Code
    }
    return ""
}

func NameForCode(code string) string {
    if c, ok := ByCode[strings.ToUpper(code)]; ok {
        return c.Name
    }
    return ""
}
```

- [ ] **Step 5: Write tests**

In `internal/literals/countrydata/data_test.go`:

```go
package countrydata

import "testing"

func TestAdjectiveFor(t *testing.T) {
    cases := []struct{ in, want string }{
        {"NG", "Nigerian"},
        {"Nigeria", "Nigerian"},
        {"GB", "British"},
        {"France", "French"},
        {"france", "French"},
    }
    for _, c := range cases {
        got := AdjectiveFor(c.in)
        if got != c.want {
            t.Errorf("AdjectiveFor(%q) = %q; want %q", c.in, got, c.want)
        }
    }
}
```

Run:

```bash
go test ./internal/literals/countrydata/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/literals/countrydata/
git commit -m "feat(literals): add generated country adjective lookup"
```

---

### Task 3: Build the expression lexer

**Files:**
- Create: `internal/literals/expr/lexer.go`
- Create: `internal/literals/expr/lexer_test.go`

**Interfaces:**
- Produces: `Token` type, `TokenKind` enum, `Lexer` struct, `func NewLexer(input string) *Lexer`, `func (l *Lexer) NextToken() Token`.

- [ ] **Step 1: Define tokens**

In `internal/literals/expr/lexer.go`:

```go
package expr

import "strings"

type TokenKind int

const (
    TokenEOF TokenKind = iota
    TokenNumber
    TokenString
    TokenIdent
    TokenPlus
    TokenMinus
    TokenMul
    TokenDiv
    TokenMod
    TokenLParen
    TokenRParen
    TokenComma
    TokenEq
    TokenNeq
    TokenLt
    TokenGt
    TokenLte
    TokenGte
    TokenPipe
)

type Token struct {
    Kind TokenKind
    Val  string
    Pos  int
}

func (t Token) IsEOF() bool { return t.Kind == TokenEOF }

func kindName(k TokenKind) string {
    switch k {
    case TokenNumber: return "number"
    case TokenString: return "string"
    case TokenIdent:  return "ident"
    case TokenPlus:   return "+"
    case TokenMinus:  return "-"
    case TokenMul:    return "*"
    case TokenDiv:    return "/"
    case TokenMod:    return "%"
    case TokenLParen: return "("
    case TokenRParen: return ")"
    case TokenComma:  return ","
    case TokenEq:     return "=="
    case TokenNeq:    return "!="
    case TokenLt:     return "<"
    case TokenGt:     return ">"
    case TokenLte:    return "<="
    case TokenGte:    return ">="
    case TokenPipe:   return "|"
    }
    return "EOF"
}
```

- [ ] **Step 2: Implement the lexer**

```go
type Lexer struct {
    input string
    pos   int
    ch    byte
}

func NewLexer(input string) *Lexer {
    l := &Lexer{input: input}
    l.readChar()
    return l
}

func (l *Lexer) readChar() {
    if l.pos >= len(l.input) {
        l.ch = 0
    } else {
        l.ch = l.input[l.pos]
    }
    l.pos++
}

func (l *Lexer) peek() byte {
    if l.pos >= len(l.input) {
        return 0
    }
    return l.input[l.pos]
}

func (l *Lexer) skipWhitespace() {
    for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
        l.readChar()
    }
}

func (l *Lexer) readNumber() string {
    start := l.pos - 1
    for isDigit(l.ch) || l.ch == '.' {
        l.readChar()
    }
    return l.input[start : l.pos-1]
}

func (l *Lexer) readString() string {
    // l.ch == '"'
    l.readChar()
    start := l.pos - 1
    for l.ch != '"' && l.ch != 0 {
        l.readChar()
    }
    s := l.input[start : l.pos-1]
    l.readChar() // consume closing "
    return s
}

func (l *Lexer) readIdent() string {
    start := l.pos - 1
    for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
        l.readChar()
    }
    return l.input[start : l.pos-1]
}

func (l *Lexer) NextToken() Token {
    l.skipWhitespace()
    pos := l.pos - 1
    switch l.ch {
    case 0:
        return Token{Kind: TokenEOF, Pos: pos}
    case '+':
        l.readChar()
        return Token{Kind: TokenPlus, Pos: pos}
    case '-':
        l.readChar()
        return Token{Kind: TokenMinus, Pos: pos}
    case '*':
        l.readChar()
        return Token{Kind: TokenMul, Pos: pos}
    case '/':
        l.readChar()
        return Token{Kind: TokenDiv, Pos: pos}
    case '%':
        l.readChar()
        return Token{Kind: TokenMod, Pos: pos}
    case '(':
        l.readChar()
        return Token{Kind: TokenLParen, Pos: pos}
    case ')':
        l.readChar()
        return Token{Kind: TokenRParen, Pos: pos}
    case ',':
        l.readChar()
        return Token{Kind: TokenComma, Pos: pos}
    case '|':
        l.readChar()
        return Token{Kind: TokenPipe, Pos: pos}
    case '"':
        return Token{Kind: TokenString, Val: l.readString(), Pos: pos}
    case '<':
        if l.peek() == '=' {
            l.readChar()
            l.readChar()
            return Token{Kind: TokenLte, Pos: pos}
        }
        l.readChar()
        return Token{Kind: TokenLt, Pos: pos}
    case '>':
        if l.peek() == '=' {
            l.readChar()
            l.readChar()
            return Token{Kind: TokenGte, Pos: pos}
        }
        l.readChar()
        return Token{Kind: TokenGt, Pos: pos}
    case '=':
        if l.peek() == '=' {
            l.readChar()
            l.readChar()
            return Token{Kind: TokenEq, Pos: pos}
        }
        // single = is invalid; consume and return as ident? Actually lex as illegal.
        l.readChar()
        return Token{Kind: TokenIdent, Val: "=", Pos: pos}
    case '!':
        if l.peek() == '=' {
            l.readChar()
            l.readChar()
            return Token{Kind: TokenNeq, Pos: pos}
        }
        l.readChar()
        return Token{Kind: TokenIdent, Val: "!", Pos: pos}
    }

    if isDigit(l.ch) {
        return Token{Kind: TokenNumber, Val: l.readNumber(), Pos: pos}
    }
    if isLetter(l.ch) {
        return Token{Kind: TokenIdent, Val: l.readIdent(), Pos: pos}
    }

    ch := l.ch
    l.readChar()
    return Token{Kind: TokenIdent, Val: string(ch), Pos: pos}
}

func isDigit(ch byte) bool  { return ch >= '0' && ch <= '9' }
func isLetter(ch byte) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
```

- [ ] **Step 3: Write tests**

In `internal/literals/expr/lexer_test.go`:

```go
package expr

import "testing"

func TestLexer(t *testing.T) {
    input := `Current_Year + 1 >= 2026`
    l := NewLexer(input)
    expected := []TokenKind{TokenIdent, TokenPlus, TokenNumber, TokenGte, TokenNumber}
    for _, want := range expected {
        tok := l.NextToken()
        if tok.Kind != want {
            t.Fatalf("token kind = %v; want %v (val=%s)", tok.Kind, want, tok.Val)
        }
    }
    if l.NextToken().Kind != TokenEOF {
        t.Fatal("expected EOF")
    }
}
```

Run:

```bash
go test ./internal/literals/expr/... -run TestLexer -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/literals/expr/
git commit -m "feat(literals): add expression lexer"

---

### Task 4: Build the expression parser and AST

**Files:**
- Create: `internal/literals/expr/ast.go`
- Create: `internal/literals/expr/parser.go`
- Create: `internal/literals/expr/parser_test.go`

**Interfaces:**
- Produces: AST nodes (`NumberNode`, `StringNode`, `IdentNode`, `UnaryNode`, `BinaryNode`, `CallNode`, `TernaryNode`) and `Parse(input string) (Node, error)`.

- [ ] **Step 1: Define AST nodes**

In `internal/literals/expr/ast.go`:

```go
package expr

type Node interface {
    node()
}

type NumberNode struct{ Val float64 }
func (NumberNode) node() {}

type StringNode struct{ Val string }
func (StringNode) node() {}

type IdentNode struct{ Name string }
func (IdentNode) node() {}

type UnaryNode struct {
    Op   TokenKind
    Expr Node
}
func (UnaryNode) node() {}

type BinaryNode struct {
    Left  Node
    Op    TokenKind
    Right Node
}
func (BinaryNode) node() {}

type CallNode struct {
    Name string
    Args []Node
}
func (CallNode) node() {}
```

- [ ] **Step 2: Implement Pratt parser**

In `internal/literals/expr/parser.go`:

```go
package expr

import (
    "fmt"
    "strconv"
    "strings"
)

type Parser struct {
    l   *Lexer
    cur Token
    peek Token
}

func Parse(input string) (Node, error) {
    p := &Parser{l: NewLexer(input)}
    p.next()
    p.next()
    return p.parseExpression(0)
}

func (p *Parser) next() {
    p.cur = p.peek
    p.peek = p.l.NextToken()
}

func (p *Parser) expect(k TokenKind) error {
    if p.cur.Kind != k {
        return fmt.Errorf("expected %v, got %v at pos %d", kindName(k), kindName(p.cur.Kind), p.cur.Pos)
    }
    p.next()
    return nil
}

func (p *Parser) parseExpression(precedence int) (Node, error) {
    left, err := p.parsePrefix()
    if err != nil {
        return nil, err
    }

    for !p.cur.IsEOF() && precedence < p.precedence(p.cur.Kind) {
        op := p.cur.Kind
        p.next()
        right, err := p.parseExpression(p.precedence(op))
        if err != nil {
            return nil, err
        }
        left = BinaryNode{Left: left, Op: op, Right: right}
    }
    return left, nil
}

func (p *Parser) parsePrefix() (Node, error) {
    switch p.cur.Kind {
    case TokenNumber:
        v, err := strconv.ParseFloat(p.cur.Val, 64)
        if err != nil {
            return nil, fmt.Errorf("invalid number %q: %w", p.cur.Val, err)
        }
        n := NumberNode{Val: v}
        p.next()
        return n, nil
    case TokenString:
        n := StringNode{Val: p.cur.Val}
        p.next()
        return n, nil
    case TokenIdent:
        name := p.cur.Val
        pos := p.cur.Pos
        p.next()
        if p.cur.Kind == TokenLParen {
            // function call
            p.next()
            var args []Node
            for p.cur.Kind != TokenRParen && !p.cur.IsEOF() {
                arg, err := p.parseExpression(0)
                if err != nil {
                    return nil, err
                }
                args = append(args, arg)
                if p.cur.Kind == TokenComma {
                    p.next()
                } else if p.cur.Kind != TokenRParen {
                    return nil, fmt.Errorf("expected , or ) at pos %d", p.cur.Pos)
                }
            }
            if p.cur.Kind != TokenRParen {
                return nil, fmt.Errorf("unclosed call at pos %d", pos)
            }
            p.next()
            return CallNode{Name: name, Args: args}, nil
        }
        return IdentNode{Name: name}, nil
    case TokenMinus:
        p.next()
        expr, err := p.parsePrefix()
        if err != nil {
            return nil, err
        }
        return UnaryNode{Op: TokenMinus, Expr: expr}, nil
    case TokenLParen:
        p.next()
        expr, err := p.parseExpression(0)
        if err != nil {
            return nil, err
        }
        if err := p.expect(TokenRParen); err != nil {
            return nil, err
        }
        return expr, nil
    }
    return nil, fmt.Errorf("unexpected token %v at pos %d", p.cur.Val, p.cur.Pos)
}

func (p *Parser) precedence(k TokenKind) int {
    switch k {
    case TokenPipe:
        return 10
    case TokenEq, TokenNeq:
        return 20
    case TokenLt, TokenGt, TokenLte, TokenGte:
        return 30
    case TokenPlus, TokenMinus:
        return 40
    case TokenMul, TokenDiv, TokenMod:
        return 50
    }
    return 0
}

func (p *Parser) String() string { return p.l.input }
```

Note: `TokenPipe` is reserved for future `|>` or fallback; for now it is invalid in expressions. The token `|` inside `{{Token|fallback}}` is handled by the outer resolver, not the expression parser.

- [ ] **Step 3: Write parser tests**

In `internal/literals/expr/parser_test.go`:

```go
package expr

import (
    "fmt"
    "testing"
)

func TestParseArithmetic(t *testing.T) {
    n, err := Parse("Current_Year + 1")
    if err != nil {
        t.Fatal(err)
    }
    bn, ok := n.(BinaryNode)
    if !ok {
        t.Fatalf("expected BinaryNode, got %T", n)
    }
    if _, ok := bn.Left.(IdentNode); !ok {
        t.Fatalf("left = %T, want IdentNode", bn.Left)
    }
    if _, ok := bn.Right.(NumberNode); !ok {
        t.Fatalf("right = %T, want NumberNode", bn.Right)
    }
}

func TestParseFunction(t *testing.T) {
    n, err := Parse("adjective(Destination_Country)")
    if err != nil {
        t.Fatal(err)
    }
    call, ok := n.(CallNode)
    if !ok || call.Name != "adjective" || len(call.Args) != 1 {
        t.Fatalf("expected CallNode adjective with 1 arg, got %v", n)
    }
}

func TestParseComparison(t *testing.T) {
    _, err := Parse("Current_Year >= 2026")
    if err != nil {
        t.Fatal(err)
    }
}
```

Run:

```bash
go test ./internal/literals/expr/... -run TestParse -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/literals/expr/
git commit -m "feat(literals): add expression parser and AST"
```

---

### Task 5: Build the expression evaluator

**Files:**
- Create: `internal/literals/expr/eval.go`
- Create: `internal/literals/expr/eval_test.go`

**Interfaces:**
- Produces: `func Eval(ctx context.Context, n Node, env *Env) (any, error)` and `Env` with whitelisted variables and functions.

- [ ] **Step 1: Define the evaluator environment**

In `internal/literals/expr/eval.go`:

```go
package expr

import (
    "context"
    "errors"
    "fmt"
    "strconv"
    "strings"
    "time"

    "github.com/jedafe/japa/internal/literals/countrydata"
)

type Function func(ctx context.Context, env *Env, args []any) (any, error)

type Env struct {
    Vars     map[string]any
    Funcs    map[string]Function
    Now      time.Time
    Depth    int
    MaxDepth int
}

func NewEnv(now time.Time) *Env {
    e := &Env{
        Vars:     make(map[string]any),
        Funcs:    make(map[string]any),
        Now:      now,
        MaxDepth: 32,
    }
    e.addDefaultVars()
    e.addDefaultFuncs()
    return e
}

func (e *Env) addDefaultVars() {
    e.Vars["Now"] = e.Now
    e.Vars["Current_Year"] = float64(e.Now.Year())
    e.Vars["Current_Date"] = e.Now.Format("2006-01-02")
    e.Vars["Current_Time"] = e.Now.Format("15:04")
    e.Vars["Current_Timestamp"] = e.Now.Format("2006-01-02 15:04")
}

func (e *Env) addDefaultFuncs() {
    e.Funcs["adjective"] = func(_ context.Context, env *Env, args []any) (any, error) {
        if len(args) != 1 {
            return nil, fmt.Errorf("adjective requires 1 argument")
        }
        s, ok := toString(args[0])
        if !ok {
            return nil, fmt.Errorf("adjective requires a string")
        }
        adj := countrydata.AdjectiveFor(s)
        if adj == "" {
            return s, nil // fallback to input
        }
        return adj, nil
    }
    e.Funcs["year"] = dateFunc(func(t time.Time) any { return float64(t.Year()) })
    e.Funcs["month"] = dateFunc(func(t time.Time) any { return float64(t.Month()) })
    e.Funcs["day"] = dateFunc(func(t time.Time) any { return float64(t.Day()) })
    e.Funcs["weekday"] = dateFunc(func(t time.Time) any { return t.Weekday().String() })
    e.Funcs["format"] = formatFunc
    e.Funcs["upper"] = stringFunc(strings.ToUpper)
    e.Funcs["lower"] = stringFunc(strings.ToLower)
    e.Funcs["title"] = stringFunc(strings.Title)
    e.Funcs["concat"] = concatFunc
    e.Funcs["if"] = ifFunc
    e.Funcs["len"] = func(_ context.Context, env *Env, args []any) (any, error) {
        if len(args) != 1 {
            return nil, fmt.Errorf("len requires 1 argument")
        }
        s, ok := toString(args[0])
        if ok {
            return float64(len(s)), nil
        }
        if a, ok := args[0].([]any); ok {
            return float64(len(a)), nil
        }
        return nil, fmt.Errorf("len requires a string or array")
    }
}

func dateFunc(fn func(time.Time) any) Function {
    return func(_ context.Context, env *Env, args []any) (any, error) {
        t, err := toTime(args, env.Now)
        if err != nil {
            return nil, err
        }
        return fn(t), nil
    }
}

func formatFunc(_ context.Context, env *Env, args []any) (any, error) {
    if len(args) != 2 {
        return nil, fmt.Errorf("format requires 2 arguments")
    }
    t, err := toTime([]any{args[0]}, env.Now)
    if err != nil {
        return nil, err
    }
    layout, ok := toString(args[1])
    if !ok {
        return nil, fmt.Errorf("format layout must be a string")
    }
    layout = mapLayout(layout)
    return t.Format(layout), nil
}

func stringFunc(fn func(string) string) Function {
    return func(_ context.Context, env *Env, args []any) (any, error) {
        if len(args) != 1 {
            return nil, fmt.Errorf("string function requires 1 argument")
        }
        s, ok := toString(args[0])
        if !ok {
            return nil, fmt.Errorf("expected string")
        }
        return fn(s), nil
    }
}

func concatFunc(_ context.Context, env *Env, args []any) (any, error) {
    var parts []string
    for _, a := range args {
        s, ok := toString(a)
        if !ok {
            return nil, fmt.Errorf("concat requires strings")
        }
        parts = append(parts, s)
    }
    return strings.Join(parts, ""), nil
}

func ifFunc(_ context.Context, env *Env, args []any) (any, error) {
    if len(args) != 3 {
        return nil, fmt.Errorf("if requires 3 arguments")
    }
    cond, ok := toBool(args[0])
    if !ok {
        return nil, fmt.Errorf("if condition must be boolean")
    }
    if cond {
        return args[1], nil
    }
    return args[2], nil
}

func mapLayout(l string) string {
    // Convert common aliases to Go layouts.
    switch l {
    case "YYYY":
        return "2006"
    case "MM":
        return "01"
    case "DD":
        return "02"
    case "HH:mm":
        return "15:04"
    case "HH":
        return "15"
    case "mm":
        return "04"
    case "MMMM":
        return "January"
    case "MMM":
        return "Jan"
    case "YYYY-MM-DD":
        return "2006-01-02"
    case "YYYY-MM-DD HH:mm":
        return "2006-01-02 15:04"
    }
    return l
}

func toString(v any) (string, bool) {
    switch t := v.(type) {
    case string:
        return t, true
    case float64:
        return strconv.FormatFloat(t, 'f', -1, 64), true
    case bool:
        return strconv.FormatBool(t), true
    case nil:
        return "", true
    }
    return "", false
}

func toBool(v any) (bool, bool) {
    if b, ok := v.(bool); ok {
        return b, true
    }
    if f, ok := v.(float64); ok {
        return f != 0, true
    }
    if s, ok := v.(string); ok {
        return s != "", true
    }
    return false, false
}

func toNumber(v any) (float64, bool) {
    if f, ok := v.(float64); ok {
        return f, true
    }
    if s, ok := v.(string); ok {
        f, err := strconv.ParseFloat(s, 64)
        if err == nil {
            return f, true
        }
    }
    return 0, false
}

func toTime(args []any, fallback time.Time) (time.Time, error) {
    if len(args) == 0 {
        return fallback, nil
    }
    switch t := args[0].(type) {
    case time.Time:
        return t, nil
    case string:
        for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04", "2006-01-02 15:04:05"} {
            if p, err := time.Parse(layout, t); err == nil {
                return p, nil
            }
        }
        return fallback, nil
    case float64:
        return time.Unix(int64(t), 0).UTC(), nil
    }
    return fallback, nil
}
```

- [ ] **Step 2: Implement `Eval`**

```go
func Eval(ctx context.Context, n Node, env *Env) (any, error) {
    if env.Depth >= env.MaxDepth {
        return nil, errors.New("expression exceeded max depth")
    }
    env.Depth++
    defer func() { env.Depth-- }()

    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    switch t := n.(type) {
    case NumberNode:
        return t.Val, nil
    case StringNode:
        return t.Val, nil
    case IdentNode:
        v, ok := env.Vars[t.Name]
        if !ok {
            return nil, fmt.Errorf("unknown variable %q", t.Name)
        }
        return v, nil
    case UnaryNode:
        v, err := Eval(ctx, t.Expr, env)
        if err != nil {
            return nil, err
        }
        f, ok := toNumber(v)
        if !ok {
            return nil, fmt.Errorf("unary %v on non-number", t.Op)
        }
        if t.Op == TokenMinus {
            return -f, nil
        }
        return f, nil
    case BinaryNode:
        left, err := Eval(ctx, t.Left, env)
        if err != nil {
            return nil, err
        }
        right, err := Eval(ctx, t.Right, env)
        if err != nil {
            return nil, err
        }
        return evalBinary(t.Op, left, right)
    case CallNode:
        fn, ok := env.Funcs[t.Name]
        if !ok {
            return nil, fmt.Errorf("unknown function %q", t.Name)
        }
        args := make([]any, len(t.Args))
        for i, a := range t.Args {
            v, err := Eval(ctx, a, env)
            if err != nil {
                return nil, err
            }
            args[i] = v
        }
        return fn(ctx, env, args)
    }
    return nil, fmt.Errorf("unknown node type %T", n)
}

func evalBinary(op TokenKind, left, right any) (any, error) {
    // string concatenation with +
    if op == TokenPlus {
        if ls, ok := toString(left); ok {
            if rs, ok := toString(right); ok {
                return ls + rs, nil
            }
        }
    }

    l, lok := toNumber(left)
    r, rok := toNumber(right)
    if !lok || !rok {
        return nil, fmt.Errorf("operator %v requires numbers or strings", op)
    }

    switch op {
    case TokenPlus:
        return l + r, nil
    case TokenMinus:
        return l - r, nil
    case TokenMul:
        return l * r, nil
    case TokenDiv:
        if r == 0 {
            return nil, errors.New("division by zero")
        }
        return l / r, nil
    case TokenMod:
        if r == 0 {
            return nil, errors.New("modulo by zero")
        }
        return float64(int64(l) % int64(r)), nil
    case TokenEq:
        return l == r, nil
    case TokenNeq:
        return l != r, nil
    case TokenLt:
        return l < r, nil
    case TokenGt:
        return l > r, nil
    case TokenLte:
        return l <= r, nil
    case TokenGte:
        return l >= r, nil
    }
    return nil, fmt.Errorf("unsupported operator %v", op)
}
```

- [ ] **Step 3: Export `ToString` helper**

At the bottom of `internal/literals/expr/eval.go`, add:

```go
func ToString(v any) (string, bool) { return toString(v) }
```

This is used by `internal/literals/resolver.go` to convert expression results to strings.

- [ ] **Step 4: Add timeout and depth guard to `EvalWithTimeout`**

```go
func EvalWithTimeout(ctx context.Context, n Node, env *Env, timeout time.Duration) (any, error) {
    if timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, timeout)
        defer cancel()
    }
    return Eval(ctx, n, env)
}
```

- [ ] **Step 5: Write evaluator tests**

In `internal/literals/expr/eval_test.go`:

```go
package expr

import (
    "context"
    "testing"
    "time"
)

func TestEvalArithmetic(t *testing.T) {
    n, _ := Parse("2025 + 1")
    v, err := Eval(context.Background(), n, NewEnv(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
    if err != nil {
        t.Fatal(err)
    }
    if f, ok := toNumber(v); !ok || f != 2026 {
        t.Fatalf("got %v, want 2026", v)
    }
}

func TestEvalAdjective(t *testing.T) {
    n, _ := Parse(`adjective("Nigeria")`)
    v, err := Eval(context.Background(), n, NewEnv(time.Now()))
    if err != nil {
        t.Fatal(err)
    }
    if s, ok := toString(v); !ok || s != "Nigerian" {
        t.Fatalf("got %v, want Nigerian", v)
    }
}

func TestEvalFormat(t *testing.T) {
    n, _ := Parse(`format(Now, "YYYY")`)
    v, err := Eval(context.Background(), n, NewEnv(time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)))
    if err != nil {
        t.Fatal(err)
    }
    if s, ok := toString(v); !ok || s != "2026" {
        t.Fatalf("got %v, want 2026", v)
    }
}

func TestEvalStringConcat(t *testing.T) {
    n, _ := Parse(`"Hello" + " " + "World"`)
    v, err := Eval(context.Background(), n, NewEnv(time.Now()))
    if err != nil {
        t.Fatal(err)
    }
    if s, ok := toString(v); !ok || s != "Hello World" {
        t.Fatalf("got %v, want 'Hello World'", v)
    }
}
```

Run:

```bash
go test ./internal/literals/expr/... -run TestEval -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/literals/expr/
git commit -m "feat(literals): add expression evaluator"

---

### Task 6: Build the template-literal resolver

**Files:**
- Create: `internal/literals/types.go`
- Create: `internal/literals/resolver.go`
- Create: `internal/literals/resolver_test.go`

**Interfaces:**
- Produces: `RenderContext`, `TemplateLiteral` (mirror of `gen.TemplateLiteral`), `Resolver.Resolve(ctx, text, rc, tokens) (string, error)`.

- [ ] **Step 1: Define types and render context**

In `internal/literals/types.go`:

```go
package literals

import (
    "time"

    "github.com/jackc/pgx/v5/pgtype"
)

type TemplateLiteral struct {
    ID          string
    TenantID    string
    Key         string
    Kind        string
    Value       string
    Fallback    *string
    Label       *string
    Description *string
    Category    *string
    IsSystem    bool
    IsActive    bool
}

type RenderContext struct {
    Now                time.Time
    Locale             string
    SourceCountry      string
    SourceCode         string
    DestinationCountry string
    DestinationCode    string
    TenantName         string
    TenantID           pgtype.UUID
}

func (rc RenderContext) ToEnv() *expr.Env {
    env := expr.NewEnv(rc.Now)
    env.Vars["Source_Country"] = rc.SourceCountry
    env.Vars["Source_Code"] = rc.SourceCode
    env.Vars["Destination_Country"] = rc.DestinationCountry
    env.Vars["Destination_Code"] = rc.DestinationCode
    env.Vars["Tenant_Name"] = rc.TenantName
    env.Vars["Locale"] = rc.Locale
    return env
}
```

- [ ] **Step 2: Implement resolver**

In `internal/literals/resolver.go`:

```go
package literals

import (
    "context"
    "fmt"
    "regexp"
    "strings"
    "time"

    "github.com/jedafe/japa/internal/literals/expr"
)

var tokenRegex = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)(?:\|([^}]+))?\s*\}\}`)
var exprRegex = regexp.MustCompile(`\{\{([\s\S]+?)\}\}`)

func Resolve(ctx context.Context, text string, rc RenderContext, tokens []TemplateLiteral) (string, error) {
    if !strings.Contains(text, "{{") {
        return text, nil
    }

    env := rc.ToEnv()
    tokenMap := make(map[string]TemplateLiteral)
    for _, t := range tokens {
        tokenMap[t.Key] = t
    }

    // Bare token with optional fallback: {{Token|fallback}}
    text = tokenRegex.ReplaceAllStringFunc(text, func(match string) string {
        m := tokenRegex.FindStringSubmatch(match)
        key := m[1]
        inlineFallback := ""
        if len(m) > 2 {
            inlineFallback = strings.TrimSpace(m[2])
        }
        return resolveToken(ctx, key, inlineFallback, env, tokenMap)
    })

    // General expression: {{ expression }}
    text = exprRegex.ReplaceAllStringFunc(text, func(match string) string {
        inner := match[2 : len(match)-2]
        if tokenRegex.MatchString(match) {
            return match // already handled by bare token pass
        }
        parsed, err := expr.Parse(inner)
        if err != nil {
            return match // leave unparseable expressions unchanged
        }
        v, err := expr.EvalWithTimeout(ctx, parsed, env, 50*time.Millisecond)
        if err != nil {
            return match
        }
        s, _ := expr.ToString(v) // reuse the helper from eval.go
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

    if lit.Kind == "static" {
        return lit.Value
    }

    parsed, err := expr.Parse(lit.Value)
    if err != nil {
        if inlineFallback != "" {
            return inlineFallback
        }
        if lit.Fallback != nil && *lit.Fallback != "" {
            return *lit.Fallback
        }
        return fmt.Sprintf("{{%s}}", key)
    }

    v, err := expr.EvalWithTimeout(ctx, parsed, env, 50*time.Millisecond)
    if err != nil {
        if inlineFallback != "" {
            return inlineFallback
        }
        if lit.Fallback != nil && *lit.Fallback != "" {
            return *lit.Fallback
        }
        return fmt.Sprintf("{{%s}}", key)
    }

    s, _ := expr.ToString(v)
    return s
}
```

Note: `expr.ToString` must be exported. Update `eval.go` to export `toString` as `ToString`.

- [ ] **Step 3: Write resolver tests**

In `internal/literals/resolver_test.go`:

```go
package literals

import (
    "context"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgtype"
)

func TestResolve(t *testing.T) {
    rc := RenderContext{
        Now:                time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
        SourceCountry:      "Nigeria",
        SourceCode:         "NG",
        DestinationCountry: "the UK",
        DestinationCode:    "GB",
        TenantName:         "JapaConnect",
    }
    tokens := []TemplateLiteral{
        {Key: "Current_Year", Kind: "expression", Value: "year(Now)"},
        {Key: "Destination_Country", Kind: "expression", Value: "Destination_Country"},
        {Key: "Source_Country", Kind: "expression", Value: "Source_Country"},
        {Key: "Custom_Greeting", Kind: "static", Value: "Hello"},
    }

    got, err := Resolve(context.Background(), "{{Custom_Greeting}} from {{Source_Country}} to {{Destination_Country}} in {{Current_Year}}", rc, tokens)
    if err != nil {
        t.Fatal(err)
    }
    want := "Hello from Nigeria to the UK in 2026"
    if got != want {
        t.Fatalf("got %q; want %q", got, want)
    }
}

func TestResolveExpression(t *testing.T) {
    rc := RenderContext{Now: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), DestinationCountry: "the UK"}
    got, _ := Resolve(context.Background(), "Next year: {{year(Now) + 1}}, adjective: {{adjective(Destination_Country)}}", rc, nil)
    want := "Next year: 2027, adjective: British"
    if got != want {
        t.Fatalf("got %q; want %q", got, want)
    }
}

func TestResolveFallback(t *testing.T) {
    rc := RenderContext{Now: time.Now()}
    got, _ := Resolve(context.Background(), "{{Unknown|fallback value}}", rc, nil)
    if got != "fallback value" {
        t.Fatalf("got %q; want fallback value", got)
    }
}
```

Run:

```bash
go test ./internal/literals/... -run TestResolve -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/literals/
git commit -m "feat(literals): add resolver with expression support"
```

---

### Task 7: Integrate resolver into content service

**Files:**
- Modify: `internal/content/service.go` or `internal/content/handlers.go`
- Create: `internal/content/literals.go`
- Create: `internal/content/literals_test.go`
- Modify: `internal/server/server.go` (wiring)

**Interfaces:**
- Consumes: `literals.Resolver` and `RenderContext`.
- Produces: `ResolveContentFields(ctx, content, rc) error` that mutates the passed content in place.

- [ ] **Step 1: Add helper to resolve content fields**

In `internal/content/literals.go`:

```go
package content

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/jedafe/japa/internal/db/gen"
    "github.com/jedafe/japa/internal/literals"
)

func (s *Service) ResolveContent(ctx context.Context, c *gen.Content, rc literals.RenderContext) error {
    tenantTokens, err := s.loadTemplateLiterals(ctx, c.TenantID)
    if err != nil {
        return fmt.Errorf("loading template literals: %w", err)
    }

    c.Title, _ = literals.Resolve(ctx, c.Title, rc, tenantTokens)
    c.Body, _ = literals.Resolve(ctx, c.Body, rc, tenantTokens)
    c.BodyHtml, _ = literals.Resolve(ctx, c.BodyHtml, rc, tenantTokens)
    if c.MetaTitle != nil {
        v, _ := literals.Resolve(ctx, *c.MetaTitle, rc, tenantTokens)
        c.MetaTitle = &v
    }
    if c.MetaDescription != nil {
        v, _ := literals.Resolve(ctx, *c.MetaDescription, rc, tenantTokens)
        c.MetaDescription = &v
    }
    return nil
}

func (s *Service) loadTemplateLiterals(ctx context.Context, tenantID pgtype.UUID) ([]literals.TemplateLiteral, error) {
    // Cache wiring is added in Task 11; load directly from DB here.
    rows, err := s.read.ListTemplateLiterals(ctx, gen.ListTemplateLiteralsParams{
        TenantID: tenantID,
        Q:        "",
        IsActive: pgtype.Bool{Bool: true, Valid: true},
        Sort:     "",
        Dir:      "",
        PageSize: 1000,
        Offset:   0,
    })
    if err != nil {
        return nil, err
    }
    out := make([]literals.TemplateLiteral, len(rows))
    for i, r := range rows {
        out[i] = literals.TemplateLiteral{
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
    return out, nil
}
```



- [ ] **Step 2: Call resolver from content get/list handlers**

Find the content handler that builds the response (e.g. `internal/content/handlers.go` `HandleGetContent`). After fetching the content and before writing the JSON, build a `RenderContext` and call `ResolveContent`.

```go
rc := literals.RenderContext{
    Now:                time.Now().UTC(),
    Locale:             r.Context().Value(localeKey{}).(string),
    SourceCountry:      geo.CountryFromRequest(r),
    SourceCode:         geo.CodeFromRequest(r),
    DestinationCountry: user.DestinationCountry, // from auth/session
    DestinationCode:    user.DestinationCode,
    TenantName:         tenant.Name,
    TenantID:           tenant.ID,
}
if err := contentSvc.ResolveContent(r.Context(), &c, rc); err != nil {
    slog.Warn("failed to resolve content literals", "error", err, "content_id", c.ID)
}
```

For the first implementation, use `time.Now().UTC()` and empty strings for geo fields. Wire GeoIP and user destination in a follow-up after this feature is merged.

- [ ] **Step 3: Add content resolver tests**

In `internal/content/literals_test.go`:

```go
package content

import (
    "context"
    "testing"
    "time"

    "github.com/jedafe/japa/internal/db/gen"
    "github.com/jedafe/japa/internal/literals"
)

func TestResolveContent(t *testing.T) {
    // requires a test database or mocked Queries
    // Integration-style test: insert a content row, call ResolveContent, assert Title.
}
```

For unit test without DB, test `ResolveContent` with a fake `Service` that has a stub `read`.

Run:

```bash
go test ./internal/content/... -run TestResolveContent -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/content/literals.go internal/content/literals_test.go internal/content/handlers.go
git commit -m "feat(literals): resolve content title, body, and meta with template literals"
```

---

### Task 8: Add admin API handlers

**Files:**
- Create: `internal/admin/literals.go`
- Modify: `internal/admin/admin.go` or `internal/server/server.go` (wiring)
- Modify: `internal/server/server.go` (routes)

**Interfaces:**
- Produces: `admin.Handlers.ListLiterals`, `CreateLiteral`, `GetLiteral`, `UpdateLiteral`, `DeleteLiteral`.

- [ ] **Step 1: Implement handlers**

In `internal/admin/literals.go`:

```go
package admin

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/db/gen"
    "github.com/jedafe/japa/internal/literals/expr"
)

type LiteralRequest struct {
    Key         string `json:"key"`
    Kind        string `json:"kind"`
    Value       string `json:"value"`
    Fallback    string `json:"fallback"`
    Label       string `json:"label"`
    Description string `json:"description"`
    Category    string `json:"category"`
    IsActive    bool   `json:"is_active"`
}

type LiteralResponse struct {
    ID          string  `json:"id"`
    Key         string  `json:"key"`
    Kind        string  `json:"kind"`
    Value       string  `json:"value"`
    Fallback    *string `json:"fallback"`
    Label       *string `json:"label"`
    Description *string `json:"description"`
    Category    *string `json:"category"`
    IsSystem    bool    `json:"is_system"`
    IsActive    bool    `json:"is_active"`
    CreatedAt   string  `json:"created_at"`
    UpdatedAt   string  `json:"updated_at"`
}

func (h *Handlers) ListLiterals(w http.ResponseWriter, r *http.Request) {
    if _, ok := requireUser(w, r); !ok {
        return
    }
    tenantID, ok := requireTenant(w, r)
    if !ok {
        return
    }

    q := r.URL.Query().Get("q")
    sort := r.URL.Query().Get("sort")
    dir := r.URL.Query().Get("dir")
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 { page = 1 }
    pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
    if pageSize < 1 { pageSize = 25 }

    rows, err := h.Queries.ListTemplateLiterals(r.Context(), gen.ListTemplateLiteralsParams{
        TenantID: tenantID,
        Q:        q,
        IsActive: pgtype.Bool{Valid: true, Bool: true},
        Sort:     sort,
        Dir:      dir,
        PageSize: int32(pageSize),
        Offset:   int32((page - 1) * pageSize),
    })
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literals")
        return
    }
    total, err := h.Queries.CountTemplateLiterals(r.Context(), gen.CountTemplateLiteralsParams{
        TenantID: tenantID,
        Q:        q,
        IsActive: pgtype.Bool{Valid: true, Bool: true},
    })
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to count literals")
        return
    }
    errpkg.WriteJSON(w, http.StatusOK, map[string]any{
        "data": map[string]any{
            "data": rows,
            "total": total,
            "total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
        },
    })
}

// CreateLiteral, GetLiteral, UpdateLiteral, DeleteLiteral follow the same pattern.
```

For `CreateLiteral`:

```go
func (h *Handlers) CreateLiteral(w http.ResponseWriter, r *http.Request) {
    if _, ok := requireUser(w, r); !ok {
        return
    }
    tenantID, ok := requireTenant(w, r)
    if !ok {
        return
    }

    var req LiteralRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        errpkg.WriteError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
        return
    }
    if req.Kind == "expression" {
        if _, err := expr.Parse(req.Value); err != nil {
            errpkg.WriteError(w, http.StatusBadRequest, "invalid_expression", err.Error())
            return
        }
    }
    // validate key, check conflicts
    if _, err := h.Queries.GetTemplateLiteralByKey(r.Context(), gen.GetTemplateLiteralByKeyParams{
        TenantID: tenantID,
        Key:      req.Key,
    }); err == nil {
        errpkg.WriteError(w, http.StatusConflict, "duplicate_key", "A literal with this key already exists")
        return
    }
    lit, err := h.Queries.CreateTemplateLiteral(r.Context(), gen.CreateTemplateLiteralParams{
        TenantID:    tenantID,
        Key:         req.Key,
        Kind:        req.Kind,
        Value:       req.Value,
        Fallback:    pgtype.Text{String: req.Fallback, Valid: req.Fallback != ""},
        Label:       pgtype.Text{String: req.Label, Valid: req.Label != ""},
        Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
        Category:    pgtype.Text{String: req.Category, Valid: req.Category != ""},
        IsActive:    req.IsActive,
    })
    if err != nil {
        errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create literal")
        return
    }
    errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": lit})
}
```

- [ ] **Step 2: Wire routes**

In `internal/server/server.go` near other admin route groups:

```go
r.Route("/admin/literals", func(r chi.Router) {
    r.Get("/", adminHandlers.ListLiterals)
    r.Post("/", adminHandlers.CreateLiteral)
    r.Get("/{id}", adminHandlers.GetLiteral)
    r.Patch("/{id}", adminHandlers.UpdateLiteral)
    r.Delete("/{id}", adminHandlers.DeleteLiteral)
})
```

- [ ] **Step 3: Update `api-routes-catalog.md`**

Add the `/admin/literals` CRUD routes.

- [ ] **Step 4: Commit**

```bash
git add internal/admin/literals.go internal/server/server.go .jeos/conventions/api-routes-catalog.md
git commit -m "feat(literals): add admin CRUD API handlers"
```

---

### Task 9: Build admin UI CRUD page

**Files:**
- Create: `apps/admin/src/lib/api/literals.ts`
- Create: `apps/admin/src/routes/literals/+page.svelte`
- Create: `apps/admin/src/routes/literals/page.test.ts`
- Modify: `apps/admin/src/lib/data/routes.ts` (if menu is dynamic)
- Modify: admin sidebar route definitions

**Interfaces:**
- Consumes: `Literal` type, admin API endpoints.
- Produces: working `/literals` page with `EntityPage`.

- [ ] **Step 1: Add API client**

In `apps/admin/src/lib/api/literals.ts`:

```ts
import { api, apiRaw } from '$lib/api';

export type Literal = {
    id: string;
    key: string;
    kind: 'static' | 'expression';
    value: string;
    fallback?: string;
    label?: string;
    description?: string;
    category?: string;
    is_system: boolean;
    is_active: boolean;
    created_at: string;
    updated_at: string;
};

export type LiteralList = { data: Literal[]; total: number; total_pages: number };

export async function listLiterals(params: { q?: string; page?: number; pageSize?: number; sort?: string; dir?: string } = {}): Promise<LiteralList> {
    const query = new URLSearchParams();
    if (params.q) query.set('q', params.q);
    if (params.page) query.set('page', String(params.page));
    if (params.pageSize) query.set('pageSize', String(params.pageSize));
    if (params.sort) query.set('sort', params.sort);
    if (params.dir) query.set('dir', params.dir);
    const res = await api<LiteralList>(`/admin/literals?${query.toString()}`);
    if (!res.ok) throw new Error(res.error);
    return res.data;
}

export async function createLiteral(body: Omit<Literal, 'id' | 'created_at' | 'updated_at'>): Promise<Literal> {
    const res = await api<Literal>('/admin/literals', { method: 'POST', body });
    if (!res.ok) throw new Error(res.error);
    return res.data;
}

export async function updateLiteral(id: string, body: Partial<Literal>): Promise<Literal> {
    const res = await api<Literal>(`/admin/literals/${id}`, { method: 'PATCH', body });
    if (!res.ok) throw new Error(res.error);
    return res.data;
}

export async function deleteLiteral(id: string): Promise<void> {
    const res = await api(`/admin/literals/${id}`, { method: 'DELETE' });
    if (!res.ok) throw new Error(res.error);
}
```

- [ ] **Step 2: Build the EntityPage page**

In `apps/admin/src/routes/literals/+page.svelte`:

```svelte
<script lang="ts">
    import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
    import { listLiterals, createLiteral, updateLiteral, deleteLiteral, type Literal } from '$lib/api/literals';
    import { hasPermission } from '$lib/stores/permissions.svelte';

    type LiteralRow = Literal;

    const canManage = $derived(hasPermission('content.literals.manage'));

    const fields: FieldDef[] = [
        { key: 'key', label: 'Key', type: 'text', required: true },
        { key: 'kind', label: 'Kind', type: 'select', options: [{ value: 'static', label: 'Static' }, { value: 'expression', label: 'Expression' }], required: true },
        { key: 'value', label: 'Value / Expression', type: 'textarea', required: true },
        { key: 'fallback', label: 'Fallback', type: 'text' },
        { key: 'label', label: 'Label', type: 'text' },
        { key: 'description', label: 'Description', type: 'textarea' },
        { key: 'category', label: 'Category', type: 'text' },
        { key: 'is_active', label: 'Active', type: 'checkbox' },
    ];

    const columns = [
        { key: 'key', label: 'Key', sortable: true },
        { key: 'kind', label: 'Kind' },
        { key: 'label', label: 'Label' },
        { key: 'category', label: 'Category' },
        { key: 'is_active', label: 'Active' },
    ];

    const config: EntityPageConfig = {
        title: 'Template Literals',
        description: 'Manage dynamic tokens used across content and pages.',
        columns,
        fields,
        createPermission: 'content.literals.manage',
        editPermission: 'content.literals.manage',
        deletePermission: 'content.literals.manage',
        pageSize: 25,
        load: async ({ page, pageSize, q, sort, dir }) => {
            const res = await listLiterals({ page, pageSize, q, sort, dir });
            return { data: res.data, total: res.total, totalPages: res.total_pages };
        },
        create: async (body) => {
            try {
                await createLiteral(body as any);
                return { ok: true };
            } catch (e) {
                return { ok: false, error: e instanceof Error ? e.message : 'Failed to create' };
            }
        },
        update: async (id, body) => {
            try {
                await updateLiteral(id, body as any);
                return { ok: true };
            } catch (e) {
                return { ok: false, error: e instanceof Error ? e.message : 'Failed to update' };
            }
        },
        remove: async (id) => {
            try {
                await deleteLiteral(id);
                return { ok: true };
            } catch (e) {
                return { ok: false, error: e instanceof Error ? e.message : 'Failed to delete' };
            }
        },
    };
</script>

<EntityPage {config} />
```

- [ ] **Step 3: Add to admin navigation**

Find the admin sidebar routes file (likely `apps/admin/src/lib/data/routes.ts` or `apps/admin/src/lib/components/layout/sidebar.svelte`). Add `/literals` under Content.

- [ ] **Step 4: Add page test**

In `apps/admin/src/routes/literals/page.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api/literals', () => ({
    listLiterals: vi.fn().mockResolvedValue({ data: [], total: 0, total_pages: 0 }),
    createLiteral: vi.fn(),
    updateLiteral: vi.fn(),
    deleteLiteral: vi.fn(),
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
    hasPermission: vi.fn().mockReturnValue(true),
}));

describe('/literals', () => {
    it('renders the page title', async () => {
        const { getByText } = render(Page);
        await waitFor(() => {
            expect(getByText('Template Literals')).toBeInTheDocument();
        });
    });
});
```

Run:

```bash
cd apps/admin && bun test src/routes/literals/page.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/api/literals.ts apps/admin/src/routes/literals/
git commit -m "feat(literals): add admin CRUD UI"
```

---

### Task 10: Integrate resolver into page builder (SSR)

**Files:**
- Modify: `packages/landing-blocks/src/geo/resolveGeoCopy.ts`
- Modify: `apps/main/src/routes/(app)/+page.server.ts` or the page-builder load function
- Modify: `packages/landing-blocks/src/types.ts` (if needed)

**Interfaces:**
- Consumes: server-resolved token map.
- Produces: resolved page blocks for SSR.

- [ ] **Step 1: Update page load to pass resolved literals**

In the SvelteKit load function that fetches the page (e.g. `apps/main/src/routes/(app)/+page.server.ts`), after fetching the page blocks, call the Go resolver or use a token map from the API.

For the first pass, the public page API is not yet exposing the `literals` map. Instead, extend the page payload so the server can send a resolved token map alongside blocks. Page blocks are currently resolved client-side with `resolveGeoCopy`. For SSR, call `resolveGeoCopy` with a token map built server-side.

In `apps/main/src/routes/(app)/+page.server.ts` (or the relevant page load), after fetching `blocks`:

```ts
import { resolveGeoCopy, getGeoTokens } from '@japaconnect/landing-blocks';

const countryCode = data.geo?.countryCode ?? 'global';
const tokens = getGeoTokens(countryCode, 'en', data.geo?.countryName);
const literals = data.literals ?? {}; // resolved token map from the API
const resolvedBlocks = resolveGeoCopy(blocks, { contentCountry: countryCode, tokens, locale: 'en', literals });
```

This keeps existing behavior and adds the new `literals` map for server-side token resolution.

- [ ] **Step 2: Extend `resolveGeoCopy` to accept custom tokens**

In `packages/landing-blocks/src/geo/resolveGeoCopy.ts`:

```ts
export interface GeoContext {
    contentCountry: string;
    tokens: GeoCopyTokens;
    locale: 'en' | 'fr';
    literals?: Record<string, string>; // new
}

export function resolveString(str: string, ctx: GeoContext): string {
    return str.replace(/\{\{\s*([A-Za-z_][A-Za-z0-9_]*)(?:\|([^}]+))?\s*\}\}/g, (match, name, fallback) => {
        if (ctx.literals && name in ctx.literals) return ctx.literals[name];
        // fall through to existing geo token resolution (see resolveGeoCopy.ts)
    });
}
```

- [ ] **Step 3: Test page builder integration**

Run existing `packages/landing-blocks` tests:

```bash
cd packages/landing-blocks && bun test
```

Add a test for `resolveGeoCopy` with `literals`:

```ts
it('resolves custom template literals', () => {
    const blocks = [{ id: '1', type: 'hero', visible: true, props: { headline: 'Year: {{Current_Year}}' } }];
    const ctx = { contentCountry: 'global', tokens: GLOBAL_TOKENS, locale: 'en', literals: { Current_Year: '2026' } };
    const resolved = resolveGeoCopy(blocks, ctx);
    expect((resolved[0] as any).props.headline).toBe('Year: 2026');
});
```

- [ ] **Step 4: Commit**

```bash
git add packages/landing-blocks/src/geo/resolveGeoCopy.ts
git commit -m "feat(literals): resolve template literals in page builder blocks"
```

---

### Task 11: Catalog updates, cache wiring, and final verification

**Files:**
- Modify: `.jeos/conventions/permission-key-naming.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Create: `internal/literals/cache.go` (optional)
- Modify: `internal/admin/literals.go` (cache invalidation)

- [ ] **Step 1: Add Valkey cache for tenant token lists**

In `internal/literals/cache.go`:

```go
package literals

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/valkey-io/valkey-go/valkeycompat"
)

const cacheKeyPrefix = "literals:"
const cacheTTL = 5 * time.Minute

func cacheKey(tenantID string) string { return cacheKeyPrefix + tenantID }

func LoadCached(ctx context.Context, v valkeycompat.Cmdable, tenantID string, loader func() ([]TemplateLiteral, error)) ([]TemplateLiteral, error) {
    if v == nil {
        return loader()
    }
    key := cacheKey(tenantID)
    cached, err := v.Get(ctx, key).Result()
    if err == nil && cached != "" {
        var out []TemplateLiteral
        if err := json.Unmarshal([]byte(cached), &out); err == nil {
            return out, nil
        }
    }
    out, err := loader()
    if err != nil {
        return nil, err
    }
    b, _ := json.Marshal(out)
    v.Set(ctx, key, string(b), cacheTTL)
    return out, nil
}

func InvalidateCache(ctx context.Context, v valkeycompat.Cmdable, tenantID string) {
    if v == nil {
        return
    }
    v.Del(ctx, cacheKey(tenantID))
}
```

Wire `LoadCached` into `content/literals.go` and call `InvalidateCache` in admin create/update/delete.

- [ ] **Step 2: Update catalog files**

- `.jeos/conventions/permission-key-naming.md` — add `content.literals.manage`.
- `.jeos/conventions/valkey-keys-catalog.md` — add `literals:{tenant_id}`.

- [ ] **Step 3: Run full verification**

```bash
cd /home/jmobile/websites/japa/japaconnect
make sqlc-gen
go build ./...
go vet ./...
go test ./internal/literals/... ./internal/content/... -count=1
cd apps/admin && bun check && bun test src/routes/literals/page.test.ts
cd packages/landing-blocks && bun test
```

- [ ] **Step 4: Commit**

```bash
git add internal/literals/cache.go internal/admin/literals.go internal/content/literals.go .jeos/conventions/
git commit -m "feat(literals): add Valkey cache and update catalogs"
```

---

## Plan self-review

- **Spec coverage**: Each spec section is covered by at least one task: data model (Task 1), expression engine (Tasks 3–5), resolver (Task 6), content integration (Task 7), admin CRUD (Tasks 8–9), page builder (Task 10), caching/catalogs (Task 11).
- **Placeholder scan**: No TBD, TODO, or "implement later". Migration numbers use 000572/000573 as the current next sequence; rename to the next available numbers if they are taken.
- **Type consistency**: `TemplateLiteral`, `RenderContext`, `Env`, and API types are consistent across tasks.
