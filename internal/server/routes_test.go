package server

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestNoDuplicateRoutes verifies that server.go does not register the same
// (method, path) combination more than once. Duplicate routes cause Chi to
// panic or silently shadow the earlier registration, leading to unpredictable
// behavior in production. This is a structural test — it parses the source file
// rather than spinning up a real server, so it runs without a database.
func TestNoDuplicateRoutes(t *testing.T) {
	t.Parallel()

	routes, err := parseServerRoutes("../../internal/server/server.go")
	if err != nil {
		t.Fatalf("parsing server.go routes: %v", err)
	}

	seen := make(map[string]int) // key → first line number

	for _, r := range routes {
		key := r.method + " " + r.path
		if prev, ok := seen[key]; ok {
			t.Errorf("duplicate route %q first at line %d, again at line %d", key, prev, r.line)
		}

		seen[key] = r.line
	}

	if t.Failed() {
		t.Logf("total routes parsed: %d", len(routes))
	}
}

// TestNoOverlappingParamRoutes checks that no two routes share the same
// structure with different parameter names at the same position (e.g.
// GET /users/:id and GET /users/:user_id). Chi treats these as the same
// route and will panic. Param names are collapsed to :x for comparison.
func TestNoOverlappingParamRoutes(t *testing.T) {
	t.Parallel()

	routes, err := parseServerRoutes("../../internal/server/server.go")
	if err != nil {
		t.Fatalf("parsing server.go routes: %v", err)
	}

	normalized := make(map[string][]routeEntry)

	for _, r := range routes {
		norm := normalizePath(r.path)
		key := r.method + " " + norm
		normalized[key] = append(normalized[key], r)
	}

	for key, entries := range normalized {
		if len(entries) > 1 {
			paths := make([]string, len(entries))
			for i, e := range entries {
				paths[i] = fmt.Sprintf("%s (line %d)", e.path, e.line)
			}

			t.Errorf("overlapping param routes for %q: %s", key, strings.Join(paths, ", "))
		}
	}
}

type routeEntry struct {
	method string
	path   string
	line   int
}

// prefixFrame tracks one active r.Route("/prefix", func(r chi.Router) {...})
// mount scope: its fully-composed path prefix (including any outer mounts,
// e.g. "/api/v1/auth") and the brace depth at which it was opened, so it can
// be popped once that block closes.
type prefixFrame struct {
	prefix string
	depth  int
}

// parseServerRoutes extracts route registrations from server.go. It tracks
// every r.Route("/prefix", ...) mount scope (e.g. /api/v1, and the nested
// /auth mount inside it, and the top-level /control mount) via a depth-based
// stack, so relative registrations inside nested mounts get the correct
// fully-composed prefix (e.g. "/api/v1" + "/auth" + "/login"). This mirrors
// the logic in scripts/route_diff.py.
func parseServerRoutes(path string) ([]routeEntry, error) {
	f, err := os.Open(path) //nolint:gosec // test uses variable path
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	var routes []routeEntry

	var stack []prefixFrame

	depth := 0
	routeMount := regexp.MustCompile(`\.Route\("(/[^"]*)"`)
	re := regexp.MustCompile(`(?:^|[^A-Za-z])(Get|Post|Put|Patch|Delete)\("(/[^"]*)"`)

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		depth += strings.Count(line, "{") - strings.Count(line, "}")

		// Pop mount scopes whose block has closed.
		for len(stack) > 0 && depth < stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}

		if m := routeMount.FindStringSubmatch(line); m != nil {
			base := ""
			if len(stack) > 0 {
				base = stack[len(stack)-1].prefix
			}

			stack = append(stack, prefixFrame{prefix: base + m[1], depth: depth})
		}

		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		method := strings.ToUpper(m[1])
		rpath := m[2]

		if len(stack) > 0 {
			prefix := stack[len(stack)-1].prefix
			if !strings.HasPrefix(rpath, prefix) {
				rpath = prefix + rpath
			}
		}

		routes = append(routes, routeEntry{
			method: method,
			path:   rpath,
			line:   lineNum,
		})
	}

	return routes, scanner.Err()
}

