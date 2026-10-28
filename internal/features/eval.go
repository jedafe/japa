package features

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/geoip"
)

// SurfaceHeader is the request header identifying the caller's client surface
// (main, tools, academy, docs, mobile, admin, extension, desktop). It is
// advisory — surface rules are visibility controls, not authorization.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.2.
const SurfaceHeader = "X-JC-Surface"

// CallerScope carries the per-request attributes that refine flag resolution
// beyond the global enabled state. A zero-value scope is an anonymous caller
// on no surface — global behavior applies.
type CallerScope struct {
	Surface     string          // validated X-JC-Surface, "" = none
	CountryCode string          // geoip GeoContext.CountryCode, "" = unknown
	GeoValid    bool            // GeoContext.IsValid
	GroupSlugs  map[string]bool // caller's group slugs; nil = anonymous
	Permissions map[string]bool // caller's resolved permission keys; nil = anonymous
	SuperAdmin  bool            // bypasses audience checks ONLY — not geo/state
	SubjectID   string          // caller's user UUID — drives per-user rollout bucketing; "" = anonymous (platform bucket)
}

// EvalResult is the outcome of caller-scoped flag resolution.
type EvalResult struct {
	Enabled bool // routes may serve (Gate contract — false → 404)
	Visible bool // appears in the surface's flag map (false → hidden from nav)
}

// AudienceResolver supplies caller group/permission data without features
// importing internal/rbac — the rbac package implements this interface and is
// injected at startup via SetAudienceResolver.
// Ref: internal/rbac/rbac.go (UserContext, GetUserContext, IsSuperAdmin).
type AudienceResolver interface {
	ResolveScope(ctx context.Context, userID pgtype.UUID) (groups, perms map[string]bool, superAdmin bool, err error)
}

// SetAudienceResolver wires the RBAC-backed scope resolver. Call once at
// startup before serving traffic (same contract as Handlers.SetOnFlagEnabled).
func (c *Checker) SetAudienceResolver(resolver AudienceResolver) {
	c.audienceResolver = resolver
}

// SurfaceFrom returns the validated X-JC-Surface header value, or "" when the
// header is absent or carries an unknown surface. Unknown/absent surfaces get
// global behavior — no surface rule is applied and the flag stays in the
// caller's flag map (back-compat for public API consumers).
func SurfaceFrom(r *http.Request) string {
	s := r.Header.Get(SurfaceHeader)
	if !ValidSurface(s) {
		return ""
	}

	return s
}

// BuildScope assembles the CallerScope for a request: the validated surface,
// the GeoContext populated by geoip.Middleware, and — when a user is
// authenticated and an AudienceResolver is wired — the caller's groups,
// permissions, and super_admin status. Anonymous requests and resolver
// failures leave the audience fields nil so audience checks fail closed.
func (c *Checker) BuildScope(r *http.Request, surface string) CallerScope {
	sc := CallerScope{}
	if ValidSurface(surface) {
		sc.Surface = surface
	}

	geo := geoip.FromContext(r.Context())
	sc.CountryCode = geo.CountryCode
	sc.GeoValid = geo.IsValid

	user := auth.FromContext(r.Context())
	if user == nil || c.audienceResolver == nil {
		return sc
	}

	groups, perms, superAdmin, err := c.audienceResolver.ResolveScope(r.Context(), user.UserID)
	if err != nil {
		// Audience is authorization-adjacent — a resolver failure must not
		// silently widen access. Treat the caller as anonymous.
		slog.Warn("features: audience resolution failed, treating caller as anonymous", "error", err)
		return sc
	}

	sc.GroupSlugs = groups
	sc.Permissions = perms
	sc.SuperAdmin = superAdmin
	if user.UserID.Valid {
		sc.SubjectID = user.UserID.String()
	}

	return sc
}

// EvalFlag resolves a flag for a specific caller. The global enabled/rollout
// result comes from the shared L1/L2 cache; surface state, geo, and audience
// rules are per-request overlays evaluated against the cached config blob —
// no extra DB hit on the hot path.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.1.
func (c *Checker) EvalFlag(ctx context.Context, key string, sc CallerScope) EvalResult {
	raw, config := c.resolveFlag(ctx, key)
	cfg := flagConfigFrom(config)

	return evalScoped(key, effectiveEnabled(c.bucketID(ctx, sc), key, raw, cfg), cfg, sc)
}

