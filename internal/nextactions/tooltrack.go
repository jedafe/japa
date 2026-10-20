package nextactions

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
)

// ToolRoute maps an API endpoint (method + path prefix, relative to /api/v1)
// to the tool slug recorded in `tool.used` meta.tool (§122 predicate feed).
// Match rule: the first entry whose method matches (or Method == "") and
// whose Prefix is a path prefix of the request path.
//
// Mapping convention: action endpoints (POST/calculate/generate/export)
// where the tool has one; the guide's detail view where reading the guide
// IS the tool's deliverable.
type ToolRoute struct {
	Method string
	Prefix string
	Slug   string
}

// ToolUsageRoutes is the canonical endpoint→tool map. Keep slugs aligned
// with the /tools/<slug> frontend routes and the §122 seed predicates.
var ToolUsageRoutes = []ToolRoute{
	{Method: http.MethodPost, Prefix: "/crs/calculate", Slug: "crs-calculator"},
	{Method: http.MethodPost, Prefix: "/assessment/submit", Slug: "readiness-assessment"},
	{Method: http.MethodPost, Prefix: "/cv-formatter/format", Slug: "cv-formatter"},
	{Method: http.MethodPost, Prefix: "/sop/generate", Slug: "sop-generator"},
	{Method: http.MethodPost, Prefix: "/credentials/", Slug: "credential-checker"},
	{Method: http.MethodPost, Prefix: "/jce/comparisons", Slug: "comparison-tool"},
	{Method: http.MethodPost, Prefix: "/jce/selections", Slug: "comparison-tool"},
	{Method: http.MethodPost, Prefix: "/community-locator", Slug: "community-locator"},
	{Method: http.MethodPost, Prefix: "/me/appointment-alerts", Slug: "embassy-alerter"},
	{Method: http.MethodPost, Prefix: "/me/visa-timelines", Slug: "processing-tracker"},
	{Method: http.MethodPatch, Prefix: "/checklist/", Slug: "visa-checklist"},
	{Method: http.MethodGet, Prefix: "/calculator/cost-of-living/compare", Slug: "cost-of-living"},
	{Method: http.MethodGet, Prefix: "/pof/calculate", Slug: "pof-calculator"},
	{Method: http.MethodGet, Prefix: "/fx/convert", Slug: "currency-converter"},
	{Method: http.MethodGet, Prefix: "/remittance/calculate", Slug: "remittance"},
	{Method: http.MethodGet, Prefix: "/eca/guides/", Slug: "eca-guide"},
	{Method: http.MethodGet, Prefix: "/eca/compare", Slug: "eca-guide"},
	{Method: http.MethodGet, Prefix: "/attestation/", Slug: "attestation-guide"},
	{Method: http.MethodGet, Prefix: "/housing-guide/", Slug: "housing-guide"},
	{Method: http.MethodGet, Prefix: "/language-tests/", Slug: "language-test"},
	{Method: http.MethodGet, Prefix: "/credentials/", Slug: "credential-checker"},
	{Method: http.MethodGet, Prefix: "/checklist/", Slug: "visa-checklist"},
	{Method: http.MethodGet, Prefix: "/arrival-checklist/", Slug: "arrival-checklist"},
	{Method: http.MethodGet, Prefix: "/community-locator/", Slug: "community-locator"},
	{Method: http.MethodGet, Prefix: "/jce/comparisons/", Slug: "comparison-tool"},
	{Method: http.MethodGet, Prefix: "/scholarships/", Slug: "scholarships"},
	{Method: http.MethodGet, Prefix: "/processing-times", Slug: "processing-tracker"},
	{Method: http.MethodGet, Prefix: "/visa-routes/", Slug: "visa-routes"},
	{Method: http.MethodGet, Prefix: "/tax-calculator/", Slug: "tax-calculator"},
}

// apiPrefix is the mount point the middleware strips before matching.
const apiPrefix = "/api/v1"

// TrackToolUsage is the §122 tool.used feed — emits one platform_event per
// successful request that maps to a tool action/engagement endpoint. Works
// for guests (session_id = client IP) and members (JTI + user_id) alike,
// so guest tool_use predicates and member tool_not_used predicates both
// resolve. Mounted once on the /api/v1 router.
func TrackToolUsage(tracker EventTracker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// WrapResponseWriter preserves optional interfaces (Flusher,
			// Hijacker, ReaderFrom) so streaming endpoints are unaffected.
			rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(rec, r)

			if tracker == nil || rec.Status() >= 400 {
				return
			}

			path := r.URL.Path
			if rest, ok := strings.CutPrefix(path, apiPrefix); ok {
				path = rest
			}

			for _, rt := range ToolUsageRoutes {
				if rt.Method != "" && r.Method != rt.Method {
					continue
				}

				if !strings.HasPrefix(path, rt.Prefix) {
					continue
				}

				emitToolUsed(r, tracker, rt.Slug)

				return // first match wins
			}
		})
	}
}

func emitToolUsed(r *http.Request, tracker EventTracker, slug string) {
	var userID pgtype.UUID

	session := clientip.FromRequest(r)
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
		if u.SessionID != "" {
			session = u.SessionID
		}
	}

	_, _ = tracker.TrackEvent(r.Context(), analytics.TrackEventInput{
		EventType:  analytics.EventToolUsed,
		UserID:     userID,
		EntityType: "tool",
		Meta:       map[string]any{"tool": slug},
		SessionID:  session,
	})
}