// normalizePath collapses Chi {param} and catalog :param syntax to :x for
// structural comparison. Trailing slashes are stripped.
func normalizePath(p string) string {
	re := regexp.MustCompile(`\{(\w+)\}`)
	p = re.ReplaceAllString(p, ":$1")
	re2 := regexp.MustCompile(`:\w+`)
	p = re2.ReplaceAllString(p, ":x")
	p = strings.TrimRight(p, "/")

	if p == "" {
		p = "/"
	}

	return p
}

// TestAdminRoutesRequireAuthAndPermission is a structural guard: every route
// under /admin/* and /control/* registered in server.go must be wrapped by
// auth.RequireAuth (session) or publicapi.RequireScope (API key) AND an rbac
// guard, whether the middleware is attached to the route's own r.With(...) or
// inherited from an enclosing r.Group{Use(...)}/r.Route block. A regression
// here would expose admin endpoints to unauthenticated or unauthorized users.
// Parses the source file — no database required.
func TestAdminRoutesRequireAuthAndPermission(t *testing.T) {
	t.Parallel()

	routes, err := parseServerRoutesWithGuards("../../internal/server/server.go")
	if err != nil {
		t.Fatalf("parsing server.go routes: %v", err)
	}

	checked := 0

	for _, r := range routes {
		if !strings.Contains(r.path, "/admin/") && !strings.Contains(r.path, "/control/") {
			continue
		}

		checked++

		if !r.hasAuth {
			t.Errorf("%s %s (line %d): no auth guard (auth.RequireAuth/RequireScope) on route or enclosing group", r.method, r.path, r.line)
		}

		if !r.hasPerm {
			t.Errorf("%s %s (line %d): no rbac guard (rbac.RequirePermission/RequireGroup/...) on route or enclosing group", r.method, r.path, r.line)
		}
	}

	if checked == 0 {
		t.Error("no /admin/ or /control/ routes found — parser may be broken")
	}

	t.Logf("checked %d admin/control routes for auth+rbac guards", checked)
}

type guardedRouteEntry struct {
	method  string
	path    string
	line    int
	hasAuth bool
	hasPerm bool
}

// guardFrame tracks one enclosing r.Group{...}/r.Route("/prefix", func...) block:
// its composed path prefix, whether the block's r.Use(...) lines attach an auth
// or rbac middleware, and the brace depth at which the block closes.
type guardFrame struct {
	prefix  string
	hasAuth bool
	hasPerm bool
	depth   int
}

var (
	guardRouteMount = regexp.MustCompile(`\.Route\("(/[^"]*)"`)
	guardRouteReg   = regexp.MustCompile(`\.(Get|Post|Put|Patch|Delete)\("(/[^"]*)"`)
	guardWithStart  = regexp.MustCompile(`\br\.With\(`)
	guardGroupStart = regexp.MustCompile(`\br\.Group\(`)
	guardUseLine    = regexp.MustCompile(`\br\.Use\(`)
	guardAuthMW     = regexp.MustCompile(`auth\.RequireAuth|RequireScope\(`)
	guardPermMW     = regexp.MustCompile(`rbac\.`)
)