// EvalAll returns the caller-scoped flags map for GET /api/v1/feature-flags:
// key → Enabled && Visible. Registry defaults seed the map so keys without a
// stored row still appear; stored rows overlay them. The per-caller result is
// deliberately NOT cached — only the underlying row snapshot (jc:flags:_all)
// and per-flag resolutions are.
func (c *Checker) EvalAll(ctx context.Context, sc CallerScope) map[string]bool {
	result := make(map[string]bool, len(Registry))
	bid := c.bucketID(ctx, sc)

	for _, f := range Registry {
		r := evalScoped(f.Key, f.Default, FlagConfig{}, sc)
		result[f.Key] = r.Enabled && r.Visible
	}

	for _, row := range c.AllFlagRows(ctx) {
		cfg := flagConfigFrom(row.Config)
		// internal:true opts the flag out of the public projection —
		// delete so registry-seeded entries are removed too (BUG-010).
		if cfg.Internal {
			delete(result, row.Key)
			continue
		}
		r := evalScoped(row.Key, effectiveEnabled(bid, row.Key, row.Enabled, cfg), cfg, sc)
		result[row.Key] = r.Enabled && r.Visible
	}

	return result
}

// evalScoped applies the spec §5.1 resolution order to an already-resolved
// global enabled state and raw config:
//
//	enabled=false              → {false,false}
//	surface state = disabled   → {false,false}
//	geo_deny contains country  → {false,false} (fails OPEN when country unknown)
//	geo_allow misses country   → {false,false} (fails CLOSED when country unknown)
//	audience set + no match    → {false,false} (super_admin bypasses audience only)
//	surface state = hidden     → {true,false}
//	otherwise                  → {true,true}
//
// An absent/unknown surface applies no scope. When the surface has no rule in
// stored config, the registry's DefaultSurfaces applies (same merge Control →
// Extensions displays), so registry-declared defaults are eval-effective even
// before a flag's first write.
func evalScoped(key string, enabled bool, cfg FlagConfig, sc CallerScope) EvalResult {
	if !enabled {
		return EvalResult{}
	}

	rule, ok := surfaceRuleFor(key, cfg, sc.Surface)
	if !ok {
		return EvalResult{Enabled: true, Visible: true}
	}

	if rule.State == "disabled" || !geoPasses(rule, sc) || !audiencePasses(rule, sc) {
		return EvalResult{}
	}

	if rule.State == "hidden" {
		return EvalResult{Enabled: true, Visible: false}
	}

	return EvalResult{Enabled: true, Visible: true}
}

// surfaceRuleFor extracts the SurfaceRule for the caller's surface. Stored
// config wins; when it has no entry for the surface the registry's
// DefaultSurfaces applies — the same merge Control → Extensions displays via
// buildExtensionEntry. It returns ok=false — meaning "global behavior
// applies" — when the surface is empty/unknown, the config is malformed
// (visibility is a rollout control, not a security boundary, so malformed
// config fails open), and no stored or default rule covers the surface.
func surfaceRuleFor(key string, cfg FlagConfig, surface string) (SurfaceRule, bool) {
	if !ValidSurface(surface) {
		return SurfaceRule{}, false
	}

	if rule, ok := cfg.Surfaces[surface]; ok {
		return rule, true
	}

	if f, ok := RegistryLookup(key); ok {
		if state, ok := f.DefaultSurfaces[surface]; ok {
			return SurfaceRule{State: state}, true
		}
	}

	return SurfaceRule{}, false
}

// geoPasses evaluates the geo overlay: geo_deny runs before geo_allow so a
// country in both lists is denied. Unknown country (GeoValid=false) never
// satisfies a deny rule (fails open) and never satisfies an allow rule
// (fails closed). Geo is location truth — super_admin does not bypass it.
func geoPasses(rule SurfaceRule, sc CallerScope) bool {
	if sc.GeoValid && sc.CountryCode != "" && countryIn(rule.GeoDeny, sc.CountryCode) {
		return false
	}

	if len(rule.GeoAllow) > 0 && (!sc.GeoValid || !countryIn(rule.GeoAllow, sc.CountryCode)) {
		return false
	}

	return true
}

// audiencePasses evaluates the audience overlay: an empty audience matches
// everyone; super_admin bypasses the check entirely; anything else requires
// at least one matching group:<slug> or perm:<node> entry.
func audiencePasses(rule SurfaceRule, sc CallerScope) bool {
	if len(rule.Audience) == 0 || sc.SuperAdmin {
		return true
	}

	return inAudience(rule.Audience, sc)
}

// countryIn reports whether code is in the ISO alpha-2 list. Stored codes and
// GeoLite results are both uppercase, so matching is exact.
func countryIn(codes []string, code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}

	return false
}

// inAudience reports whether the caller matches any audience entry:
// group:<slug> = caller's group slug, perm:<node> = caller holds the
// permission node. Nil group/perm maps (anonymous) match nothing.
func inAudience(audience []string, sc CallerScope) bool {
	for _, entry := range audience {
		if slug, ok := strings.CutPrefix(entry, "group:"); ok {
			if sc.GroupSlugs[slug] {
				return true
			}

			continue
		}

		if node, ok := strings.CutPrefix(entry, "perm:"); ok {
			if sc.Permissions[node] {
				return true
			}
		}
	}

	return false
}
