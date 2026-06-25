# Section 42 — Feature Flags & Pluggable Architecture

**Status:** ✅ Locked

## Philosophy

Every non-core platform feature is toggleable by admin without causing breakage site-wide. "Core" = auth, RBAC, multi-tenancy, basic content creation, media upload. Everything else — Forum, Blog, Ratings, DMs, Wallet, Learning Platform, Ticket System, Ads, Referral, etc. — is feature-flagged.

## Schema

```sql
CREATE TABLE feature_flags (
  key          text NOT NULL,
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  enabled      boolean NOT NULL DEFAULT true,
  config       jsonb NOT NULL DEFAULT '{}',   -- feature-specific settings
  updated_at   timestamptz NOT NULL DEFAULT now(),
  updated_by   uuid REFERENCES users(id),
  PRIMARY KEY (key, tenant_id)
);
```

## Feature Registry (seeded at install)

```go
var FeatureRegistry = []Feature{
  // Content
  {Key: "feature.blog",             Label: "Blog",              Default: true},
  {Key: "feature.forum",            Label: "Forum / Q&A",       Default: true},
  {Key: "feature.comparison",       Label: "Comparison Pages",  Default: true},
  {Key: "feature.learning",         Label: "Learning Platform", Default: false},
  // Social
  {Key: "feature.follow",           Label: "Follow System",     Default: true},
  {Key: "feature.dm",               Label: "Direct Messages",   Default: true},
  {Key: "feature.reactions",        Label: "Reactions",         Default: true},
  {Key: "feature.bookmarks",        Label: "Bookmarks",         Default: true},
  {Key: "feature.ratings_reviews",  Label: "Ratings & Reviews", Default: true},
  {Key: "feature.mentorship",       Label: "Mentorship Matching", Default: false},
  // Commerce
  {Key: "feature.wallet",           Label: "Wallet & Credits",  Default: false},
  {Key: "feature.ads",              Label: "Advertising",       Default: false},
  {Key: "feature.referral",         Label: "Referral Program",  Default: false},
  // Tools
  {Key: "feature.tickets",          Label: "Ticket System",     Default: true},
  {Key: "feature.newsletter",       Label: "Newsletter",        Default: false},
  {Key: "feature.notifications",    Label: "Notifications",     Default: true},
  // AI
  {Key: "feature.japabot",          Label: "JapaBot AI Chat",   Default: false},
  {Key: "feature.ai_content_gen",   Label: "AI Content Generation", Default: false},
}
```

## Middleware Check

```go
func FeatureGate(key string) func(http.Handler) http.Handler {
  return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      tenantID := TenantFromCtx(r.Context())
      if !IsFeatureEnabled(r.Context(), tenantID, key) {
        // API: 404 with JSON body {"error":"feature_disabled"}
        // Frontend: 404 page via SvelteKit routing
        w.WriteHeader(http.StatusNotFound)
        json.NewEncoder(w).Encode(map[string]string{"error": "feature_disabled"})
        return
      }
      next.ServeHTTP(w, r)
    })
  }
}

// SvelteKit layout guard (client side):
// if (!$page.data.features['feature.dm']) redirect(302, '/');
```

## Frontend Routing Guard

All feature-gated routes in SvelteKit check the features object available in page data:

```ts
// +layout.server.ts — fetched once per session, cached
export async function load({ locals }) {
  return {
    features: await getEnabledFeatures(locals.tenantID)
  };
}

// Feature-gated route: src/routes/(app)/messages/+page.ts
export async function load({ parent }) {
  const { features } = await parent();
  if (!features['feature.dm']) throw redirect(302, '/');
}
```

When a feature is disabled:
- API routes return 404 (not 403 — don't reveal the feature exists but is disabled)
- Nav items that require the feature are hidden (checked in `getNavItems` — Section 23)
- No null-pointer errors in disabled code paths: feature-gated handlers are wrapped, not conditionally compiled

## Admin Settings Structure

Admin Panel → Settings is organized into named categories, each with its own dedicated page. Categories are defined in the `settings_categories` registry and rendered dynamically — admin never sees a flat wall of settings:

```go
var SettingsCategories = []Category{
  {Key: "general",       Label: "General",         Icon: "settings"},
  {Key: "appearance",    Label: "Appearance",       Icon: "palette"},
  {Key: "features",      Label: "Features",         Icon: "toggle-left"},
  {Key: "users",         Label: "Users & Access",   Icon: "users"},
  {Key: "content",       Label: "Content",          Icon: "file-text"},
  {Key: "email",         Label: "Email",            Icon: "mail"},
  {Key: "payment",       Label: "Payments",         Icon: "credit-card"},
  {Key: "media",         Label: "Media & Storage",  Icon: "hard-drive"},
  {Key: "seo",           Label: "SEO & Discovery",  Icon: "search"},
  {Key: "security",      Label: "Security",         Icon: "shield"},
  {Key: "integrations",  Label: "Integrations",     Icon: "plug"},
  {Key: "ai",            Label: "AI & Automation",  Icon: "cpu"},
  {Key: "advanced",      Label: "Advanced",         Icon: "tool"},
}
```

## See Also

Section 21 (Feature Flags UI in Control Panel), Section 22 (Admin Panel Settings categories), ADR-042.