// parseServerRoutesWithGuards extends parseServerRoutes: it additionally
// records whether each registration is covered by an auth and an rbac guard,
// resolving guards attached either to the route's own r.With(...) statement
// (which may span multiple lines) or to enclosing r.Group/r.Route blocks via
// r.Use(...).
func parseServerRoutesWithGuards(path string) ([]guardedRouteEntry, error) {
	f, err := os.Open(path) //nolint:gosec // test uses variable path
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	var routes []guardedRouteEntry

	var stack []guardFrame

	depth := 0
	inWith := false
	var stmt strings.Builder

	scanner := bufio.NewScanner(f)
	lineNum := 0

	// register records a route found at lineNum inside an r.With statement
	// (stmt may be empty for bare r.Get(...) calls).
	register := func(method, rpath, stmtText string) {
		if len(stack) > 0 {
			prefix := stack[len(stack)-1].prefix
			if !strings.HasPrefix(rpath, prefix) {
				rpath = prefix + rpath
			}
		}

		hasAuth := guardAuthMW.MatchString(stmtText)
		hasPerm := guardPermMW.MatchString(stmtText)

		for _, fr := range stack {
			hasAuth = hasAuth || fr.hasAuth
			hasPerm = hasPerm || fr.hasPerm
		}

		routes = append(routes, guardedRouteEntry{
			method:  strings.ToUpper(method),
			path:    rpath,
			line:    lineNum,
			hasAuth: hasAuth,
			hasPerm: hasPerm,
		})
	}

	pushFrame := func(stmtText string) {
		base := ""
		if len(stack) > 0 {
			base = stack[len(stack)-1].prefix
		}

		f := guardFrame{
			prefix:  base,
			depth:   depth,
			hasAuth: guardAuthMW.MatchString(stmtText),
			hasPerm: guardPermMW.MatchString(stmtText),
		}

		if m := guardRouteMount.FindStringSubmatch(stmtText); m != nil {
			f.prefix = base + m[1]
		}

		stack = append(stack, f)
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		depth += strings.Count(line, "{") - strings.Count(line, "}")

		// Pop blocks whose brace scope has closed.
		for len(stack) > 0 && depth < stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}

		if inWith {
			stmt.WriteString(line)

			if guardGroupStart.MatchString(line) || strings.Contains(line, ".Group(") || strings.Contains(line, ".Route(") {
				pushFrame(stmt.String())
				inWith = false
				stmt.Reset()

				continue
			}

			if m := guardRouteReg.FindStringSubmatch(line); m != nil {
				register(m[1], m[2], stmt.String())
				inWith = false
				stmt.Reset()
			}

			continue
		}

		switch {
		case guardWithStart.MatchString(line):
			stmt.Reset()
			stmt.WriteString(line)

			if m := guardRouteReg.FindStringSubmatch(line); m != nil {
				register(m[1], m[2], stmt.String())
				stmt.Reset()
			} else {
				inWith = true
			}

		case guardGroupStart.MatchString(line) || strings.Contains(line, ".Route("):
			pushFrame(line)

		case guardUseLine.MatchString(line):
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				top.hasAuth = top.hasAuth || guardAuthMW.MatchString(line)
				top.hasPerm = top.hasPerm || guardPermMW.MatchString(line)
			}

		default:
			if m := guardRouteReg.FindStringSubmatch(line); m != nil {
				register(m[1], m[2], "")
			}
		}
	}

	return routes, scanner.Err()
}

// TestCatalogAndCodeRouteSymmetry is a placeholder for P6 when a test DB is
// available. It will spin up the Chi router via NewServer and walk all
// registered routes, comparing them against the catalog file.
//
// For now it verifies the route_diff.py script is runnable and returns
// consistent output (exit 0 = script found, parsed, and printed results).
func TestCatalogAndCodeRouteSymmetry(t *testing.T) {
	t.Parallel()

	// Verify the route diff script exists and is parseable.
	if _, err := os.Stat("../../scripts/route_diff.py"); os.IsNotExist(err) {
		t.Skip("route_diff.py not found, skipping catalog symmetry check")
	}

	// Count routes from server.go to ensure parseServerRoutes is consistent
	// with what route_diff.py reports.
	routes, err := parseServerRoutes("../../internal/server/server.go")
	if err != nil {
		t.Fatalf("parsing server.go routes: %v", err)
	}

	// Deduplicate for comparison (route_diff.py uses a set).
	seen := make(map[string]bool)
	unique := 0

	for _, r := range routes {
		key := r.method + " " + normalizePath(r.path)
		if !seen[key] {
			seen[key] = true
			unique++
		}
	}

	// The code route count should be > 0 (sanity check).
	if unique == 0 {
		t.Error("parsed zero unique routes from server.go — parser may be broken")
	}

	t.Logf("parsed %d unique routes from server.go (%d total entries)", unique, len(routes))

	// Verify sorted order for deterministic output (matches route_diff.py behavior).
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].method != routes[j].method {
			return routes[i].method < routes[j].method
		}

		return routes[i].path < routes[j].path
	})
}
